package absenceshttp

import (
	"context"
	"encoding/json"
	"errors"
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

func TestAbsenceDayLimitLockKeyNormalizesWCode(t *testing.T) {
	first := absenceDayLimitLockKey(" W250389 ", "course-1")
	second := absenceDayLimitLockKey("w250389", "course-1")
	if first != second {
		t.Fatalf("lock keys differ: %q != %q", first, second)
	}
	if first != "absence-limit:w250389:course-1" {
		t.Fatalf("lock key = %q", first)
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
func TestParentVerificationStatusUsesPOSTBodyAndNoURLTokenRoute(t *testing.T) {
	s := &server{}
	post := httptest.NewRequest(http.MethodPost, "/api/v1/absences/parent-verification/status", strings.NewReader(`{"token":""}`))
	post.Header.Set("Content-Type", "application/json")
	postRecorder := httptest.NewRecorder()
	s.handleAbsencesDispatch(postRecorder, post)
	if postRecorder.Code == http.StatusNotFound {
		t.Fatal("POST status endpoint was not registered")
	}

	get := httptest.NewRequest(http.MethodGet, "/api/v1/absences/parent-verification/opaque-token", nil)
	getRecorder := httptest.NewRecorder()
	s.handleAbsencesDispatch(getRecorder, get)
	if getRecorder.Code != http.StatusNotFound {
		t.Fatalf("GET URL token endpoint status = %d, want 404", getRecorder.Code)
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

type courseResponseCountFields struct {
	TotalCourseDays      int32 `json:"total_course_days"`
	UsedAbsenceDays      int32 `json:"used_absence_days"`
	MaximumAbsenceDays   int32 `json:"maximum_absence_days"`
	RemainingAbsenceDays int32 `json:"remaining_absence_days"`
	AbsenceLimitReached  bool  `json:"absence_limit_reached"`
}

func TestCourseResponse_SerializesCountFields(t *testing.T) {
	resp := courseResponseCountFields{
		TotalCourseDays:      20,
		UsedAbsenceDays:      3,
		MaximumAbsenceDays:   4,
		RemainingAbsenceDays: 1,
		AbsenceLimitReached:  false,
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if v, ok := decoded["used_absence_days"]; !ok {
		t.Fatal("missing used_absence_days field in JSON response")
	} else if v.(float64) != 3 {
		t.Fatalf("used_absence_days = %v, want 3", v)
	}
	if v, ok := decoded["total_course_days"]; !ok {
		t.Fatal("missing total_course_days field in JSON response")
	} else if v.(float64) != 20 {
		t.Fatalf("total_course_days = %v, want 20", v)
	}
	if v := decoded["maximum_absence_days"].(float64); v != 4 {
		t.Fatalf("maximum_absence_days = %v, want 4", v)
	}
	if v := decoded["remaining_absence_days"].(float64); v != 1 {
		t.Fatalf("remaining_absence_days = %v, want 1", v)
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
