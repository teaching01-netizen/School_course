package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/config"
	"warwick-institute/internal/crmimport"
	"warwick-institute/internal/crmimport/crossstudy"
	"warwick-institute/internal/crmimport/queue"
	"warwick-institute/internal/crmimport/reconcile"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/emailnotifier"
	"warwick-institute/internal/emailreminder"
	"warwick-institute/internal/httpapi"
	"warwick-institute/internal/logging"
	"warwick-institute/internal/otpdelivery"
	"warwick-institute/internal/pg"
	"warwick-institute/internal/realtime"
	"warwick-institute/internal/schedulepolicy"
	"warwick-institute/internal/scheduling"
	"warwick-institute/internal/series"
	"warwick-institute/internal/sessionchangedelivery"
	"warwick-institute/internal/sessionchangeimpact"
	"warwick-institute/internal/smartsms"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("config error", "err", err)
		os.Exit(2)
	}

	log := logging.New(cfg.LogLevel)
	slog.SetDefault(log)
	log.Info("starting", "addr", cfg.Addr, "static_dir", cfg.StaticDir)
	if cfg.TrustedProxyCIDRs == "" {
		log.Warn("TRUSTED_PROXY_CIDRS is not set: per-IP rate limits will use the direct peer address. Set TRUSTED_PROXY_CIDRS to the reverse proxy CIDRs (nginx/Cloudflare/Railway) when running behind a proxy.")
	}

	dbpool, err := pg.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer dbpool.Close()

	// Hourly sweep of expired auth sessions and stale rate-limit events; both
	// tables only accumulate rows, so the sweeper is what keeps them bounded.
	sweeper := newMaintenanceSweeper(log, dbpool)
	sweeper.Start(context.Background())
	retentionCleanup := newRetentionCleanup(log, dbpool)
	retentionCleanup.Start(context.Background())

	realtimeDatabaseURL, err := pg.ResolveRealtimeDatabaseURL(
		cfg.DatabaseURL,
		cfg.RealtimeDatabaseURL,
		os.Getenv("PGBOUNCER") != "",
	)
	if err != nil {
		log.Error("realtime database config", "err", err)
		os.Exit(1)
	}
	realtimeDBPool, err := pg.NewRealtimePool(context.Background(), realtimeDatabaseURL)
	if err != nil {
		log.Error("realtime database connect", "err", err)
		os.Exit(1)
	}
	defer realtimeDBPool.Close()

	// CRM services.
	snapshotSvc, err := crmimport.NewSnapshotService(dbpool, cfg.InstituteTZ)
	if err != nil {
		log.Error("init snapshot service", "error", err)
		os.Exit(1)
	}
	syncSvc := crmimport.NewStudentSyncService(dbpool)
	seriesSvc, err := series.NewService(dbpool, cfg.InstituteTZ)
	if err != nil {
		log.Error("init series service", "error", err)
		os.Exit(1)
	}
	schedulingSvc, err := scheduling.NewServiceWithPolicy(dbpool, cfg.InstituteTZ, seriesSvc, schedulepolicy.NewDBReader(), log)
	if err != nil {
		log.Error("init scheduling service", "error", err)
		os.Exit(1)
	}
	reconcileV2Svc := reconcile.NewReconcileV2Service(dbpool, schedulingSvc)

	// Start the CRM v2 queue worker.
	queueStore := queue.NewPostgresQueueStore(dbpool)
	worker := queue.NewQueueWorker(log, queueStore, "crm-worker-main")

	crossStudyStore := crossstudy.NewStore(dbpool, schedulingSvc)
	crossStudyProc := crossstudy.NewProcessor(dbpool, crossStudyStore, log)

	// Register job handlers.
	worker.RegisterHandler(queue.JobTypeImportSnapshot,
		crmimport.ImportSnapshotJobHandler(snapshotSvc, syncSvc, reconcileV2Svc, worker, crossStudyStore))
	worker.RegisterHandler(queue.JobTypeStudentSync,
		crmimport.StudentSyncJobHandler(syncSvc, snapshotSvc))
	worker.RegisterHandler(queue.JobTypeCourseReconcileApply,
		reconcile.CourseReconcileJobHandler(reconcileV2Svc, worker))
	worker.RegisterHandler(queue.JobTypeCourseReconcileDiff,
		reconcile.CourseReconcileJobHandler(reconcileV2Svc, worker))
	worker.RegisterHandler(queue.JobTypeCrossStudyProcess,
		crossStudyJobHandler(crossStudyProc))

	workerCtx, workerCancel := context.WithCancel(context.Background())
	worker.Start(workerCtx)

	uploadV2Svc, err := crmimport.NewUploadV2Service(dbpool, worker, cfg.InstituteTZ)
	if err != nil {
		log.Error("init upload v2 service", "error", err)
		os.Exit(1)
	}

	q := sqldb.New(dbpool)
	emailDeps := httpapi.NewEmailDeps(log, cfg, dbpool, q)
	otpDeliveryDeps, otpDeliveryCancel, err := newOTPDeliveryRuntime(log, cfg, dbpool)
	if err != nil {
		log.Error("init OTP delivery runtime", "error", err)
		os.Exit(1)
	}
	realtimeCtx, realtimeCancel := context.WithCancel(context.Background())
	postgresFanout := realtime.NewPostgresFanout(realtimeDBPool, log)
	realtimeHub := realtime.NewHubWithFanout(
		realtimeCtx,
		"",
		postgresFanout,
		log,
	)
	realtimeReadyCtx, realtimeReadyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := postgresFanout.WaitReady(realtimeReadyCtx); err != nil {
		realtimeReadyCancel()
		log.Error("realtime database listener not ready", "err", err)
		os.Exit(1)
	}
	realtimeReadyCancel()
	impactSvc := sessionchangeimpact.New(dbpool, q, cfg.InstituteTZ, realtimeHub, log)
	impactCtx, impactCancel := context.WithCancel(context.Background())
	for range 3 {
		go impactSvc.Run(impactCtx)
	}
	notificationSMS, err := newScheduleImpactSMS(cfg)
	if err != nil {
		log.Error("init schedule impact SMS delivery", "error", err)
		os.Exit(1)
	}
	notificationWorker := sessionchangedelivery.New(q, notificationSMS, emailDeps.Service, log)
	notificationCtx, notificationCancel := context.WithCancel(context.Background())
	go notificationWorker.Run(notificationCtx)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.NewHandler(log, cfg, dbpool, uploadV2Svc, reconcileV2Svc, worker, emailDeps, otpDeliveryDeps, impactSvc, realtimeHub),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	scheduler := emailreminder.New(log, emailreminder.Config{
		Enabled: cfg.EmailReminderEnabled,
		Time:    cfg.EmailReminderTime,
	}, cfg.InstituteTZ, func(ctx context.Context) error {
		emailnotifier.SendAllEnabledWorkflows(ctx, emailnotifier.SendAllDeps{
			WorkflowStore: emailDeps.WorkflowStore,
			TemplateStore: emailDeps.TemplateStore,
			Service:       emailDeps.Service,
			InstituteTZ:   cfg.InstituteTZ,
			InstituteName: cfg.InstituteName,
			Log:           log,
			SitInQuery:    emailDeps.SitInQuery,
		})
		return nil
	})
	scheduler.Start(context.Background())

	legacyWorkerCtx, legacyWorkerCancel := context.WithCancel(context.Background())
	legacyWorker, err := startLegacySyncProcess(legacyWorkerCtx, log)
	if err != nil {
		legacyWorkerCancel()
		log.Error("start embedded legacy sync worker", "err", err)
		os.Exit(1)
	}
	if legacyWorker != nil {
		go monitorLegacySyncProcess(legacyWorkerCtx, legacyWorker, log)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stop:
		log.Info("shutting down", "sig", sig.String())
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}

	scheduler.Stop()
	sweeper.Stop()
	retentionCleanup.Stop()
	otpDeliveryCancel()
	realtimeHub.Close()
	realtimeCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	legacyWorkerCancel()
	if legacyWorker != nil {
		_ = legacyWorker.Wait(ctx)
	}
	_ = srv.Shutdown(ctx)

	workerCancel()
	worker.Stop()
	impactCancel()
	notificationCancel()
}

func newOTPDeliveryRuntime(log *slog.Logger, cfg config.Config, db *pgxpool.Pool) (*httpapi.OTPDeliveryDeps, context.CancelFunc, error) {
	noopCancel := func() {}
	if !cfg.OTPAsyncDeliveryEnabled {
		return nil, noopCancel, nil
	}
	keyring, err := otpdelivery.ParseKeyring(cfg.OTPDeliveryEncryptionKeys)
	if err != nil {
		return nil, noopCancel, err
	}

	var sms smartsms.SMSProvider
	var sender smartsms.OTPProvider
	var provider otpdelivery.Provider
	if cfg.OTPSMSProvider == "smartsms" && cfg.SMSServiceUsername != "" && cfg.SMSServicePassword != "" {
		client, err := smartsms.New(smartsms.Config{
			BaseURL:  cfg.SMSServiceBaseURL,
			Username: cfg.SMSServiceUsername,
			Password: cfg.SMSServicePassword,
		})
		if err != nil {
			return nil, noopCancel, err
		}
		adapter := &smartsms.OTPAdapter{Client: client}
		sms, sender, provider = client, adapter, adapter
	} else {
		mock := &smartsms.MockProvider{}
		sms, sender, provider = mock, mock, mock
	}

	store := otpdelivery.NewStore(db)
	dispatcher := otpdelivery.NewDispatcher(store, keyring)
	workerID, _ := os.Hostname()
	if workerID == "" {
		workerID = "server"
	}
	worker := otpdelivery.NewWorker(store, provider, keyring, otpdelivery.WorkerConfig{
		WorkerID: "otp-" + workerID,
		Log:      log,
	})
	circuitBreaker := smartsms.NewCircuitBreaker(db, cfg.OTPSMSProvider)
	worker.SetCircuitReporter(circuitBreaker)
	workerCtx, cancel := context.WithCancel(context.Background())
	go worker.Run(workerCtx)
	log.Info("OTP delivery worker started", "worker_id", "otp-"+workerID)
	return &httpapi.OTPDeliveryDeps{
		SMS: sms, Sender: sender, Dispatcher: dispatcher, Store: store, CircuitBreaker: circuitBreaker,
	}, cancel, nil
}

func newScheduleImpactSMS(cfg config.Config) (smartsms.SMSProvider, error) {
	if cfg.SMSServiceUsername == "" || cfg.SMSServicePassword == "" {
		return &smartsms.MockProvider{}, nil
	}
	return smartsms.New(smartsms.Config{
		BaseURL: cfg.SMSServiceBaseURL, Username: cfg.SMSServiceUsername, Password: cfg.SMSServicePassword,
	})
}

func crossStudyJobHandler(proc *crossstudy.Processor) queue.JobHandler {
	return func(ctx context.Context, job queue.JobRow) error {
		var payload crmimport.CrossStudyProcessPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal cross-study payload: %w", err)
		}
		return proc.ProcessSnapshot(ctx, payload.SnapshotID)
	}
}
