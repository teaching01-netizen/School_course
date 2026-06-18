package absenceshttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestDispatchDelete_RouteRegistered(t *testing.T) {
	server := &server{}
	fakeID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/absences/"+fakeID.String(), nil)
	w := httptest.NewRecorder()

	server.handleAbsencesDispatch(w, req)

	// Should NOT return 404 — route exists
	if w.Code == http.StatusNotFound {
		t.Fatalf("DELETE /api/v1/absences/{id} should route to a handler, got 404")
	}
}

func TestDispatchDelete_WrongMethod(t *testing.T) {
	server := &server{}
	fakeID := uuid.New()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/absences/"+fakeID.String(), nil)
	w := httptest.NewRecorder()

	server.handleAbsencesDispatch(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("PATCH /api/v1/absences/{id} should return 404, got %d", w.Code)
	}
}

func TestDispatchDelete_NotFoundOnSubpath(t *testing.T) {
	server := &server{}
	fakeID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/absences/"+fakeID.String()+"/unknown-subpath", nil)
	w := httptest.NewRecorder()

	server.handleAbsencesDispatch(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("DELETE /api/v1/absences/{id}/unknown-subpath should return 404, got %d", w.Code)
	}
}

func TestDispatchGet_RoutesToHandler(t *testing.T) {
	server := &server{}
	fakeID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/absences/"+fakeID.String(), nil)
	w := httptest.NewRecorder()

	server.handleAbsencesDispatch(w, req)

	// Should NOT return 404 — GET /absences/{id} is a valid route
	if w.Code == http.StatusNotFound {
		t.Fatalf("GET /api/v1/absences/{id} should route to a handler, got 404")
	}
}

func TestDispatchPost_OnIDPathReturns405(t *testing.T) {
	server := &server{}
	fakeID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/absences/"+fakeID.String(), nil)
	w := httptest.NewRecorder()

	server.handleAbsencesDispatch(w, req)

	// POST on /absences/{id} is explicitly handled as method not allowed
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/v1/absences/{id} should return 405, got %d", w.Code)
	}
}

func TestResolveDateRangeForSessionStartsUsesAllReturnedCourseSessions(t *testing.T) {
	fallbackFrom := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	fallbackTo := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)

	gotFrom, gotTo := resolveDateRangeForSessionStarts([]string{
		"2026-06-09T10:00:00Z",
		"2026-06-02T10:00:00Z",
		"2026-06-16T10:00:00Z",
	}, fallbackFrom, fallbackTo)

	if got := gotFrom.Format("2006-01-02"); got != "2026-06-02" {
		t.Fatalf("from = %s, want 2026-06-02", got)
	}
	if got := gotTo.Format("2006-01-02"); got != "2026-06-16" {
		t.Fatalf("to = %s, want 2026-06-16", got)
	}
}

func TestProjectedAbsenceRecordLimitExceededUsesSubmittedAbsence(t *testing.T) {
	if !projectedAbsenceRecordLimitExceeded(10, 2, 1) {
		t.Fatalf("10-session course with 2 existing records should reject the next absence record")
	}
}

func TestProjectedAbsenceRecordLimitExceededAllowsBoundaryBeforeNextRecord(t *testing.T) {
	if projectedAbsenceRecordLimitExceeded(10, 1, 1) {
		t.Fatalf("10-session course with 1 existing record should allow one more absence record")
	}
}

func TestResolveClientStudentEmailRejectsInvalidEmail(t *testing.T) {
	_, _, err := resolveClientStudentEmail(ptr("not an email"), pgtype.Text{}, pgtype.Text{})
	if err == nil {
		t.Fatalf("invalid client email should be rejected")
	}
}

func TestResolveClientStudentEmailRejectsOverrideWhenStoredEmailExists(t *testing.T) {
	storedSystem := pgtype.Text{String: "stored@example.com", Valid: true}
	_, _, err := resolveClientStudentEmail(ptr("new@example.com"), pgtype.Text{}, storedSystem)
	if err == nil {
		t.Fatalf("client email should not override an existing stored email")
	}
}

func TestResolveClientStudentEmailAcceptsValidEmailWhenNoStoredEmailExists(t *testing.T) {
	email, shouldPersist, err := resolveClientStudentEmail(ptr(" student@example.com "), pgtype.Text{}, pgtype.Text{})
	if err != nil {
		t.Fatalf("resolveClientStudentEmail: %v", err)
	}
	if !email.Valid || email.String != "student@example.com" {
		t.Fatalf("email = %#v, want valid trimmed email", email)
	}
	if !shouldPersist {
		t.Fatalf("valid client email with no stored email should be persisted")
	}
}

func TestClientStudentEmailProvidedIgnoresNilAndBlankValues(t *testing.T) {
	if clientStudentEmailProvided(nil) {
		t.Fatalf("nil email should not count as provided")
	}
	if clientStudentEmailProvided(ptr(" \t ")) {
		t.Fatalf("blank email should not count as provided")
	}
	if !clientStudentEmailProvided(ptr("student@example.com")) {
		t.Fatalf("non-blank email should count as provided")
	}
}

func TestValidateSessionTimingRejectsTooCloseBeforeSession(t *testing.T) {
	now := time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
	settings := absenceFormSettings{MinHoursBeforeSession: 2}
	sessions := []sessionTimingInfo{{
		StartAt: pgtype.Timestamptz{Time: now.Add(90 * time.Minute), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: now.Add(3 * time.Hour), Valid: true},
	}}

	err := validateSessionTiming(settings, now, sessions)
	if err == nil || err.code != "too_close_to_session" || !strings.Contains(err.message, "at least 2 hours") {
		t.Fatalf("validateSessionTiming error = %#v, want too_close_to_session with 2-hour message", err)
	}
}

func TestValidateSessionTimingAllowsExactBeforeBoundary(t *testing.T) {
	now := time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
	settings := absenceFormSettings{MinHoursBeforeSession: 2}
	sessions := []sessionTimingInfo{{
		StartAt: pgtype.Timestamptz{Time: now.Add(2 * time.Hour), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: now.Add(3 * time.Hour), Valid: true},
	}}

	if err := validateSessionTiming(settings, now, sessions); err != nil {
		t.Fatalf("validateSessionTiming exact boundary = %v, want allowed", err)
	}
}

func TestValidateSessionTimingRejectsExpiredGracePeriod(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	settings := absenceFormSettings{MaxHoursAfterSession: 1}
	sessions := []sessionTimingInfo{{
		StartAt: pgtype.Timestamptz{Time: now.Add(-3 * time.Hour), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: now.Add(-61 * time.Minute), Valid: true},
	}}

	err := validateSessionTiming(settings, now, sessions)
	if err == nil || err.code != "grace_period_expired" || !strings.Contains(err.message, "ended 1 hour after class") {
		t.Fatalf("validateSessionTiming error = %#v, want grace_period_expired with 1-hour message", err)
	}
}

func TestValidateSessionTimingDisabledPoliciesAllowPastAndNearSessions(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	settings := absenceFormSettings{}
	sessions := []sessionTimingInfo{
		{StartAt: pgtype.Timestamptz{Time: now.Add(5 * time.Minute), Valid: true}, EndAt: pgtype.Timestamptz{Time: now.Add(65 * time.Minute), Valid: true}},
		{StartAt: pgtype.Timestamptz{Time: now.Add(-3 * time.Hour), Valid: true}, EndAt: pgtype.Timestamptz{Time: now.Add(-2 * time.Hour), Valid: true}},
	}

	if err := validateSessionTiming(settings, now, sessions); err != nil {
		t.Fatalf("disabled timing policies = %v, want allowed", err)
	}
}

func ptr(s string) *string {
	return &s
}
