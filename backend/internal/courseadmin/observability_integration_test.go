package courseadmin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func TestObservability_AuditRecordCreated(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	actorID := f.createTeacher(t, "Admin")
	courseID := f.createCourse(t, "OBS004-"+f.suffix)

	v1 := f.courseVersion(t, courseID)
	if v1 != 1 {
		t.Fatalf("expected initial version 1, got %d", v1)
	}

	// Update with version 1→2, A primary + B assigned.
	cmd := UpdateCourseCommand{
		CourseID:        courseID,
		ActorID:         actorID,
		ExpectedVersion: v1,
		Code:            "OBS004-" + f.suffix,
		Name:            "Observability Test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	}

	result, err := f.runUpdate(t, svc, cmd)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if result.Version != 2 {
		t.Fatalf("expected version 2, got %d", result.Version)
	}

	// Query audit_log for the course update action.
	ctx := context.Background()
	var payloadJSON []byte
	if err := f.pool.QueryRow(ctx,
		`SELECT payload FROM audit_log WHERE action = 'course.teachers_updated' AND payload->>'course_id' = $1 ORDER BY created_at DESC LIMIT 1`,
		courseID.String(),
	).Scan(&payloadJSON); err != nil {
		t.Fatal(err)
	}

	var payload map[string]any
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatal(err)
	}

	if got, ok := payload["course_id"].(string); !ok || got != courseID.String() {
		t.Fatalf("expected course_id %s, got %v", courseID.String(), payload["course_id"])
	}

	if got, ok := payload["teacher_count_before"].(float64); !ok || int(got) != 0 {
		t.Fatalf("expected teacher_count_before 0, got %v", payload["teacher_count_before"])
	}

	if got, ok := payload["teacher_count_after"].(float64); !ok || int(got) != 2 {
		t.Fatalf("expected teacher_count_after 2, got %v", payload["teacher_count_after"])
	}

	expectedPrimary := uuidString(teacherA)
	if got, ok := payload["primary_teacher_after"].(string); !ok || got != expectedPrimary {
		t.Fatalf("expected primary_teacher_after %s, got %v", expectedPrimary, payload["primary_teacher_after"])
	}

	if payload["primary_teacher_before"] != nil {
		t.Fatalf("expected primary_teacher_before nil, got %v", payload["primary_teacher_before"])
	}

	if got, ok := payload["version"].(float64); !ok || int(got) != 2 {
		t.Fatalf("expected version 2, got %v", payload["version"])
	}
}

func TestObservability_AuditActorIntegrity(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	actorID1 := f.createTeacher(t, "Admin")
	actorID2 := f.createTeacher(t, "Admin")
	courseID := f.createCourse(t, "SEC009-"+f.suffix)

	v1 := f.courseVersion(t, courseID)

	cmd := UpdateCourseCommand{
		CourseID:        courseID,
		ActorID:         actorID1,
		ExpectedVersion: v1,
		Code:            "SEC009-" + f.suffix,
		Name:            "Actor Integrity Test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
		},
	}

	_, err := f.runUpdate(t, svc, cmd)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// Query the most recent audit entry's actor_user_id.
	ctx := context.Background()
	var storedActorID pgtype.UUID
	if err := f.pool.QueryRow(ctx,
		`SELECT actor_user_id FROM audit_log WHERE action = 'course.teachers_updated' ORDER BY created_at DESC LIMIT 1`,
	).Scan(&storedActorID); err != nil {
		t.Fatal(err)
	}

	if !storedActorID.Valid {
		t.Fatal("expected valid actor_user_id")
	}
	if storedActorID.Bytes != actorID1.Bytes {
		t.Fatalf("expected actor_user_id to match actorID1 (%v), got %v", actorID1, storedActorID)
	}
	if storedActorID.Bytes == actorID2.Bytes {
		t.Fatal("actor_user_id should not match actorID2")
	}
}

// runCreate wraps CreateCourseTx in its own transaction, analogous to runUpdate.
func (f *testFixture) runCreate(t *testing.T, svc *Service, cmd CreateCourseCommand) (CreateCourseResult, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	qtx := sqldb.New(tx)
	result, err := svc.CreateCourseTx(ctx, qtx, cmd)
	if err != nil {
		return CreateCourseResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return result, nil
}

func TestObservability_AuditOnCreate(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	actorID := f.createTeacher(t, "Admin")

	cmd := CreateCourseCommand{
		ActorID: actorID,
		Code:    "CRT-OBS-" + f.suffix,
		Name:    "Create Observability Test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
		},
	}

	result, err := f.runCreate(t, svc, cmd)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !result.CourseID.Valid {
		t.Fatal("expected valid course ID")
	}
	if result.Version != 1 {
		t.Fatalf("expected version 1, got %d", result.Version)
	}

	// Query audit_log for the create action.
	ctx := context.Background()
	var payloadJSON []byte
	if err := f.pool.QueryRow(ctx,
		`SELECT payload FROM audit_log WHERE action = 'course.created' AND payload->>'course_id' = $1 ORDER BY created_at DESC LIMIT 1`,
		result.CourseID.String(),
	).Scan(&payloadJSON); err != nil {
		t.Fatal(err)
	}

	var payload map[string]any
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatal(err)
	}

	if got, ok := payload["course_id"].(string); !ok || got != result.CourseID.String() {
		t.Fatalf("expected course_id %s, got %v", result.CourseID.String(), payload["course_id"])
	}

	if got, ok := payload["teacher_count"].(float64); !ok || int(got) != 1 {
		t.Fatalf("expected teacher_count 1, got %v", payload["teacher_count"])
	}

	expectedPrimary := uuidString(teacherA)
	if got, ok := payload["primary_teacher"].(string); !ok || got != expectedPrimary {
		t.Fatalf("expected primary_teacher %s, got %v", expectedPrimary, payload["primary_teacher"])
	}

	if got, ok := payload["version"].(float64); !ok || int(got) != 1 {
		t.Fatalf("expected version 1, got %v", payload["version"])
	}
}
