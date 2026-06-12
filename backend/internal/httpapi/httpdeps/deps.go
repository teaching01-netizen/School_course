package httpdeps

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/crmimport"
	"warwick-institute/internal/crmimport/queue"
	"warwick-institute/internal/crmimport/reconcile"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/emailnotifier"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/otp"
	"warwick-institute/internal/ratelimit"
	"warwick-institute/internal/scheduling"
	"warwick-institute/internal/smartsms"
	"warwick-institute/internal/users"
)

// Deps is the minimal dependency bundle for http route modules.
//
// Keep this small and stable: it is the interface (test surface) for httpapi route modules.
type Deps struct {
	Log         *slog.Logger
	Auth        httpadapter.AuthService
	Q           *sqldb.Queries
	DB          *pgxpool.Pool
	Scheduling  *scheduling.Service
	AdminUsers  *users.AdminProvisioningService
	InstituteTZ string

	CRMUploadV2     *crmimport.UploadV2Service
	CRMReconcileV2  *reconcile.ReconcileV2Service
	CRMWorker       *queue.QueueWorker

	SMS                  smartsms.SMSProvider
	OTPSender            smartsms.OTPProvider
	OTP                  *otp.Service
	RateLimiter          *ratelimit.Store
	CircuitBreaker       *smartsms.CircuitBreaker
	AppOrigin            string
	LegacySyncURL        string
	LegacySyncUsername   string
	LegacySyncPassword   string

	EmailTemplateStore emailnotifier.TemplateStore
	EmailWorkflowStore emailnotifier.WorkflowStore
	EmailService       *emailnotifier.Service
	InstituteName      string

	SitInQuery func(ctx context.Context, instituteTZ string) ([]emailnotifier.SitInReminderRow, error)
}
