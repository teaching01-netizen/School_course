package courseshttp

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	sqldb "warwick-institute/internal/db"
)

func newGroupCourse(t *testing.T, fx *testFixture, label string) string {
	t.Helper()
	course, err := fx.q.CourseCreate(context.Background(), sqldb.CourseCreateParams{
		Code: "C-CONT-" + label + "-" + uuid.NewString()[:8],
		Name: "Continuation " + label,
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := uuidString(course.ID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func postGroup(t *testing.T, fx *testFixture, body map[string]any) (int, map[string]any) {
	t.Helper()
	body["name"] = "Group " + uuid.NewString()[:8]
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/course-groups", body)
	status := resp.StatusCode
	var out map[string]any
	parseResponse(t, resp, &out)
	return status, out
}

func TestCourseGroupContinuationRejectsBadRequests(t *testing.T) {
	fx := setupTestServer(t)
	a, b := fx.courseIDStr, newGroupCourse(t, fx, "bad")
	outside := newGroupCourse(t, fx, "outside")

	tests := []struct {
		name string
		body map[string]any
		code string
	}{
		{"unknown kind", map[string]any{"course_ids": []string{a, b}, "kind": "bogus"}, "invalid_kind"},
		{"merge with a rule source", map[string]any{"course_ids": []string{a, b}, "rule_source_course_id": a}, "invalid_rule_source"},
		{"continuation without a source", map[string]any{"course_ids": []string{a, b}, "kind": "continuation"}, "invalid_rule_source"},
		{"continuation with a malformed source", map[string]any{"course_ids": []string{a, b}, "kind": "continuation", "rule_source_course_id": "nope"}, "invalid_rule_source"},
		{"source outside the pair", map[string]any{"course_ids": []string{a, b}, "kind": "continuation", "rule_source_course_id": outside}, "invalid_rule_source"},
		{"duplicate course ids", map[string]any{"course_ids": []string{a, a}, "kind": "continuation", "rule_source_course_id": a}, "invalid_course_ids"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, body := postGroup(t, fx, test.body)
			if status != http.StatusBadRequest || body["code"] != test.code {
				t.Fatalf("got %d %v, want 400 %s", status, body, test.code)
			}
		})
	}
}

func TestCourseGroupContinuationLifecycle(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()
	source, split := fx.courseIDStr, newGroupCourse(t, fx, "life")

	status, created := postGroup(t, fx, map[string]any{
		"course_ids": []string{source, split}, "kind": "continuation", "rule_source_course_id": source,
	})
	if status != http.StatusCreated || created["kind"] != "continuation" || created["rule_source_course_id"] != source {
		t.Fatalf("create = %d %v", status, created)
	}
	groupID := created["id"].(string)

	var getBody map[string]any
	parseResponse(t, doRequest(t, fx.server.URL, "GET", "/api/v1/course-groups/"+groupID, nil), &getBody)
	if getBody["kind"] != "continuation" || getBody["rule_source_course_id"] != source {
		t.Fatalf("get = %v", getBody)
	}
	var list []map[string]any
	parseResponse(t, doRequest(t, fx.server.URL, "GET", "/api/v1/course-groups", nil), &list)
	found := false
	for _, item := range list {
		if item["id"] == groupID {
			found = item["kind"] == "continuation" && item["rule_source_course_id"] == source
		}
	}
	if !found {
		t.Fatalf("list does not expose the continuation: %v", list)
	}

	var audited string
	if err := fx.dbpool.QueryRow(ctx, `
		SELECT payload->>'rule_source_course_id' FROM audit_log
		WHERE action = 'course_group.created' AND payload->>'group_id' = $1
	`, groupID).Scan(&audited); err != nil || audited != source {
		t.Fatalf("audit rule source = %q, err %v; want %s", audited, err, source)
	}

	// A grouped course cannot join a second link.
	status, body := postGroup(t, fx, map[string]any{
		"course_ids": []string{split, newGroupCourse(t, fx, "third")}, "kind": "continuation", "rule_source_course_id": split,
	})
	if status != http.StatusBadRequest || body["code"] != "course_already_grouped" {
		t.Fatalf("already grouped = %d %v", status, body)
	}

	// The rule source cannot be deleted while linked.
	resp := doRequest(t, fx.server.URL, "DELETE", "/api/v1/courses/"+source, nil)
	var deleteBody map[string]any
	parseResponse(t, resp, &deleteBody)
	if resp.StatusCode != http.StatusConflict || deleteBody["code"] != "continuation_rule_source" {
		t.Fatalf("delete source = %d %v", resp.StatusCode, deleteBody)
	}

	// Once absences depend on the link it cannot be removed.
	var wcode string
	if err := fx.dbpool.QueryRow(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, 'Cont') RETURNING wcode`, "wcg"+uuid.NewString()[:8]).Scan(&wcode); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.dbpool.Exec(ctx, `
		INSERT INTO student_absences (wcode, course_id, date_from, date_to, merge_group_id)
		VALUES ($1, $2, current_date, current_date, $3)
	`, wcode, split, groupID); err != nil {
		t.Fatal(err)
	}
	resp = doRequest(t, fx.server.URL, "DELETE", "/api/v1/course-groups/"+groupID, nil)
	parseResponse(t, resp, &deleteBody)
	if resp.StatusCode != http.StatusConflict || deleteBody["code"] != "continuation_in_use" {
		t.Fatalf("unlink with absences = %d %v", resp.StatusCode, deleteBody)
	}
}

func TestCourseGroupDefaultsToMerge(t *testing.T) {
	fx := setupTestServer(t)
	status, created := postGroup(t, fx, map[string]any{"course_ids": []string{fx.courseIDStr, newGroupCourse(t, fx, "merge")}})
	if status != http.StatusCreated || created["kind"] != "merge" || created["rule_source_course_id"] != nil {
		t.Fatalf("create = %d %v", status, created)
	}
}
