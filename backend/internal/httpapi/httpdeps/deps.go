package httpdeps

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/crmimport"
	"warwick-institute/internal/crmimport/queue"
	"warwick-institute/internal/crmimport/reconcile"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
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

	SMS smartsms.SMSProvider
}
