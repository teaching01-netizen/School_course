package absenceshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/auth"
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

func TestProjectedAbsenceSessionLimitExceededRejectsExcessiveSessions(t *testing.T) {
	if !projectedAbsenceSessionLimitExceeded(10, 0, 3) {
		t.Fatalf("10-session course with 0 existing should reject 3 sessions (30%%)")
	}
}

func TestProjectedAbsenceSessionLimitExceededAllowsExactBoundary(t *testing.T) {
	if projectedAbsenceSessionLimitExceeded(10, 0, 2) {
		t.Fatalf("10-session course with 0 existing should allow 2 sessions (20%%)")
	}
}

func TestProjectedAbsenceSessionLimitExceededAllowsCumulativeLimit(t *testing.T) {
	if projectedAbsenceSessionLimitExceeded(10, 1, 1) {
		t.Fatalf("10-session course with 1 existing should allow 1 more session (20%% total)")
	}
}

func TestProjectedAbsenceSessionLimitExceededRejectsCumulativeOverLimit(t *testing.T) {
	if !projectedAbsenceSessionLimitExceeded(10, 1, 2) {
		t.Fatalf("10-session course with 1 existing should reject 2 more sessions (30%% total)")
	}
}

func TestProjectedAbsenceSessionLimitExceededHandlesZeroTotalSessions(t *testing.T) {
	if projectedAbsenceSessionLimitExceeded(0, 0, 1) {
		t.Fatalf("0-session course should not allow any sessions")
	}
}

func TestProjectedAbsenceSessionLimitExceededHandlesZeroSubmittingSessions(t *testing.T) {
	if projectedAbsenceSessionLimitExceeded(10, 0, 0) {
		t.Fatalf("0 submitting sessions should not be exceeded")
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

func TestValidateSessionTimingAllowsEndedSessionInsideGracePeriodWhenBeforeCutoffEnabled(t *testing.T) {
	now := time.Date(2026, 6, 18, 9, 30, 0, 0, time.UTC)
	settings := absenceFormSettings{MinHoursBeforeSession: 2, MaxHoursAfterSession: 1}
	sessions := []sessionTimingInfo{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC), Valid: true},
	}}

	if err := validateSessionTiming(settings, now, sessions); err != nil {
		t.Fatalf("ended session inside configured grace period = %v, want allowed", err)
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

func TestSessionAllowedByTimingPolicyFiltersLookupSessions(t *testing.T) {
	now := time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
	settings := absenceFormSettings{MinHoursBeforeSession: 2, MaxHoursAfterSession: 1}

	allowed := sessionTimingInfo{
		StartAt: pgtype.Timestamptz{Time: now.Add(2*time.Hour + time.Minute), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: now.Add(4 * time.Hour), Valid: true},
	}
	tooClose := sessionTimingInfo{
		StartAt: pgtype.Timestamptz{Time: now.Add(90 * time.Minute), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: now.Add(3 * time.Hour), Valid: true},
	}
	expired := sessionTimingInfo{
		StartAt: pgtype.Timestamptz{Time: now.Add(-3 * time.Hour), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: now.Add(-61 * time.Minute), Valid: true},
	}

	if !sessionAllowedByTimingPolicy(settings, now, allowed) {
		t.Fatalf("session outside cutoff windows should remain visible")
	}
	if sessionAllowedByTimingPolicy(settings, now, tooClose) {
		t.Fatalf("session inside the pre-session cutoff should be filtered")
	}
	if sessionAllowedByTimingPolicy(settings, now, expired) {
		t.Fatalf("session after the post-session grace period should be filtered")
	}
}

func TestSessionsInRangeQueryAppliesRequestedDateBounds(t *testing.T) {
	sql := sessionsInRangeSelectSQL()

	if !strings.Contains(sql, "sess.start_at >= $2") {
		t.Fatalf("sessions-in-range query should apply date_from bound, SQL: %s", sql)
	}
	if !strings.Contains(sql, "sess.start_at < $3") {
		t.Fatalf("sessions-in-range query should apply exclusive date_to bound, SQL: %s", sql)
	}
}

func TestResolveDateRangeForSessionStartsUsesInstituteTimezone(t *testing.T) {
	fallbackFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fallbackTo := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	from, to := resolveDateRangeForSessionStarts([]string{"2026-01-15T17:00:00Z"}, fallbackFrom, fallbackTo)

	want := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
	if !from.Equal(want) || !to.Equal(want) {
		t.Fatalf("resolveDateRangeForSessionStarts = %s to %s, want Bangkok date %s", from.Format(time.RFC3339), to.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestSessionDateKeyUsesInstituteTimezone(t *testing.T) {
	got := sessionDateKey("2026-01-15T17:00:00Z", "Asia/Bangkok")
	if got != "2026-01-16" {
		t.Fatalf("sessionDateKey = %q, want Bangkok date 2026-01-16", got)
	}
}

func TestParseInstituteLocalDateReturnsUTCBoundaryForBangkokDay(t *testing.T) {
	got, err := parseInstituteLocalDate("2026-01-16", "Asia/Bangkok")
	if err != nil {
		t.Fatalf("parseInstituteLocalDate: %v", err)
	}
	want := time.Date(2026, 1, 15, 17, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("parseInstituteLocalDate = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestSessionsInRangeResponseUsesInstituteTimezoneDateKey(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "routes.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read routes.go: %v", err)
	}
	source := string(data)

	if strings.Contains(source, "Date:          sess.StartAt[:10]") {
		t.Fatal("sessions-in-range response must not derive session date by slicing the UTC timestamp")
	}
	if !strings.Contains(source, "sessionDateKey(sess.StartAt, s.deps.InstituteTZ)") {
		t.Fatal("sessions-in-range response should derive session date in the institute timezone")
	}
}

func TestSessionsInRangeAllSubjectsQueryUsesExplicitSubjectFilter(t *testing.T) {
	sql := sessionsInRangeAllSubjectsSelectSQL()

	if !strings.Contains(sql, "sub.id::text = ANY(string_to_array($1, ','))") {
		t.Fatalf("all-subjects query should require explicit subject IDs, SQL: %s", sql)
	}
	if strings.Contains(sql, "course_students") {
		t.Fatalf("all-subjects query should not require student enrollment, SQL: %s", sql)
	}
}

func TestMaxSessionsLookupRangeDaysIncludesPostSessionLookback(t *testing.T) {
	tests := []struct {
		name     string
		settings absenceFormSettings
		want     int
	}{
		{name: "disabled", settings: absenceFormSettings{MaxDateRangeDays: 30}, want: 30},
		{name: "one hour crosses a calendar boundary", settings: absenceFormSettings{MaxDateRangeDays: 30, MaxHoursAfterSession: 1}, want: 31},
		{name: "twenty five hours needs two days", settings: absenceFormSettings{MaxDateRangeDays: 30, MaxHoursAfterSession: 25}, want: 32},
		{name: "forty eight hours needs two days", settings: absenceFormSettings{MaxDateRangeDays: 30, MaxHoursAfterSession: 48}, want: 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maxSessionsLookupRangeDays(tt.settings); got != tt.want {
				t.Fatalf("maxSessionsLookupRangeDays(%+v) = %d, want %d", tt.settings, got, tt.want)
			}
		})
	}
}

func TestProjectedAbsenceRecordLimitExceeded_ZeroTotalSessions(t *testing.T) {
	if projectedAbsenceRecordLimitExceeded(0, 1, 1) {
		t.Fatal("zero total sessions should not be exceeded (guard)")
	}
}

func TestProjectedAbsenceRecordLimitExceeded_ZeroSubmittingRecords(t *testing.T) {
	if projectedAbsenceRecordLimitExceeded(10, 2, 0) {
		t.Fatal("zero submitting records should not be exceeded (guard)")
	}
}

func TestProjectedAbsenceRecordLimitExceeded_AllowsBoundary(t *testing.T) {
	// Formula: (existing+1)*5 <= total → allowed
	tests := []struct {
		total, existing int32
	}{
		{total: 5, existing: 0},  // (0+1)*5 = 5 <= 5
		{total: 9, existing: 0},  // (0+1)*5 = 5 <= 9
		{total: 10, existing: 1}, // (1+1)*5 = 10 <= 10
		{total: 14, existing: 1}, // (1+1)*5 = 10 <= 14
		{total: 15, existing: 2}, // (2+1)*5 = 15 <= 15
		{total: 20, existing: 3}, // (3+1)*5 = 20 <= 20
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%d-sessions-%d-existing", tt.total, tt.existing)
		t.Run(name, func(t *testing.T) {
			if projectedAbsenceRecordLimitExceeded(tt.total, tt.existing, 1) {
				t.Fatalf("%d-session course with %d existing records should allow one more (want false)", tt.total, tt.existing)
			}
		})
	}
}

func TestProjectedAbsenceRecordLimitExceeded_BlocksPastBoundary(t *testing.T) {
	// Formula: (existing+1)*5 > total → blocked
	tests := []struct {
		total, existing int32
	}{
		{total: 4, existing: 0},  // (0+1)*5 = 5 > 4
		{total: 5, existing: 1},  // (1+1)*5 = 10 > 5
		{total: 9, existing: 1},  // (1+1)*5 = 10 > 9
		{total: 10, existing: 2}, // (2+1)*5 = 15 > 10
		{total: 14, existing: 2}, // (2+1)*5 = 15 > 14
		{total: 15, existing: 3}, // (3+1)*5 = 20 > 15
		{total: 20, existing: 4}, // (4+1)*5 = 25 > 20
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%d-sessions-%d-existing", tt.total, tt.existing)
		t.Run(name, func(t *testing.T) {
			if !projectedAbsenceRecordLimitExceeded(tt.total, tt.existing, 1) {
				t.Fatalf("%d-session course with %d existing records should reject the next absence (want true)", tt.total, tt.existing)
			}
		})
	}
}

type courseResponseCountFields struct {
	ExistingAbsenceCount int32 `json:"existing_absence_count"`
	TotalSessionCount    int32 `json:"total_session_count"`
}

func TestCourseResponse_SerializesCountFields(t *testing.T) {
	resp := courseResponseCountFields{
		ExistingAbsenceCount: 3,
		TotalSessionCount:    20,
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if v, ok := decoded["existing_absence_count"]; !ok {
		t.Fatal("missing existing_absence_count field in JSON response")
	} else if v.(float64) != 3 {
		t.Fatalf("existing_absence_count = %v, want 3", v)
	}
	if v, ok := decoded["total_session_count"]; !ok {
		t.Fatal("missing total_session_count field in JSON response")
	} else if v.(float64) != 20 {
		t.Fatalf("total_session_count = %v, want 20", v)
	}
}

func ptr(s string) *string {
	return &s
}

type mockSessionValidator struct {
	user auth.AuthenticatedUser
	err  error
}

func (m mockSessionValidator) RequireUser(ctx context.Context, r *http.Request) (auth.AuthenticatedUser, error) {
	return m.user, m.err
}

func TestIsAdminRequest_AdminUserReturnsTrue(t *testing.T) {
	v := mockSessionValidator{user: auth.AuthenticatedUser{Role: "Admin"}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if !isAdminRequest(v, req) {
		t.Fatal("isAdminRequest should return true for admin user")
	}
}

func TestIsAdminRequest_NonAdminUserReturnsFalse(t *testing.T) {
	v := mockSessionValidator{user: auth.AuthenticatedUser{Role: "User"}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if isAdminRequest(v, req) {
		t.Fatal("isAdminRequest should return false for non-admin user")
	}
}

func TestIsAdminRequest_UnauthenticatedReturnsFalse(t *testing.T) {
	v := mockSessionValidator{err: errors.New("no session")}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if isAdminRequest(v, req) {
		t.Fatal("isAdminRequest should return false for unauthenticated request")
	}
}
