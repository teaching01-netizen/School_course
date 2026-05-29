package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"warwick-institute/internal/config"
	"warwick-institute/internal/crmimport"
	"warwick-institute/internal/crmimport/queue"
	"warwick-institute/internal/crmimport/reconcile"
	"warwick-institute/internal/devseed"
	"warwick-institute/internal/httpapi"
	"warwick-institute/internal/logging"
	"warwick-institute/internal/pg"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("config error", "err", err)
		os.Exit(2)
	}

	log := logging.New(cfg.LogLevel)
	log.Info("starting", "addr", cfg.Addr, "static_dir", cfg.StaticDir)

	dbpool, err := pg.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer dbpool.Close()

	// Optional dev-only admin seeding.
	// Set ADMIN_USERNAME and ADMIN_PASSWORD to enable.
	if u := os.Getenv("ADMIN_USERNAME"); u != "" {
		if err := devseed.EnsureAdmin(context.Background(), log, dbpool, devseed.EnsureAdminParams{
			Username: u,
			Password: os.Getenv("ADMIN_PASSWORD"),
			Pepper:   cfg.AuthPepper,
		}); err != nil {
			log.Error("ensure admin user", "err", err)
			os.Exit(1)
		}
	}

	// CRM services.
	snapshotSvc, err := crmimport.NewSnapshotService(dbpool, cfg.InstituteTZ)
	if err != nil {
		log.Error("init snapshot service", "error", err)
		os.Exit(1)
	}
	syncSvc := crmimport.NewStudentSyncService(dbpool)
	reconcileV2Svc := reconcile.NewReconcileV2Service(dbpool)

	// Start the CRM v2 queue worker.
	queueStore := queue.NewPostgresQueueStore(dbpool)
	worker := queue.NewQueueWorker(log, queueStore, "crm-worker-main")

	// Register job handlers.
	worker.RegisterHandler(queue.JobTypeImportSnapshot,
		crmimport.ImportSnapshotJobHandler(snapshotSvc, syncSvc, reconcileV2Svc, worker))
	worker.RegisterHandler(queue.JobTypeStudentSync,
		crmimport.StudentSyncJobHandler(syncSvc))
	worker.RegisterHandler(queue.JobTypeCourseReconcileApply,
		reconcile.CourseReconcileJobHandler(reconcileV2Svc, worker))
	worker.RegisterHandler(queue.JobTypeCourseReconcileDiff,
		reconcile.CourseReconcileJobHandler(reconcileV2Svc, worker))

	workerCtx, workerCancel := context.WithCancel(context.Background())
	worker.Start(workerCtx)

	uploadV2Svc, err := crmimport.NewUploadV2Service(dbpool, worker, cfg.InstituteTZ)
	if err != nil {
		log.Error("init upload v2 service", "error", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.NewHandler(log, cfg, dbpool, uploadV2Svc, reconcileV2Svc, worker),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)

	workerCancel()
	worker.Stop()
}
