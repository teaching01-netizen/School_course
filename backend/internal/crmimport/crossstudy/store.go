package crossstudy

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// ExcludeStudent removes a student from course_students.
// The trigger on course_students automatically soft-deletes student_busy_ranges.
func (s *Store) ExcludeStudent(ctx context.Context, tx pgx.Tx, courseID, studentID uuid.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM course_students WHERE course_id = $1 AND student_id = $2`, courseID, studentID)
	return err
}

// IncludeStudent adds a student to course_students.
// The trigger on course_students automatically inserts student_busy_ranges.
func (s *Store) IncludeStudent(ctx context.Context, tx pgx.Tx, courseID, studentID uuid.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO course_students (course_id, student_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, courseID, studentID)
	return err
}

func (s *Store) courseStudentExists(ctx context.Context, tx pgx.Tx, courseID, studentID uuid.UUID) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM course_students WHERE course_id = $1 AND student_id = $2
		)
	`, courseID, studentID).Scan(&exists)
	return exists, err
}

func (s *Store) upsertCrossStudyOverride(ctx context.Context, tx pgx.Tx, courseID, studentID, userID, assignmentID uuid.UUID, action string) error {
	tag, err := tx.Exec(ctx, `
		INSERT INTO course_roster_overrides
			(course_id, student_id, action, created_by_user_id, override_source, cross_study_assignment_id)
		VALUES ($1, $2, $3::override_action, $4, 'cross_study', $5)
		ON CONFLICT (course_id, student_id) DO UPDATE SET
			action = EXCLUDED.action,
			updated_by_user_id = EXCLUDED.created_by_user_id,
			updated_at = now(),
			deleted_at = NULL,
			override_source = 'cross_study',
			cross_study_assignment_id = EXCLUDED.cross_study_assignment_id
		WHERE course_roster_overrides.override_source = 'cross_study'
		   OR course_roster_overrides.deleted_at IS NOT NULL
	`, courseID, studentID, action, userID, assignmentID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("manual roster override already exists for course/student")
	}
	return nil
}

func (s *Store) deleteCrossStudyOverride(ctx context.Context, tx pgx.Tx, courseID, studentID uuid.UUID, action string) error {
	_, err := tx.Exec(ctx, `
		DELETE FROM course_roster_overrides
		WHERE course_id = $1
		  AND student_id = $2
		  AND action = $3::override_action
		  AND override_source = 'cross_study'
	`, courseID, studentID, action)
	return err
}

func destinationCourses(input SaveAssignmentInput) []uuid.UUID {
	if input.DestCourseAID == input.DestCourseBID {
		return []uuid.UUID{input.DestCourseAID}
	}
	return []uuid.UUID{input.DestCourseAID, input.DestCourseBID}
}

func courseInDestinations(courseID, destAID, destBID uuid.UUID) bool {
	return courseID == destAID || courseID == destBID
}

// LookupStudent finds a student and their latest CRM row.
func (s *Store) LookupStudent(ctx context.Context, wcode string) (*StudentLookupResponse, error) {
	resp := &StudentLookupResponse{
		Student: &StudentInfo{},
		CRMRow:  &CRMRowInfo{},
	}

	err := s.db.QueryRow(ctx, `
		SELECT id, wcode, COALESCE(full_name, '') FROM students WHERE wcode = $1
	`, wcode).Scan(&resp.Student.ID, &resp.Student.WCode, &resp.Student.FullName)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("student not found")
		}
		return nil, fmt.Errorf("query student: %w", err)
	}

	row := s.db.QueryRow(ctx, `
		SELECT cr.snapshot_id, cr.course_name, cr.extra_note, cs.created_at
		FROM crm_rows cr
		JOIN crm_state cs ON cr.snapshot_id = cs.active_snapshot_id
		WHERE cr.wcode = $1 AND cs.singleton = true
		ORDER BY cr.xlsx_row_number ASC
		LIMIT 1
	`, wcode)

	var snapID uuid.UUID
	var importedAt time.Time
	err = row.Scan(&snapID, &resp.CRMRow.CourseName, &resp.CRMRow.ExtraNote, &importedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			resp.CRMRow = nil
			return resp, nil
		}
		return nil, fmt.Errorf("query crm row: %w", err)
	}

	resp.CRMRow.SnapshotID = snapID.String()
	resp.CRMRow.ImportedAt = importedAt.Format(time.RFC3339)

	courseRow := s.db.QueryRow(ctx, `
		SELECT id FROM courses WHERE name = $1
	`, resp.CRMRow.CourseName)

	var courseID uuid.UUID
	if err := courseRow.Scan(&courseID); err == nil {
		resp.CRMRow.CourseID = courseID.String()
	}

	assignRow := s.db.QueryRow(ctx, `
		SELECT a.id, a.source_course_id, a.dest_course_a_id, a.dest_course_b_id, a.assigned_course_id,
		       a.status, a.extra_note_snapshot, a.source_valid, a.updated_at
		FROM crm_cross_study_assignments a
		WHERE a.wcode = $1 AND a.deleted_at IS NULL
		ORDER BY a.updated_at DESC LIMIT 1
	`, wcode)

	var aID, srcID, dcaID, dcbID, acID uuid.UUID
	var status, noteSnap string
	var srcValid bool
	var updatedAt time.Time
	err = assignRow.Scan(&aID, &srcID, &dcaID, &dcbID, &acID, &status, &noteSnap, &srcValid, &updatedAt)
	if err == nil {
		dto := &AssignmentDTO{
			ID:                aID.String(),
			AssignedCourseID:  acID.String(),
			Status:            status,
			ExtraNoteSnapshot: noteSnap,
			SourceValid:       srcValid,
			UpdatedAt:         updatedAt.Format(time.RFC3339),
		}

		dto.SourceCourse = lookupCourseRef(ctx, s.db, srcID)
		dto.DestCourseA = lookupCourseRef(ctx, s.db, dcaID)
		dto.DestCourseB = lookupCourseRef(ctx, s.db, dcbID)
		resp.CurrentAssignment = dto
	}

	return resp, nil
}

func lookupCourseRef(ctx context.Context, db *pgxpool.Pool, id uuid.UUID) *CourseRef {
	row := db.QueryRow(ctx, `
		SELECT c.id::text, c.code, c.name, COALESCE(s.name, '') AS subject_name
		FROM courses c
		LEFT JOIN subjects s ON s.id = c.subject_id
		WHERE c.id = $1
	`, id)
	ref := &CourseRef{}
	if err := row.Scan(&ref.ID, &ref.Code, &ref.Name, &ref.SubjectName); err != nil {
		return nil
	}
	return ref
}

// SaveAssignment creates or updates a cross-study assignment and its roster overrides.
func (s *Store) SaveAssignment(ctx context.Context, input SaveAssignmentInput, userID uuid.UUID) error {
	if input.AssignedCourseID != input.DestCourseAID && input.AssignedCourseID != input.DestCourseBID {
		return fmt.Errorf("assigned_course_id must be one of dest_course_a_id or dest_course_b_id")
	}

	noteHash := hashExtraNote(input.ExtraNoteText)

	var studentID uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT id FROM students WHERE wcode = $1`, input.WCode).Scan(&studentID)
	if err != nil {
		return fmt.Errorf("lookup student: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var existingAssignmentID uuid.UUID
	var existingDestCourseAID uuid.UUID
	var existingDestCourseBID uuid.UUID
	var existingAssignedCourseID uuid.UUID
	var existingAssignedCourseEnrollmentCreated bool
	var existingDestCourseAEnrollmentCreated bool
	var existingDestCourseBEnrollmentCreated bool
	var existingSourceCourseEnrollmentRemoved bool
	hasExistingAssignment := false
	err = tx.QueryRow(ctx, `
		SELECT id, dest_course_a_id, dest_course_b_id, assigned_course_id,
		       assigned_course_enrollment_created,
		       dest_course_a_enrollment_created,
		       dest_course_b_enrollment_created,
		       source_course_enrollment_removed
		FROM crm_cross_study_assignments
		WHERE wcode = $1 AND source_course_id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, input.WCode, input.SourceCourseID).Scan(
		&existingAssignmentID,
		&existingDestCourseAID,
		&existingDestCourseBID,
		&existingAssignedCourseID,
		&existingAssignedCourseEnrollmentCreated,
		&existingDestCourseAEnrollmentCreated,
		&existingDestCourseBEnrollmentCreated,
		&existingSourceCourseEnrollmentRemoved,
	)
	if err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("load existing assignment: %w", err)
	}
	hasExistingAssignment = err == nil

	destAAlreadyEnrolled, err := s.courseStudentExists(ctx, tx, input.DestCourseAID, studentID)
	if err != nil {
		return fmt.Errorf("check destination A enrollment: %w", err)
	}
	destBAlreadyEnrolled, err := s.courseStudentExists(ctx, tx, input.DestCourseBID, studentID)
	if err != nil {
		return fmt.Errorf("check destination B enrollment: %w", err)
	}
	destCourseAEnrollmentCreated := !destAAlreadyEnrolled
	if hasExistingAssignment && existingDestCourseAID == input.DestCourseAID {
		destCourseAEnrollmentCreated = existingDestCourseAEnrollmentCreated
	}
	destCourseBEnrollmentCreated := false
	if input.DestCourseBID != input.DestCourseAID {
		destCourseBEnrollmentCreated = !destBAlreadyEnrolled
		if hasExistingAssignment && existingDestCourseBID == input.DestCourseBID {
			destCourseBEnrollmentCreated = existingDestCourseBEnrollmentCreated
		}
	}
	assignedCourseEnrollmentCreated := false
	switch input.AssignedCourseID {
	case input.DestCourseAID:
		assignedCourseEnrollmentCreated = destCourseAEnrollmentCreated
	case input.DestCourseBID:
		assignedCourseEnrollmentCreated = destCourseBEnrollmentCreated
	default:
		if hasExistingAssignment && existingAssignedCourseID == input.AssignedCourseID {
			assignedCourseEnrollmentCreated = existingAssignedCourseEnrollmentCreated
		}
	}

	var assignmentID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO crm_cross_study_assignments
			(snapshot_id, wcode, source_course_id, dest_course_a_id, dest_course_b_id,
			 assigned_course_id, extra_note_snapshot, extra_note_hash,
			 assigned_course_enrollment_created,
			 dest_course_a_enrollment_created,
			 dest_course_b_enrollment_created,
			 source_course_enrollment_removed,
			 source_valid, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, false, true, 'pending')
		ON CONFLICT (wcode, source_course_id) DO UPDATE SET
			dest_course_a_id = EXCLUDED.dest_course_a_id,
			dest_course_b_id = EXCLUDED.dest_course_b_id,
			assigned_course_id = EXCLUDED.assigned_course_id,
			extra_note_snapshot = EXCLUDED.extra_note_snapshot,
			extra_note_hash = EXCLUDED.extra_note_hash,
			assigned_course_enrollment_created = EXCLUDED.assigned_course_enrollment_created,
			dest_course_a_enrollment_created = EXCLUDED.dest_course_a_enrollment_created,
			dest_course_b_enrollment_created = EXCLUDED.dest_course_b_enrollment_created,
			source_course_enrollment_removed = EXCLUDED.source_course_enrollment_removed,
			source_valid = true,
			status = 'pending',
			snapshot_id = EXCLUDED.snapshot_id,
			deleted_at = NULL,
			updated_at = now()
		RETURNING id
	`, input.SnapshotID, input.WCode, input.SourceCourseID,
		input.DestCourseAID, input.DestCourseBID, input.AssignedCourseID,
		input.ExtraNoteText, noteHash, assignedCourseEnrollmentCreated,
		destCourseAEnrollmentCreated, destCourseBEnrollmentCreated).Scan(&assignmentID)
	if err != nil {
		return fmt.Errorf("upsert assignment: %w", err)
	}

	if !courseInDestinations(input.SourceCourseID, input.DestCourseAID, input.DestCourseBID) {
		if err := s.upsertCrossStudyOverride(ctx, tx, input.SourceCourseID, studentID, userID, assignmentID, "exclude"); err != nil {
			return fmt.Errorf("insert exclude override: %w", err)
		}
	} else if hasExistingAssignment && !courseInDestinations(input.SourceCourseID, existingDestCourseAID, existingDestCourseBID) {
		excludedByOtherAssignment, err := s.crossStudyExcludesSourceCourse(ctx, tx, studentID, input.SourceCourseID, assignmentID)
		if err != nil {
			return fmt.Errorf("check stale source exclusion: %w", err)
		}
		if !excludedByOtherAssignment {
			if err := s.deleteCrossStudyOverride(ctx, tx, input.SourceCourseID, studentID, "exclude"); err != nil {
				return fmt.Errorf("delete stale source override: %w", err)
			}
		}
	}

	for _, courseID := range destinationCourses(input) {
		if err := s.upsertCrossStudyOverride(ctx, tx, courseID, studentID, userID, assignmentID, "include"); err != nil {
			return fmt.Errorf("insert include override: %w", err)
		}
	}

	// Apply immediate roster effect so preflight sees correct enrollment.
	if hasExistingAssignment {
		oldDestCreated := map[uuid.UUID]bool{
			existingDestCourseAID: existingDestCourseAEnrollmentCreated,
		}
		if existingDestCourseBID != existingDestCourseAID {
			oldDestCreated[existingDestCourseBID] = existingDestCourseBEnrollmentCreated
		}
		for oldCourseID, created := range oldDestCreated {
			if courseInDestinations(oldCourseID, input.DestCourseAID, input.DestCourseBID) {
				continue
			}
			required, err := s.crossStudyRequiresCourse(ctx, tx, studentID, oldCourseID, assignmentID)
			if err != nil {
				return fmt.Errorf("check previous destination course ownership: %w", err)
			}
			if !required && created {
				if err := s.ExcludeStudent(ctx, tx, oldCourseID, studentID); err != nil {
					return fmt.Errorf("remove previous destination course_students: %w", err)
				}
			}
			if !required {
				if err := s.deleteCrossStudyOverride(ctx, tx, oldCourseID, studentID, "include"); err != nil {
					return fmt.Errorf("delete stale include override: %w", err)
				}
			}
		}
	}
	sourceCourseEnrollmentRemoved := false
	if !courseInDestinations(input.SourceCourseID, input.DestCourseAID, input.DestCourseBID) {
		sourceCourseEnrollmentRemoved = hasExistingAssignment && existingSourceCourseEnrollmentRemoved
		required, err := s.crossStudyRequiresCourse(ctx, tx, studentID, input.SourceCourseID, assignmentID)
		if err != nil {
			return fmt.Errorf("check source course ownership: %w", err)
		}
		if !required {
			sourceEnrolled, err := s.courseStudentExists(ctx, tx, input.SourceCourseID, studentID)
			if err != nil {
				return fmt.Errorf("check source enrollment: %w", err)
			}
			sourceCourseEnrollmentRemoved = sourceCourseEnrollmentRemoved || sourceEnrolled
			if sourceEnrolled {
				if err := s.ExcludeStudent(ctx, tx, input.SourceCourseID, studentID); err != nil {
					return fmt.Errorf("exclude from source course_students: %w", err)
				}
			}
		}
	}
	for _, courseID := range destinationCourses(input) {
		if err := s.IncludeStudent(ctx, tx, courseID, studentID); err != nil {
			return fmt.Errorf("include in destination course_students: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE crm_cross_study_assignments
		SET assigned_course_enrollment_created = $2,
		    dest_course_a_enrollment_created = $3,
		    dest_course_b_enrollment_created = $4,
		    source_course_enrollment_removed = $5,
		    updated_at = now()
		WHERE id = $1
	`, assignmentID, assignedCourseEnrollmentCreated,
		destCourseAEnrollmentCreated, destCourseBEnrollmentCreated,
		sourceCourseEnrollmentRemoved); err != nil {
		return fmt.Errorf("update assignment roster ownership: %w", err)
	}

	return tx.Commit(ctx)
}

// DeleteAssignment soft-deletes an assignment and removes its overrides.
func (s *Store) DeleteAssignment(ctx context.Context, id uuid.UUID) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var wcode string
	var assignmentID, srcCourseID, destCourseAID, destCourseBID, asgnCourseID uuid.UUID
	var assignedCourseEnrollmentCreated bool
	var destCourseAEnrollmentCreated bool
	var destCourseBEnrollmentCreated bool
	var sourceCourseEnrollmentRemoved bool
	err = tx.QueryRow(ctx, `
		UPDATE crm_cross_study_assignments
		SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, wcode, source_course_id,
		          dest_course_a_id, dest_course_b_id, assigned_course_id,
		          assigned_course_enrollment_created,
		          dest_course_a_enrollment_created,
		          dest_course_b_enrollment_created,
		          source_course_enrollment_removed
	`, id).Scan(
		&assignmentID,
		&wcode,
		&srcCourseID,
		&destCourseAID,
		&destCourseBID,
		&asgnCourseID,
		&assignedCourseEnrollmentCreated,
		&destCourseAEnrollmentCreated,
		&destCourseBEnrollmentCreated,
		&sourceCourseEnrollmentRemoved,
	)
	if err != nil {
		return fmt.Errorf("soft delete assignment: %w", err)
	}

	var studentID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM students WHERE wcode = $1`, wcode).Scan(&studentID)
	if err != nil {
		return fmt.Errorf("lookup student for override cleanup: %w", err)
	}

	_ = asgnCourseID
	_ = assignedCourseEnrollmentCreated

	// Restore roster: remove destination courses only when cross-study created them.
	destCreated := map[uuid.UUID]bool{
		destCourseAID: destCourseAEnrollmentCreated,
	}
	if destCourseBID != destCourseAID {
		destCreated[destCourseBID] = destCourseBEnrollmentCreated
	}
	for courseID, created := range destCreated {
		if courseID == srcCourseID {
			continue
		}
		required, err := s.crossStudyRequiresCourse(ctx, tx, studentID, courseID, assignmentID)
		if err != nil {
			return fmt.Errorf("check destination course ownership: %w", err)
		}
		if !required && created {
			if err := s.ExcludeStudent(ctx, tx, courseID, studentID); err != nil {
				return fmt.Errorf("remove from destination course_students: %w", err)
			}
		}
		if !required {
			if err := s.deleteCrossStudyOverride(ctx, tx, courseID, studentID, "include"); err != nil {
				return fmt.Errorf("delete include override: %w", err)
			}
		}
	}
	if !courseInDestinations(srcCourseID, destCourseAID, destCourseBID) {
		excludedByOtherAssignment, err := s.crossStudyExcludesSourceCourse(ctx, tx, studentID, srcCourseID, assignmentID)
		if err != nil {
			return fmt.Errorf("check source course exclusion: %w", err)
		}
		if !excludedByOtherAssignment && sourceCourseEnrollmentRemoved {
			if err := s.IncludeStudent(ctx, tx, srcCourseID, studentID); err != nil {
				return fmt.Errorf("restore to source course_students: %w", err)
			}
		}
		if !excludedByOtherAssignment {
			if err := s.deleteCrossStudyOverride(ctx, tx, srcCourseID, studentID, "exclude"); err != nil {
				return fmt.Errorf("delete exclude override: %w", err)
			}
		}
	}

	return tx.Commit(ctx)
}

func (s *Store) crossStudyRequiresCourse(ctx context.Context, tx pgx.Tx, studentID, courseID, exceptAssignmentID uuid.UUID) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM crm_cross_study_assignments a
			JOIN students s ON s.wcode = a.wcode
			WHERE s.id = $1
			  AND (a.dest_course_a_id = $2 OR a.dest_course_b_id = $2)
			  AND a.id <> $3
			  AND a.deleted_at IS NULL
		)
	`, studentID, courseID, exceptAssignmentID).Scan(&exists)
	return exists, err
}

func (s *Store) crossStudyExcludesSourceCourse(ctx context.Context, tx pgx.Tx, studentID, courseID, exceptAssignmentID uuid.UUID) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM crm_cross_study_assignments a
			JOIN students s ON s.wcode = a.wcode
			WHERE s.id = $1
			  AND a.source_course_id = $2
			  AND a.dest_course_a_id <> a.source_course_id
			  AND a.dest_course_b_id <> a.source_course_id
			  AND a.id <> $3
			  AND a.deleted_at IS NULL
		)
	`, studentID, courseID, exceptAssignmentID).Scan(&exists)
	return exists, err
}

// ListAssignmentsWithCourseInfo returns all non-deleted assignments with student and course names.
func (s *Store) ListAssignmentsWithCourseInfo(ctx context.Context, statusFilter, searchQuery string) ([]AssignmentSummary, error) {
	where := "a.deleted_at IS NULL"
	args := []any{}
	argIdx := 1

	if statusFilter != "" {
		where += fmt.Sprintf(" AND a.status = $%d", argIdx)
		args = append(args, statusFilter)
		argIdx++
	}
	if searchQuery != "" {
		where += fmt.Sprintf(" AND (a.wcode ILIKE $%d OR s.full_name ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+searchQuery+"%")
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT a.id, a.wcode, COALESCE(s.full_name, '') AS full_name,
		       COALESCE(src.name, '') AS source_course_name, a.source_course_id,
		       COALESCE(dest.name, '') AS assigned_course_name, a.assigned_course_id,
		       a.status, a.updated_at
		FROM crm_cross_study_assignments a
		LEFT JOIN courses src ON src.id = a.source_course_id
		LEFT JOIN courses dest ON dest.id = a.assigned_course_id
		LEFT JOIN students s ON s.wcode = a.wcode
		WHERE %s
		ORDER BY a.updated_at DESC
	`, where)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query assignments: %w", err)
	}
	defer rows.Close()

	var out []AssignmentSummary
	for rows.Next() {
		var item AssignmentSummary
		var updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.WCode, &item.FullName,
			&item.SourceCourseName, &item.SourceCourseID,
			&item.AssignedCourseName, &item.AssignedCourseID,
			&item.Status, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan assignment: %w", err)
		}
		item.UpdatedAt = updatedAt.Format(time.RFC3339)
		out = append(out, item)
	}
	return out, nil
}

// HasAnyAssignment returns true if any non-deleted cross-study assignments exist.
func (s *Store) HasAnyAssignment(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM crm_cross_study_assignments WHERE deleted_at IS NULL)`).Scan(&exists)
	return exists, err
}

// LoadPendingChanges loads all assignments that need status re-check for a given snapshot.
func (s *Store) LoadPendingChanges(ctx context.Context, snapshotID uuid.UUID) ([]AssignmentChange, error) {
	rows, err := s.db.Query(ctx, `
		SELECT a.id, a.wcode, COALESCE(cr.extra_note, '') AS current_note,
		       COALESCE(cr.course_name, '') AS current_course,
		       a.extra_note_hash
		FROM crm_cross_study_assignments a
		LEFT JOIN crm_rows cr ON cr.wcode = a.wcode AND cr.snapshot_id = $1
		WHERE a.deleted_at IS NULL
	`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AssignmentChange
	for rows.Next() {
		var ch AssignmentChange
		if err := rows.Scan(&ch.ID, &ch.WCode, &ch.CurrentNote, &ch.CurrentCourseName, &ch.StoredHash); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, nil
}

// UpdateStatus sets the status for an assignment.
func (s *Store) UpdateStatus(ctx context.Context, id uuid.UUID, status string, sourceValid bool) error {
	_, err := s.db.Exec(ctx, `
		UPDATE crm_cross_study_assignments
		SET status = $2, source_valid = $3, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, id, status, sourceValid)
	return err
}

// DB returns the underlying pool for processor access.
func (s *Store) DB() *pgxpool.Pool { return s.db }

// Helper for pgtype.UUID conversion.
func uuidFromString(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	parsed, err := uuid.Parse(s)
	if err != nil {
		return id, err
	}
	id.Bytes = parsed
	id.Valid = true
	return id, nil
}
