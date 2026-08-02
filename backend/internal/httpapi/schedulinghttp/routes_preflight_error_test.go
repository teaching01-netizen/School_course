package schedulinghttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/scheduling"
)

// TestClassifySchedulingErr verifies that classifySchedulingErr correctly maps
// each error type to the appropriate HTTP status code and safe code/message.
// This is the critical path that ensures infrastructure failures don't leak
// as 409 business conflicts.
func TestClassifySchedulingErr(t *testing.T) {
	// Need a logger for the classifySchedulingErr path that logs unknown errors.
	var buf strings.Builder
	s := &server{
		a: httpadapter.Adapter{},
		deps: httpdeps.Deps{
			Log: slog.New(slog.NewTextHandler(&buf, nil)),
		},
	}

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "business conflict should never reach classify (guarded by se != nil)",
			err:        &scheduling.Err{Code: "schedule_conflict", Message: "test"},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal",
		},
		{
			name:       "generic infrastructure error → 500 internal",
			err:        errors.New("database connection lost"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal",
		},
		{
			name:       "context deadline exceeded → 504 timeout",
			err:        context.DeadlineExceeded,
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "timeout",
		},
		{
			name:       "context canceled → 503 canceled",
			err:        context.Canceled,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "canceled",
		},
		{
			name:       "PostgreSQL internal error → 500 db_error",
			err:        &pgconn.PgError{Code: "XX000", Message: "internal error"},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "db_error",
		},
		{
			name:       "PostgreSQL XX001 → 500 db_error",
			err:        &pgconn.PgError{Code: "XX001", Message: "data corruption"},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "db_error",
		},
		{
			name:       "scheduled conflict PG error → 409 with schedule_conflict code",
			err:        &pgconn.PgError{Code: "23P01", Message: "exclusion violation"},
			wantStatus: http.StatusConflict,
			wantCode:   "schedule_conflict",
		},
		{
			name:       "unknown application error → 500 internal",
			err:        errors.New("something bad happened"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, code, _ := s.classifySchedulingErr(tc.err)
			if status != tc.wantStatus {
				t.Errorf("expected status %d, got %d", tc.wantStatus, status)
			}
			if code != tc.wantCode {
				t.Errorf("expected code %q, got %q", tc.wantCode, code)
			}
		})
	}
}
