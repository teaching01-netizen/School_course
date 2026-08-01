package httpdeps

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/absences/sitinresolver"
	"warwick-institute/internal/courseadmin"
	"warwick-institute/internal/crmimport"
	"warwick-institute/internal/crmimport/crossstudy"
	"warwick-institute/internal/crmimport/queue"
	"warwick-institute/internal/crmimport/reconcile"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/emailnotifier"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/otp"
	"warwick-institute/internal/otpdelivery"
	"warwick-institute/internal/ratelimit"
	"warwick-institute/internal/realtime"
	"warwick-institute/internal/scheduling"
	"warwick-institute/internal/sessionchangeimpact"
	"warwick-institute/internal/smartsms"
	"warwick-institute/internal/users"
)

// Deps is the minimal dependency bundle for http route modules.
//
// Keep this small and stable: it is the interface (test surface) for httpapi route modules.
type Deps struct {
	Log                 *slog.Logger
	Auth                httpadapter.AuthService
	Q                   *sqldb.Queries
	DB                  *pgxpool.Pool
	Scheduling          *scheduling.Service
	CourseAdmin         *courseadmin.Service
	SessionChangeImpact *sessionchangeimpact.Service
	SitInResolver       *sitinresolver.Service
	AdminUsers          *users.AdminProvisioningService
	InstituteTZ         string

	CRMUploadV2    *crmimport.UploadV2Service
	CRMReconcileV2 *reconcile.ReconcileV2Service
	CRMWorker      *queue.QueueWorker
	CrossStudy     *crossstudy.Store

	SMS                smartsms.SMSProvider
	OTPSender          smartsms.OTPProvider
	OTP                *otp.Service
	OTPDelivery        *otpdelivery.Dispatcher
	OTPDeliveryStore   *otpdelivery.Store
	OTPAsyncDelivery   bool
	RateLimiter        *ratelimit.Store
	Realtime           *realtime.Hub
	CircuitBreaker     *smartsms.CircuitBreaker
	AppOrigin          string
	LegacySyncURL      string
	LegacySyncUsername string
	LegacySyncPassword string

	EmailTemplateStore emailnotifier.TemplateStore
	EmailWorkflowStore emailnotifier.WorkflowStore
	EmailService       *emailnotifier.Service
	InstituteName      string

	SitInQuery func(ctx context.Context, instituteTZ string) ([]emailnotifier.SitInReminderRow, error)
}
