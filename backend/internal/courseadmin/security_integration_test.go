// Security integration tests for course teacher management.
//
// These tests verify that the domain service rejects dangerous or invalid
// teacher-assignment inputs and leaves the database unchanged on failure.
package courseadmin

import (
	"testing"
)

// TestSecurity_NonTeacherSelfAssign (SEC-004)
//
// A user without a teachable role cannot be added as a teacher via the
// Teachers slice, even when they are the authenticated caller. This test
// proves that the service validates every teacher in the set against the
// database role, regardless of identity.
func TestSecurity_NonTeacherSelfAssign(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	admin := f.createTeacher(t, "Admin")
	teacherA := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "SEC004-"+f.suffix)

	// Attempt update with one valid teacher and one admin who lacks a
	// teachable role.  The service must reject the admin in the teachers
	// slice with role_not_allowed.
	ce := requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "SEC004-" + f.suffix,
		Name:            "Security Test SEC-004",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: false},
			{TeacherID: admin, IsPrimary: false},
		},
	}), "invalid_teacher")

	// Verify admin is the only flagged teacher.
	teachers, ok := ce.Details["teachers"].([]map[string]any)
	if !ok {
		t.Fatalf("expected details.teachers array, got %#v", ce.Details["teachers"])
	}
	for _, item := range teachers {
		if item["teacher_id"] == admin.String() && item["reason"] != "role_not_allowed" {
			t.Fatalf("expected admin to be role_not_allowed, got %v", item["reason"])
		}
		if item["teacher_id"] == teacherA.String() {
			t.Fatalf("valid teacher %v must not be flagged", teacherA)
		}
	}

	// Verify: no changes committed.
	if v := f.courseVersion(t, courseID); v != 1 {
		t.Fatalf("expected version 1 (unchanged), got %d", v)
	}
	if set := f.courseTeacherIDs(t, courseID); len(set) != 0 {
		t.Fatalf("expected no teachers after failed update, got %d", len(set))
	}
}

// TestSecurity_OversizedTeacherList (SEC-006)
//
// A request with more than MaxTeachersPerCourse teachers must be rejected
// before any database access.  The structural guard in
// validateTeacherAssignments returns too_many_teachers.
func TestSecurity_OversizedTeacherList(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	// Create one more teacher than the maximum allowed.
	const overflow = MaxTeachersPerCourse + 1
	teachers := make([]TeacherAssignment, 0, overflow)
	for range overflow {
		teacherID := f.createTeacher(t, "Teacher")
		teachers = append(teachers, TeacherAssignment{TeacherID: teacherID, IsPrimary: false})
	}

	courseID := f.createCourse(t, "SEC006-"+f.suffix)

	ce := requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "SEC006-" + f.suffix,
		Name:            "Security Test SEC-006",
		Teachers:        teachers,
	}), "too_many_teachers")

	if ce.Details == nil {
		t.Fatal("expected error details")
	}
	if max, ok := ce.Details["maximum"].(int); !ok || max != MaxTeachersPerCourse {
		t.Fatalf("expected maximum %d, got %v", MaxTeachersPerCourse, ce.Details["maximum"])
	}
	if recv, ok := ce.Details["received"].(int); !ok || recv != overflow {
		t.Fatalf("expected received %d, got %v", overflow, ce.Details["received"])
	}

	// Verify: no changes committed.
	if v := f.courseVersion(t, courseID); v != 1 {
		t.Fatalf("expected version 1 (unchanged), got %d", v)
	}
	if set := f.courseTeacherIDs(t, courseID); len(set) != 0 {
		t.Fatalf("expected no teachers after failed update, got %d", len(set))
	}
}

// TestSecurity_AuditActorIntegrity (SEC-009)
//
// The domain service does NOT accept a caller-provided identity from the
// Teachers slice that would override the authenticated actor.  Even when
// the ActorID (set by the HTTP layer from the authenticated user) is valid,
// a non-teacher in the Teachers slice must be rejected.
//
// This is an architectural invariant: Teacher IDs in the slice are validated
// independently of the actor; putting yourself in the slice does not grant
// teaching privileges.
func TestSecurity_AuditActorIntegrity(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	admin := f.createTeacher(t, "Admin")
	teacherA := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "SEC009-"+f.suffix)

	// The admin (who is the authenticated actor) tries to add themselves
	// into the teachers slice.  The service must reject them — ActorID is
	// for audit, not for bypassing teacher validation.
	ce := requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ActorID:         admin, // authenticated actor
		ExpectedVersion: 1,
		Code:            "SEC009-" + f.suffix,
		Name:            "Security Test SEC-009",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: admin, IsPrimary: false},
		},
	}), "invalid_teacher")

	// The admin must be the only invalid teacher, flagged as role_not_allowed.
	teachers, ok := ce.Details["teachers"].([]map[string]any)
	if !ok {
		t.Fatalf("expected details.teachers array, got %#v", ce.Details["teachers"])
	}
	adminFlagged := false
	for _, item := range teachers {
		if item["teacher_id"] == admin.String() {
			adminFlagged = true
			if item["reason"] != "role_not_allowed" {
				t.Fatalf("expected admin to be role_not_allowed, got %v", item["reason"])
			}
		}
		if item["teacher_id"] == teacherA.String() {
			t.Fatalf("valid teacher %v must not be flagged", teacherA)
		}
	}
	if !adminFlagged {
		t.Fatal("expected admin to appear in invalid teachers")
	}

	// Verify: no changes committed.
	if v := f.courseVersion(t, courseID); v != 1 {
		t.Fatalf("expected version 1 (unchanged), got %d", v)
	}
	if set := f.courseTeacherIDs(t, courseID); len(set) != 0 {
		t.Fatalf("expected no teachers after failed update, got %d", len(set))
	}
}
