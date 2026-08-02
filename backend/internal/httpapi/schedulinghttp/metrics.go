package schedulinghttp

import (
	"sync/atomic"

	"warwick-institute/internal/httpapi/httpadapter"
)

var (
	preflightRequestsTotal  atomic.Int64
	preflightConflictsTotal atomic.Int64
	preflightErrorsTotal    atomic.Int64
)

func init() {
	// Log counters periodically or expose via a /debug/metrics endpoint
}

// MetricsSnapshot returns a snapshot of all scheduling HTTP metrics counters.
func MetricsSnapshot() map[string]int64 {
	return map[string]int64{
		"schedule_preflight_requests_total":  preflightRequestsTotal.Load(),
		"schedule_preflight_conflicts_total": preflightConflictsTotal.Load(),
		"schedule_preflight_errors_total":    preflightErrorsTotal.Load(),
		"schedule_transaction_retries_total": httpadapter.TxRetriesTotal.Load(),
	}
}
