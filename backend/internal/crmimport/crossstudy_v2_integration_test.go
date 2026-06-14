package crmimport

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/crmimport/crossstudy"
	"warwick-institute/internal/crmimport/xlsx"
)

// ============================================================================
// Test helpers
// ============================================================================

func createTestUser(t *testing.T, ctx context.Context, dbpool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := dbpool.Exec(ctx, `INSERT INTO users (id, username, password_hash, role, password_version)
		VALUES ($1, 'cross-study-test-' || gen_random_uuid()::text, 'hash', 'Admin', 1)`, id)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return id
}

func createTestCourseSimple(t *testing.T, ctx context.Context, dbpool *pgxpool.Pool, code, name string) uuid.UUID {
	t.Helper()
	_, err := dbpool.Exec(ctx, `INSERT INTO courses (id, code, name) VALUES (gen_random_uuid(), $1, $2)`, code, name)
	if err != nil {
		t.Fatalf("create course: %v", err)
	}
	var id uuid.UUID
	err = dbpool.QueryRow(ctx, `SELECT id FROM courses WHERE code = $1`, code).Scan(&id)
	if err != nil {
		t.Fatalf("get course id: %v", err)
	}
	return id
}

func requireDB(t *testing.T) string {
	t.Helper()
	return requireTestDBV2(t)
}

func uuidFromPG(t *testing.T, id pgtype.UUID) uuid.UUID {
	t.Helper()
	parsed, err := uuid.FromBytes(id.Bytes[:])
	if err != nil {
		t.Fatalf("convert pgtype.UUID: %v", err)
	}
	return parsed
}

func createTestStudent(t *testing.T, ctx context.Context, dbpool *pgxpool.Pool, wcode, fullName string) {
	t.Helper()
	_, err := dbpool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES ($1, $2, '') ON CONFLICT (wcode) DO NOTHING`, wcode, fullName)
	if err != nil {
		t.Fatalf("create test student: %v", err)
	}
}

func activateSnapshot(t *testing.T, ctx context.Context, dbpool *pgxpool.Pool, snapshotID pgtype.UUID) {
	t.Helper()
	_, err := dbpool.Exec(ctx, `UPDATE crm_state SET active_snapshot_id = $1, updated_at = now() WHERE singleton = true`, snapshotID)
	if err != nil {
		t.Fatalf("activate snapshot: %v", err)
	}
}

// ============================================================================
// Tests
// ============================================================================

// TestCrossStudy_LookupStudent_ReturnsCRMRowAndExtraNote verifies that
// LookupStudent returns a CRM row with extra_note populated when the student
// exists in the active snapshot.
func TestCrossStudy_LookupStudent_ReturnsCRMRowAndExtraNote(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Arrange: create a snapshot with a row that has ExtraNote set
	snapID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{
			WCode:      "W260001",
			CourseName: "CrossStudy Test Course A",
			CycleLabel: "Cycle A",
			ExtraNote:  "extra-section-info",
		},
	})
	activateSnapshot(t, ctx, dbpool, snapID)
	createTestStudent(t, ctx, dbpool, "W260001", "Test Student 001")

	// Act: lookup the student
	store := crossstudy.NewStore(dbpool)
	resp, err := store.LookupStudent(ctx, "W260001")
	if err != nil {
		t.Fatalf("LookupStudent failed: %v", err)
	}

	// Assert: CRM row is returned
	if resp.CRMRow == nil {
		t.Fatal("expected crm_row to be non-nil")
	}
	if resp.CRMRow.ExtraNote != "extra-section-info" {
		t.Fatalf("expected extra_note='extra-section-info', got %q", resp.CRMRow.ExtraNote)
	}
	if resp.CRMRow.CourseName != "CrossStudy Test Course A" {
		t.Fatalf("expected course_name='CrossStudy Test Course A', got %q", resp.CRMRow.CourseName)
	}
	if resp.Student.WCode != "W260001" {
		t.Fatalf("expected wcode='W260001', got %q", resp.Student.WCode)
	}
	// No assignment should exist yet
	if resp.CurrentAssignment != nil {
		t.Fatal("expected no current assignment for first lookup")
	}
}

// TestCrossStudy_SaveAssignment_CreatesAssignmentAndOverrides verifies that
// SaveAssignment creates a pending assignment and the corresponding roster
// overrides (include on assigned, exclude on source).
func TestCrossStudy_SaveAssignment_CreatesAssignmentAndOverrides(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Arrange
	sourceCourseID := createTestCourseSimple(t, ctx, dbpool, "CS-SRC-01", "CrossStudy Source")
	destAID := createTestCourseSimple(t, ctx, dbpool, "CS-DST-A1", "CrossStudy Dest A")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-DST-B1", "CrossStudy Dest B")

	snapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{
			WCode:      "W260002",
			CourseName: "CrossStudy Source",
			CycleLabel: "Cycle A",
			ExtraNote:  "test-note",
		},
	})
	activateSnapshot(t, ctx, dbpool, snapshotID)
	createTestStudent(t, ctx, dbpool, "W260002", "Test Student 002")

	userID := createTestUser(t, ctx, dbpool)

	// Act
	store := crossstudy.NewStore(dbpool)
	err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260002",
		SourceCourseID:   sourceCourseID,
		SnapshotID:       uuidFromPG(t, snapshotID),
		DestCourseAID:    destAID,
		DestCourseBID:    destBID,
		AssignedCourseID: destAID,
		ExtraNoteText:    "test-note",
	}, userID)
	if err != nil {
		t.Fatalf("SaveAssignment failed: %v", err)
	}

	// Assert: assignment exists
	resp, err := store.LookupStudent(ctx, "W260002")
	if err != nil {
		t.Fatalf("LookupStudent after save failed: %v", err)
	}
	if resp.CurrentAssignment == nil {
		t.Fatal("expected current assignment after save")
	}
	if resp.CurrentAssignment.Status != "pending" {
		t.Fatalf("expected status='pending', got %q", resp.CurrentAssignment.Status)
	}
	if resp.CurrentAssignment.ExtraNoteSnapshot != "test-note" {
		t.Fatalf("expected extra_note_snapshot='test-note', got %q", resp.CurrentAssignment.ExtraNoteSnapshot)
	}
	// Verify override_source is set on the overrides
	var overrideCount int
	err = dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_roster_overrides
		WHERE override_source = 'cross_study'
		  AND student_id = (SELECT id FROM students WHERE wcode = 'W260002')
	`).Scan(&overrideCount)
	if err != nil {
		t.Fatalf("count overrides: %v", err)
	}
	if overrideCount != 3 {
		t.Fatalf("expected 3 cross_study overrides, got %d", overrideCount)
	}
}

func TestCrossStudy_SaveAssignment_AssignsStudentToBothDestinationCourses(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sourceCourseID := createTestCourseSimple(t, ctx, dbpool, "CS-BOTH-SRC", "CrossStudy Both Source")
	destAID := createTestCourseSimple(t, ctx, dbpool, "CS-BOTH-DST-A", "CrossStudy Both Writing")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-BOTH-DST-B", "CrossStudy Both Reading")

	snapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{
			WCode:      "W260202",
			CourseName: "CrossStudy Both Source",
			CycleLabel: "Cycle A",
			ExtraNote:  "เรียนไขว้ Sec.1&Sec.2 Tue Writing & Sat Reading",
		},
	})
	activateSnapshot(t, ctx, dbpool, snapshotID)
	createTestStudent(t, ctx, dbpool, "W260202", "Both Destinations Student")

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)
	if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260202",
		SourceCourseID:   sourceCourseID,
		SnapshotID:       uuidFromPG(t, snapshotID),
		DestCourseAID:    destAID,
		DestCourseBID:    destBID,
		AssignedCourseID: destAID,
		ExtraNoteText:    "เรียนไขว้ Sec.1&Sec.2 Tue Writing & Sat Reading",
	}, userID); err != nil {
		t.Fatalf("SaveAssignment failed: %v", err)
	}

	for label, courseID := range map[string]uuid.UUID{
		"Course A": destAID,
		"Course B": destBID,
	} {
		var enrolled int
		if err := dbpool.QueryRow(ctx, `
			SELECT COUNT(*) FROM course_students
			WHERE course_id = $1
			  AND student_id = (SELECT id FROM students WHERE wcode = 'W260202')
		`, courseID).Scan(&enrolled); err != nil {
			t.Fatalf("count %s enrollment: %v", label, err)
		}
		if enrolled != 1 {
			t.Fatalf("expected student enrolled in %s destination course, got %d rows", label, enrolled)
		}
	}

	var includeOverrides int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_roster_overrides
		WHERE override_source = 'cross_study'
		  AND action = 'include'
		  AND student_id = (SELECT id FROM students WHERE wcode = 'W260202')
		  AND course_id IN ($1, $2)
	`, destAID, destBID).Scan(&includeOverrides); err != nil {
		t.Fatalf("count include overrides: %v", err)
	}
	if includeOverrides != 2 {
		t.Fatalf("expected include overrides for both destination courses, got %d", includeOverrides)
	}
}

// TestCrossStudy_DeleteAssignment_SoftDeletesAndCleansOverrides verifies
// that DeleteAssignment marks the assignment as deleted and removes all
// associated cross_study overrides.
func TestCrossStudy_DeleteAssignment_SoftDeletesAndCleansOverrides(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Arrange
	sourceCourseID := createTestCourseSimple(t, ctx, dbpool, "CS-SRC-02", "CrossStudy Source 2")
	destAID := createTestCourseSimple(t, ctx, dbpool, "CS-DST-A2", "CrossStudy Dest A2")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-DST-B2", "CrossStudy Dest B2")

	snapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{
			WCode:      "W260003",
			CourseName: "CrossStudy Source 2",
			CycleLabel: "Cycle A",
			ExtraNote:  "note",
		},
	})
	activateSnapshot(t, ctx, dbpool, snapshotID)
	createTestStudent(t, ctx, dbpool, "W260003", "Test Student 003")

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)

	err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260003",
		SourceCourseID:   sourceCourseID,
		SnapshotID:       uuidFromPG(t, snapshotID),
		DestCourseAID:    destAID,
		DestCourseBID:    destBID,
		AssignedCourseID: destAID,
		ExtraNoteText:    "note",
	}, userID)
	if err != nil {
		t.Fatalf("SaveAssignment for setup failed: %v", err)
	}

	// Get assignment ID
	resp, err := store.LookupStudent(ctx, "W260003")
	if err != nil {
		t.Fatalf("LookupStudent: %v", err)
	}
	if resp.CurrentAssignment == nil {
		t.Fatal("expected assignment after save")
	}
	assignmentID, err := uuid.Parse(resp.CurrentAssignment.ID)
	if err != nil {
		t.Fatalf("parse assignment id: %v", err)
	}

	// Act: delete
	err = store.DeleteAssignment(ctx, assignmentID)
	if err != nil {
		t.Fatalf("DeleteAssignment failed: %v", err)
	}

	// Assert: assignment is soft-deleted (not returned by LookupStudent)
	resp2, err := store.LookupStudent(ctx, "W260003")
	if err != nil {
		t.Fatalf("LookupStudent after delete: %v", err)
	}
	if resp2.CurrentAssignment != nil {
		t.Fatal("expected no current assignment after delete")
	}

	// Assert: overrides removed
	var overrideCount int
	err = dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_roster_overrides
		WHERE override_source = 'cross_study'
		  AND student_id = (SELECT id FROM students WHERE wcode = 'W260003')
	`).Scan(&overrideCount)
	if err != nil {
		t.Fatalf("count overrides: %v", err)
	}
	if overrideCount != 0 {
		t.Fatalf("expected 0 overrides after delete, got %d", overrideCount)
	}
}

// TestCrossStudy_ListAssignments_ReturnsFilteredAndSorted verifies that
// ListAssignmentsWithCourseInfo returns assignments with correct course names
// and respects the status filter.
func TestCrossStudy_ListAssignments_ReturnsFilteredAndSorted(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Arrange
	sourceAID := createTestCourseSimple(t, ctx, dbpool, "CS-LIST-SRC-A", "List Source A")
	destA1ID := createTestCourseSimple(t, ctx, dbpool, "CS-LIST-A1", "List Dest A1")
	destB1ID := createTestCourseSimple(t, ctx, dbpool, "CS-LIST-B1", "List Dest B1")

	snapID4 := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{
			WCode:      "W260010",
			CourseName: "List Source A",
			CycleLabel: "Cycle A",
			ExtraNote:  "a",
		},
	})
	activateSnapshot(t, ctx, dbpool, snapID4)
	createTestStudent(t, ctx, dbpool, "W260010", "Test Student 010")

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)

	err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260010",
		SourceCourseID:   sourceAID,
		SnapshotID:       uuidFromPG(t, snapID4),
		DestCourseAID:    destA1ID,
		DestCourseBID:    destB1ID,
		AssignedCourseID: destA1ID,
		ExtraNoteText:    "a",
	}, userID)
	if err != nil {
		t.Fatalf("SaveAssignment: %v", err)
	}

	// Act: list all
	items, err := store.ListAssignmentsWithCourseInfo(ctx, "", "")
	if err != nil {
		t.Fatalf("ListAssignmentsWithCourseInfo failed: %v", err)
	}

	// Assert
	if len(items) < 1 {
		t.Fatal("expected at least 1 assignment")
	}
	found := false
	for _, item := range items {
		if item.WCode == "W260010" {
			found = true
			if item.SourceCourseName != "List Source A" {
				t.Fatalf("expected source_course_name='List Source A', got %q", item.SourceCourseName)
			}
			if item.AssignedCourseName != "List Dest A1" {
				t.Fatalf("expected assigned_course_name='List Dest A1', got %q", item.AssignedCourseName)
			}
			if item.FullName == "" {
				t.Fatal("expected FullName to be non-empty")
			}
			break
		}
	}
	if !found {
		t.Fatal("assignment for W260010 not found in list")
	}

	// Act: filter by status
	pendingItems, err := store.ListAssignmentsWithCourseInfo(ctx, "pending", "")
	if err != nil {
		t.Fatalf("ListAssignmentsWithCourseInfo(status=pending) failed: %v", err)
	}
	if len(pendingItems) < 1 {
		t.Fatal("expected at least 1 pending assignment")
	}
	for _, item := range pendingItems {
		if item.WCode == "W260010" && item.Status != "pending" {
			t.Fatalf("expected status='pending', got %q", item.Status)
		}
	}

	// Act: filter by non-matching status
	orphanedItems, err := store.ListAssignmentsWithCourseInfo(ctx, "orphaned", "")
	if err != nil {
		t.Fatalf("ListAssignmentsWithCourseInfo(status=orphaned) failed: %v", err)
	}
	for _, item := range orphanedItems {
		if item.WCode == "W260010" {
			t.Fatal("orphaned filter returned active assignment")
		}
	}
}

// TestCrossStudy_LoadPendingChanges_DetectsOrphaned verifies that
// LoadPendingChanges returns an empty current_course when the student's
// CRM row is gone (orphaned scenario).
func TestCrossStudy_LoadPendingChanges_DetectsOrphaned(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Arrange: create a snapshot with a student, then create a second snapshot
	// without that student.
	sourceID := createTestCourseSimple(t, ctx, dbpool, "CS-ORPH-SRC", "Orphan Source")
	destAID := createTestCourseSimple(t, ctx, dbpool, "CS-ORPH-DST", "Orphan Dest")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-ORPH-DST2", "Orphan Dest 2")

	oldSnapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{
			WCode:      "W260020",
			CourseName: "Orphan Source",
			CycleLabel: "Cycle A",
			ExtraNote:  "original",
		},
	})
	createTestStudent(t, ctx, dbpool, "W260020", "Test Student 020")

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)

	err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260020",
		SourceCourseID:   sourceID,
		SnapshotID:       uuidFromPG(t, oldSnapshotID),
		DestCourseAID:    destAID,
		DestCourseBID:    destBID,
		AssignedCourseID: destAID,
		ExtraNoteText:    "original",
	}, userID)
	if err != nil {
		t.Fatalf("SaveAssignment: %v", err)
	}

	// Act: LoadPendingChanges with old snapshot (student exists)
	oldChanges, err := store.LoadPendingChanges(ctx, uuidFromPG(t, oldSnapshotID))
	if err != nil {
		t.Fatalf("LoadPendingChanges(old snapshot) failed: %v", err)
	}

	if len(oldChanges) == 0 {
		t.Fatal("expected changes for old snapshot")
	}

	// Act: load with a new empty snapshot ID (no rows)
	newSnapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{})
	newChanges, err := store.LoadPendingChanges(ctx, uuidFromPG(t, newSnapshotID))
	if err != nil {
		t.Fatalf("LoadPendingChanges(new snapshot) failed: %v", err)
	}

	if len(newChanges) == 0 {
		t.Fatal("expected changes for new snapshot")
	}
	for _, ch := range newChanges {
		if ch.WCode == "W260020" {
			if ch.CurrentCourseName != "" {
				t.Fatalf("expected empty current_course_name for orphaned student, got %q", ch.CurrentCourseName)
			}
		}
	}
}

// TestCrossStudy_RosterEffect_UpdatesCourseStudents verifies that SaveAssignment
// immediately updates course_students (and thus student_busy_ranges via triggers)
// without waiting for a reconcile cycle.
func TestCrossStudy_RosterEffect_UpdatesCourseStudents(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Arrange
	sourceID := createTestCourseSimple(t, ctx, dbpool, "CS-ROSTER-SRC", "Roster Source")
	destAID := createTestCourseSimple(t, ctx, dbpool, "CS-ROSTER-DST-A", "Roster Dest A")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-ROSTER-DST-B", "Roster Dest B")

	// Also create a session for the assigned course so we can verify student_busy_ranges.
	_, err := dbpool.Exec(ctx, `
		INSERT INTO sessions (id, course_id, teacher_id, start_at, end_at)
		VALUES (gen_random_uuid(), $1, (SELECT id FROM users WHERE role='Admin' LIMIT 1), now(), now() + interval '1 hour')
	`, destAID)
	if err != nil {
		t.Fatalf("create test session: %v", err)
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{
			WCode:      "W260099",
			CourseName: "Roster Source",
			CycleLabel: "Cycle A",
			ExtraNote:  "roster-test",
		},
	})
	activateSnapshot(t, ctx, dbpool, snapshotID)
	createTestStudent(t, ctx, dbpool, "W260099", "Roster Test Student")

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)

	// Initially: student is NOT in either dest course's course_students (no reconcile yet)
	var csCount int
	err = dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students WHERE course_id = $1
		AND student_id = (SELECT id FROM students WHERE wcode = 'W260099')
	`, destAID).Scan(&csCount)
	if err != nil {
		t.Fatalf("check initial course_students destA: %v", err)
	}
	if csCount != 0 {
		t.Fatalf("expected 0 in destA course_students before save, got %d", csCount)
	}

	// Act: save assignment mapping from source → destA
	err = store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260099",
		SourceCourseID:   sourceID,
		SnapshotID:       uuidFromPG(t, snapshotID),
		DestCourseAID:    destAID,
		DestCourseBID:    destBID,
		AssignedCourseID: destAID,
		ExtraNoteText:    "roster-test",
	}, userID)
	if err != nil {
		t.Fatalf("SaveAssignment failed: %v", err)
	}

	// Assert 1: student is in assigned course's course_students
	err = dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students WHERE course_id = $1
		AND student_id = (SELECT id FROM students WHERE wcode = 'W260099')
	`, destAID).Scan(&csCount)
	if err != nil {
		t.Fatalf("check course_students destA after save: %v", err)
	}
	if csCount != 1 {
		t.Fatalf("expected 1 in destA course_students after save, got %d", csCount)
	}

	// Assert 2: student is NOT in source course's course_students (excluded because different)
	err = dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students WHERE course_id = $1
		AND student_id = (SELECT id FROM students WHERE wcode = 'W260099')
	`, sourceID).Scan(&csCount)
	if err != nil {
		t.Fatalf("check course_students source after save: %v", err)
	}
	if csCount != 0 {
		t.Fatalf("expected 0 in source course_students after save (excluded), got %d", csCount)
	}

	// Assert 3: student is also in Course B because cross-study assigns both destination courses.
	err = dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students WHERE course_id = $1
		AND student_id = (SELECT id FROM students WHERE wcode = 'W260099')
	`, destBID).Scan(&csCount)
	if err != nil {
		t.Fatalf("check course_students destB after save: %v", err)
	}
	if csCount != 1 {
		t.Fatalf("expected 1 in destB course_students after save, got %d", csCount)
	}

	// Assert 4: student has busy ranges for the assigned course's session (trigger-fired)
	var brCount int
	err = dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM student_busy_ranges br
		JOIN sessions s ON s.id = br.session_id
		WHERE s.course_id = $1
		AND br.student_id = (SELECT id FROM students WHERE wcode = 'W260099')
		AND br.deleted_at IS NULL
	`, destAID).Scan(&brCount)
	if err != nil {
		t.Fatalf("check student_busy_ranges: %v", err)
	}
	if brCount != 1 {
		t.Fatalf("expected 1 busy range for assigned course session, got %d", brCount)
	}

	// Act: delete the assignment
	resp, err := store.LookupStudent(ctx, "W260099")
	if err != nil {
		t.Fatalf("LookupStudent: %v", err)
	}
	if resp.CurrentAssignment == nil {
		t.Fatal("expected assignment before delete")
	}
	assignmentID, err := uuid.Parse(resp.CurrentAssignment.ID)
	if err != nil {
		t.Fatalf("parse assignment id: %v", err)
	}

	err = store.DeleteAssignment(ctx, assignmentID)
	if err != nil {
		t.Fatalf("DeleteAssignment failed: %v", err)
	}

	// Assert 5: student is removed from assigned course's course_students
	err = dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students WHERE course_id = $1
		AND student_id = (SELECT id FROM students WHERE wcode = 'W260099')
	`, destAID).Scan(&csCount)
	if err != nil {
		t.Fatalf("check course_students destA after delete: %v", err)
	}
	if csCount != 0 {
		t.Fatalf("expected 0 in destA course_students after delete, got %d", csCount)
	}

	// Assert 6: student is not invented in the source course because cross-study
	// did not remove an existing source enrollment during save.
	err = dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students WHERE course_id = $1
		AND student_id = (SELECT id FROM students WHERE wcode = 'W260099')
	`, sourceID).Scan(&csCount)
	if err != nil {
		t.Fatalf("check course_students source after delete: %v", err)
	}
	if csCount != 0 {
		t.Fatalf("expected 0 in source course_students after delete (not invented), got %d", csCount)
	}

	// Assert 7: busy ranges for assigned course are soft-deleted
	err = dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM student_busy_ranges br
		JOIN sessions s ON s.id = br.session_id
		WHERE s.course_id = $1
		AND br.student_id = (SELECT id FROM students WHERE wcode = 'W260099')
		AND br.deleted_at IS NULL
	`, destAID).Scan(&brCount)
	if err != nil {
		t.Fatalf("check busy ranges after delete: %v", err)
	}
	if brCount != 0 {
		t.Fatalf("expected 0 active busy ranges for assigned course after delete, got %d", brCount)
	}
}

func TestCrossStudy_RosterEffect_WeekdayScopeUsesSessionAttendanceOnly(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sourceID := createTestCourseSimple(t, ctx, dbpool, "CS-WEEKDAY-SRC", "Weekday Source")
	destAID := createTestCourseSimple(t, ctx, dbpool, "CS-WEEKDAY-DST-A", "Weekday Writing")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-WEEKDAY-DST-B", "Weekday Reading")
	teacherID := createTestUser(t, ctx, dbpool)

	sessions := map[string]uuid.UUID{}
	for label, spec := range map[string]struct {
		courseID uuid.UUID
		startAt  string
		endAt    string
	}{
		"writing_tue": {destAID, "2026-06-16T09:00:00+07:00", "2026-06-16T10:00:00+07:00"},
		"writing_wed": {destAID, "2026-06-17T09:00:00+07:00", "2026-06-17T10:00:00+07:00"},
		"reading_sat": {destBID, "2026-06-20T09:00:00+07:00", "2026-06-20T10:00:00+07:00"},
		"reading_sun": {destBID, "2026-06-21T09:00:00+07:00", "2026-06-21T10:00:00+07:00"},
	} {
		id := uuid.New()
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO sessions (id, course_id, teacher_id, start_at, end_at)
			VALUES ($1, $2, $3, $4, $5)
		`, id, spec.courseID, teacherID, spec.startAt, spec.endAt); err != nil {
			t.Fatalf("create %s session: %v", label, err)
		}
		sessions[label] = id
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{
			WCode:      "W260203",
			CourseName: "Weekday Source",
			CycleLabel: "Cycle A",
			ExtraNote:  "Tue Writing & Sat Reading",
		},
	})
	activateSnapshot(t, ctx, dbpool, snapshotID)
	createTestStudent(t, ctx, dbpool, "W260203", "Weekday Scope Student")

	store := crossstudy.NewStore(dbpool)
	if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:               "W260203",
		SourceCourseID:      sourceID,
		SnapshotID:          uuidFromPG(t, snapshotID),
		DestCourseAID:       destAID,
		DestCourseBID:       destBID,
		DestCourseAWeekdays: []int16{2},
		DestCourseBWeekdays: []int16{6},
		AssignedCourseID:    destAID,
		ExtraNoteText:       "Tue Writing & Sat Reading",
	}, teacherID); err != nil {
		t.Fatalf("SaveAssignment failed: %v", err)
	}

	var courseEnrollmentCount int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students
		WHERE student_id = (SELECT id FROM students WHERE wcode = 'W260203')
		  AND course_id IN ($1, $2)
	`, destAID, destBID).Scan(&courseEnrollmentCount); err != nil {
		t.Fatalf("count destination course_students: %v", err)
	}
	if courseEnrollmentCount != 0 {
		t.Fatalf("expected scoped assignment to avoid full-course enrollment, got %d course_students rows", courseEnrollmentCount)
	}

	expectedIncluded := map[uuid.UUID]bool{
		sessions["writing_tue"]: true,
		sessions["reading_sat"]: true,
	}
	rows, err := dbpool.Query(ctx, `
		SELECT session_id
		FROM session_attendance
		WHERE student_id = (SELECT id FROM students WHERE wcode = 'W260203')
		  AND status = 'included'
		  AND override_source = 'cross_study'
	`)
	if err != nil {
		t.Fatalf("query session attendance: %v", err)
	}
	defer rows.Close()

	actualIncluded := map[uuid.UUID]bool{}
	for rows.Next() {
		var sessionID uuid.UUID
		if err := rows.Scan(&sessionID); err != nil {
			t.Fatalf("scan session attendance: %v", err)
		}
		actualIncluded[sessionID] = true
	}
	for sessionID := range expectedIncluded {
		if !actualIncluded[sessionID] {
			t.Fatalf("expected scoped session %s to be included, got %#v", sessionID, actualIncluded)
		}
	}
	if len(actualIncluded) != len(expectedIncluded) {
		t.Fatalf("expected only scoped sessions included, got %#v", actualIncluded)
	}

	var activeBusyRanges int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM student_busy_ranges
		WHERE student_id = (SELECT id FROM students WHERE wcode = 'W260203')
		  AND deleted_at IS NULL
	`).Scan(&activeBusyRanges); err != nil {
		t.Fatalf("count active busy ranges: %v", err)
	}
	if activeBusyRanges != 2 {
		t.Fatalf("expected 2 active busy ranges for scoped sessions, got %d", activeBusyRanges)
	}
}

// TestCrossStudy_RosterEffect_AssignedIsSource verifies that when assigned course
// equals source course, no exclude happens and the student stays in the source roster.
func TestCrossStudy_RosterEffect_AssignedIsSource(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Arrange: source and assigned are the same course
	sourceID := createTestCourseSimple(t, ctx, dbpool, "CS-SRC-ASGN", "Source Is Assigned")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-DST-B-ONLY", "Dest B Only")

	// Pre-add student to source course's course_students (simulating roster from reconcile)
	createTestStudent(t, ctx, dbpool, "W260100", "Same Course Student")
	_, err := dbpool.Exec(ctx, `
		INSERT INTO course_students (course_id, student_id)
		VALUES ($1, (SELECT id FROM students WHERE wcode = 'W260100'))
	`, sourceID)
	if err != nil {
		t.Fatalf("seed course_students: %v", err)
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{
			WCode:      "W260100",
			CourseName: "Source Is Assigned",
			CycleLabel: "Cycle A",
		},
	})
	activateSnapshot(t, ctx, dbpool, snapshotID)

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)

	// Act: save with assigned == source
	err = store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260100",
		SourceCourseID:   sourceID,
		SnapshotID:       uuidFromPG(t, snapshotID),
		DestCourseAID:    sourceID,
		DestCourseBID:    destBID,
		AssignedCourseID: sourceID,
		ExtraNoteText:    "",
	}, userID)
	if err != nil {
		t.Fatalf("SaveAssignment failed: %v", err)
	}

	// Assert: student still in source course's course_students (no exclude)
	var csCount int
	err = dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students WHERE course_id = $1
		AND student_id = (SELECT id FROM students WHERE wcode = 'W260100')
	`, sourceID).Scan(&csCount)
	if err != nil {
		t.Fatalf("check course_students: %v", err)
	}
	if csCount != 1 {
		t.Fatalf("expected 1 in source course_students (assigned==source), got %d", csCount)
	}

	// Act: delete
	resp, err := store.LookupStudent(ctx, "W260100")
	if err != nil {
		t.Fatalf("LookupStudent: %v", err)
	}
	if resp.CurrentAssignment == nil {
		t.Fatal("expected assignment")
	}
	assignmentID, err := uuid.Parse(resp.CurrentAssignment.ID)
	if err != nil {
		t.Fatalf("parse assignment id: %v", err)
	}

	err = store.DeleteAssignment(ctx, assignmentID)
	if err != nil {
		t.Fatalf("DeleteAssignment failed: %v", err)
	}

	// Assert: student still in source course_students after delete (was never removed)
	err = dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students WHERE course_id = $1
		AND student_id = (SELECT id FROM students WHERE wcode = 'W260100')
	`, sourceID).Scan(&csCount)
	if err != nil {
		t.Fatalf("check course_students after delete: %v", err)
	}
	if csCount != 1 {
		t.Fatalf("expected 1 in source course_students after delete (never removed), got %d", csCount)
	}
}

func TestCrossStudy_DeleteAssignment_PreservesPreExistingAssignedEnrollment(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sourceID := createTestCourseSimple(t, ctx, dbpool, "CS-OWN-SRC-DEL", "Ownership Source Delete")
	destAID := createTestCourseSimple(t, ctx, dbpool, "CS-OWN-DST-A-DEL", "Ownership Dest A Delete")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-OWN-DST-B-DEL", "Ownership Dest B Delete")

	snapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{WCode: "W260110", CourseName: "Ownership Source Delete", CycleLabel: "Cycle A", ExtraNote: "ownership-delete"},
	})
	activateSnapshot(t, ctx, dbpool, snapshotID)
	createTestStudent(t, ctx, dbpool, "W260110", "Preexisting Assigned Student")

	_, err := dbpool.Exec(ctx, `
		INSERT INTO course_students (course_id, student_id)
		VALUES ($1, (SELECT id FROM students WHERE wcode = 'W260110'))
	`, destAID)
	if err != nil {
		t.Fatalf("seed pre-existing assigned enrollment: %v", err)
	}

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)
	if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260110",
		SourceCourseID:   sourceID,
		SnapshotID:       uuidFromPG(t, snapshotID),
		DestCourseAID:    destAID,
		DestCourseBID:    destBID,
		AssignedCourseID: destAID,
		ExtraNoteText:    "ownership-delete",
	}, userID); err != nil {
		t.Fatalf("save assignment: %v", err)
	}

	resp, err := store.LookupStudent(ctx, "W260110")
	if err != nil {
		t.Fatalf("lookup assignment: %v", err)
	}
	assignmentID, err := uuid.Parse(resp.CurrentAssignment.ID)
	if err != nil {
		t.Fatalf("parse assignment id: %v", err)
	}
	if err := store.DeleteAssignment(ctx, assignmentID); err != nil {
		t.Fatalf("delete assignment: %v", err)
	}

	var enrolled int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students
		WHERE course_id = $1
		  AND student_id = (SELECT id FROM students WHERE wcode = 'W260110')
	`, destAID).Scan(&enrolled); err != nil {
		t.Fatalf("count assigned enrollment: %v", err)
	}
	if enrolled != 1 {
		t.Fatalf("expected pre-existing assigned enrollment to remain after delete, got %d rows", enrolled)
	}
}

func TestCrossStudy_SaveAssignment_PreservesPreExistingPreviousAssignedEnrollmentOnChange(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sourceID := createTestCourseSimple(t, ctx, dbpool, "CS-OWN-SRC-CHG", "Ownership Source Change")
	destAID := createTestCourseSimple(t, ctx, dbpool, "CS-OWN-DST-A-CHG", "Ownership Dest A Change")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-OWN-DST-B-CHG", "Ownership Dest B Change")

	snapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{WCode: "W260111", CourseName: "Ownership Source Change", CycleLabel: "Cycle A", ExtraNote: "ownership-change"},
	})
	activateSnapshot(t, ctx, dbpool, snapshotID)
	createTestStudent(t, ctx, dbpool, "W260111", "Preexisting Previous Assigned Student")

	_, err := dbpool.Exec(ctx, `
		INSERT INTO course_students (course_id, student_id)
		VALUES ($1, (SELECT id FROM students WHERE wcode = 'W260111'))
	`, destAID)
	if err != nil {
		t.Fatalf("seed pre-existing previous assigned enrollment: %v", err)
	}

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)
	if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260111",
		SourceCourseID:   sourceID,
		SnapshotID:       uuidFromPG(t, snapshotID),
		DestCourseAID:    destAID,
		DestCourseBID:    destBID,
		AssignedCourseID: destAID,
		ExtraNoteText:    "ownership-change",
	}, userID); err != nil {
		t.Fatalf("save initial assignment: %v", err)
	}
	if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260111",
		SourceCourseID:   sourceID,
		SnapshotID:       uuidFromPG(t, snapshotID),
		DestCourseAID:    destAID,
		DestCourseBID:    destBID,
		AssignedCourseID: destBID,
		ExtraNoteText:    "ownership-change",
	}, userID); err != nil {
		t.Fatalf("change assignment: %v", err)
	}

	var enrolled int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students
		WHERE course_id = $1
		  AND student_id = (SELECT id FROM students WHERE wcode = 'W260111')
	`, destAID).Scan(&enrolled); err != nil {
		t.Fatalf("count previous assigned enrollment: %v", err)
	}
	if enrolled != 1 {
		t.Fatalf("expected pre-existing previous assigned enrollment to remain after change, got %d rows", enrolled)
	}
}

func TestCrossStudy_DeleteAssignment_RestoresOnlySourceEnrollmentRemovedByCrossStudy(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sourceWithEnrollmentID := createTestCourseSimple(t, ctx, dbpool, "CS-OWN-SRC-RESTORE", "Ownership Source Restore")
	sourceWithoutEnrollmentID := createTestCourseSimple(t, ctx, dbpool, "CS-OWN-SRC-NORESTORE", "Ownership Source No Restore")
	destAID := createTestCourseSimple(t, ctx, dbpool, "CS-OWN-DST-A-RESTORE", "Ownership Dest A Restore")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-OWN-DST-B-RESTORE", "Ownership Dest B Restore")

	snapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{WCode: "W260112", CourseName: "Ownership Source Restore", CycleLabel: "Cycle A", ExtraNote: "source-restore"},
		{WCode: "W260113", CourseName: "Ownership Source No Restore", CycleLabel: "Cycle A", ExtraNote: "source-no-restore"},
	})
	activateSnapshot(t, ctx, dbpool, snapshotID)
	createTestStudent(t, ctx, dbpool, "W260112", "Source Restore Student")
	createTestStudent(t, ctx, dbpool, "W260113", "Source No Restore Student")

	_, err := dbpool.Exec(ctx, `
		INSERT INTO course_students (course_id, student_id)
		VALUES ($1, (SELECT id FROM students WHERE wcode = 'W260112'))
	`, sourceWithEnrollmentID)
	if err != nil {
		t.Fatalf("seed source enrollment: %v", err)
	}

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)
	for _, tc := range []struct {
		wcode    string
		sourceID uuid.UUID
		note     string
	}{
		{wcode: "W260112", sourceID: sourceWithEnrollmentID, note: "source-restore"},
		{wcode: "W260113", sourceID: sourceWithoutEnrollmentID, note: "source-no-restore"},
	} {
		if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
			WCode:            tc.wcode,
			SourceCourseID:   tc.sourceID,
			SnapshotID:       uuidFromPG(t, snapshotID),
			DestCourseAID:    destAID,
			DestCourseBID:    destBID,
			AssignedCourseID: destAID,
			ExtraNoteText:    tc.note,
		}, userID); err != nil {
			t.Fatalf("save assignment for %s: %v", tc.wcode, err)
		}
		resp, err := store.LookupStudent(ctx, tc.wcode)
		if err != nil {
			t.Fatalf("lookup assignment for %s: %v", tc.wcode, err)
		}
		assignmentID, err := uuid.Parse(resp.CurrentAssignment.ID)
		if err != nil {
			t.Fatalf("parse assignment id for %s: %v", tc.wcode, err)
		}
		if err := store.DeleteAssignment(ctx, assignmentID); err != nil {
			t.Fatalf("delete assignment for %s: %v", tc.wcode, err)
		}
	}

	var restored int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students
		WHERE course_id = $1
		  AND student_id = (SELECT id FROM students WHERE wcode = 'W260112')
	`, sourceWithEnrollmentID).Scan(&restored); err != nil {
		t.Fatalf("count restored source enrollment: %v", err)
	}
	if restored != 1 {
		t.Fatalf("expected removed source enrollment to be restored, got %d rows", restored)
	}

	var invented int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students
		WHERE course_id = $1
		  AND student_id = (SELECT id FROM students WHERE wcode = 'W260113')
	`, sourceWithoutEnrollmentID).Scan(&invented); err != nil {
		t.Fatalf("count source enrollment that should not be invented: %v", err)
	}
	if invented != 0 {
		t.Fatalf("expected source enrollment not to be invented after delete, got %d rows", invented)
	}
}

// TestCrossStudy_SaveAssignment_PreservesOtherAssignmentForSameStudent proves
// cross-study cleanup is scoped to the assignment being edited, not the student.
func TestCrossStudy_SaveAssignment_PreservesOtherAssignmentForSameStudent(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sourceAID := createTestCourseSimple(t, ctx, dbpool, "CS-MULTI-SRC-A", "Multi Source A")
	sourceBID := createTestCourseSimple(t, ctx, dbpool, "CS-MULTI-SRC-B", "Multi Source B")
	destAID := createTestCourseSimple(t, ctx, dbpool, "CS-MULTI-DST-A", "Multi Dest A")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-MULTI-DST-B", "Multi Dest B")

	snapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{WCode: "W260101", CourseName: "Multi Source A", CycleLabel: "Cycle A", ExtraNote: "source-a"},
		{WCode: "W260101", CourseName: "Multi Source B", CycleLabel: "Cycle B", ExtraNote: "source-b"},
	})
	activateSnapshot(t, ctx, dbpool, snapshotID)
	createTestStudent(t, ctx, dbpool, "W260101", "Multi Assignment Student")

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)

	if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260101",
		SourceCourseID:   sourceAID,
		SnapshotID:       uuidFromPG(t, snapshotID),
		DestCourseAID:    destAID,
		DestCourseBID:    destBID,
		AssignedCourseID: destAID,
		ExtraNoteText:    "source-a",
	}, userID); err != nil {
		t.Fatalf("save first assignment: %v", err)
	}

	if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260101",
		SourceCourseID:   sourceBID,
		SnapshotID:       uuidFromPG(t, snapshotID),
		DestCourseAID:    destAID,
		DestCourseBID:    destBID,
		AssignedCourseID: destBID,
		ExtraNoteText:    "source-b",
	}, userID); err != nil {
		t.Fatalf("save second assignment: %v", err)
	}

	var assignmentCount int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM crm_cross_study_assignments
		WHERE wcode = 'W260101' AND deleted_at IS NULL
	`).Scan(&assignmentCount); err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	if assignmentCount != 2 {
		t.Fatalf("expected 2 active assignments, got %d", assignmentCount)
	}

	var overrideCount int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_roster_overrides
		WHERE override_source = 'cross_study'
		  AND student_id = (SELECT id FROM students WHERE wcode = 'W260101')
	`).Scan(&overrideCount); err != nil {
		t.Fatalf("count cross-study overrides: %v", err)
	}
	if overrideCount != 4 {
		t.Fatalf("expected 4 cross-study overrides across two assignments, got %d", overrideCount)
	}

	for _, courseID := range []uuid.UUID{destAID, destBID} {
		var enrolled int
		if err := dbpool.QueryRow(ctx, `
			SELECT COUNT(*) FROM course_students
			WHERE course_id = $1
			  AND student_id = (SELECT id FROM students WHERE wcode = 'W260101')
		`, courseID).Scan(&enrolled); err != nil {
			t.Fatalf("count course_students for %s: %v", courseID, err)
		}
		if enrolled != 1 {
			t.Fatalf("expected student to remain enrolled in assigned course %s, got %d rows", courseID, enrolled)
		}
	}

	var firstAssignmentID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		SELECT id FROM crm_cross_study_assignments
		WHERE wcode = 'W260101' AND source_course_id = $1 AND deleted_at IS NULL
	`, sourceAID).Scan(&firstAssignmentID); err != nil {
		t.Fatalf("lookup first assignment id: %v", err)
	}
	if err := store.DeleteAssignment(ctx, firstAssignmentID); err != nil {
		t.Fatalf("delete first assignment: %v", err)
	}

	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_roster_overrides
		WHERE override_source = 'cross_study'
		  AND student_id = (SELECT id FROM students WHERE wcode = 'W260101')
	`).Scan(&overrideCount); err != nil {
		t.Fatalf("count cross-study overrides after delete: %v", err)
	}
	if overrideCount != 3 {
		t.Fatalf("expected 3 cross-study overrides for remaining assignment, got %d", overrideCount)
	}

	var remainingAssigned int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_students
		WHERE course_id = $1
		  AND student_id = (SELECT id FROM students WHERE wcode = 'W260101')
	`, destBID).Scan(&remainingAssigned); err != nil {
		t.Fatalf("count remaining assigned course_students: %v", err)
	}
	if remainingAssigned != 1 {
		t.Fatalf("expected second assignment enrollment to remain, got %d rows", remainingAssigned)
	}
}

// TestCrossStudy_Processor_UpdatesStatus verifies that ProcessSnapshot
// updates assignments to the correct status based on the current snapshot.
func TestCrossStudy_Processor_UpdatesStatus(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Arrange
	sourceID := createTestCourseSimple(t, ctx, dbpool, "CS-PROC-SRC", "Proc Source")
	destAID := createTestCourseSimple(t, ctx, dbpool, "CS-PROC-DST", "Proc Dest")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-PROC-DST2", "Proc Dest 2")

	firstSnapID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{
			WCode:      "W260030",
			CourseName: "Proc Source",
			CycleLabel: "Cycle A",
			ExtraNote:  "initial",
		},
	})
	activateSnapshot(t, ctx, dbpool, firstSnapID)
	createTestStudent(t, ctx, dbpool, "W260030", "Test Student 030")

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)

	err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260030",
		SourceCourseID:   sourceID,
		SnapshotID:       uuidFromPG(t, firstSnapID),
		DestCourseAID:    destAID,
		DestCourseBID:    destBID,
		AssignedCourseID: destAID,
		ExtraNoteText:    "initial",
	}, userID)
	if err != nil {
		t.Fatalf("SaveAssignment: %v", err)
	}

	logger := tLogger
	proc := crossstudy.NewProcessor(dbpool, store, logger)

	// Act 1: process first snapshot (should update to active)
	err = proc.ProcessSnapshot(ctx, uuidFromPG(t, firstSnapID))
	if err != nil {
		t.Fatalf("ProcessSnapshot(first) failed: %v", err)
	}

	resp, err := store.LookupStudent(ctx, "W260030")
	if err != nil {
		t.Fatalf("LookupStudent: %v", err)
	}
	if resp.CurrentAssignment == nil {
		t.Fatal("expected assignment")
	}
	if resp.CurrentAssignment.Status != "active" {
		t.Fatalf("expected status='active', got %q", resp.CurrentAssignment.Status)
	}

	// Act 2: create second snapshot with changed extra_note
	secondSnapID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{
			WCode:      "W260030",
			CourseName: "Proc Source",
			CycleLabel: "Cycle A",
			ExtraNote:  "changed-note",
		},
	})
	activateSnapshot(t, ctx, dbpool, secondSnapID)

	err = proc.ProcessSnapshot(ctx, uuidFromPG(t, secondSnapID))
	if err != nil {
		t.Fatalf("ProcessSnapshot(second) failed: %v", err)
	}

	resp2, err := store.LookupStudent(ctx, "W260030")
	if err != nil {
		t.Fatalf("LookupStudent after second snapshot: %v", err)
	}
	if resp2.CurrentAssignment == nil {
		t.Fatal("expected assignment after second snapshot")
	}
	if resp2.CurrentAssignment.Status != "notes_changed" {
		t.Fatalf("expected status='notes_changed' when extra note changes, got %q", resp2.CurrentAssignment.Status)
	}
	if !resp2.CurrentAssignment.SourceValid {
		t.Fatal("expected source_valid=true when source course still exists")
	}
}
