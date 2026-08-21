package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type StudentSubjectRow struct {
	StudentID      pgtype.UUID `json:"student_id"`
	Wcode          string      `json:"wcode"`
	FullName       string      `json:"full_name"`
	StudentPhone   pgtype.Text `json:"student_phone"`
	ParentPhone    pgtype.Text `json:"parent_phone"`
	Email          pgtype.Text `json:"email"` // resolved = COALESCE(email_crm, email_system)
	EmailCRM       pgtype.Text `json:"email_crm"`
	EmailSystem    pgtype.Text `json:"email_system"`
	Nickname       pgtype.Text `json:"nickname"`
	School         pgtype.Text `json:"school"`
	SubjectID      pgtype.UUID `json:"subject_id"`
	SubjectCode    string      `json:"subject_code"`
	SubjectName    string      `json:"subject_name"`
	ActiveCourseID pgtype.UUID `json:"active_course_id"`
}

// active_course_id is the admin-configured course from subject_active_courses.
// It is NULL when no active course has been set for the subject; it is never
// derived from enrollment (MIN(uuid) would return an arbitrary course).
func (q *Queries) StudentSubjectByWCode(ctx context.Context, wcode string) ([]StudentSubjectRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT s.id, s.wcode, s.full_name, s.student_phone, s.parent_phone,
		       COALESCE(s.email_crm, s.email_system) AS email,
		       s.email_crm, s.email_system, s.nickname, s.school,
		       sub.id, sub.code, sub.name,
		       MAX(sac.course_id::text)::uuid AS active_course_id
		FROM students s
		JOIN course_students cs ON cs.student_id = s.id AND cs.status = 'enrolled'
		JOIN courses c ON c.id = cs.course_id
		JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN subject_active_courses sac ON sac.subject_id = sub.id
		WHERE s.wcode = $1
		GROUP BY s.id, s.wcode, s.full_name, s.email_crm, s.email_system, s.school, sub.id, sub.code, sub.name
		ORDER BY sub.code ASC
	`, wcode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StudentSubjectRow
	for rows.Next() {
		var r StudentSubjectRow
		if err := rows.Scan(&r.StudentID, &r.Wcode, &r.FullName, &r.StudentPhone, &r.ParentPhone, &r.Email, &r.EmailCRM, &r.EmailSystem, &r.Nickname, &r.School, &r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.ActiveCourseID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type StudentEnrolledCourse struct {
	CourseID    pgtype.UUID `json:"course_id"`
	CourseCode  string      `json:"course_code"`
	CourseName  string      `json:"course_name"`
	SubjectID   pgtype.UUID `json:"subject_id"`
	CourseLevel pgtype.Text `json:"course_level"`
	LevelOrder  pgtype.Int2 `json:"level_order"`
}

func (q *Queries) StudentEnrolledCoursesBySubject(ctx context.Context, studentID pgtype.UUID, subjectID pgtype.UUID) ([]StudentEnrolledCourse, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.code, c.name, c.subject_id, c.course_level, c.level_order
		FROM course_students cs
		JOIN courses c ON c.id = cs.course_id
		WHERE cs.student_id = $1 AND c.subject_id = $2 AND cs.status = 'enrolled'
		ORDER BY c.level_order ASC NULLS LAST
	`, studentID, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StudentEnrolledCourse
	for rows.Next() {
		var r StudentEnrolledCourse
		if err := rows.Scan(&r.CourseID, &r.CourseCode, &r.CourseName, &r.SubjectID, &r.CourseLevel, &r.LevelOrder); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type SubjectCourse struct {
	ID          pgtype.UUID `json:"id"`
	Code        string      `json:"code"`
	Name        string      `json:"name"`
	SubjectID   pgtype.UUID `json:"subject_id"`
	CourseLevel pgtype.Text `json:"course_level"`
	LevelOrder  pgtype.Int2 `json:"level_order"`
}

func (q *Queries) SubjectCoursesBySubject(ctx context.Context, subjectID pgtype.UUID) ([]SubjectCourse, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, code, name, subject_id, course_level, level_order
		FROM courses
		WHERE subject_id = $1 AND course_level IS NOT NULL
		ORDER BY level_order ASC NULLS LAST
	`, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SubjectCourse
	for rows.Next() {
		var r SubjectCourse
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.SubjectID, &r.CourseLevel, &r.LevelOrder); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type SessionInRange struct {
	ID       pgtype.UUID        `json:"id"`
	CourseID pgtype.UUID        `json:"course_id"`
	RoomID   pgtype.UUID        `json:"room_id"`
	StartAt  pgtype.Timestamptz `json:"start_at"`
	EndAt    pgtype.Timestamptz `json:"end_at"`
}

func (q *Queries) SessionsByCourseInRange(ctx context.Context, courseID pgtype.UUID, dateFrom time.Time, dateTo time.Time) ([]SessionInRange, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, course_id, room_id, start_at, end_at
		FROM sessions
		WHERE course_id = $1
		  AND deleted_at IS NULL
		  AND start_at >= $2::timestamptz
		  AND start_at < ($3::timestamptz + interval '1 day')
		ORDER BY start_at ASC
	`, courseID, dateFrom, dateTo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionInRange
	for rows.Next() {
		var r SessionInRange
		if err := rows.Scan(&r.ID, &r.CourseID, &r.RoomID, &r.StartAt, &r.EndAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queries) SessionsByCourse(ctx context.Context, courseID pgtype.UUID) ([]SessionInRange, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, course_id, room_id, start_at, end_at
		FROM sessions
		WHERE course_id = $1
		  AND deleted_at IS NULL
		ORDER BY start_at ASC
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionInRange
	for rows.Next() {
		var r SessionInRange
		if err := rows.Scan(&r.ID, &r.CourseID, &r.RoomID, &r.StartAt, &r.EndAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// AbsenceSitInsCreate creates sit-in assignments for an absence without snapshot data.
//
// Deprecated: Use AbsenceSitInsCreateWithSnapshot instead. This method does not
// populate snapshot columns, which violates the domain invariant that all new
// sit-in assignments must carry a session snapshot. This path is retained only
// for backward compatibility with integration tests and will be removed once all
// callers migrate.
func (q *Queries) AbsenceSitInsCreate(ctx context.Context, absenceID pgtype.UUID, sessionIDs []pgtype.UUID) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	_, err := q.db.Exec(ctx, `
		WITH inserted AS (
			INSERT INTO absence_sit_ins (absence_id, session_id, session_version_at_assignment, assigned_at)
			SELECT $1, sess.id, sess.version, now()
			FROM sessions sess
			WHERE sess.id = ANY($2::uuid[])
			ON CONFLICT DO NOTHING
			RETURNING absence_id, session_id
		)
		INSERT INTO absence_sit_in_assignment_events (absence_id, new_session_id, action, reason)
		SELECT absence_id, session_id, 'assigned', 'absence_submission'
		FROM inserted
	`, absenceID, sessionIDs)
	if err != nil {
		return err
	}
	return nil
}

// SitInSnapshotInput contains the data needed to create a single sit-in assignment
// with a session snapshot.
type SitInSnapshotInput struct {
	SessionID       pgtype.UUID
	ExpectedVersion *int32 // nil means no version check
}

// AbsenceSitInsCreateWithSnapshot creates sit-in assignments with attached session
// snapshots. For each session:
//  1. Loads the session with display labels (course, teacher, room)
//  2. Optionally verifies the session version matches expectedVersion
//  3. Inserts the absence_sit_ins row with snapshot metadata
//  4. Writes assignment audit events
//
// This is the snapshot-enforcing replacement for AbsenceSitInsCreate.
func (q *Queries) AbsenceSitInsCreateWithSnapshot(
	ctx context.Context,
	absenceID pgtype.UUID,
	inputs []SitInSnapshotInput,
	timezone string,
	snapshotFunc SnapshotBuilderFunc,
) error {
	if len(inputs) == 0 {
		return nil
	}
	capturedAt := time.Now().UTC()

	for _, input := range inputs {
		row, err := q.SessionGetByIDForSnapshot(ctx, input.SessionID)
		if err != nil {
			return fmt.Errorf("load session for snapshot: %w", err)
		}

		if input.ExpectedVersion != nil && row.Version != *input.ExpectedVersion {
			return &SessionVersionConflictError{
				SessionID:       input.SessionID.String(),
				ExpectedVersion: int(*input.ExpectedVersion),
				ActualVersion:   int(row.Version),
			}
		}

		var roomName *string
		if row.RoomName.Valid {
			roomName = &row.RoomName.String
		}

		snapshotJSON, schemaVersion, err := snapshotFunc(
			row.CourseCode, row.CourseName, row.TeacherName, roomName,
			input.SessionID, row.SeriesID, row.CourseID, row.RoomID, row.TeacherID,
			row.StartAt, row.EndAt, row.Version, capturedAt, timezone,
		)
		if err != nil {
			return fmt.Errorf("build snapshot for session %s: %w", input.SessionID.String(), err)
		}

		capturedAtPg := pgtype.Timestamptz{Time: capturedAt, Valid: true}
		_, err = q.db.Exec(ctx, `
			WITH inserted AS (
				INSERT INTO absence_sit_ins (
					absence_id, session_id, session_version_at_assignment,
					assigned_at, assignment_source,
					session_snapshot_at_assignment, snapshot_schema_version,
					snapshot_captured_at, snapshot_quality, snapshot_source
				)
				VALUES ($1, $2, $3, now(), 'absence_submission',
				        $4, $5, $6, 'exact', 'captured_at_assignment')
				ON CONFLICT DO NOTHING
				RETURNING absence_id, session_id
			)
			INSERT INTO absence_sit_in_assignment_events (absence_id, new_session_id, action, reason)
			SELECT absence_id, session_id, 'assigned', 'absence_submission'
			FROM inserted
		`,
			absenceID, input.SessionID, row.Version,
			string(snapshotJSON), schemaVersion, capturedAtPg,
		)
		if err != nil {
			return fmt.Errorf("insert sit-in assignment with snapshot: %w", err)
		}
	}
	return nil
}

// SnapshotBuilderFunc is the function signature for building a session snapshot.
type SnapshotBuilderFunc = func(courseCode, courseName, teacherName string, roomName *string, sessionID pgtype.UUID, seriesID pgtype.UUID, courseID pgtype.UUID, roomID pgtype.UUID, teacherID pgtype.UUID, startAt, endAt pgtype.Timestamptz, version int32, capturedAt time.Time, tz string) ([]byte, int16, error)

type CourseLevelRow struct {
	ID          pgtype.UUID `json:"id"`
	Code        string      `json:"code"`
	Name        string      `json:"name"`
	SubjectID   pgtype.UUID `json:"subject_id"`
	SubjectCode string      `json:"subject_code"`
	SubjectName string      `json:"subject_name"`
	CourseLevel pgtype.Text `json:"course_level"`
	LevelOrder  pgtype.Int2 `json:"level_order"`
}

func (q *Queries) CourseLevelsList(ctx context.Context) ([]CourseLevelRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.code, c.name, c.subject_id, COALESCE(sub.code, ''), COALESCE(sub.name, ''),
		       c.course_level, c.level_order
		FROM courses c
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		ORDER BY sub.code ASC NULLS LAST, c.level_order ASC NULLS LAST, c.code ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CourseLevelRow
	for rows.Next() {
		var r CourseLevelRow
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.CourseLevel, &r.LevelOrder); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queries) CourseLevelUpdate(ctx context.Context, courseID pgtype.UUID, courseLevel pgtype.Text, levelOrder pgtype.Int2) error {
	_, err := q.db.Exec(ctx, `
		UPDATE courses
		SET course_level = $2, level_order = $3, updated_at = now()
		WHERE id = $1
	`, courseID, courseLevel, levelOrder)
	return err
}

type CourseLevelRowV2 struct {
	ID                  pgtype.UUID `json:"id"`
	Code                string      `json:"code"`
	Name                string      `json:"name"`
	SubjectID           pgtype.UUID `json:"subject_id"`
	SubjectCode         string      `json:"subject_code"`
	SubjectName         string      `json:"subject_name"`
	CycleID             pgtype.Text `json:"cycle_id"`
	CycleLabel          pgtype.Text `json:"cycle_label"`
	Level               pgtype.Int2 `json:"level"`
	RootCourseGroupID   pgtype.UUID `json:"root_course_group_id"`
	RootCourseGroupName pgtype.Text `json:"root_course_group_name"`
}

func (q *Queries) CourseLevelsListV2(ctx context.Context) ([]CourseLevelRowV2, error) {
	rows, err := q.db.Query(ctx, `
	SELECT c.id, c.code, c.name, c.subject_id, COALESCE(sub.code, ''), COALESCE(sub.name, ''),
	       c.cycle_id, cy.label, c.level,
	       c.root_course_group_id, rcg.name
		FROM courses c
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN crm_cycles cy ON cy.id = c.cycle_id
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		WHERE c.subject_id IS NOT NULL
		ORDER BY sub.code ASC NULLS LAST, c.cycle_id ASC NULLS LAST, c.level ASC NULLS LAST, c.code ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CourseLevelRowV2
	for rows.Next() {
		var r CourseLevelRowV2
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.CycleID, &r.CycleLabel, &r.Level, &r.RootCourseGroupID, &r.RootCourseGroupName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queries) CourseLevelsListV2Paginated(ctx context.Context, limit, offset int) ([]CourseLevelRowV2, int64, int16, error) {
	var total int64
	var maxLevel pgtype.Int2
	if err := q.db.QueryRow(ctx, `
		SELECT count(*), max(level)
		FROM courses
		WHERE subject_id IS NOT NULL
	`).Scan(&total, &maxLevel); err != nil {
		return nil, 0, 0, err
	}

	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.code, c.name, c.subject_id, COALESCE(sub.code, ''), COALESCE(sub.name, ''),
		       c.cycle_id, cy.label, c.level,
		       c.root_course_group_id, rcg.name
		FROM courses c
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN crm_cycles cy ON cy.id = c.cycle_id
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		WHERE c.subject_id IS NOT NULL
		ORDER BY sub.code ASC NULLS LAST, c.cycle_id ASC NULLS LAST, c.level ASC NULLS LAST, c.code ASC, c.id ASC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	var out []CourseLevelRowV2
	for rows.Next() {
		var r CourseLevelRowV2
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.CycleID, &r.CycleLabel, &r.Level, &r.RootCourseGroupID, &r.RootCourseGroupName); err != nil {
			return nil, 0, 0, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}
	return out, total, maxLevel.Int16, nil
}

func (q *Queries) CourseLevelUpdateV2(ctx context.Context, courseID pgtype.UUID, cycleID pgtype.Text, level pgtype.Int2) error {
	_, err := q.db.Exec(ctx, `
		UPDATE courses
		SET cycle_id = $2, level = $3, updated_at = now()
		WHERE id = $1
	`, courseID, cycleID, level)
	return err
}

type StudentEnrolledCourseV2 struct {
	CourseID          pgtype.UUID `json:"course_id"`
	CourseCode        string      `json:"course_code"`
	CourseName        string      `json:"course_name"`
	SubjectID         pgtype.UUID `json:"subject_id"`
	CycleID           pgtype.Text `json:"cycle_id"`
	Level             pgtype.Int2 `json:"level"`
	RootCourseGroupID pgtype.UUID `json:"root_course_group_id"`
	SitInRuleID       pgtype.UUID `json:"sit_in_rule_id"`
}

func (q *Queries) StudentEnrolledCoursesBySubjectV2(ctx context.Context, studentID pgtype.UUID, subjectID pgtype.UUID) ([]StudentEnrolledCourseV2, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.code, c.name, c.subject_id, c.cycle_id, c.level, c.root_course_group_id, rcg.sit_in_rule_id
		FROM course_students cs
		JOIN courses c ON c.id = cs.course_id
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		WHERE cs.student_id = $1 AND c.subject_id = $2 AND cs.status = 'enrolled'
		ORDER BY c.level ASC NULLS LAST
	`, studentID, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StudentEnrolledCourseV2
	for rows.Next() {
		var r StudentEnrolledCourseV2
		if err := rows.Scan(&r.CourseID, &r.CourseCode, &r.CourseName, &r.SubjectID, &r.CycleID, &r.Level, &r.RootCourseGroupID, &r.SitInRuleID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queries) StudentEnrolledCoursesByRootCourseGroup(ctx context.Context, studentID pgtype.UUID, rootCourseGroupID pgtype.UUID) ([]StudentEnrolledCourseV2, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.code, c.name, c.subject_id, c.cycle_id, c.level, c.root_course_group_id, rcg.sit_in_rule_id
		FROM course_students cs
		JOIN courses c ON c.id = cs.course_id
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		WHERE cs.student_id = $1
		  AND c.root_course_group_id = $2
		  AND cs.status = 'enrolled'
		ORDER BY c.level ASC NULLS LAST
	`, studentID, rootCourseGroupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StudentEnrolledCourseV2
	for rows.Next() {
		var r StudentEnrolledCourseV2
		if err := rows.Scan(&r.CourseID, &r.CourseCode, &r.CourseName, &r.SubjectID, &r.CycleID, &r.Level, &r.RootCourseGroupID, &r.SitInRuleID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type SubjectCourseV2 struct {
	ID                pgtype.UUID `json:"id"`
	Code              string      `json:"code"`
	Name              string      `json:"name"`
	SubjectID         pgtype.UUID `json:"subject_id"`
	SubjectCode       string      `json:"subject_code"`
	SubjectName       string      `json:"subject_name"`
	CycleID           pgtype.Text `json:"cycle_id"`
	Level             pgtype.Int2 `json:"level"`
	RootCourseGroupID pgtype.UUID `json:"root_course_group_id"`
	SitInRuleID       pgtype.UUID `json:"sit_in_rule_id"`
}

func (q *Queries) CoursesBySubjectAndCycle(ctx context.Context, subjectID pgtype.UUID, cycleID pgtype.Text) ([]SubjectCourseV2, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.code, c.name, c.subject_id, COALESCE(sub.code, ''), COALESCE(sub.name, ''),
		       c.cycle_id, c.level, c.root_course_group_id, rcg.sit_in_rule_id
		FROM courses c
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		WHERE c.subject_id = $1 AND c.cycle_id = $2 AND c.level IS NOT NULL
		ORDER BY c.level ASC
	`, subjectID, cycleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SubjectCourseV2
	for rows.Next() {
		var r SubjectCourseV2
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.CycleID, &r.Level, &r.RootCourseGroupID, &r.SitInRuleID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queries) CoursesByRootCourseGroup(ctx context.Context, rootCourseGroupID pgtype.UUID) ([]SubjectCourseV2, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.code, c.name, c.subject_id, COALESCE(sub.code, ''), COALESCE(sub.name, ''),
		       c.cycle_id, c.level, c.root_course_group_id, rcg.sit_in_rule_id
		FROM courses c
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		WHERE c.root_course_group_id = $1
		  AND c.level IS NOT NULL
		ORDER BY c.level ASC
	`, rootCourseGroupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SubjectCourseV2
	for rows.Next() {
		var r SubjectCourseV2
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.CycleID, &r.Level, &r.RootCourseGroupID, &r.SitInRuleID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queries) CoursesByRootCourseGroupAndCycle(ctx context.Context, rootCourseGroupID pgtype.UUID, cycleID pgtype.Text) ([]SubjectCourseV2, error) {
	var rows pgx.Rows
	var err error
	if cycleID.Valid {
		rows, err = q.db.Query(ctx, `
			SELECT c.id, c.code, c.name, c.subject_id, COALESCE(sub.code, ''), COALESCE(sub.name, ''),
			       c.cycle_id, c.level, c.root_course_group_id, rcg.sit_in_rule_id
			FROM courses c
			LEFT JOIN subjects sub ON sub.id = c.subject_id
			LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
			WHERE c.root_course_group_id = $1
			  AND c.level IS NOT NULL
			  AND c.cycle_id = $2
			ORDER BY c.level ASC
		`, rootCourseGroupID, cycleID.String)
	} else {
		rows, err = q.db.Query(ctx, `
			SELECT c.id, c.code, c.name, c.subject_id, COALESCE(sub.code, ''), COALESCE(sub.name, ''),
			       c.cycle_id, c.level, c.root_course_group_id, rcg.sit_in_rule_id
			FROM courses c
			LEFT JOIN subjects sub ON sub.id = c.subject_id
			LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
			WHERE c.root_course_group_id = $1
			  AND c.level IS NOT NULL
			ORDER BY c.level ASC
		`, rootCourseGroupID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SubjectCourseV2
	for rows.Next() {
		var r SubjectCourseV2
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.CycleID, &r.Level, &r.RootCourseGroupID, &r.SitInRuleID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type SubjectAndCycleFromCourseRow struct {
	SubjectID pgtype.UUID `json:"subject_id"`
	CycleID   pgtype.Text `json:"cycle_id"`
}

func (q *Queries) SubjectAndCycleFromCourse(ctx context.Context, courseID pgtype.UUID) (SubjectAndCycleFromCourseRow, error) {
	row := q.db.QueryRow(ctx, `
		SELECT subject_id, cycle_id
		FROM courses
		WHERE id = $1
	`, courseID)
	var r SubjectAndCycleFromCourseRow
	err := row.Scan(&r.SubjectID, &r.CycleID)
	return r, err
}

func (q *Queries) CourseUpdateRootCourseGroup(ctx context.Context, courseID pgtype.UUID, rootCourseGroupID pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `
		UPDATE courses
		SET root_course_group_id = $2, updated_at = now()
		WHERE id = $1
	`, courseID, rootCourseGroupID)
	return err
}

func (q *Queries) RootCourseGroupGetByID(ctx context.Context, id pgtype.UUID) (string, pgtype.UUID, error) {
	var name string
	var sitInRuleID pgtype.UUID
	err := q.db.QueryRow(ctx, `
		SELECT name, sit_in_rule_id
		FROM root_course_groups
		WHERE id = $1
	`, id).Scan(&name, &sitInRuleID)
	return name, sitInRuleID, err
}

func (q *Queries) RootCourseGroupExists(ctx context.Context, id pgtype.UUID) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM root_course_groups WHERE id = $1)
	`, id).Scan(&exists)
	return exists, err
}

type RootCourseGroupRow struct {
	ID          pgtype.UUID
	Name        string
	CourseCount int32
	SitInRuleID pgtype.UUID
	CreatedAt   pgtype.Timestamptz
	UpdatedAt   pgtype.Timestamptz
}

func (q *Queries) RootCourseGroupsList(ctx context.Context) ([]RootCourseGroupRow, error) {
	rows, err := q.db.Query(ctx, `
	SELECT g.id, g.name, COUNT(c.id)::int4 AS course_count,
	       g.sit_in_rule_id, g.created_at, g.updated_at
	FROM root_course_groups g
	LEFT JOIN courses c ON c.root_course_group_id = g.id
	GROUP BY g.id, g.name, g.sit_in_rule_id, g.created_at, g.updated_at
	ORDER BY g.name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RootCourseGroupRow
	for rows.Next() {
		var r RootCourseGroupRow
		if err := rows.Scan(&r.ID, &r.Name, &r.CourseCount, &r.SitInRuleID, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queries) RootCourseGroupCreate(ctx context.Context, name string, sitInRuleID pgtype.UUID) (pgtype.UUID, string, pgtype.UUID, error) {
	var id pgtype.UUID
	var sid pgtype.UUID
	err := q.db.QueryRow(ctx, `
		INSERT INTO root_course_groups (name, sit_in_rule_id)
		VALUES ($1, NULLIF($2::uuid, '00000000-0000-0000-0000-000000000000'::uuid))
		RETURNING id, name, sit_in_rule_id
	`, name, sitInRuleID).Scan(&id, &name, &sid)
	return id, name, sid, err
}

func (q *Queries) RootCourseGroupUpdate(ctx context.Context, id pgtype.UUID, name string, sitInRuleID pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `
		UPDATE root_course_groups
		SET name = $2, sit_in_rule_id = NULLIF($3::uuid, '00000000-0000-0000-0000-000000000000'::uuid), updated_at = now()
		WHERE id = $1
	`, id, name, sitInRuleID)
	return err
}

func (q *Queries) RootCourseGroupDelete(ctx context.Context, id pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `
		DELETE FROM root_course_groups WHERE id = $1
	`, id)
	return err
}

func (q *Queries) StudentAbsenceCountForCourse(ctx context.Context, wcode string, courseID pgtype.UUID) (int32, error) {
	var count int32
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)::int4 FROM student_absences
		WHERE wcode = $1 AND course_id = $2 AND status NOT IN ('cancelled', 'special_approved')
		-- Status values must match absences.StatusCancelled / absences.StatusSpecialApproved
	`, wcode, courseID).Scan(&count)
	return count, err
}

type AbsenceDayCountsForCourseParams struct {
	Wcode               string
	CourseID            pgtype.UUID
	CandidateSessionIDs []pgtype.UUID
	DateFrom            pgtype.Date
	DateTo              pgtype.Date
	InstituteTZ         string
}

type AbsenceDayCounts struct {
	TotalCourseDays      int32
	UsedAbsenceDays      int32
	CandidateAbsenceDays int32
	ProjectedAbsenceDays int32
}

func (q *Queries) AbsenceDayCountsForCourse(ctx context.Context, arg AbsenceDayCountsForCourseParams) (AbsenceDayCounts, error) {
	timezone := strings.TrimSpace(arg.InstituteTZ)
	if timezone == "" {
		timezone = "Asia/Bangkok"
	}
	candidateSessionIDs := arg.CandidateSessionIDs
	if candidateSessionIDs == nil {
		candidateSessionIDs = []pgtype.UUID{}
	}

	var counts AbsenceDayCounts
	err := q.db.QueryRow(ctx, `
		WITH course_days AS (
			SELECT DISTINCT (s.start_at AT TIME ZONE $6)::date AS day
			FROM sessions s
			WHERE s.course_id = $2
			  AND s.deleted_at IS NULL
		), explicit_absence_days AS (
			SELECT DISTINCT (s.start_at AT TIME ZONE $6)::date AS day
			FROM student_absences sa
			JOIN absence_missed_sessions ams ON ams.absence_id = sa.id
			JOIN sessions s ON s.id = ams.session_id
			WHERE lower(sa.wcode) = lower($1)
			  AND sa.course_id = $2
			  AND s.course_id = $2
			  AND s.deleted_at IS NULL
			  AND sa.status NOT IN ('cancelled', 'special_approved')
		), legacy_absence_days AS (
			SELECT DISTINCT cd.day
			FROM student_absences sa
			JOIN course_days cd ON cd.day BETWEEN sa.date_from AND sa.date_to
			WHERE lower(sa.wcode) = lower($1)
			  AND sa.course_id = $2
			  AND sa.status NOT IN ('cancelled', 'special_approved')
			  AND NOT EXISTS (
				SELECT 1 FROM absence_missed_sessions ams WHERE ams.absence_id = sa.id
			  )
		), used_days AS (
			SELECT day FROM explicit_absence_days
			UNION
			SELECT day FROM legacy_absence_days
		), candidate_days AS (
			SELECT DISTINCT (s.start_at AT TIME ZONE $6)::date AS day
			FROM sessions s
			WHERE s.course_id = $2
			  AND s.deleted_at IS NULL
			  AND (
				(cardinality($3::uuid[]) > 0 AND s.id = ANY($3::uuid[]))
				OR
				(cardinality($3::uuid[]) = 0 AND (s.start_at AT TIME ZONE $6)::date BETWEEN $4 AND $5)
			  )
		), projected_days AS (
			SELECT day FROM used_days
			UNION
			SELECT day FROM candidate_days
		)
		SELECT
			(SELECT count(*) FROM course_days)::int4,
			(SELECT count(*) FROM used_days)::int4,
			(SELECT count(*) FROM candidate_days)::int4,
			(SELECT count(*) FROM projected_days)::int4
	`, arg.Wcode, arg.CourseID, candidateSessionIDs, arg.DateFrom, arg.DateTo, timezone).Scan(
		&counts.TotalCourseDays,
		&counts.UsedAbsenceDays,
		&counts.CandidateAbsenceDays,
		&counts.ProjectedAbsenceDays,
	)
	return counts, err
}

func (q *Queries) StudentSetSystemEmail(ctx context.Context, wcode string, email string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE students
		SET email_system = $2, updated_at = now()
		WHERE wcode = $1
	`, wcode, email)
	return err
}
