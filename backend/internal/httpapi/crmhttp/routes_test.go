package crmhttp

import "testing"

func TestBuildCourseFilterJobStatusResponse_ReturnsFailedForStructuredConflict(t *testing.T) {
	lastError := `apply reconcile: {"message":"Student schedule conflict: Jane (W250001) cannot be added to SAT","details":{"kind":"crm_student_schedule_conflict","student":{"wcode":"W250001","full_name":"Jane"},"target_course":{"code":"SAT"},"conflicts":[{"course":{"code":"ALG"},"start_at":"2026-05-20T10:00:00Z","end_at":"2026-05-20T11:00:00Z"}]}}`

	resp := buildCourseFilterJobStatusResponse("job-1", "retry", "course_reconcile_apply", lastError, nil)

	if resp["status"] != "failed" {
		t.Fatalf("status = %q, want failed", resp["status"])
	}
	if resp["message"] != "Student schedule conflict: Jane (W250001) cannot be added to SAT" {
		t.Fatalf("message = %q", resp["message"])
	}
	details, ok := resp["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured details, got %#v", resp["details"])
	}
	if details["kind"] != "crm_student_schedule_conflict" {
		t.Fatalf("details.kind = %q, want crm_student_schedule_conflict", details["kind"])
	}
}

func TestBuildCourseFilterJobStatusResponse_DoesNotReportSuccessWhenStaleConflictErrorExists(t *testing.T) {
	lastError := `{"message":"Student schedule conflict: Jane cannot be added to SAT","details":{"kind":"crm_student_schedule_conflict","student":{"wcode":"W250001","full_name":"Jane"},"target_course":{"code":"SAT"},"conflicts":[]}}`

	resp := buildCourseFilterJobStatusResponse("job-1", "succeeded", "course_reconcile_apply", lastError, []byte(`{"added":0}`))

	if resp["status"] != "failed" {
		t.Fatalf("status = %q, want failed", resp["status"])
	}
	if _, ok := resp["details"]; !ok {
		t.Fatal("expected stale structured conflict details to be returned")
	}
	if _, ok := resp["result"]; !ok {
		t.Fatal("expected result to remain included for diagnostics")
	}
}
