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

func createTestCourseWithCRMFilter(t *testing.T, ctx context.Context, dbpool *pgxpool.Pool, code, name, crmCourseName string) uuid.UUID {
	t.Helper()
	_, err := dbpool.Exec(ctx, `
		INSERT INTO courses (id, code, name, crm_filter_enabled, crm_filter)
		VALUES (gen_random_uuid(), $1, $2, true, jsonb_build_object('course_name_values', jsonb_build_array($3::text)))
	`, code, name, crmCourseName)
	if err != nil {
		t.Fatalf("create crm-filtered course: %v", err)
	}
	var id uuid.UUID
	if err := dbpool.QueryRow(ctx, `SELECT id FROM courses WHERE code = $1`, code).Scan(&id); err != nil {
		t.Fatalf("get crm-filtered course id: %v", err)
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
	// wcodes are stored lowercase (see 00066/00072 and the store's
	// normalizeWCode); every roster/override lookup joins on that canonical form.
	_, err := dbpool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES (LOWER(TRIM($1)), $2, '') ON CONFLICT DO NOTHING`, wcode, fullName)
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
	if resp.Student.WCode != "w260001" {
		t.Fatalf("expected wcode='w260001', got %q", resp.Student.WCode)
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

	// Arrange: the source course carries a CRM filter so the destination-only
	// store can exclude it by CRM course name.
	sourceCourseID := createTestCourseWithCRMFilter(t, ctx, dbpool, "CS-SRC-01", "CrossStudy Source", "CrossStudy Source")
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
		CRMCourseName:    "CrossStudy Source",
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
		  AND student_id = (SELECT id FROM students WHERE wcode = 'w260002')
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
			  AND student_id = (SELECT id FROM students WHERE wcode = 'w260202')
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
		  AND student_id = (SELECT id FROM students WHERE wcode = 'w260202')
		  AND course_id IN ($1, $2)
	`, destAID, destBID).Scan(&includeOverrides); err != nil {
		t.Fatalf("count include overrides: %v", err)
	}
	if includeOverrides != 2 {
		t.Fatalf("expected include overrides for both destination courses, got %d", includeOverrides)
	}
}

// TestCrossStudy_SaveAssignment_UpdatesExistingAssignmentWhenDestAChanges verifies
// that saving an existing assignment with a changed destination course A updates
// the student's assignment row (keyed by wcode) instead of inserting a second
// active assignment, and that overrides and session attendance remain attached
// to the single surviving row.
func TestCrossStudy_SaveAssignment_UpdatesExistingAssignmentWhenDestAChanges(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	destXID := createTestCourseSimple(t, ctx, dbpool, "CS-CHG-DST-X", "Change Dest X")
	destYID := createTestCourseSimple(t, ctx, dbpool, "CS-CHG-DST-Y", "Change Dest Y")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-CHG-DST-B", "Change Dest B")
	teacherID := createTestUser(t, ctx, dbpool)

	// Tuesday session in each destination-A course and a Saturday session in
	// dest B so weekday-scoped cross-study attendance rows are created.
	for label, spec := range map[string]struct {
		courseID uuid.UUID
		startAt  string
	}{
		"x_tue": {destXID, "2026-06-16T09:00:00+07:00"},
		"y_tue": {destYID, "2026-06-16T13:00:00+07:00"},
		"b_sat": {destBID, "2026-06-20T09:00:00+07:00"},
	} {
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO sessions (id, course_id, teacher_id, start_at, end_at)
			VALUES (gen_random_uuid(), $1, $2, $3::timestamptz, $3::timestamptz + interval '1 hour')
		`, spec.courseID, teacherID, spec.startAt); err != nil {
			t.Fatalf("create %s session: %v", label, err)
		}
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{WCode: "W260115", CourseName: "Change Source", CycleLabel: "Cycle A", ExtraNote: "change-dest-a"},
	})
	activateSnapshot(t, ctx, dbpool, snapshotID)
	createTestStudent(t, ctx, dbpool, "W260115", "Change Dest A Student")

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)

	save := func(destA uuid.UUID) {
		t.Helper()
		if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
			WCode:               "W260115",
			SnapshotID:          uuidFromPG(t, snapshotID),
			DestCourseAID:       destA,
			DestCourseBID:       destBID,
			DestCourseAWeekdays: []int16{2},
			DestCourseBWeekdays: []int16{6},
			AssignedCourseID:    destA,
			ExtraNoteText:       "change-dest-a",
		}, userID); err != nil {
			t.Fatalf("SaveAssignment with dest A %s failed: %v", destA, err)
		}
	}

	// Save with Dest A = X, then save the same student with Dest A = Y.
	save(destXID)
	save(destYID)

	// The change must update the existing row: exactly one active assignment.
	var assignmentCount int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM crm_cross_study_assignments
		WHERE wcode = 'w260115' AND deleted_at IS NULL
	`).Scan(&assignmentCount); err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	if assignmentCount != 1 {
		t.Fatalf("expected exactly 1 active assignment after destination A change, got %d", assignmentCount)
	}

	// ... and that single assignment points at the new destination A.
	var assignmentID, destA uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		SELECT id, dest_course_a_id FROM crm_cross_study_assignments
		WHERE wcode = 'w260115' AND deleted_at IS NULL
	`).Scan(&assignmentID, &destA); err != nil {
		t.Fatalf("load single assignment: %v", err)
	}
	if destA != destYID {
		t.Fatalf("expected dest_course_a_id = changed course (Y), got %s", destA)
	}

	// Session attendance must be scoped to the single assignment row
	// (the rows from the original destination A are rebuilt for dest Y,
	// never orphaned under a stale assignment id).
	var attendanceCount int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM session_attendance
		WHERE student_id = (SELECT id FROM students WHERE wcode = 'w260115')
		  AND override_source = 'cross_study'
		  AND cross_study_assignment_id = $1
	`, assignmentID).Scan(&attendanceCount); err != nil {
		t.Fatalf("count attendance for single assignment: %v", err)
	}
	if attendanceCount != 2 {
		t.Fatalf("expected 2 scoped attendance rows (new dest A Tue + dest B Sat) on the single assignment, got %d", attendanceCount)
	}
	var orphanedAttendance int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM session_attendance
		WHERE student_id = (SELECT id FROM students WHERE wcode = 'w260115')
		  AND override_source = 'cross_study'
		  AND cross_study_assignment_id <> $1
	`, assignmentID).Scan(&orphanedAttendance); err != nil {
		t.Fatalf("count orphaned attendance: %v", err)
	}
	if orphanedAttendance != 0 {
		t.Fatalf("expected no attendance orphaned under other assignment ids, got %d", orphanedAttendance)
	}

	// Include overrides must point at the single assignment row.
	var overrideCount int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_roster_overrides
		WHERE override_source = 'cross_study'
		  AND student_id = (SELECT id FROM students WHERE wcode = 'w260115')
		  AND cross_study_assignment_id = $1
	`, assignmentID).Scan(&overrideCount); err != nil {
		t.Fatalf("count overrides for single assignment: %v", err)
	}
	if overrideCount != 2 {
		t.Fatalf("expected 2 include overrides (dest Y + dest B) on the single assignment, got %d", overrideCount)
	}
}

// TestCrossStudy_SaveAssignment_FallbackSelectsMostRecentAssignment pins the
// fallback row selection: when the (wcode, destination A) lookup misses, the
// store must update the student's MOST RECENT assignment (updated_at DESC
// LIMIT 1), never an arbitrary legacy row for the same wcode.
func TestCrossStudy_SaveAssignment_FallbackSelectsMostRecentAssignment(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	destXID := createTestCourseSimple(t, ctx, dbpool, "CS-MR-DST-X", "Most Recent Dest X")
	destYID := createTestCourseSimple(t, ctx, dbpool, "CS-MR-DST-Y", "Most Recent Dest Y")
	destZID := createTestCourseSimple(t, ctx, dbpool, "CS-MR-DST-Z", "Most Recent Dest Z")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-MR-DST-B", "Most Recent Dest B")

	snapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{WCode: "W260126", CourseName: "Most Recent Source", CycleLabel: "Cycle A", ExtraNote: "most-recent"},
	})
	activateSnapshot(t, ctx, dbpool, snapshotID)
	createTestStudent(t, ctx, dbpool, "W260126", "Most Recent Student")

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)

	// The current assignment: Dest A = X.
	if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260126",
		SnapshotID:       uuidFromPG(t, snapshotID),
		DestCourseAID:    destXID,
		DestCourseBID:    destBID,
		AssignedCourseID: destXID,
		ExtraNoteText:    "most-recent",
	}, userID); err != nil {
		t.Fatalf("save current assignment: %v", err)
	}
	var currentID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		SELECT id FROM crm_cross_study_assignments
		WHERE wcode = 'w260126' AND source_course_id = $1 AND deleted_at IS NULL
	`, destXID).Scan(&currentID); err != nil {
		t.Fatalf("load current assignment id: %v", err)
	}

	// A legacy duplicate row for the same wcode, older than the current one.
	var legacyID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO crm_cross_study_assignments
			(snapshot_id, wcode, source_course_id, dest_course_a_id, dest_course_b_id,
			 assigned_course_id, extra_note_snapshot, extra_note_hash, status, updated_at)
		VALUES ($1, 'w260126', $2, $2, $3, $3, '', '', 'active', now() - interval '1 day')
		RETURNING id
	`, uuidFromPG(t, snapshotID), destZID, destBID).Scan(&legacyID); err != nil {
		t.Fatalf("seed legacy assignment: %v", err)
	}

	// Save with Dest A = Y: primary lookup misses, fallback must pick the
	// most recent row (the current assignment), not the legacy one.
	if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260126",
		SnapshotID:       uuidFromPG(t, snapshotID),
		DestCourseAID:    destYID,
		DestCourseBID:    destBID,
		AssignedCourseID: destYID,
		ExtraNoteText:    "most-recent",
	}, userID); err != nil {
		t.Fatalf("save with changed destination A: %v", err)
	}

	var activeCount int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM crm_cross_study_assignments
		WHERE wcode = 'w260126' AND deleted_at IS NULL
	`).Scan(&activeCount); err != nil {
		t.Fatalf("count active assignments: %v", err)
	}
	if activeCount != 2 {
		t.Fatalf("expected 2 active assignments (current + legacy), got %d", activeCount)
	}

	// The updated row must be the most recent one (current assignment id).
	var updatedID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		SELECT id FROM crm_cross_study_assignments
		WHERE wcode = 'w260126' AND source_course_id = $1 AND deleted_at IS NULL
	`, destYID).Scan(&updatedID); err != nil {
		t.Fatalf("load updated assignment id: %v", err)
	}
	if updatedID != currentID {
		t.Fatalf("fallback updated wrong row: got assignment %s, want most recent %s", updatedID, currentID)
	}

	// The legacy row must be untouched.
	var legacyDestA uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		SELECT dest_course_a_id FROM crm_cross_study_assignments WHERE id = $1
	`, legacyID).Scan(&legacyDestA); err != nil {
		t.Fatalf("load legacy assignment: %v", err)
	}
	if legacyDestA != destZID {
		t.Fatalf("legacy assignment was mutated: dest_course_a_id = %s, want %s", legacyDestA, destZID)
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
		  AND student_id = (SELECT id FROM students WHERE wcode = 'w260003')
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
	items, err := store.ListAssignmentsWithCourseInfo(ctx, "", "", 1000, 0)
	if err != nil {
		t.Fatalf("ListAssignmentsWithCourseInfo failed: %v", err)
	}

	// Assert
	if len(items) < 1 {
		t.Fatal("expected at least 1 assignment")
	}
	found := false
	for _, item := range items {
		if item.WCode == "w260010" {
			found = true
			if item.SourceCourseName != "List Dest A1" {
				t.Fatalf("expected source_course_name to mirror destination A ('List Dest A1'), got %q", item.SourceCourseName)
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
	pendingItems, err := store.ListAssignmentsWithCourseInfo(ctx, "pending", "", 1000, 0)
	if err != nil {
		t.Fatalf("ListAssignmentsWithCourseInfo(status=pending) failed: %v", err)
	}
	if len(pendingItems) < 1 {
		t.Fatal("expected at least 1 pending assignment")
	}
	for _, item := range pendingItems {
		if item.WCode == "w260010" && item.Status != "pending" {
			t.Fatalf("expected status='pending', got %q", item.Status)
		}
	}

	// Act: filter by non-matching status
	orphanedItems, err := store.ListAssignmentsWithCourseInfo(ctx, "orphaned", "", 1000, 0)
	if err != nil {
		t.Fatalf("ListAssignmentsWithCourseInfo(status=orphaned) failed: %v", err)
	}
	for _, item := range orphanedItems {
		if item.WCode == "w260010" {
			t.Fatal("orphaned filter returned active assignment")
		}
	}
}

// TestCrossStudy_ListAssignments_PaginatesAndCounts verifies that the list is
// bounded (LIMIT/OFFSET), that CountAssignments matches the same filters, and
// that pages are disjoint (deterministic ordering, no duplicates).
func TestCrossStudy_ListAssignments_PaginatesAndCounts(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sourceAID := createTestCourseSimple(t, ctx, dbpool, "CS-PG-SRC-A", "Page Source A")
	destA1ID := createTestCourseSimple(t, ctx, dbpool, "CS-PG-A1", "Page Dest A1")
	destB1ID := createTestCourseSimple(t, ctx, dbpool, "CS-PG-B1", "Page Dest B1")

	snapID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{
		{WCode: "W260011", CourseName: "Page Source A", CycleLabel: "Cycle A", ExtraNote: "a"},
	})
	activateSnapshot(t, ctx, dbpool, snapID)
	for _, wcode := range []string{"W260011", "W260012", "W260013"} {
		createTestStudent(t, ctx, dbpool, wcode, "Page Student "+wcode)
	}

	userID := createTestUser(t, ctx, dbpool)
	store := crossstudy.NewStore(dbpool)
	for _, wcode := range []string{"w260011", "w260012", "w260013"} {
		if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
			WCode:            wcode,
			SourceCourseID:   sourceAID,
			SnapshotID:       uuidFromPG(t, snapID),
			DestCourseAID:    destA1ID,
			DestCourseBID:    destB1ID,
			AssignedCourseID: destA1ID,
			ExtraNoteText:    "a",
		}, userID); err != nil {
			t.Fatalf("SaveAssignment(%s): %v", wcode, err)
		}
	}

	// Act: page 1 (limit 2) and page 2 (limit 2)
	page1, err := store.ListAssignmentsWithCourseInfo(ctx, "", "", 2, 0)
	if err != nil {
		t.Fatalf("ListAssignmentsWithCourseInfo(page 1) failed: %v", err)
	}
	page2, err := store.ListAssignmentsWithCourseInfo(ctx, "", "", 2, 2)
	if err != nil {
		t.Fatalf("ListAssignmentsWithCourseInfo(page 2) failed: %v", err)
	}
	total, err := store.CountAssignments(ctx, "", "")
	if err != nil {
		t.Fatalf("CountAssignments failed: %v", err)
	}

	// Assert: bounded pages, disjoint across pages, total matches.
	if len(page1) != 2 {
		t.Fatalf("expected 2 items on page 1, got %d", len(page1))
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 item on page 2, got %d", len(page2))
	}
	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
	seen := map[string]bool{}
	for _, item := range append(page1, page2...) {
		if seen[item.WCode] {
			t.Fatalf("assignment %s appeared on two pages (nondeterministic ordering)", item.WCode)
		}
		seen[item.WCode] = true
	}
	for _, wcode := range []string{"w260011", "w260012", "w260013"} {
		if !seen[wcode] {
			t.Fatalf("assignment %s missing across pages", wcode)
		}
	}

	// Act: search + count with the same filter
	matches, err := store.ListAssignmentsWithCourseInfo(ctx, "", "w260012", 1000, 0)
	if err != nil {
		t.Fatalf("ListAssignmentsWithCourseInfo(search) failed: %v", err)
	}
	searchTotal, err := store.CountAssignments(ctx, "", "w260012")
	if err != nil {
		t.Fatalf("CountAssignments(search) failed: %v", err)
	}
	if len(matches) != 1 || matches[0].WCode != "w260012" {
		t.Fatalf("expected exactly w260012 for search, got %+v", matches)
	}
	if searchTotal != 1 {
		t.Fatalf("expected search total 1, got %d", searchTotal)
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
		if ch.WCode == "w260020" {
			if ch.CurrentCourseName != "" {
				t.Fatalf("expected empty current_course_name for orphaned student, got %q", ch.CurrentCourseName)
			}
		}
	}
}

func TestCrossStudy_ProcessSnapshot_MatchesLegacyUppercaseAssignmentToLowercaseImport(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sourceID := createTestCourseSimple(t, ctx, dbpool, "CS-CASE-SRC", "Case Source")
	destAID := createTestCourseSimple(t, ctx, dbpool, "CS-CASE-A", "Case Dest A")
	destBID := createTestCourseSimple(t, ctx, dbpool, "CS-CASE-B", "Case Dest B")
	snapshotID := createTestSnapshot(t, ctx, dbpool, []xlsx.Row{{
		WCode:      "w240591",
		CourseName: "Case Source",
		CycleLabel: "Cycle A",
		ExtraNote:  "unchanged",
	}})
	createTestStudent(t, ctx, dbpool, "w240591", "Case Student")

	store := crossstudy.NewStore(dbpool)
	if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "w240591",
		SourceCourseID:   sourceID,
		SnapshotID:       uuidFromPG(t, snapshotID),
		CRMCourseName:    "Case Source",
		DestCourseAID:    destAID,
		DestCourseBID:    destBID,
		AssignedCourseID: destAID,
		ExtraNoteText:    "unchanged",
	}, createTestUser(t, ctx, dbpool)); err != nil {
		t.Fatalf("SaveAssignment: %v", err)
	}

	// Reproduce data created before Wcode normalization was applied to the
	// cross-study table. XLSX import stores the same identity in lowercase.
	if _, err := dbpool.Exec(ctx, `
		UPDATE crm_cross_study_assignments
		SET wcode = 'W240591', status = 'pending'
		WHERE source_course_id = $1
	`, destAID); err != nil {
		t.Fatalf("seed legacy uppercase assignment: %v", err)
	}

	processor := crossstudy.NewProcessor(dbpool, store, tLogger)
	if err := processor.ProcessSnapshot(ctx, uuidFromPG(t, snapshotID)); err != nil {
		t.Fatalf("ProcessSnapshot: %v", err)
	}

	resp, err := store.LookupStudent(ctx, "W240591")
	if err != nil {
		t.Fatalf("LookupStudent: %v", err)
	}
	if resp.CurrentAssignment == nil {
		t.Fatal("expected legacy uppercase assignment to reconnect")
	}
	if resp.CurrentAssignment.Status != "active" {
		t.Fatalf("status = %q, want active", resp.CurrentAssignment.Status)
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
		AND student_id = (SELECT id FROM students WHERE wcode = 'w260099')
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
		AND student_id = (SELECT id FROM students WHERE wcode = 'w260099')
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
		AND student_id = (SELECT id FROM students WHERE wcode = 'w260099')
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
		AND student_id = (SELECT id FROM students WHERE wcode = 'w260099')
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
		AND br.student_id = (SELECT id FROM students WHERE wcode = 'w260099')
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
		AND student_id = (SELECT id FROM students WHERE wcode = 'w260099')
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
		AND student_id = (SELECT id FROM students WHERE wcode = 'w260099')
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
		AND br.student_id = (SELECT id FROM students WHERE wcode = 'w260099')
		AND br.deleted_at IS NULL
	`, destAID).Scan(&brCount)
	if err != nil {
		t.Fatalf("check busy ranges after delete: %v", err)
	}
	if brCount != 0 {
		t.Fatalf("expected 0 active busy ranges for assigned course after delete, got %d", brCount)
	}
}

func TestCrossStudy_RosterEffect_WeekdayScopeCreatesEnrollmentAndScopedSessionAttendance(t *testing.T) {
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
		WHERE student_id = (SELECT id FROM students WHERE wcode = 'w260203')
		  AND course_id IN ($1, $2)
	`, destAID, destBID).Scan(&courseEnrollmentCount); err != nil {
		t.Fatalf("count destination course_students: %v", err)
	}
	if courseEnrollmentCount != 2 {
		t.Fatalf("expected student enrolled in both destination courses, got %d course_students rows", courseEnrollmentCount)
	}

	expectedIncluded := map[uuid.UUID]bool{
		sessions["writing_tue"]: true,
		sessions["reading_sat"]: true,
	}
	rows, err := dbpool.Query(ctx, `
		SELECT session_id
		FROM session_attendance
		WHERE student_id = (SELECT id FROM students WHERE wcode = 'w260203')
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
		WHERE student_id = (SELECT id FROM students WHERE wcode = 'w260203')
		  AND deleted_at IS NULL
	`).Scan(&activeBusyRanges); err != nil {
		t.Fatalf("count active busy ranges: %v", err)
	}
	// 2 course_students entries × 2 sessions each (all sessions in each course via trigger) = 4
	if activeBusyRanges != 4 {
		t.Fatalf("expected 4 active busy ranges (all sessions in both courses via course_students trigger), got %d", activeBusyRanges)
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
		VALUES ($1, (SELECT id FROM students WHERE wcode = 'w260100'))
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
		AND student_id = (SELECT id FROM students WHERE wcode = 'w260100')
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
		AND student_id = (SELECT id FROM students WHERE wcode = 'w260100')
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
		VALUES ($1, (SELECT id FROM students WHERE wcode = 'w260110'))
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
		  AND student_id = (SELECT id FROM students WHERE wcode = 'w260110')
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
		VALUES ($1, (SELECT id FROM students WHERE wcode = 'w260111'))
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
		  AND student_id = (SELECT id FROM students WHERE wcode = 'w260111')
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
		VALUES ($1, (SELECT id FROM students WHERE wcode = 'w260112'))
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
		  AND student_id = (SELECT id FROM students WHERE wcode = 'w260112')
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
		  AND student_id = (SELECT id FROM students WHERE wcode = 'w260113')
	`, sourceWithoutEnrollmentID).Scan(&invented); err != nil {
		t.Fatalf("count source enrollment that should not be invented: %v", err)
	}
	if invented != 0 {
		t.Fatalf("expected source enrollment not to be invented after delete, got %d rows", invented)
	}
}

// TestCrossStudy_SaveAssignment_MovesOwnershipWhenDestAChanges proves that a
// destination-A change updates the student's existing assignment (keyed by
// wcode) instead of inserting a second active assignment, and that roster
// ownership (overrides, course_students) follows the single surviving row.
func TestCrossStudy_SaveAssignment_MovesOwnershipWhenDestAChanges(t *testing.T) {
	databaseURL := requireDB(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	destA1ID := createTestCourseSimple(t, ctx, dbpool, "CS-MULTI-DST-A", "Multi Dest A")
	destA2ID := createTestCourseSimple(t, ctx, dbpool, "CS-MULTI-DST-A2", "Multi Dest A2")
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
		SnapshotID:       uuidFromPG(t, snapshotID),
		CRMCourseName:    "Multi Source A",
		DestCourseAID:    destA1ID,
		DestCourseBID:    destBID,
		AssignedCourseID: destA1ID,
		ExtraNoteText:    "source-a",
	}, userID); err != nil {
		t.Fatalf("save first assignment: %v", err)
	}

	// Save with a changed destination A: the existing row is updated in place.
	if err := store.SaveAssignment(ctx, crossstudy.SaveAssignmentInput{
		WCode:            "W260101",
		SnapshotID:       uuidFromPG(t, snapshotID),
		CRMCourseName:    "Multi Source B",
		DestCourseAID:    destA2ID,
		DestCourseBID:    destBID,
		AssignedCourseID: destBID,
		ExtraNoteText:    "source-b",
	}, userID); err != nil {
		t.Fatalf("save second assignment: %v", err)
	}

	var assignmentCount int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM crm_cross_study_assignments
		WHERE wcode = 'w260101' AND deleted_at IS NULL
	`).Scan(&assignmentCount); err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	if assignmentCount != 1 {
		t.Fatalf("expected exactly 1 active assignment after destination A change, got %d", assignmentCount)
	}

	var assignmentID, destA uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		SELECT id, dest_course_a_id FROM crm_cross_study_assignments
		WHERE wcode = 'w260101' AND deleted_at IS NULL
	`).Scan(&assignmentID, &destA); err != nil {
		t.Fatalf("load single assignment: %v", err)
	}
	if destA != destA2ID {
		t.Fatalf("expected dest_course_a_id = %s (latest), got %s", destA2ID, destA)
	}

	var overrideCount int
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_roster_overrides
		WHERE override_source = 'cross_study'
		  AND student_id = (SELECT id FROM students WHERE wcode = 'w260101')
	`).Scan(&overrideCount); err != nil {
		t.Fatalf("count cross-study overrides: %v", err)
	}
	// destA1's include moved to destA2; destB stays with the surviving
	// assignment. All overrides point at the single assignment row.
	if overrideCount != 2 {
		t.Fatalf("expected 2 cross-study overrides on the single assignment, got %d", overrideCount)
	}
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_roster_overrides
		WHERE cross_study_assignment_id = $1
		  AND action = 'include'::override_action
		  AND override_source = 'cross_study'
		  AND deleted_at IS NULL
	`, assignmentID).Scan(&overrideCount); err != nil {
		t.Fatalf("count include overrides on assignment: %v", err)
	}
	if overrideCount != 2 {
		t.Fatalf("expected 2 include overrides (dest A2 + dest B) on the assignment, got %d", overrideCount)
	}

	// Enrollments follow ownership: new Dest A2 and shared Dest B remain,
	// the cross-study-created Dest A1 enrollment is removed.
	for _, tc := range []struct {
		courseID uuid.UUID
		want     int
	}{
		{destA1ID, 0},
		{destA2ID, 1},
		{destBID, 1},
	} {
		var enrolled int
		if err := dbpool.QueryRow(ctx, `
			SELECT COUNT(*) FROM course_students
			WHERE course_id = $1
			  AND student_id = (SELECT id FROM students WHERE wcode = 'w260101')
		`, tc.courseID).Scan(&enrolled); err != nil {
			t.Fatalf("count course_students for %s: %v", tc.courseID, err)
		}
		if enrolled != tc.want {
			t.Fatalf("expected %d course_students rows for course %s, got %d", tc.want, tc.courseID, enrolled)
		}
	}

	// Deleting the single assignment removes the remaining overrides and the
	// cross-study-created enrollments.
	if err := store.DeleteAssignment(ctx, assignmentID); err != nil {
		t.Fatalf("delete assignment: %v", err)
	}
	if err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*) FROM course_roster_overrides
		WHERE override_source = 'cross_study'
		  AND student_id = (SELECT id FROM students WHERE wcode = 'w260101')
	`).Scan(&overrideCount); err != nil {
		t.Fatalf("count cross-study overrides after delete: %v", err)
	}
	if overrideCount != 0 {
		t.Fatalf("expected 0 cross-study overrides after delete, got %d", overrideCount)
	}
	for _, courseID := range []uuid.UUID{destA2ID, destBID} {
		var enrolled int
		if err := dbpool.QueryRow(ctx, `
			SELECT COUNT(*) FROM course_students
			WHERE course_id = $1
			  AND student_id = (SELECT id FROM students WHERE wcode = 'w260101')
		`, courseID).Scan(&enrolled); err != nil {
			t.Fatalf("count course_students for %s after delete: %v", courseID, err)
		}
		if enrolled != 0 {
			t.Fatalf("expected 0 course_students rows for %s after delete, got %d", courseID, enrolled)
		}
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
		CRMCourseName:    "Proc Source",
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
