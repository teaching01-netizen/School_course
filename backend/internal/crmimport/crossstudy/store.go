package crossstudy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/schedulelock"
	"warwick-institute/internal/scheduling"
)

type Store struct {
	db         *pgxpool.Pool
	scheduling SchedulingWriter
}

type SchedulingWriter interface {
	AddCourseStudentWithWarningsTx(context.Context, pgx.Tx, *sqldb.Queries, pgtype.UUID, pgtype.UUID, scheduling.CourseStudentStatus) ([]scheduling.ScheduleWarning, error)
	UpsertSessionAttendanceWithWarningsTx(context.Context, pgx.Tx, *sqldb.Queries, pgtype.UUID, pgtype.UUID, string) ([]scheduling.ScheduleWarning, error)
}

type scheduleWarningCollectorKey struct{}

type scheduleWarningCollector struct {
	warnings []scheduling.ScheduleWarning
}

func scheduleUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte(id), Valid: id != uuid.Nil}
}

func NewStore(db *pgxpool.Pool, schedulingService SchedulingWriter) *Store {
	return &Store{db: db, scheduling: schedulingService}
}

func normalizeWCode(wcode string) string {
	return strings.ToLower(strings.TrimSpace(wcode))
}

// ExcludeStudent removes a student from course_students.
// The trigger on course_students automatically soft-deletes student_busy_ranges.
func (s *Store) ExcludeStudent(ctx context.Context, tx pgx.Tx, courseID, studentID uuid.UUID) error {
	// The caller holds the affected course and explicit student locks.
	_, err := tx.Exec(ctx, `DELETE FROM course_students WHERE course_id = $1 AND student_id = $2`, courseID, studentID)
	return err
}

// IncludeStudent adds a student to course_students.
// The trigger on course_students automatically inserts student_busy_ranges.
func (s *Store) IncludeStudent(ctx context.Context, tx pgx.Tx, courseID, studentID uuid.UUID) error {
	// The caller holds the affected course and explicit student locks.
	_, err := tx.Exec(ctx, `INSERT INTO course_students (course_id, student_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, courseID, studentID)
	return err
}

func (s *Store) includeStudentWithWarnings(ctx context.Context, tx pgx.Tx, courseID, studentID uuid.UUID) error {
	if s.scheduling == nil {
		return fmt.Errorf("cross-study: scheduling writer is required")
	}
	warnings, err := s.scheduling.AddCourseStudentWithWarningsTx(
		ctx,
		tx,
		sqldb.New(tx),
		scheduleUUID(courseID),
		scheduleUUID(studentID),
		scheduling.CourseStudentStatusEnrolled,
	)
	if err != nil {
		return err
	}
	appendScheduleWarnings(ctx, warnings)
	return nil
}

func (s *Store) SaveAssignmentWithWarnings(ctx context.Context, input SaveAssignmentInput, userID uuid.UUID) ([]scheduling.ScheduleWarning, error) {
	collector := &scheduleWarningCollector{}
	ctx = context.WithValue(ctx, scheduleWarningCollectorKey{}, collector)
	err := s.SaveAssignment(ctx, input, userID)
	return collector.warnings, err
}

func (s *Store) DeleteAssignmentWithWarnings(ctx context.Context, id uuid.UUID) ([]scheduling.ScheduleWarning, error) {
	collector := &scheduleWarningCollector{}
	ctx = context.WithValue(ctx, scheduleWarningCollectorKey{}, collector)
	err := s.DeleteAssignment(ctx, id)
	return collector.warnings, err
}

func appendScheduleWarnings(ctx context.Context, warnings []scheduling.ScheduleWarning) {
	if len(warnings) == 0 {
		return
	}
	collector, ok := ctx.Value(scheduleWarningCollectorKey{}).(*scheduleWarningCollector)
	if !ok || collector == nil {
		return
	}
	collector.warnings = append(collector.warnings, warnings...)
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

func (s *Store) deleteCrossStudySessionAttendance(ctx context.Context, tx pgx.Tx, assignmentID uuid.UUID) error {
	// The caller holds all assignment course locks and the explicit student lock.
	_, err := tx.Exec(ctx, `
		DELETE FROM session_attendance
		WHERE override_source = 'cross_study'
		  AND cross_study_assignment_id = $1
	`, assignmentID)
	return err
}

func (s *Store) insertCrossStudySessionAttendance(ctx context.Context, tx pgx.Tx, assignmentID, studentID uuid.UUID, input SaveAssignmentInput) error {
	_, err := s.insertCrossStudySessionAttendanceWithWarnings(ctx, tx, assignmentID, studentID, input)
	return err
}

func (s *Store) insertCrossStudySessionAttendanceWithWarnings(ctx context.Context, tx pgx.Tx, assignmentID, studentID uuid.UUID, input SaveAssignmentInput) ([]scheduling.ScheduleWarning, error) {
	if s.scheduling == nil {
		return nil, fmt.Errorf("cross-study: scheduling writer is required")
	}
	expanded, err := s.expandDestinationCourses(ctx, tx, input)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT s.id
		FROM sessions s
		WHERE s.deleted_at IS NULL
		  AND (
		    (
		      s.course_id = ANY($2::uuid[])
		      AND EXTRACT(ISODOW FROM (s.start_at AT TIME ZONE 'Asia/Bangkok'))::int = ANY($3::smallint[])
		    )
		    OR (
		      s.course_id = ANY($4::uuid[])
		      AND EXTRACT(ISODOW FROM (s.start_at AT TIME ZONE 'Asia/Bangkok'))::int = ANY($5::smallint[])
		    )
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM session_attendance sa
		    WHERE sa.session_id = s.id AND sa.student_id = $1
		  )
	`, studentID, databaseUUIDs(expanded.CourseA), input.DestCourseAWeekdays, databaseUUIDs(expanded.CourseB), input.DestCourseBWeekdays)
	if err != nil {
		return nil, err
	}
	var sessionIDs []uuid.UUID
	for rows.Next() {
		var sessionID uuid.UUID
		if err := rows.Scan(&sessionID); err != nil {
			rows.Close()
			return nil, err
		}
		sessionIDs = append(sessionIDs, sessionID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	qtx := sqldb.New(tx)
	var warnings []scheduling.ScheduleWarning
	for _, sessionID := range sessionIDs {
		currentWarnings, err := s.scheduling.UpsertSessionAttendanceWithWarningsTx(
			ctx,
			tx,
			qtx,
			scheduleUUID(sessionID),
			scheduleUUID(studentID),
			"included",
		)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, currentWarnings...)
		if _, err := tx.Exec(ctx, `
			UPDATE session_attendance
			SET override_source = 'cross_study', cross_study_assignment_id = $3
			WHERE session_id = $1 AND student_id = $2
		`, sessionID, studentID, assignmentID); err != nil {
			return nil, err
		}
	}
	return warnings, nil
}

type expandedDestinationSet struct {
	All     []uuid.UUID
	CourseA []uuid.UUID
	CourseB []uuid.UUID
}

func (s *Store) expandDestinationCourses(ctx context.Context, tx pgx.Tx, input SaveAssignmentInput) (expandedDestinationSet, error) {
	courseA, err := mergeGroupCourseIDs(ctx, tx, input.DestCourseAID)
	if err != nil {
		return expandedDestinationSet{}, fmt.Errorf("expand destination A merge group: %w", err)
	}
	courseB, err := mergeGroupCourseIDs(ctx, tx, input.DestCourseBID)
	if err != nil {
		return expandedDestinationSet{}, fmt.Errorf("expand destination B merge group: %w", err)
	}
	all := appendUniqueCourseIDs(nil, courseA...)
	all = appendUniqueCourseIDs(all, courseB...)
	return expandedDestinationSet{All: all, CourseA: courseA, CourseB: courseB}, nil
}

func mergeGroupCourseIDs(ctx context.Context, tx pgx.Tx, courseID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT related.course_id
		FROM (
			SELECT $1::uuid AS course_id
			UNION
			SELECT sibling.course_id
			FROM course_merge_group_members selected
			JOIN course_merge_group_members sibling ON sibling.group_id = selected.group_id
			WHERE selected.course_id = $1
		) related
		ORDER BY related.course_id
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]uuid.UUID, 0, 2)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func appendUniqueCourseIDs(ids []uuid.UUID, candidates ...uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids)+len(candidates))
	for _, id := range ids {
		seen[id] = struct{}{}
	}
	for _, id := range candidates {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func databaseUUIDs(ids []uuid.UUID) []pgtype.UUID {
	values := make([]pgtype.UUID, len(ids))
	for index, id := range ids {
		values[index] = scheduleUUID(id)
	}
	return values
}

func courseInIDs(courseID uuid.UUID, courseIDs []uuid.UUID) bool {
	for _, candidate := range courseIDs {
		if candidate == courseID {
			return true
		}
	}
	return false
}

func normalizeWeekdays(values []int16) []int16 {
	if len(values) == 0 {
		return []int16{1, 2, 3, 4, 5, 6, 7}
	}
	seen := map[int16]bool{}
	out := make([]int16, 0, len(values))
	for _, value := range values {
		if value < 1 || value > 7 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	if len(out) == 0 {
		return []int16{1, 2, 3, 4, 5, 6, 7}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func weekdaysAreAll(values []int16) bool {
	values = normalizeWeekdays(values)
	if len(values) != 7 {
		return false
	}
	for i, value := range values {
		if value != int16(i+1) {
			return false
		}
	}
	return true
}

func assignmentUsesFullCourseEnrollment(input SaveAssignmentInput) bool {
	return weekdaysAreAll(input.DestCourseAWeekdays) && weekdaysAreAll(input.DestCourseBWeekdays)
}

// LookupStudent finds a student and their CRM rows in the active snapshot.
func (s *Store) LookupStudent(ctx context.Context, wcode string) (*StudentLookupResponse, error) {
	wcode = normalizeWCode(wcode)
	resp := &StudentLookupResponse{
		Student: &StudentInfo{},
		CRMRows: make([]CRMRowInfo, 0),
	}

	err := s.db.QueryRow(ctx, `
		SELECT id, wcode, COALESCE(full_name, '') FROM students WHERE LOWER(wcode) = $1
	`, wcode).Scan(&resp.Student.ID, &resp.Student.WCode, &resp.Student.FullName)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("student not found")
		}
		return nil, fmt.Errorf("query student: %w", err)
	}

	rows, err := s.db.Query(ctx, `
		SELECT cr.snapshot_id, cr.row_hash, cr.xlsx_row_number, cr.course_name, cr.extra_note,
		       cs.created_at,
		       COALESCE(matched.course_id, ''), COALESCE(matched.merge_group_id, ''),
		       COALESCE(matched.merge_group_name, ''), COALESCE(matched.peer_course_id, ''),
		       COALESCE(matched.peer_course_name, '')
		FROM crm_rows cr
		JOIN crm_state cs ON cr.snapshot_id = cs.active_snapshot_id
		LEFT JOIN LATERAL (
			SELECT c.id::text AS course_id, merge_group.id::text AS merge_group_id,
			       merge_group.name AS merge_group_name, peer.id::text AS peer_course_id,
			       peer.name AS peer_course_name
			FROM courses c
			LEFT JOIN course_merge_group_members member ON member.course_id = c.id
			LEFT JOIN course_merge_groups merge_group ON merge_group.id = member.group_id
			LEFT JOIN course_merge_group_members peer_member
			  ON peer_member.group_id = member.group_id AND peer_member.course_id <> c.id
			LEFT JOIN courses peer ON peer.id = peer_member.course_id
			WHERE c.name = cr.course_name
			ORDER BY c.course_no DESC, peer_member.position ASC
			LIMIT 1
		) matched ON true
		WHERE LOWER(cr.wcode) = $1 AND cs.singleton = true
		ORDER BY cr.xlsx_row_number ASC
	`, wcode)
	if err != nil {
		return nil, fmt.Errorf("query crm rows: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var crmRow CRMRowInfo
		var snapID uuid.UUID
		var importedAt time.Time
		if err := rows.Scan(
			&snapID,
			&crmRow.RowHash,
			&crmRow.XLSXRowNumber,
			&crmRow.CourseName,
			&crmRow.ExtraNote,
			&importedAt,
			&crmRow.CourseID,
			&crmRow.MergeGroupID,
			&crmRow.MergeGroupName,
			&crmRow.MergeGroupPeerCourseID,
			&crmRow.MergeGroupPeerCourseName,
		); err != nil {
			return nil, fmt.Errorf("scan crm row: %w", err)
		}
		crmRow.SnapshotID = snapID.String()
		crmRow.ImportedAt = importedAt.Format(time.RFC3339)
		resp.CRMRows = append(resp.CRMRows, crmRow)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate crm rows: %w", err)
	}

	assignRow := s.db.QueryRow(ctx, `
		SELECT a.id, a.dest_course_a_id, a.dest_course_b_id, a.assigned_course_id,
		       a.dest_course_a_weekdays, a.dest_course_b_weekdays,
		       a.status, a.extra_note_snapshot, a.source_valid, a.updated_at
		FROM crm_cross_study_assignments a
		WHERE LOWER(BTRIM(a.wcode)) = $1 AND a.deleted_at IS NULL
		ORDER BY a.updated_at DESC LIMIT 1
	`, wcode)

	var aID, dcaID, dcbID, acID uuid.UUID
	var destAWeekdays, destBWeekdays []int16
	var status, noteSnap string
	var srcValid bool
	var updatedAt time.Time
	err = assignRow.Scan(&aID, &dcaID, &dcbID, &acID, &destAWeekdays, &destBWeekdays, &status, &noteSnap, &srcValid, &updatedAt)
	if err == nil {
		dto := &AssignmentDTO{
			ID:                  aID.String(),
			DestCourseAWeekdays: normalizeWeekdays(destAWeekdays),
			DestCourseBWeekdays: normalizeWeekdays(destBWeekdays),
			AssignedCourseID:    acID.String(),
			Status:              status,
			ExtraNoteSnapshot:   noteSnap,
			SourceValid:         srcValid,
			UpdatedAt:           updatedAt.Format(time.RFC3339),
		}

		dto.DestCourseA = lookupCourseRef(ctx, s.db, dcaID)
		dto.DestCourseB = lookupCourseRef(ctx, s.db, dcbID)
		resp.CurrentAssignment = dto
	}

	return resp, nil
}

func lookupCourseRef(ctx context.Context, db *pgxpool.Pool, id uuid.UUID) *CourseRef {
	row := db.QueryRow(ctx, `
		SELECT c.id::text, c.code, c.name, COALESCE(s.name, '') AS subject_name,
		       COALESCE(merge_group.id::text, ''), COALESCE(merge_group.name, '')
		FROM courses c
		LEFT JOIN subjects s ON s.id = c.subject_id
		LEFT JOIN course_merge_group_members member ON member.course_id = c.id
		LEFT JOIN course_merge_groups merge_group ON merge_group.id = member.group_id
		WHERE c.id = $1
	`, id)
	ref := &CourseRef{}
	if err := row.Scan(&ref.ID, &ref.Code, &ref.Name, &ref.SubjectName, &ref.MergeGroupID, &ref.MergeGroupName); err != nil {
		return nil
	}
	return ref
}

// SaveAssignment creates or updates a cross-study assignment and its roster overrides.
func (s *Store) SaveAssignment(ctx context.Context, input SaveAssignmentInput, userID uuid.UUID) error {
	input.WCode = normalizeWCode(input.WCode)
	if input.WCode == "" {
		return fmt.Errorf("wcode is required")
	}
	if input.AssignedCourseID != input.DestCourseAID && input.AssignedCourseID != input.DestCourseBID {
		return fmt.Errorf("assigned_course_id must be one of dest_course_a_id or dest_course_b_id")
	}
	input.DestCourseAWeekdays = normalizeWeekdays(input.DestCourseAWeekdays)
	input.DestCourseBWeekdays = normalizeWeekdays(input.DestCourseBWeekdays)
	usesFullCourseEnrollment := assignmentUsesFullCourseEnrollment(input)

	noteHash := hashExtraNote(input.ExtraNoteText)

	var studentID uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT id FROM students WHERE LOWER(wcode) = $1`, input.WCode).Scan(&studentID)
	if err != nil {
		return fmt.Errorf("lookup student: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var existingAssignmentID uuid.UUID
	var existingSourceCourseID uuid.UUID
	var existingDestCourseAID uuid.UUID
	var existingDestCourseBID uuid.UUID
	var existingAssignedCourseID uuid.UUID
	var existingAssignedCourseEnrollmentCreated bool
	var existingDestCourseAEnrollmentCreated bool
	var existingDestCourseBEnrollmentCreated bool
	var existingSourceCourseEnrollmentRemoved bool
	var existingExpandedDestinationCourseIDs []uuid.UUID
	var existingExpandedEnrollmentCreatedIDs []uuid.UUID
	hasExistingAssignment := false
	err = tx.QueryRow(ctx, `
		SELECT id, dest_course_a_id, dest_course_b_id, assigned_course_id,
		       source_course_id,
		       assigned_course_enrollment_created,
		       dest_course_a_enrollment_created,
		       dest_course_b_enrollment_created,
		       source_course_enrollment_removed,
		       expanded_destination_course_ids,
		       expanded_enrollment_created_ids
		FROM crm_cross_study_assignments
		WHERE LOWER(BTRIM(wcode)) = $1 AND deleted_at IS NULL
		  AND source_course_id = $2
		ORDER BY updated_at DESC
		LIMIT 1
		FOR UPDATE
	`, input.WCode, input.DestCourseAID).Scan(
		&existingAssignmentID,
		&existingDestCourseAID,
		&existingDestCourseBID,
		&existingAssignedCourseID,
		&existingSourceCourseID,
		&existingAssignedCourseEnrollmentCreated,
		&existingDestCourseAEnrollmentCreated,
		&existingDestCourseBEnrollmentCreated,
		&existingSourceCourseEnrollmentRemoved,
		&existingExpandedDestinationCourseIDs,
		&existingExpandedEnrollmentCreatedIDs,
	)
	if err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("load existing assignment: %w", err)
	}
	hasExistingAssignment = err == nil
	// Fallback: when the primary (wcode, destination A) lookup misses, the
	// assignment's destination course A likely changed since it was saved.
	// Update the student's most recent assignment (updated_at DESC LIMIT 1)
	// instead of inserting a duplicate — the old row would otherwise keep its
	// include overrides re-pointed to the new row and its session_attendance
	// rows orphaned.
	if !hasExistingAssignment {
		err = tx.QueryRow(ctx, `
			SELECT id, dest_course_a_id, dest_course_b_id, assigned_course_id,
			       source_course_id,
			       assigned_course_enrollment_created,
			       dest_course_a_enrollment_created,
			       dest_course_b_enrollment_created,
			       source_course_enrollment_removed,
			       expanded_destination_course_ids,
			       expanded_enrollment_created_ids
			FROM crm_cross_study_assignments
			WHERE LOWER(BTRIM(wcode)) = $1 AND deleted_at IS NULL
			ORDER BY updated_at DESC
			LIMIT 1
			FOR UPDATE
		`, input.WCode).Scan(
			&existingAssignmentID,
			&existingDestCourseAID,
			&existingDestCourseBID,
			&existingAssignedCourseID,
			&existingSourceCourseID,
			&existingAssignedCourseEnrollmentCreated,
			&existingDestCourseAEnrollmentCreated,
			&existingDestCourseBEnrollmentCreated,
			&existingSourceCourseEnrollmentRemoved,
			&existingExpandedDestinationCourseIDs,
			&existingExpandedEnrollmentCreatedIDs,
		)
		if err != nil && err != pgx.ErrNoRows {
			return fmt.Errorf("load latest assignment: %w", err)
		}
		hasExistingAssignment = err == nil
	}
	storageSourceCourseID := input.DestCourseAID
	expandedDestinations, err := s.expandDestinationCourses(ctx, tx, input)
	if err != nil {
		return err
	}

	affectedCourses := appendUniqueCourseIDs(nil, expandedDestinations.All...)
	affectedCourses = appendUniqueCourseIDs(affectedCourses, input.AssignedCourseID)
	if hasExistingAssignment {
		affectedCourses = appendUniqueCourseIDs(affectedCourses, existingExpandedDestinationCourseIDs...)
		affectedCourses = appendUniqueCourseIDs(affectedCourses, existingSourceCourseID, existingDestCourseAID, existingDestCourseBID, existingAssignedCourseID)
	}
	if input.CRMCourseName != "" {
		matching, matchErr := s.coursesMatchingCRMCourseName(ctx, tx, input.CRMCourseName)
		if matchErr != nil {
			return fmt.Errorf("find affected source courses: %w", matchErr)
		}
		affectedCourses = append(affectedCourses, matching...)
	}
	courseLocks := make([]pgtype.UUID, 0, len(affectedCourses))
	for _, id := range affectedCourses {
		courseLocks = append(courseLocks, scheduleUUID(id))
	}
	if err := schedulelock.LockResources(ctx, sqldb.New(tx), schedulelock.ResourceLocks{
		CourseIDs: courseLocks, StudentIDs: []pgtype.UUID{scheduleUUID(studentID)},
	}); err != nil {
		return fmt.Errorf("lock cross-study roster resources: %w", err)
	}

	existingCreated := make(map[uuid.UUID]bool, len(existingExpandedEnrollmentCreatedIDs))
	for _, courseID := range existingExpandedEnrollmentCreatedIDs {
		existingCreated[courseID] = true
	}
	createdByCrossStudy := make(map[uuid.UUID]bool, len(expandedDestinations.All))
	for _, courseID := range expandedDestinations.All {
		if courseInIDs(courseID, existingExpandedDestinationCourseIDs) {
			createdByCrossStudy[courseID] = existingCreated[courseID]
			continue
		}
		alreadyEnrolled, err := s.courseStudentExists(ctx, tx, courseID, studentID)
		if err != nil {
			return fmt.Errorf("check expanded destination enrollment: %w", err)
		}
		createdByCrossStudy[courseID] = !alreadyEnrolled
	}
	expandedEnrollmentCreatedIDs := make([]uuid.UUID, 0, len(createdByCrossStudy))
	for _, courseID := range expandedDestinations.All {
		if createdByCrossStudy[courseID] {
			expandedEnrollmentCreatedIDs = append(expandedEnrollmentCreatedIDs, courseID)
		}
	}
	destCourseAEnrollmentCreated := createdByCrossStudy[input.DestCourseAID]
	destCourseBEnrollmentCreated := input.DestCourseBID != input.DestCourseAID && createdByCrossStudy[input.DestCourseBID]
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
	if hasExistingAssignment {
		assignmentID = existingAssignmentID
		_, err = tx.Exec(ctx, `
			UPDATE crm_cross_study_assignments
			SET snapshot_id = $2,
			    source_course_id = $3,
			    dest_course_a_id = $4,
			    dest_course_b_id = $5,
			    assigned_course_id = $6,
			    crm_course_name_snapshot = $7,
			    crm_row_hash_snapshot = $8,
			    crm_xlsx_row_number_snapshot = NULLIF($9, 0),
			    extra_note_snapshot = $10,
			    extra_note_hash = $11,
			    dest_course_a_weekdays = $12,
			    dest_course_b_weekdays = $13,
			    assigned_course_enrollment_created = $14,
			    dest_course_a_enrollment_created = $15,
			    dest_course_b_enrollment_created = $16,
			    expanded_destination_course_ids = $17::uuid[],
			    expanded_enrollment_created_ids = $18::uuid[],
			    source_course_enrollment_removed = false,
			    source_valid = true,
			    status = 'pending',
			    deleted_at = NULL,
			    updated_at = now()
			WHERE id = $1
		`, assignmentID, input.SnapshotID, storageSourceCourseID,
			input.DestCourseAID, input.DestCourseBID, input.AssignedCourseID,
			input.CRMCourseName, input.CRMRowHash, input.CRMXLSXRowNumber,
			input.ExtraNoteText, noteHash, input.DestCourseAWeekdays,
			input.DestCourseBWeekdays, assignedCourseEnrollmentCreated,
			destCourseAEnrollmentCreated, destCourseBEnrollmentCreated,
			databaseUUIDs(expandedDestinations.All), databaseUUIDs(expandedEnrollmentCreatedIDs))
	} else {
		err = tx.QueryRow(ctx, `
			INSERT INTO crm_cross_study_assignments
				(snapshot_id, wcode, source_course_id, dest_course_a_id, dest_course_b_id,
				 assigned_course_id, crm_course_name_snapshot, crm_row_hash_snapshot,
				 crm_xlsx_row_number_snapshot, extra_note_snapshot, extra_note_hash,
				 dest_course_a_weekdays,
				 dest_course_b_weekdays,
				 assigned_course_enrollment_created,
				 dest_course_a_enrollment_created,
				 dest_course_b_enrollment_created,
				 expanded_destination_course_ids,
				 expanded_enrollment_created_ids,
				 source_course_enrollment_removed,
				 source_valid, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, 0), $10, $11, $12, $13, $14, $15, $16, $17::uuid[], $18::uuid[], false, true, 'pending')
			ON CONFLICT (wcode, source_course_id) DO UPDATE SET
				dest_course_a_id = EXCLUDED.dest_course_a_id,
				dest_course_b_id = EXCLUDED.dest_course_b_id,
				assigned_course_id = EXCLUDED.assigned_course_id,
				crm_course_name_snapshot = EXCLUDED.crm_course_name_snapshot,
				crm_row_hash_snapshot = EXCLUDED.crm_row_hash_snapshot,
				crm_xlsx_row_number_snapshot = EXCLUDED.crm_xlsx_row_number_snapshot,
				extra_note_snapshot = EXCLUDED.extra_note_snapshot,
				extra_note_hash = EXCLUDED.extra_note_hash,
				dest_course_a_weekdays = EXCLUDED.dest_course_a_weekdays,
				dest_course_b_weekdays = EXCLUDED.dest_course_b_weekdays,
				assigned_course_enrollment_created = EXCLUDED.assigned_course_enrollment_created,
				dest_course_a_enrollment_created = EXCLUDED.dest_course_a_enrollment_created,
				dest_course_b_enrollment_created = EXCLUDED.dest_course_b_enrollment_created,
				expanded_destination_course_ids = EXCLUDED.expanded_destination_course_ids,
				expanded_enrollment_created_ids = EXCLUDED.expanded_enrollment_created_ids,
				source_course_enrollment_removed = false,
				source_valid = true,
				status = 'pending',
				snapshot_id = EXCLUDED.snapshot_id,
				deleted_at = NULL,
				updated_at = now()
			RETURNING id
		`, input.SnapshotID, input.WCode, storageSourceCourseID,
			input.DestCourseAID, input.DestCourseBID, input.AssignedCourseID,
			input.CRMCourseName, input.CRMRowHash, input.CRMXLSXRowNumber,
			input.ExtraNoteText, noteHash, input.DestCourseAWeekdays,
			input.DestCourseBWeekdays, assignedCourseEnrollmentCreated,
			destCourseAEnrollmentCreated, destCourseBEnrollmentCreated,
			databaseUUIDs(expandedDestinations.All), databaseUUIDs(expandedEnrollmentCreatedIDs)).Scan(&assignmentID)
	}
	if err != nil {
		return fmt.Errorf("upsert assignment: %w", err)
	}

	if hasExistingAssignment && !courseInIDs(existingSourceCourseID, expandedDestinations.All) {
		excludedByOtherAssignment, err := s.crossStudyExcludesSourceCourse(ctx, tx, studentID, existingSourceCourseID, assignmentID)
		if err != nil {
			return fmt.Errorf("check legacy source exclusion: %w", err)
		}
		if !excludedByOtherAssignment {
			if existingSourceCourseEnrollmentRemoved {
				if err := s.includeStudentWithWarnings(ctx, tx, existingSourceCourseID, studentID); err != nil {
					return fmt.Errorf("restore legacy source course_students: %w", err)
				}
			}
			if err := s.deleteCrossStudyOverride(ctx, tx, existingSourceCourseID, studentID, "exclude"); err != nil {
				return fmt.Errorf("delete legacy source override: %w", err)
			}
		}
	}

	if err := s.deleteCrossStudySessionAttendance(ctx, tx, assignmentID); err != nil {
		return fmt.Errorf("delete stale scoped session attendance: %w", err)
	}

	for _, courseID := range expandedDestinations.All {
		if err := s.upsertCrossStudyOverride(ctx, tx, courseID, studentID, userID, assignmentID, "include"); err != nil {
			return fmt.Errorf("insert include override: %w", err)
		}
	}

	// Clear stale exclude overrides owned by this assignment before rebuilding.
	if _, err := tx.Exec(ctx, `
		DELETE FROM course_roster_overrides
		WHERE cross_study_assignment_id = $1
		  AND action = 'exclude'::override_action
		  AND override_source = 'cross_study'
	`, assignmentID); err != nil {
		return fmt.Errorf("delete stale exclude overrides: %w", err)
	}

	// Exclude student from courses whose CRM filter matches the CRM course name
	// but are not destination courses (or cohort-sibling destinations) for this assignment.
	if input.CRMCourseName != "" {
		srcIDs, err := s.coursesMatchingCRMCourseName(ctx, tx, input.CRMCourseName)
		if err != nil {
			return fmt.Errorf("find courses matching CRM course name: %w", err)
		}

		expandedDests := make(map[uuid.UUID]bool, len(expandedDestinations.All))
		for _, courseID := range expandedDestinations.All {
			expandedDests[courseID] = true
		}

		for _, srcID := range srcIDs {
			if expandedDests[srcID] {
				continue
			}
			if err := s.upsertCrossStudyOverride(ctx, tx, srcID, studentID, userID, assignmentID, "exclude"); err != nil {
				return fmt.Errorf("insert source exclude override: %w", err)
			}
			if err := s.ExcludeStudent(ctx, tx, srcID, studentID); err != nil {
				return fmt.Errorf("remove from source course_students: %w", err)
			}
		}
	}

	// Apply immediate roster effect so preflight sees correct enrollment.
	if hasExistingAssignment {
		oldDestCreated := make(map[uuid.UUID]bool, len(existingExpandedDestinationCourseIDs))
		for _, courseID := range existingExpandedDestinationCourseIDs {
			oldDestCreated[courseID] = existingCreated[courseID]
		}
		for oldCourseID, created := range oldDestCreated {
			if courseInIDs(oldCourseID, expandedDestinations.All) {
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
	for _, courseID := range expandedDestinations.All {
		if err := s.includeStudentWithWarnings(ctx, tx, courseID, studentID); err != nil {
			return fmt.Errorf("include in destination course_students: %w", err)
		}
	}
	if !usesFullCourseEnrollment {
		warnings, err := s.insertCrossStudySessionAttendanceWithWarnings(ctx, tx, assignmentID, studentID, input)
		if err != nil {
			return fmt.Errorf("insert scoped session attendance: %w", err)
		}
		appendScheduleWarnings(ctx, warnings)
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
		false); err != nil {
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
	var expandedDestinationCourseIDs []uuid.UUID
	var expandedEnrollmentCreatedIDs []uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE crm_cross_study_assignments
		SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, wcode, source_course_id,
		          dest_course_a_id, dest_course_b_id, assigned_course_id,
		          assigned_course_enrollment_created,
		          dest_course_a_enrollment_created,
		          dest_course_b_enrollment_created,
		          source_course_enrollment_removed,
		          expanded_destination_course_ids,
		          expanded_enrollment_created_ids
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
		&expandedDestinationCourseIDs,
		&expandedEnrollmentCreatedIDs,
	)
	if err != nil {
		return fmt.Errorf("soft delete assignment: %w", err)
	}

	var studentID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM students WHERE LOWER(wcode) = $1`, normalizeWCode(wcode)).Scan(&studentID)
	if err != nil {
		return fmt.Errorf("lookup student for override cleanup: %w", err)
	}

	// Discover every course this assignment may mutate before taking the
	// canonical course→student locks.
	excludeRows, err := tx.Query(ctx, `
		SELECT course_id FROM course_roster_overrides
		WHERE cross_study_assignment_id = $1
		  AND action = 'exclude'::override_action
		  AND override_source = 'cross_study'
		  AND deleted_at IS NULL
	`, assignmentID)
	if err != nil {
		return fmt.Errorf("query excluded courses: %w", err)
	}
	var excludedCourseIDs []uuid.UUID
	for excludeRows.Next() {
		var cid uuid.UUID
		if err := excludeRows.Scan(&cid); err != nil {
			excludeRows.Close()
			return fmt.Errorf("scan excluded course: %w", err)
		}
		excludedCourseIDs = append(excludedCourseIDs, cid)
	}
	excludeRows.Close()
	courseIDs := appendUniqueCourseIDs(nil, expandedDestinationCourseIDs...)
	courseIDs = appendUniqueCourseIDs(courseIDs, srcCourseID, destCourseAID, destCourseBID, asgnCourseID)
	courseIDs = appendUniqueCourseIDs(courseIDs, excludedCourseIDs...)
	courseLocks := make([]pgtype.UUID, 0, len(courseIDs))
	for _, courseID := range courseIDs {
		courseLocks = append(courseLocks, scheduleUUID(courseID))
	}
	if err := schedulelock.LockResources(ctx, sqldb.New(tx), schedulelock.ResourceLocks{
		CourseIDs: courseLocks, StudentIDs: []pgtype.UUID{scheduleUUID(studentID)},
	}); err != nil {
		return fmt.Errorf("lock cross-study delete resources: %w", err)
	}

	// Course and student locks are held for this direct attendance write.
	if err := s.deleteCrossStudySessionAttendance(ctx, tx, assignmentID); err != nil {
		return fmt.Errorf("delete scoped session attendance: %w", err)
	}

	_ = asgnCourseID
	_ = assignedCourseEnrollmentCreated
	_ = srcCourseID
	_ = sourceCourseEnrollmentRemoved

	// Restore roster: remove destination courses only when cross-study created them.
	destCreated := make(map[uuid.UUID]bool, len(expandedDestinationCourseIDs))
	for _, courseID := range expandedDestinationCourseIDs {
		destCreated[courseID] = courseInIDs(courseID, expandedEnrollmentCreatedIDs)
	}
	if len(destCreated) == 0 {
		destCreated[destCourseAID] = destCourseAEnrollmentCreated
		if destCourseBID != destCourseAID {
			destCreated[destCourseBID] = destCourseBEnrollmentCreated
		}
	}
	for courseID, created := range destCreated {
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
	// Restore all source courses that were excluded by this cross-study assignment.
	for _, courseID := range excludedCourseIDs {
		required, err := s.crossStudyExcludesSourceCourse(ctx, tx, studentID, courseID, assignmentID)
		if err != nil {
			return fmt.Errorf("check source course exclusion: %w", err)
		}
		if !required {
			if err := s.includeStudentWithWarnings(ctx, tx, courseID, studentID); err != nil {
				return fmt.Errorf("restore to source course_students: %w", err)
			}
			if err := s.deleteCrossStudyOverride(ctx, tx, courseID, studentID, "exclude"); err != nil {
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
			JOIN students s ON LOWER(s.wcode) = LOWER(BTRIM(a.wcode))
			WHERE s.id = $1
			  AND ($2 = ANY(a.expanded_destination_course_ids)
			       OR a.dest_course_a_id = $2 OR a.dest_course_b_id = $2)
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
			FROM course_roster_overrides
			WHERE student_id = $1
			  AND course_id = $2
			  AND action = 'exclude'
			  AND override_source = 'cross_study'
			  AND cross_study_assignment_id <> $3
			  AND deleted_at IS NULL
		)
	`, studentID, courseID, exceptAssignmentID).Scan(&exists)
	return exists, err
}

func (s *Store) coursesMatchingCRMCourseName(ctx context.Context, tx pgx.Tx, crmCourseName string) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.id
		FROM courses c
		WHERE c.crm_filter_enabled = true
		  AND c.crm_filter IS NOT NULL
		  AND EXISTS (
		    SELECT 1
		    FROM jsonb_array_elements_text(c.crm_filter->'course_name_values') AS cv
		    WHERE cv = $1
		  )
	`, crmCourseName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListAssignmentsWithCourseInfo returns a page of non-deleted assignments with
// student and course names, ordered deterministically by most recently updated.
func (s *Store) ListAssignmentsWithCourseInfo(ctx context.Context, statusFilter, searchQuery string, limit, offset int32) ([]AssignmentSummary, error) {
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
	where += fmt.Sprintf(" ORDER BY a.updated_at DESC, a.id DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	query := fmt.Sprintf(`
		SELECT a.id, a.wcode, COALESCE(s.full_name, '') AS full_name,
		       COALESCE(dest_a.name, '') AS dest_course_a_name, a.dest_course_a_id,
		       COALESCE(dest_b.name, '') AS dest_course_b_name, a.dest_course_b_id,
		       a.status, a.updated_at
		FROM crm_cross_study_assignments a
		LEFT JOIN courses dest_a ON dest_a.id = a.dest_course_a_id
		LEFT JOIN courses dest_b ON dest_b.id = a.dest_course_b_id
		LEFT JOIN students s ON LOWER(s.wcode) = LOWER(BTRIM(a.wcode))
		WHERE %s
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
			&item.DestCourseAName, &item.DestCourseAID,
			&item.DestCourseBName, &item.DestCourseBID,
			&item.Status, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan assignment: %w", err)
		}
		item.SourceCourseName = item.DestCourseAName
		item.SourceCourseID = item.DestCourseAID
		item.AssignedCourseName = item.DestCourseAName
		item.AssignedCourseID = item.DestCourseAID
		item.UpdatedAt = updatedAt.Format(time.RFC3339)
		out = append(out, item)
	}
	return out, nil
}

// CountAssignments returns the number of non-deleted assignments matching the
// same status/search filters as ListAssignmentsWithCourseInfo.
func (s *Store) CountAssignments(ctx context.Context, statusFilter, searchQuery string) (int, error) {
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

	var total int
	err := s.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*)
		FROM crm_cross_study_assignments a
		LEFT JOIN students s ON LOWER(s.wcode) = LOWER(BTRIM(a.wcode))
		WHERE %s
	`, where), args...).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("count assignments: %w", err)
	}
	return total, nil
}

// CountReviewNeeded returns assignments that need staff reconnect review.
func (s *Store) CountReviewNeeded(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM crm_cross_study_assignments
		WHERE deleted_at IS NULL
		  AND status IN ('notes_changed', 'orphaned')
	`).Scan(&count)
	return count, err
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
		SELECT a.id, a.wcode,
		       COALESCE(cr.extra_note, '') AS current_note,
		       COALESCE(cr.course_name, '') AS current_course,
		       a.crm_course_name_snapshot,
		       a.extra_note_hash,
		       a.crm_row_hash_snapshot,
		       COALESCE(a.crm_xlsx_row_number_snapshot, 0)
		FROM crm_cross_study_assignments a
		LEFT JOIN LATERAL (
			SELECT cr.extra_note, cr.course_name
			FROM crm_rows cr
			WHERE cr.snapshot_id = $1
			  AND LOWER(cr.wcode) = LOWER(BTRIM(a.wcode))
			ORDER BY
			  CASE
			    WHEN cr.row_hash = a.crm_row_hash_snapshot THEN 0
			    WHEN a.crm_xlsx_row_number_snapshot IS NOT NULL
			      AND cr.xlsx_row_number = a.crm_xlsx_row_number_snapshot THEN 1
			    ELSE 2
			  END,
			  cr.xlsx_row_number ASC
			LIMIT 1
		) cr ON true
		WHERE a.deleted_at IS NULL
	`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AssignmentChange
	for rows.Next() {
		var ch AssignmentChange
		if err := rows.Scan(
			&ch.ID,
			&ch.WCode,
			&ch.CurrentNote,
			&ch.CurrentCourseName,
			&ch.StoredCourseName,
			&ch.StoredExtraNoteHash,
			&ch.StoredRowHash,
			&ch.StoredXLSXRowNumber,
		); err != nil {
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
