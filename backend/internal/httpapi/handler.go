package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/absences/sitinresolver"
	"warwick-institute/internal/auth"
	"warwick-institute/internal/clientip"
	"warwick-institute/internal/config"
	"warwick-institute/internal/courseadmin"
	"warwick-institute/internal/crmimport"
	"warwick-institute/internal/crmimport/crossstudy"
	"warwick-institute/internal/crmimport/queue"
	"warwick-institute/internal/crmimport/reconcile"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/emailnotifier"
	"warwick-institute/internal/httpapi/absenceshttp"
	"warwick-institute/internal/httpapi/activecourseshttp"
	"warwick-institute/internal/httpapi/adminusershttp"
	"warwick-institute/internal/httpapi/audithttp"
	"warwick-institute/internal/httpapi/availabilityhttp"
	"warwick-institute/internal/httpapi/corehttp"
	"warwick-institute/internal/httpapi/courselevelshttp"
	"warwick-institute/internal/httpapi/courseshttp"
	"warwick-institute/internal/httpapi/crmhttp"
	"warwick-institute/internal/httpapi/emailnotifierhttp"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/httpapi/legacysynchttp"
	"warwick-institute/internal/httpapi/realtimehttp"
	"warwick-institute/internal/httpapi/roomshttp"
	"warwick-institute/internal/httpapi/satverbalpolicyhttp"
	"warwick-institute/internal/httpapi/schedulinghttp"
	"warwick-institute/internal/httpapi/serieshttp"
	"warwick-institute/internal/httpapi/sessionchangehttp"
	"warwick-institute/internal/httpapi/sessionshttp"
	"warwick-institute/internal/httpapi/sitinruleshttp"
	"warwick-institute/internal/httpapi/staffabsencehttp"
	"warwick-institute/internal/httpapi/studentshttp"
	"warwick-institute/internal/httpapi/subjectshttp"
	"warwick-institute/internal/httpapi/teacherhttp"
	"warwick-institute/internal/httpapi/usershttp"
	"warwick-institute/internal/otp"
	"warwick-institute/internal/otpdelivery"
	"warwick-institute/internal/ratelimit"
	"warwick-institute/internal/realtime"
	"warwick-institute/internal/scheduling"
	"warwick-institute/internal/series"
	"warwick-institute/internal/sessionchangeimpact"
	"warwick-institute/internal/smartsms"
	"warwick-institute/internal/studentauth"
	"warwick-institute/internal/users"
)

type rateLimitAdapter struct {
	store *ratelimit.Store
}

func (a *rateLimitAdapter) Allow(ctx context.Context, key string, limit int, window time.Duration) (auth.RateLimitResult, error) {
	result, err := a.store.Allow(ctx, key, limit, window)
	if err != nil {
		return auth.RateLimitResult{}, err
	}
	return auth.RateLimitResult{Allowed: result.Allowed, Remaining: result.Remaining}, nil
}

type EmailDeps struct {
	TemplateStore emailnotifier.TemplateStore
	WorkflowStore emailnotifier.WorkflowStore
	Service       *emailnotifier.Service
	InstituteName string
	SitInQuery    func(ctx context.Context, instituteTZ string) ([]emailnotifier.SitInReminderRow, error)
}

type OTPDeliveryDeps struct {
	SMS            smartsms.SMSProvider
	Sender         smartsms.OTPProvider
	Dispatcher     *otpdelivery.Dispatcher
	Store          *otpdelivery.Store
	CircuitBreaker *smartsms.CircuitBreaker
}

func NewEmailDeps(log *slog.Logger, cfg config.Config, db *pgxpool.Pool, q *sqldb.Queries) EmailDeps {
	var emailProvider emailnotifier.EmailProvider
	if cfg.EmailWebhookURL != "" {
		emailProvider = emailnotifier.NewGASWebhookProvider(cfg.EmailWebhookURL, cfg.EmailWebhookSecret, log)
	} else {
		emailProvider = emailnotifier.NewLogProvider(log)
	}
	templateStore := emailnotifier.NewSQLTemplateStore(db)
	workflowStore := emailnotifier.NewSQLWorkflowStore(db)
	return EmailDeps{
		TemplateStore: templateStore,
		WorkflowStore: workflowStore,
		Service:       emailnotifier.NewServiceWithDeliveryTracker(emailProvider, workflowStore),
		InstituteName: cfg.InstituteName,
		SitInQuery: func(ctx context.Context, instituteTZ string) ([]emailnotifier.SitInReminderRow, error) {
			loc, effectiveTZ := emailnotifier.EffectiveLocation(instituteTZ)
			today := time.Now().In(loc).Format("2006-01-02")
			dbRows, dbErr := q.QueryTodaySitIns(ctx, today, effectiveTZ)
			if dbErr != nil {
				return nil, dbErr
			}
			result := make([]emailnotifier.SitInReminderRow, len(dbRows))
			for i, r := range dbRows {
				result[i] = emailnotifier.SitInReminderRow{
					StudentName:        r.StudentName,
					StudentNickname:    r.StudentNickname,
					WCode:              r.WCode,
					CourseName:         r.CourseName,
					SitInCourseName:    r.SitInCourseName,
					TeacherName:        r.TeacherName,
					TeacherEmail:       r.TeacherEmail,
					AbsenceDateRange:   r.AbsenceDateRange,
					MissedSessionsInfo: r.MissedSessionsInfo,
					StartAt:            r.StartAt,
					EndAt:              r.EndAt,
				}
			}
			return result, nil
		},
	}
}

func NewHandler(log *slog.Logger, cfg config.Config, db *pgxpool.Pool, uploadV2 *crmimport.UploadV2Service, reconcileV2 *reconcile.ReconcileV2Service, worker *queue.QueueWorker, emailDeps EmailDeps, otpDelivery *OTPDeliveryDeps, impact *sessionchangeimpact.Service, realtimeHub ...*realtime.Hub) http.Handler {
	mux := http.NewServeMux()
	hub := realtime.NewHub()
	if len(realtimeHub) > 0 && realtimeHub[0] != nil {
		hub = realtimeHub[0]
	}

	hasher := auth.NewArgon2PasswordHasher(cfg.AuthPepper)
	sessionStore := auth.NewPGSessionStore(db, log)
	userStore := auth.NewPGUserStore(db)
	rlStore := ratelimit.NewStore(db)
	primaryLoginLimiter := auth.NewDBLoginRateLimiter(&rateLimitAdapter{store: rlStore})
	loginLimiter := auth.NewResilientLoginRateLimiter(primaryLoginLimiter, auth.NewInMemoryLoginRateLimiter())

	clientIPResolver, err := clientip.NewResolver(cfg.TrustedProxyCIDRs)
	if err != nil {
		panic(err)
	}

	authSvc := auth.NewService(auth.ServiceOptions{
		Hasher:       hasher,
		Sessions:     sessionStore,
		Limiter:      loginLimiter,
		Users:        userStore,
		Log:          log,
		CookieSecure: cfg.CookieSecure,
		IPResolver:   clientIPResolver,
	})
	q := sqldb.New(db)
	adminUsersSvc := users.NewAdminProvisioningService(
		users.SQLCAdminUserStore{Q: q},
		users.AuthPasswordHasher{Pepper: cfg.AuthPepper},
	)

	seriesSvc, err := series.NewService(db, cfg.InstituteTZ)
	if err != nil {
		panic(err)
	}
	schedulingSvc, err := scheduling.NewService(db, cfg.InstituteTZ, seriesSvc, log)
	if err != nil {
		// Fail fast at startup for invalid timezone config.
		panic(err)
	}
	courseAdminSvc := courseadmin.NewService()
	deps := httpdeps.Deps{
		Log:                 log,
		Auth:                authSvc,
		Q:                   q,
		DB:                  db,
		Scheduling:          schedulingSvc,
		CourseAdmin:         courseAdminSvc,
		SessionChangeImpact: impact,
		SitInResolver:       sitinresolver.New(q, cfg.InstituteTZ),
		ClientIP:            clientIPResolver,
		AdminUsers:          adminUsersSvc,
		InstituteTZ:         cfg.InstituteTZ,
		StudentCookieSecure: cfg.CookieSecure,
		CRMUploadV2:         uploadV2,
		CRMReconcileV2:      reconcileV2,
		CRMWorker:           worker,
		RateLimiter:         ratelimit.NewStore(db),
		Realtime:            hub,
		AppOrigin:           cfg.AppOrigin,
		LegacySyncURL:       cfg.LegacySyncURL,
		LegacySyncUsername:  cfg.LegacySyncUsername,
		LegacySyncPassword:  cfg.LegacySyncPassword,
		StudentSelfService:  studentauth.NewService(db),
	}

	otpSvc, err := otp.NewService(db, cfg.OTPHMACKey)
	if err != nil {
		panic(err)
	}
	deps.OTP = otpSvc

	otpProviderMode := cfg.OTPSMSProvider
	if otpProviderMode == "" {
		otpProviderMode = "mock"
	}

	if otpDelivery != nil {
		deps.SMS = otpDelivery.SMS
		deps.OTPSender = otpDelivery.Sender
		deps.OTPDelivery = otpDelivery.Dispatcher
		deps.OTPDeliveryStore = otpDelivery.Store
		deps.OTPAsyncDelivery = true
		deps.CircuitBreaker = otpDelivery.CircuitBreaker
	} else if otpProviderMode == "smartsms" && cfg.SMSServiceUsername != "" && cfg.SMSServicePassword != "" {
		smsClient, err := smartsms.New(smartsms.Config{
			BaseURL:  cfg.SMSServiceBaseURL,
			Username: cfg.SMSServiceUsername,
			Password: cfg.SMSServicePassword,
		})
		if err != nil {
			panic(err)
		}
		deps.SMS = smsClient
		deps.OTPSender = &smartsms.OTPAdapter{Client: smsClient}
		deps.CircuitBreaker = smartsms.NewCircuitBreaker(db, "smartsms")
	} else {
		deps.SMS = &smartsms.MockProvider{}
		deps.OTPSender = &smartsms.MockProvider{}
		deps.CircuitBreaker = smartsms.NewCircuitBreaker(db, "mock")
	}

	deps.EmailTemplateStore = emailDeps.TemplateStore
	deps.EmailWorkflowStore = emailDeps.WorkflowStore
	deps.EmailService = emailDeps.Service
	deps.InstituteName = emailDeps.InstituteName
	deps.SitInQuery = emailDeps.SitInQuery

	absenceshttp.Register(mux, deps)
	emailnotifierhttp.Register(mux, deps)
	activecourseshttp.Register(mux, deps)
	courselevelshttp.Register(mux, deps)
	corehttp.Register(mux, deps)
	courseshttp.Register(mux, deps)
	subjectshttp.Register(mux, deps)
	roomshttp.Register(mux, deps)
	satverbalpolicyhttp.Register(mux, deps)
	sitinruleshttp.Register(mux, deps)
	studentshttp.Register(mux, deps)
	sessionshttp.Register(mux, deps)
	sessionchangehttp.Register(mux, deps)
	staffabsencehttp.Register(mux, deps)
	schedulinghttp.Register(mux, deps)
	usershttp.Register(mux, deps)
	adminusershttp.Register(mux, deps)
	legacysynchttp.Register(mux, deps)
	audithttp.Register(mux, deps)
	serieshttp.Register(mux, deps)
	availabilityhttp.Register(mux, deps)
	teacherhttp.Register(mux, deps)
	crossStudyStore := crossstudy.NewStore(db)
	deps.CrossStudy = crossStudyStore
	crmhttp.Register(mux, deps)
	crmhttp.RegisterCrossStudy(mux, deps)
	realtimehttp.Register(mux, deps)

	// Static SPA (filesystem, not embedded): serve index.html fallback for client-side routing.
	mux.HandleFunc("/", staticHandler(cfg.StaticDir))

	return withRequestBodyLimit(withRequestTimeout(mux))
}

// staticHandler serves the built SPA from staticDir. Exact file hits are served
// directly. Extension-less paths (client-side routes such as /operations/schedule-impact)
// fall back to index.html. Missing paths that carry a file extension (e.g. a stale
// /assets/*.js hash from a previous deploy) return a real 404 — serving index.html
// there makes browsers reject it under strict MIME checking ("Failed to load module
// script ... MIME type of text/html") and breaks lazy-loaded routes.
func staticHandler(staticDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Try exact file first.
		cleanPath := filepath.Clean(r.URL.Path)
		if cleanPath == "/" {
			cleanPath = "/index.html"
		}
		full := filepath.Join(staticDir, strings.TrimPrefix(cleanPath, "/"))
		if st, err := os.Stat(full); err == nil && !st.IsDir() {
			setStaticCacheControl(w, r.URL.Path)
			http.ServeFile(w, r, full)
			return
		}

		// A missing path with a file extension is a genuine 404 (missing asset),
		// never a client-side route.
		if filepath.Ext(cleanPath) != "" {
			http.NotFound(w, r)
			return
		}

		// SPA fallback: index.html must always be revalidated so browsers pick up
		// the new build's chunk hashes after a deploy instead of pinning a stale bundle.
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
	}
}

// setStaticCacheControl applies cache headers for existing files: Vite content-hashes
// build assets so they are immutable; everything else (index.html shell) revalidates.
func setStaticCacheControl(w http.ResponseWriter, path string) {
	if strings.HasPrefix(path, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
}
