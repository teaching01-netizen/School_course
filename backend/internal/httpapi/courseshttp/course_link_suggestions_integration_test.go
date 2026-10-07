package courseshttp

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type courseLinkSuggestionAPIItem struct {
	ConfiguredSource struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	} `json:"configured_source"`
	Unconfigured struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	} `json:"unconfigured_course"`
	Confidence  string   `json:"confidence"`
	ReasonCodes []string `json:"reason_codes"`
	Fingerprint string   `json:"evidence_fingerprint"`
}

type courseLinkSuggestionAPIEnvelope struct {
	Enabled             bool                          `json:"enabled"`
	ConfirmationEnabled bool                          `json:"confirmation_enabled"`
	Items               []courseLinkSuggestionAPIItem `json:"items"`
	HasMore             bool                          `json:"has_more"`
	NextCursor          *string                       `json:"next_cursor"`
	Mode                string                        `json:"mode"`
	DetectorVersion     string                        `json:"detector_version"`
}

type seededCourseLinkSuggestion struct {
	sourceID        pgtype.UUID
	partialID       pgtype.UUID
	sourceIDString  string
	partialIDString string
	partialSession  pgtype.UUID
}

func TestCourseLinkSuggestionAPI_ListPaginationAndDismiss(t *testing.T) {
	fx := setupTestServer(t)
	pairA := seedCourseLinkSuggestionAPIPair(t, fx, "dismiss-a", true)
	pairB := seedCourseLinkSuggestionAPIPair(t, fx, "dismiss-b", true)
	review := seedCourseLinkSuggestionAPIPair(t, fx, "review", false)

	highPages := listAllCourseLinkSuggestionsAPI(t, fx, false, 1)
	if len(highPages) < 2 {
		t.Fatalf("pagination returned %d pages, want at least 2", len(highPages))
	}
	for index, page := range highPages {
		if page.Mode != "confirmation" || page.DetectorVersion != sqldb.CourseLinkSuggestionDetectorVersion {
			t.Fatalf("unexpected list metadata: mode=%q version=%q", page.Mode, page.DetectorVersion)
		}
		isLast := index == len(highPages)-1
		if page.HasMore == isLast || (page.HasMore && page.NextCursor == nil) {
			t.Fatalf("invalid pagination metadata on page %d: has_more=%v cursor=%v", index, page.HasMore, page.NextCursor)
		}
	}
	if findCourseLinkSuggestionAPIItem(highPages, pairA) == nil || findCourseLinkSuggestionAPIItem(highPages, pairB) == nil {
		t.Fatal("paginated results did not include both high-confidence fixture pairs")
	}
	if findCourseLinkSuggestionAPIItem(highPages, review) != nil {
		t.Fatal("default list exposed a review candidate")
	}
	withReviewPages := listAllCourseLinkSuggestionsAPI(t, fx, true, 100)
	if item := findCourseLinkSuggestionAPIItem(withReviewPages, review); item == nil || item.Confidence != "review" {
		t.Fatalf("review toggle did not expose the review pair: %+v", item)
	}

	dismissedItem := findCourseLinkSuggestionAPIItem(withReviewPages, pairA)
	if dismissedItem == nil {
		t.Fatal("dismiss fixture missing from list response")
	}
	body := map[string]string{
		"configured_course_id":   pairA.sourceIDString,
		"unconfigured_course_id": pairA.partialIDString,
		"evidence_fingerprint":   dismissedItem.Fingerprint,
		"detector_version":       sqldb.CourseLinkSuggestionDetectorVersion,
	}
	resp := doRequest(t, fx.server.URL, http.MethodPost, "/api/v1/admin/course-link-suggestions/dismiss", body)
	if resp.StatusCode != http.StatusOK {
		var apiErr map[string]any
		parseResponse(t, resp, &apiErr)
		t.Fatalf("dismiss status=%d response=%v", resp.StatusCode, apiErr)
	}
	var firstDismiss struct {
		Dismissed        bool `json:"dismissed"`
		AlreadyDismissed bool `json:"already_dismissed"`
	}
	parseResponse(t, resp, &firstDismiss)
	if !firstDismiss.Dismissed || firstDismiss.AlreadyDismissed {
		t.Fatalf("unexpected first dismissal response: %+v", firstDismiss)
	}

	resp = doRequest(t, fx.server.URL, http.MethodPost, "/api/v1/admin/course-link-suggestions/dismiss", body)
	if resp.StatusCode != http.StatusOK {
		var apiErr map[string]any
		parseResponse(t, resp, &apiErr)
		t.Fatalf("repeat dismissal status=%d response=%v", resp.StatusCode, apiErr)
	}
	var repeatDismiss struct {
		Dismissed        bool `json:"dismissed"`
		AlreadyDismissed bool `json:"already_dismissed"`
	}
	parseResponse(t, resp, &repeatDismiss)
	if !repeatDismiss.Dismissed || !repeatDismiss.AlreadyDismissed {
		t.Fatalf("repeat dismissal was not a successful no-op: %+v", repeatDismiss)
	}
	var dismissalAuditCount int
	if err := fx.dbpool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'course_link_suggestion.dismissed' AND payload->>'evidence_fingerprint' = $1`, dismissedItem.Fingerprint).Scan(&dismissalAuditCount); err != nil {
		t.Fatal(err)
	}
	if dismissalAuditCount != 1 {
		t.Fatalf("repeated dismissal wrote %d audits, want exactly 1", dismissalAuditCount)
	}
	if remaining := findCourseLinkSuggestionAPIItem(listAllCourseLinkSuggestionsAPI(t, fx, true, 100), pairA); remaining != nil {
		t.Fatal("dismissed pair remained in the detector response")
	}
}

func TestCourseLinkSuggestionAPI_ConfirmAndRejectStaleEvidence(t *testing.T) {
	fx := setupTestServer(t)
	confirmed := seedCourseLinkSuggestionAPIPair(t, fx, "confirm", true)
	stale := seedCourseLinkSuggestionAPIPair(t, fx, "stale", true)
	items := listAllCourseLinkSuggestionsAPI(t, fx, true, 100)
	confirmedItem := findCourseLinkSuggestionAPIItem(items, confirmed)
	staleItem := findCourseLinkSuggestionAPIItem(items, stale)
	if confirmedItem == nil || confirmedItem.Confidence != "high" || staleItem == nil || staleItem.Confidence != "high" {
		t.Fatalf("expected two high-confidence fixture items: confirm=%+v stale=%+v", confirmedItem, staleItem)
	}

	createBody := map[string]any{
		"name":                  "Suggestion continuation " + uuid.NewString(),
		"course_ids":            []string{confirmed.sourceIDString, confirmed.partialIDString},
		"kind":                  "continuation",
		"rule_source_course_id": confirmed.sourceIDString,
		"suggestion_precondition": map[string]string{
			"configured_course_id":   confirmed.sourceIDString,
			"unconfigured_course_id": confirmed.partialIDString,
			"evidence_fingerprint":   confirmedItem.Fingerprint,
			"detector_version":       sqldb.CourseLinkSuggestionDetectorVersion,
		},
	}
	resp := doRequest(t, fx.server.URL, http.MethodPost, "/api/v1/course-groups", createBody)
	if resp.StatusCode != http.StatusCreated {
		var apiErr map[string]any
		parseResponse(t, resp, &apiErr)
		t.Fatalf("confirmation status=%d response=%v", resp.StatusCode, apiErr)
	}
	var created struct {
		ID string `json:"id"`
	}
	parseResponse(t, resp, &created)
	if created.ID == "" {
		t.Fatal("confirmation did not return a group id")
	}
	var ruleSource string
	if err := fx.dbpool.QueryRow(context.Background(), `SELECT rule_source_course_id::text FROM course_merge_groups WHERE id = $1::uuid`, created.ID).Scan(&ruleSource); err != nil {
		t.Fatal(err)
	}
	if ruleSource != confirmed.sourceIDString {
		t.Fatalf("continuation rule source=%q, want configured course %q", ruleSource, confirmed.sourceIDString)
	}
	var memberCount int
	if err := fx.dbpool.QueryRow(context.Background(), `SELECT count(*) FROM course_merge_group_members WHERE group_id = $1::uuid`, created.ID).Scan(&memberCount); err != nil {
		t.Fatal(err)
	}
	if memberCount != 2 {
		t.Fatalf("continuation has %d members, want 2", memberCount)
	}
	var creationAuditCount int
	if err := fx.dbpool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'course_group.created' AND payload->>'suggestion_evidence_fingerprint' = $1`, confirmedItem.Fingerprint).Scan(&creationAuditCount); err != nil {
		t.Fatal(err)
	}
	if creationAuditCount != 1 {
		t.Fatalf("confirmation audit count=%d, want 1", creationAuditCount)
	}
	if findCourseLinkSuggestionAPIItem(listAllCourseLinkSuggestionsAPI(t, fx, true, 100), confirmed) != nil {
		t.Fatal("confirmed pair remained in the detector response")
	}

	if _, err := fx.dbpool.Exec(context.Background(), `UPDATE sessions SET start_at = start_at + interval '1 hour', end_at = end_at + interval '1 hour' WHERE id = $1`, stale.partialSession); err != nil {
		t.Fatal(err)
	}
	staleBody := map[string]any{
		"name":                  "Stale suggestion " + uuid.NewString(),
		"course_ids":            []string{stale.sourceIDString, stale.partialIDString},
		"kind":                  "continuation",
		"rule_source_course_id": stale.sourceIDString,
		"suggestion_precondition": map[string]string{
			"configured_course_id":   stale.sourceIDString,
			"unconfigured_course_id": stale.partialIDString,
			"evidence_fingerprint":   staleItem.Fingerprint,
			"detector_version":       sqldb.CourseLinkSuggestionDetectorVersion,
		},
	}
	resp = doRequest(t, fx.server.URL, http.MethodPost, "/api/v1/course-groups", staleBody)
	if resp.StatusCode != http.StatusConflict {
		var result map[string]any
		parseResponse(t, resp, &result)
		t.Fatalf("stale confirmation status=%d, want 409; response=%v", resp.StatusCode, result)
	}
	var staleError struct {
		Code string `json:"code"`
	}
	parseResponse(t, resp, &staleError)
	if staleError.Code != "suggestion_changed" {
		t.Fatalf("stale confirmation error code=%q, want suggestion_changed", staleError.Code)
	}
	var staleMembershipCount int
	if err := fx.dbpool.QueryRow(context.Background(), `SELECT count(*) FROM course_merge_group_members WHERE course_id IN ($1::uuid, $2::uuid)`, stale.sourceIDString, stale.partialIDString).Scan(&staleMembershipCount); err != nil {
		t.Fatal(err)
	}
	if staleMembershipCount != 0 {
		t.Fatalf("stale confirmation created %d group memberships", staleMembershipCount)
	}
}

func TestCourseLinkSuggestionAPI_ReadOnlyModes(t *testing.T) {
	discovery := setupTestServerWithCourseLinkSuggestionsMode(t, "discovery")
	pair := seedCourseLinkSuggestionAPIPair(t, discovery, "read-only", true)
	items := listAllCourseLinkSuggestionsAPI(t, discovery, false, 100)
	item := findCourseLinkSuggestionAPIItem(items, pair)
	if item == nil || item.Confidence != "high" {
		t.Fatalf("discovery mode did not return its high-confidence candidate: %+v", item)
	}
	if items[0].Mode != "discovery" || !items[0].Enabled || items[0].ConfirmationEnabled {
		t.Fatalf("unexpected discovery mode metadata: %+v", items[0])
	}

	dismissBody := map[string]string{
		"configured_course_id":   pair.sourceIDString,
		"unconfigured_course_id": pair.partialIDString,
		"evidence_fingerprint":   item.Fingerprint,
		"detector_version":       sqldb.CourseLinkSuggestionDetectorVersion,
	}
	resp := doRequest(t, discovery.server.URL, http.MethodPost, "/api/v1/admin/course-link-suggestions/dismiss", dismissBody)
	if resp.StatusCode != http.StatusConflict {
		var result map[string]any
		parseResponse(t, resp, &result)
		t.Fatalf("discovery dismissal status=%d, want 409: %v", resp.StatusCode, result)
	}
	var readOnlyError struct {
		Code string `json:"code"`
	}
	parseResponse(t, resp, &readOnlyError)
	if readOnlyError.Code != "suggestions_read_only" {
		t.Fatalf("discovery dismissal code=%q, want suggestions_read_only", readOnlyError.Code)
	}

	createBody := map[string]any{
		"name":                  "Read only suggestion " + uuid.NewString(),
		"course_ids":            []string{pair.sourceIDString, pair.partialIDString},
		"kind":                  "continuation",
		"rule_source_course_id": pair.sourceIDString,
		"suggestion_precondition": map[string]string{
			"configured_course_id":   pair.sourceIDString,
			"unconfigured_course_id": pair.partialIDString,
			"evidence_fingerprint":   item.Fingerprint,
			"detector_version":       sqldb.CourseLinkSuggestionDetectorVersion,
		},
	}
	resp = doRequest(t, discovery.server.URL, http.MethodPost, "/api/v1/course-groups", createBody)
	if resp.StatusCode != http.StatusConflict {
		var result map[string]any
		parseResponse(t, resp, &result)
		t.Fatalf("discovery confirmation status=%d, want 409: %v", resp.StatusCode, result)
	}
	parseResponse(t, resp, &readOnlyError)
	if readOnlyError.Code != "suggestions_read_only" {
		t.Fatalf("discovery confirmation code=%q, want suggestions_read_only", readOnlyError.Code)
	}

	disabled := setupTestServerWithCourseLinkSuggestionsMode(t, "disabled")
	disabledList := listCourseLinkSuggestionsAPI(t, disabled, "")
	if disabledList.Mode != "disabled" || disabledList.Enabled || len(disabledList.Items) != 0 {
		t.Fatalf("disabled mode exposed suggestions: %+v", disabledList)
	}
}

func seedCourseLinkSuggestionAPIPair(t *testing.T, fx *testFixture, label string, configuredPeriod bool) seededCourseLinkSuggestion {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	suffix := uuid.NewString()
	subject, err := fx.q.SubjectCreate(ctx, sqldb.SubjectCreateParams{Code: "E2E-" + suffix, Name: "Suggestion Subject " + label})
	if err != nil {
		t.Fatal(err)
	}
	cycleID := "e2e-course-link-" + suffix
	if _, err := fx.dbpool.Exec(ctx, `INSERT INTO crm_cycles (id, label, source_kind, start_date, end_date) VALUES ($1, $2, 'manual', DATE '2026-01-01', DATE '2026-03-31')`, cycleID, label+" "+suffix); err != nil {
		t.Fatal(err)
	}
	var rootID pgtype.UUID
	if err := fx.dbpool.QueryRow(ctx, `INSERT INTO root_course_groups (name) VALUES ($1) RETURNING id`, "E2E root "+suffix).Scan(&rootID); err != nil {
		t.Fatal(err)
	}
	source, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "E2E-LINK-A-" + suffix, Name: "English 101!"})
	if err != nil {
		t.Fatal(err)
	}
	partial, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "E2E-LINK-B-" + suffix, Name: "english-101"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.dbpool.Exec(ctx, `UPDATE courses SET subject_id = $2, level = 2, cycle_id = $3, root_course_group_id = $4 WHERE id = $1`, source.ID, subject.ID, cycleID, rootID); err != nil {
		t.Fatal(err)
	}
	partialCycleID := any(nil)
	if configuredPeriod {
		partialCycleID = cycleID
	}
	if _, err := fx.dbpool.Exec(ctx, `UPDATE courses SET subject_id = $2, level = NULL, cycle_id = $3, root_course_group_id = NULL WHERE id = $1`, partial.ID, subject.ID, partialCycleID); err != nil {
		t.Fatal(err)
	}

	sourceTeacher := createCourseLinkAPITeacher(t, ctx, fx, "Source Teacher "+label)
	partialTeacher := createCourseLinkAPITeacher(t, ctx, fx, "Partial Teacher "+label)
	if err := fx.q.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{CourseID: source.ID, TeacherID: sourceTeacher, IsPrimary: true}); err != nil {
		t.Fatal(err)
	}
	if err := fx.q.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{CourseID: partial.ID, TeacherID: partialTeacher, IsPrimary: true}); err != nil {
		t.Fatal(err)
	}

	insertSession := func(courseID, teacherID pgtype.UUID, start time.Time) pgtype.UUID {
		t.Helper()
		room, err := fx.q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "E2E-R-" + uuid.NewString(), Capacity: pgtype.Int4{Int32: 10, Valid: true}})
		if err != nil {
			t.Fatal(err)
		}
		var sessionID pgtype.UUID
		if err := fx.dbpool.QueryRow(ctx, `INSERT INTO sessions (course_id, room_id, teacher_id, start_at, end_at) VALUES ($1, $2, $3, $4, $5) RETURNING id`, courseID, room.ID, teacherID, start, start.Add(time.Hour)).Scan(&sessionID); err != nil {
			t.Fatal(err)
		}
		return sessionID
	}
	insertSession(source.ID, sourceTeacher, time.Date(2026, 1, 4, 17, 0, 0, 0, time.UTC))
	insertSession(source.ID, sourceTeacher, time.Date(2026, 1, 18, 17, 0, 0, 0, time.UTC))
	partialSession := insertSession(partial.ID, partialTeacher, time.Date(2026, 1, 11, 17, 0, 0, 0, time.UTC))

	sourceID, err := uuidString(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	partialID, err := uuidString(partial.ID)
	if err != nil {
		t.Fatal(err)
	}
	return seededCourseLinkSuggestion{
		sourceID: source.ID, partialID: partial.ID,
		sourceIDString: sourceID, partialIDString: partialID,
		partialSession: partialSession,
	}
}

func createCourseLinkAPITeacher(t *testing.T, ctx context.Context, fx *testFixture, name string) pgtype.UUID {
	t.Helper()
	id, err := fx.q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "link-teacher-" + uuid.NewString(), Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.dbpool.Exec(ctx, `UPDATE users SET full_name = $1 WHERE id = $2`, name, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func listCourseLinkSuggestionsAPI(t *testing.T, fx *testFixture, query string) courseLinkSuggestionAPIEnvelope {
	t.Helper()
	resp := doRequest(t, fx.server.URL, http.MethodGet, "/api/v1/admin/course-link-suggestions"+query, nil)
	if resp.StatusCode != http.StatusOK {
		var apiErr map[string]any
		parseResponse(t, resp, &apiErr)
		t.Fatalf("suggestion list status=%d response=%v", resp.StatusCode, apiErr)
	}
	var result courseLinkSuggestionAPIEnvelope
	parseResponse(t, resp, &result)
	return result
}

func listAllCourseLinkSuggestionsAPI(t *testing.T, fx *testFixture, includeReview bool, limit int) []courseLinkSuggestionAPIEnvelope {
	t.Helper()
	pages := make([]courseLinkSuggestionAPIEnvelope, 0)
	cursor := ""
	for len(pages) < 10000 {
		query := "?limit=" + strconv.Itoa(limit)
		if includeReview {
			query += "&include_review=true"
		}
		if cursor != "" {
			query += "&cursor=" + url.QueryEscape(cursor)
		}
		page := listCourseLinkSuggestionsAPI(t, fx, query)
		pages = append(pages, page)
		if !page.HasMore {
			return pages
		}
		if page.NextCursor == nil || *page.NextCursor == cursor {
			t.Fatalf("suggestion cursor did not advance after page %d", len(pages))
		}
		cursor = *page.NextCursor
	}
	t.Fatal("suggestion pagination exceeded 10,000 pages")
	return pages
}

func findCourseLinkSuggestionAPIItem(pages []courseLinkSuggestionAPIEnvelope, pair seededCourseLinkSuggestion) *courseLinkSuggestionAPIItem {
	for _, page := range pages {
		for index := range page.Items {
			item := &page.Items[index]
			if item.ConfiguredSource.ID == pair.sourceIDString && item.Unconfigured.ID == pair.partialIDString {
				return item
			}
		}
	}
	return nil
}
