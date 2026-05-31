package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type StudentSubjectRow struct {
	StudentID      pgtype.UUID `json:"student_id"`
	Wcode          string      `json:"wcode"`
	FullName       string      `json:"full_name"`
	ParentPhone    pgtype.Text `json:"parent_phone"`
	SubjectID      pgtype.UUID `json:"subject_id"`
	SubjectCode    string      `json:"subject_code"`
	SubjectName    string      `json:"subject_name"`
	ActiveCourseID pgtype.UUID `json:"active_course_id"`
}

func (q *Queries) StudentSubjectByWCode(ctx context.Context, wcode string) ([]StudentSubjectRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT s.id, s.wcode, s.full_name, s.parent_phone,
		       sub.id, sub.code, sub.name,
		       MIN(c.id::text)::uuid AS active_course_id
		FROM students s
		JOIN course_students cs ON cs.student_id = s.id AND cs.status = 'enrolled'
		JOIN courses c ON c.id = cs.course_id AND c.deleted_at IS NULL
		JOIN subjects sub ON sub.id = c.subject_id AND sub.deleted_at IS NULL
		WHERE s.wcode = $1
		GROUP BY s.id, s.wcode, s.full_name, sub.id, sub.code, sub.name
		ORDER BY sub.code ASC
	`, wcode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StudentSubjectRow
	for rows.Next() {
		var r StudentSubjectRow
		if err := rows.Scan(&r.StudentID, &r.Wcode, &r.FullName, &r.ParentPhone, &r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.ActiveCourseID); err != nil {
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
		JOIN courses c ON c.id = cs.course_id AND c.deleted_at IS NULL
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
		WHERE subject_id = $1 AND deleted_at IS NULL AND course_level IS NOT NULL
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

type AbsenceSitIn struct {
	ID        pgtype.UUID        `json:"id"`
	AbsenceID pgtype.UUID        `json:"absence_id"`
	SessionID pgtype.UUID        `json:"session_id"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

func (q *Queries) AbsenceSitInsCreate(ctx context.Context, absenceID pgtype.UUID, sessionIDs []pgtype.UUID) error {
	for _, sid := range sessionIDs {
		_, err := q.db.Exec(ctx, `
			INSERT INTO absence_sit_ins (absence_id, session_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, absenceID, sid)
		if err != nil {
			return err
		}
	}
	return nil
}

func (q *Queries) AbsenceSitInsByAbsence(ctx context.Context, absenceID pgtype.UUID) ([]AbsenceSitIn, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, absence_id, session_id, created_at
		FROM absence_sit_ins
		WHERE absence_id = $1
		ORDER BY created_at ASC
	`, absenceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AbsenceSitIn
	for rows.Next() {
		var r AbsenceSitIn
		if err := rows.Scan(&r.ID, &r.AbsenceID, &r.SessionID, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type AbsenceListExtendedRow struct {
	ID              pgtype.UUID        `json:"id"`
	Wcode           string             `json:"wcode"`
	CourseID        pgtype.UUID        `json:"course_id"`
	DateFrom        pgtype.Date        `json:"date_from"`
	DateTo          pgtype.Date        `json:"date_to"`
	Reason          pgtype.Text        `json:"reason"`
	SitInCourseID   pgtype.UUID        `json:"sit_in_course_id"`
	CreatedAt       pgtype.Timestamptz `json:"created_at"`
	CourseCode      string             `json:"course_code"`
	CourseName      string             `json:"course_name"`
	SitInCourseCode pgtype.Text        `json:"sit_in_course_code"`
	SitInCourseName pgtype.Text        `json:"sit_in_course_name"`
	SitInMethod     pgtype.Text        `json:"sit_in_method"`
	SubjectID       pgtype.UUID        `json:"subject_id"`
	SubjectCode     pgtype.Text        `json:"subject_code"`
	SubjectName     pgtype.Text        `json:"subject_name"`
}

func (q *Queries) AbsenceListExtended(ctx context.Context) ([]AbsenceListExtendedRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT sa.id, sa.wcode, sa.course_id, sa.date_from, sa.date_to, sa.reason, sa.sit_in_course_id, sa.created_at,
		       c.code AS course_code, c.name AS course_name,
		       sc.code AS sit_in_course_code, sc.name AS sit_in_course_name,
		       sa.sit_in_method,
		       sa.subject_id,
		       sub.code AS subject_code, sub.name AS subject_name
		FROM student_absences sa
		JOIN courses c ON c.id = sa.course_id
		LEFT JOIN courses sc ON sc.id = sa.sit_in_course_id
		LEFT JOIN subjects sub ON sub.id = sa.subject_id
		ORDER BY sa.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AbsenceListExtendedRow
	for rows.Next() {
		var r AbsenceListExtendedRow
		if err := rows.Scan(
			&r.ID, &r.Wcode, &r.CourseID, &r.DateFrom, &r.DateTo, &r.Reason, &r.SitInCourseID, &r.CreatedAt,
			&r.CourseCode, &r.CourseName,
			&r.SitInCourseCode, &r.SitInCourseName,
			&r.SitInMethod, &r.SubjectID, &r.SubjectCode, &r.SubjectName,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

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
		WHERE c.deleted_at IS NULL
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
		WHERE c.deleted_at IS NULL AND c.subject_id IS NOT NULL
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
		JOIN courses c ON c.id = cs.course_id AND c.deleted_at IS NULL
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
		JOIN courses c ON c.id = cs.course_id AND c.deleted_at IS NULL
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
		WHERE c.subject_id = $1 AND c.cycle_id = $2 AND c.deleted_at IS NULL AND c.level IS NOT NULL
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
		  AND c.deleted_at IS NULL
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
			  AND c.deleted_at IS NULL
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
			  AND c.deleted_at IS NULL
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
		WHERE id = $1 AND deleted_at IS NULL
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
	LEFT JOIN courses c ON c.root_course_group_id = g.id AND c.deleted_at IS NULL
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
