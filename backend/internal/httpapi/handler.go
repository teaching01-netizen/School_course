package httpapi

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/auth"
	"warwick-institute/internal/config"
	"warwick-institute/internal/crmimport"
	"warwick-institute/internal/crmimport/queue"
	"warwick-institute/internal/crmimport/reconcile"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/smartsms"
	"warwick-institute/internal/httpapi/absenceshttp"
	"warwick-institute/internal/httpapi/activecourseshttp"
	"warwick-institute/internal/httpapi/adminusershttp"
	"warwick-institute/internal/httpapi/audithttp"
	"warwick-institute/internal/httpapi/availabilityhttp"
	"warwick-institute/internal/httpapi/corehttp"
	"warwick-institute/internal/httpapi/courselevelshttp"
	"warwick-institute/internal/httpapi/courseshttp"
	"warwick-institute/internal/httpapi/crmhttp"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/httpapi/roomshttp"
	"warwick-institute/internal/httpapi/schedulinghttp"
	"warwick-institute/internal/httpapi/serieshttp"
	"warwick-institute/internal/httpapi/sessionshttp"
	"warwick-institute/internal/httpapi/sitinruleshttp"
	"warwick-institute/internal/httpapi/staffabsencehttp"
	"warwick-institute/internal/httpapi/studentshttp"
	"warwick-institute/internal/httpapi/subjectshttp"
	"warwick-institute/internal/httpapi/usershttp"
	"warwick-institute/internal/scheduling"
	"warwick-institute/internal/series"
	"warwick-institute/internal/users"
)

func NewHandler(log *slog.Logger, cfg config.Config, db *pgxpool.Pool, uploadV2 *crmimport.UploadV2Service, reconcileV2 *reconcile.ReconcileV2Service, worker *queue.QueueWorker) http.Handler {
	mux := http.NewServeMux()

	authSvc := auth.NewService(db, auth.Config{
		Pepper: cfg.AuthPepper,
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
	schedulingSvc, err := scheduling.NewService(db, cfg.InstituteTZ, seriesSvc)
	if err != nil {
		// Fail fast at startup for invalid timezone config.
		panic(err)
	}
	deps := httpdeps.Deps{
		Log:            log,
		Auth:           authSvc,
		Q:              q,
		DB:             db,
		Scheduling:     schedulingSvc,
		AdminUsers:     adminUsersSvc,
		InstituteTZ:    cfg.InstituteTZ,
		CRMUploadV2:    uploadV2,
		CRMReconcileV2: reconcileV2,
		CRMWorker:      worker,
	}

	if cfg.SMSServiceUsername != "" && cfg.SMSServicePassword != "" {
		smsClient, err := smartsms.New(smartsms.Config{
			BaseURL:  cfg.SMSServiceBaseURL,
			Username: cfg.SMSServiceUsername,
			Password: cfg.SMSServicePassword,
		})
		if err != nil {
			panic(err)
		}
		deps.SMS = smsClient
	} else {
		deps.SMS = &smartsms.MockProvider{}
	}

	absenceshttp.Register(mux, deps)
	activecourseshttp.Register(mux, deps)
	courselevelshttp.Register(mux, deps)
	corehttp.Register(mux, deps)
	courseshttp.Register(mux, deps)
	subjectshttp.Register(mux, deps)
	roomshttp.Register(mux, deps)
	sitinruleshttp.Register(mux, deps)
	studentshttp.Register(mux, deps)
	sessionshttp.Register(mux, deps)
	staffabsencehttp.Register(mux, deps)
	schedulinghttp.Register(mux, deps)
	usershttp.Register(mux, deps)
	adminusershttp.Register(mux, deps)
	audithttp.Register(mux, deps)
	serieshttp.Register(mux, deps)
	availabilityhttp.Register(mux, deps)
	crmhttp.Register(mux, deps)

	// Static SPA (filesystem, not embedded): serve index.html fallback for client-side routing.
	staticDir := cfg.StaticDir
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
			http.ServeFile(w, r, full)
			return
		}

		// SPA fallback.
		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
	})

	return withRequestTimeout(mux)
}
