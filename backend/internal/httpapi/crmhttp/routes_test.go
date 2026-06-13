package crmhttp

import "testing"

func TestBuildCourseFilterJobStatusResponse_ReturnsFailedForStructuredConflict(t *testing.T) {
	lastError := `apply reconcile: {"message":"Student schedule conflict: Jane (W250001) cannot be added to SAT Math","details":{"kind":"crm_student_schedule_conflict","student":{"wcode":"W250001","full_name":"Jane"},"target_course":{"id":"course-1","code":"SAT","name":"SAT Math Course","subject_name":"SAT Math"},"conflicts":[{"course":{"id":"course-2","code":"ALG","name":"Algebra Course","subject_name":"Algebra"},"start_at":"2026-05-20T10:00:00Z","end_at":"2026-05-20T11:00:00Z"}]}}`

	resp := buildCourseFilterJobStatusResponse("job-1", "retry", "course_reconcile_apply", lastError, nil)

	if resp["status"] != "failed" {
		t.Fatalf("status = %q, want failed", resp["status"])
	}
	if resp["message"] != "Student schedule conflict: Jane (W250001) cannot be added to SAT Math" {
		t.Fatalf("message = %q", resp["message"])
	}
	details, ok := resp["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured details, got %#v", resp["details"])
	}
	if details["kind"] != "crm_student_schedule_conflict" {
		t.Fatalf("details.kind = %q, want crm_student_schedule_conflict", details["kind"])
	}
	targetCourse, ok := details["target_course"].(map[string]any)
	if !ok {
		t.Fatalf("expected target_course details, got %#v", details["target_course"])
	}
	if targetCourse["id"] != "course-1" {
		t.Fatalf("target_course.id = %q, want course-1", targetCourse["id"])
	}
	if targetCourse["subject_name"] != "SAT Math" {
		t.Fatalf("target_course.subject_name = %q, want SAT Math", targetCourse["subject_name"])
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
