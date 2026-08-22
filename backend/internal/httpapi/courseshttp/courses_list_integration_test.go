package courseshttp

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"

	sqldb "warwick-institute/internal/db"
)

// ---------------------------------------------------------------------------
// GET /api/v1/courses — filtering, pagination, and the dual response shape
// ---------------------------------------------------------------------------

// courseSeed creates a course with the given type, optionally archived, and
// returns its id string.
func courseSeed(t *testing.T, fx *testFixture, code, typ string, archived bool) string {
	t.Helper()
	ctx := context.Background()
	course, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: code, Name: "Seed " + code})
	if err != nil {
		t.Fatal(err)
	}
	if typ != "" {
		if _, err := fx.dbpool.Exec(ctx, `UPDATE courses SET course_type = $2 WHERE id = $1`, course.ID, typ); err != nil {
			t.Fatal(err)
		}
	}
	if archived {
		if _, err := fx.dbpool.Exec(ctx, `UPDATE courses SET legacy_archived = true WHERE id = $1`, course.ID); err != nil {
			t.Fatal(err)
		}
	}
	return course.ID.String()
}

func TestCoursesList_DefaultBareArray_ExcludesArchived(t *testing.T) {
	fx := setupTestServer(t)
	archivedID := courseSeed(t, fx, courseCode("LARCH"), "General", true)
	liveID := courseSeed(t, fx, courseCode("LLIVE"), "Private", false)

	// No limit param: the response is the legacy bare array (backward
	// compatible with the lookups/dropdown consumers) and defaults to live only.
	resp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses", nil)
	assertResponseCode(t, resp, http.StatusOK)
	var list []map[string]any
	parseResponse(t, resp, &list)

	var live, archived *map[string]any
	for i := range list {
		id := list[i]["id"].(string)
		if id == liveID {
			live = &list[i]
		}
		if id == archivedID {
			archived = &list[i]
		}
	}
	if live == nil {
		t.Fatalf("live course %q missing from default list", liveID)
	}
	if archived != nil {
		t.Fatalf("archived course %q must not be in the default (live) list", archivedID)
	}
	if _, isEnvelope := list[0]["items"]; isEnvelope {
		t.Fatalf("bare request must return an array, got an envelope")
	}
}

func TestCoursesList_EnvelopeWhenLimitPresent(t *testing.T) {
	fx := setupTestServer(t)
	// The scratch DB accumulates courses across tests in the package, so scope
	// the assertions to this test's rows with a unique shared code token.
	token := "LBOX" + uuid.New().String()[:8]
	courseSeed(t, fx, courseCode(token+"A"), "Private", false)
	courseSeed(t, fx, courseCode(token+"B"), "General", false)
	courseSeed(t, fx, courseCode(token+"C"), "Group", false)

	resp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses?q="+token+"&limit=2&offset=0", nil)
	assertResponseCode(t, resp, http.StatusOK)
	var page map[string]any
	parseResponse(t, resp, &page)

	if page["limit"] != float64(2) || page["offset"] != float64(0) {
		t.Fatalf("unexpected limit/offset echo: %v / %v", page["limit"], page["offset"])
	}
	if page["total_count"].(float64) != 3 {
		t.Fatalf("expected total_count 3 for the scoped rows, got %v", page["total_count"])
	}
	items, ok := page["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 items on page 1, got %#v", page["items"])
	}

	// Second page returns exactly one more row; the union of pages is stable
	// (course_no DESC) and disjoint.
	resp2 := doRequest(t, fx.server.URL, "GET", "/api/v1/courses?q="+token+"&limit=2&offset=2", nil)
	assertResponseCode(t, resp2, http.StatusOK)
	var page2 map[string]any
	parseResponse(t, resp2, &page2)
	items2 := page2["items"].([]any)
	if len(items2) != 1 {
		t.Fatalf("expected 1 item on page 2, got %d", len(items2))
	}
	seen := map[string]bool{}
	for _, raw := range append(items, items2...) {
		seen[raw.(map[string]any)["id"].(string)] = true
	}
	if len(seen) != 3 {
		t.Fatalf("pages must be disjoint, saw %d unique ids", len(seen))
	}
}

func TestCoursesList_StatusArchived(t *testing.T) {
	fx := setupTestServer(t)
	archivedID := courseSeed(t, fx, courseCode("SARCH"), "General", true)
	liveID := courseSeed(t, fx, courseCode("SLIVE"), "Private", false)

	resp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses?status=archived&limit=50", nil)
	assertResponseCode(t, resp, http.StatusOK)
	var page map[string]any
	parseResponse(t, resp, &page)

	foundArchived, foundLive := false, false
	for _, raw := range page["items"].([]any) {
		id := raw.(map[string]any)["id"].(string)
		if id == archivedID {
			foundArchived = true
		}
		if id == liveID {
			foundLive = true
		}
	}
	if !foundArchived {
		t.Fatalf("archived course %q missing from status=archived", archivedID)
	}
	if foundLive {
		t.Fatalf("live course %q must not appear in status=archived", liveID)
	}
}

func TestCoursesList_TypeFilter(t *testing.T) {
	fx := setupTestServer(t)
	privateID := courseSeed(t, fx, courseCode("TPRIV"), "Private", false)
	generalID := courseSeed(t, fx, courseCode("TGEN"), "General", false)
	groupID := courseSeed(t, fx, courseCode("TGRP"), "Group", false)

	// type=private: only Private.
	resp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses?type=private&limit=50", nil)
	assertResponseCode(t, resp, http.StatusOK)
	var page map[string]any
	parseResponse(t, resp, &page)
	if !courseIDInItems(page, privateID) {
		t.Fatalf("private filter must include %q", privateID)
	}
	if courseIDInItems(page, generalID) || courseIDInItems(page, groupID) {
		t.Fatalf("private filter leaked General/Group courses")
	}

	// type=general: both General (legacy vocabulary) and Group (native vocabulary).
	resp2 := doRequest(t, fx.server.URL, "GET", "/api/v1/courses?type=general&limit=50", nil)
	assertResponseCode(t, resp2, http.StatusOK)
	var page2 map[string]any
	parseResponse(t, resp2, &page2)
	if !courseIDInItems(page2, generalID) || !courseIDInItems(page2, groupID) {
		t.Fatalf("general filter must include both General and Group courses")
	}
	if courseIDInItems(page2, privateID) {
		t.Fatalf("general filter leaked the Private course")
	}
}

func TestCoursesList_TeacherFilter(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacher := createTeacherUser(t, ctx, fx.q)
	teacherStr := teacherIDString(t, teacher)
	withID := courseSeed(t, fx, courseCode("THW"), "", false)
	if _, err := fx.dbpool.Exec(ctx, `UPDATE courses SET teacher_id = $2 WHERE id = $1`, mustParseUUID(t, withID), teacher); err != nil {
		t.Fatal(err)
	}
	noneID := courseSeed(t, fx, courseCode("THN"), "", false)

	resp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses?teacher_id="+teacherStr+"&limit=50", nil)
	assertResponseCode(t, resp, http.StatusOK)
	var page map[string]any
	parseResponse(t, resp, &page)
	if !courseIDInItems(page, withID) || courseIDInItems(page, noneID) {
		t.Fatalf("teacher uuid filter must return only the matching course")
	}

	respNone := doRequest(t, fx.server.URL, "GET", "/api/v1/courses?teacher_id=none&limit=50", nil)
	assertResponseCode(t, respNone, http.StatusOK)
	var pageNone map[string]any
	parseResponse(t, respNone, &pageNone)
	if !courseIDInItems(pageNone, noneID) || courseIDInItems(pageNone, withID) {
		t.Fatalf("teacher_id=none must return teacher-less courses only")
	}

	respBad := doRequest(t, fx.server.URL, "GET", "/api/v1/courses?teacher_id=not-a-uuid&limit=50", nil)
	assertResponseCode(t, respBad, http.StatusBadRequest)
	var errOut map[string]any
	parseResponse(t, respBad, &errOut)
	if errOut["code"] != "bad_teacher_id" {
		t.Fatalf("expected code bad_teacher_id, got %v", errOut["code"])
	}
}

func TestCoursesList_SearchParam(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	uid := uuid.New().String()[:8]
	subject, err := fx.q.SubjectCreate(ctx, sqldb.SubjectCreateParams{Code: "S-" + uid, Name: "Findable Subject"})
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := fx.q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "qsearch-" + uid, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	matchID := courseSeed(t, fx, courseCode("QM"), "", false)
	if _, err := fx.dbpool.Exec(ctx, `UPDATE courses SET subject_id = $2, name = 'Search Target Course' WHERE id = $1`, mustParseUUID(t, matchID), subject.ID); err != nil {
		t.Fatal(err)
	}
	if err := fx.q.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{CourseID: mustParseUUID(t, matchID), TeacherID: teacher, IsPrimary: false}); err != nil {
		t.Fatal(err)
	}
	missID := courseSeed(t, fx, courseCode("QMM"), "", false)

	cases := []struct {
		query string
	}{
		{"Search Target"},
		{"findable"},
		{"qsearch-" + uid},
	}
	for _, tc := range cases {
		resp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses?q="+url.QueryEscape(tc.query)+"&limit=50", nil)
		assertResponseCode(t, resp, http.StatusOK)
		var page map[string]any
		parseResponse(t, resp, &page)
		if !courseIDInItems(page, matchID) {
			t.Fatalf("q=%q must match the seeded course", tc.query)
		}
		if courseIDInItems(page, missID) {
			t.Fatalf("q=%q leaked the unrelated course", tc.query)
		}
	}
}

func TestCoursesList_EnvelopeReportsTotalForFilters(t *testing.T) {
	fx := setupTestServer(t)
	token := "TOT" + uuid.New().String()[:8]
	courseSeed(t, fx, courseCode(token+"P"), "Private", false)
	courseSeed(t, fx, courseCode(token+"G"), "General", false)

	// total_count reflects the filter, not the whole table: the unique q token
	// scopes the request to this test's two courses.
	resp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses?q="+token+"&type=private&limit=1&offset=0", nil)
	assertResponseCode(t, resp, http.StatusOK)
	var page map[string]any
	parseResponse(t, resp, &page)
	if page["total_count"].(float64) != 1 {
		t.Fatalf("expected total_count 1 for the private filter, got %v", page["total_count"])
	}
	if len(page["items"].([]any)) != 1 {
		t.Fatalf("expected 1 private item, got %#v", page["items"])
	}
}

func courseIDInItems(page map[string]any, id string) bool {
	items, ok := page["items"].([]any)
	if !ok {
		return false
	}
	for _, raw := range items {
		if raw.(map[string]any)["id"].(string) == id {
			return true
		}
	}
	return false
}

// TestCoursesList_AbsenceFormHiddenFilter verifies the audit view: the
// absence_form=hidden filter returns only hidden courses, and every row
// carries absence_form_visible so the UI can badge hidden ones.
func TestCoursesList_AbsenceFormHiddenFilter(t *testing.T) {
	fx := setupTestServer(t)
	token := "LHID" + uuid.NewString()[:8]
	visibleID := courseSeed(t, fx, courseCode(token+"A"), "Private", false)
	hiddenID := courseSeed(t, fx, courseCode(token+"B"), "Private", false)
	ctx := context.Background()
	if _, err := fx.dbpool.Exec(ctx, `UPDATE courses SET absence_form_visible = false WHERE id = $1`, hiddenID); err != nil {
		t.Fatal(err)
	}

	resp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses?limit=100&absence_form=hidden", nil)
	assertResponseCode(t, resp, http.StatusOK)
	var envelope struct {
		Items []map[string]any `json:"items"`
	}
	parseResponse(t, resp, &envelope)

	var foundVisible, foundHidden bool
	for _, item := range envelope.Items {
		switch item["id"].(string) {
		case visibleID:
			foundVisible = true
		case hiddenID:
			foundHidden = true
			if got, ok := item["absence_form_visible"].(bool); !ok || got {
				t.Fatalf("hidden course must report absence_form_visible=false, got %#v", item["absence_form_visible"])
			}
		}
	}
	if foundVisible {
		t.Fatalf("visible course must not appear in the hidden filter")
	}
	if !foundHidden {
		t.Fatalf("hidden course missing from the hidden filter")
	}

	// Without the filter both appear with their true flags.
	allResp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses?limit=100", nil)
	assertResponseCode(t, allResp, http.StatusOK)
	var allEnvelope struct {
		Items []map[string]any `json:"items"`
	}
	parseResponse(t, allResp, &allEnvelope)
	for _, item := range allEnvelope.Items {
		switch item["id"].(string) {
		case visibleID:
			if got, ok := item["absence_form_visible"].(bool); !ok || !got {
				t.Fatalf("visible course must report absence_form_visible=true, got %#v", item["absence_form_visible"])
			}
		case hiddenID:
			if got, ok := item["absence_form_visible"].(bool); !ok || got {
				t.Fatalf("hidden course must report absence_form_visible=false, got %#v", item["absence_form_visible"])
			}
		}
	}
}

// TestCoursesList_ActiveCourseFlag covers the list badge: exactly the course
// designated as its subject's active course carries is_active_course=true.
func TestCoursesList_ActiveCourseFlag(t *testing.T) {
	fx := setupTestServer(t)
	token := "LAC" + uuid.NewString()[:8]
	activeID := courseSeed(t, fx, courseCode(token+"A"), "Private", false)
	otherID := courseSeed(t, fx, courseCode(token+"B"), "Private", false)
	subjectCode := "SUBJ-" + token
	ctx := context.Background()
	if _, err := fx.dbpool.Exec(ctx, `
		INSERT INTO subjects (code, name) VALUES ($1, $1)
	`, subjectCode); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.dbpool.Exec(ctx, `
		UPDATE courses SET subject_id = (SELECT id FROM subjects WHERE code = $1)
		WHERE code IN ($2, $3)
	`, subjectCode, courseCode(token+"A"), courseCode(token+"B")); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.dbpool.Exec(ctx, `
		INSERT INTO subject_active_courses (subject_id, course_id)
		SELECT s.id, c.id FROM subjects s, courses c
		WHERE s.code = $1 AND c.code = $2
	`, subjectCode, courseCode(token+"A")); err != nil {
		t.Fatal(err)
	}

	resp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses?limit=100", nil)
	assertResponseCode(t, resp, http.StatusOK)
	var envelope struct {
		Items []map[string]any `json:"items"`
	}
	parseResponse(t, resp, &envelope)

	found := map[string]bool{}
	for _, item := range envelope.Items {
		switch item["id"].(string) {
		case activeID:
			found["active"] = true
			if got, ok := item["is_active_course"].(bool); !ok || !got {
				t.Fatalf("designated active course must report is_active_course=true, got %#v", item["is_active_course"])
			}
		case otherID:
			found["other"] = true
			if got, ok := item["is_active_course"].(bool); !ok || got {
				t.Fatalf("non-active course must report is_active_course=false, got %#v", item["is_active_course"])
			}
		}
	}
	if !found["active"] || !found["other"] {
		t.Fatalf("seeded courses missing from list response (found %v)", found)
	}
}
