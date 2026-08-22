package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type CourseMergeGroupCourseLockRow struct {
	ID           pgtype.UUID
	MergeGroupID pgtype.UUID
}

type CourseMergeGroupRow struct {
	ID   pgtype.UUID
	Name string
}

type CourseMergeGroupConfigRow struct {
	ID          pgtype.UUID
	Name        string
	Level       pgtype.Int2
	CycleID     pgtype.Text
	CycleLabel  pgtype.Text
	SitInRuleID pgtype.UUID
	CourseCodes []string
	CourseNames []string
}

type CourseMergeGroupScopeForCourseRow struct {
	ID          pgtype.UUID
	Name        string
	SitInRuleID pgtype.UUID
	CourseIDs   []pgtype.UUID
}

func (q *Queries) CourseMergeGroupScopeForCourse(ctx context.Context, courseID pgtype.UUID) (CourseMergeGroupScopeForCourseRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT g.id, g.name, g.sit_in_rule_id, m.course_id
		FROM course_merge_groups g
		JOIN course_merge_group_members m ON m.group_id = g.id
		WHERE g.id = (
			SELECT member.group_id
			FROM course_merge_group_members member
			WHERE member.course_id = $1
		)
		ORDER BY m.position ASC
	`, courseID)
	if err != nil {
		return CourseMergeGroupScopeForCourseRow{}, err
	}
	defer rows.Close()

	var item CourseMergeGroupScopeForCourseRow
	for rows.Next() {
		var groupID pgtype.UUID
		var name string
		var sitInRuleID pgtype.UUID
		var memberCourseID pgtype.UUID
		if err := rows.Scan(&groupID, &name, &sitInRuleID, &memberCourseID); err != nil {
			return CourseMergeGroupScopeForCourseRow{}, err
		}
		if !item.ID.Valid {
			item.ID = groupID
			item.Name = name
			item.SitInRuleID = sitInRuleID
		}
		item.CourseIDs = append(item.CourseIDs, memberCourseID)
	}
	if err := rows.Err(); err != nil {
		return CourseMergeGroupScopeForCourseRow{}, err
	}
	if !item.ID.Valid {
		return CourseMergeGroupScopeForCourseRow{}, pgx.ErrNoRows
	}
	return item, nil
}

type CourseMergeGroupListRow struct {
	ID          pgtype.UUID
	Name        string
	MemberCount int64
	CourseCodes []string
}

type CourseMergeGroupMemberRow struct {
	ID                pgtype.UUID
	Code              string
	Name              string
	SubjectCode       string
	SubjectName       string
	Year              pgtype.Int2
	Hour              pgtype.Int4
	StudentCount      pgtype.Int4
	CourseType        pgtype.Text
	CycleID           pgtype.Text
	RootCourseGroupID pgtype.UUID
	LegacyCourseID    pgtype.Text
	LegacyArchived    bool
}

type CourseMergeGroupSessionRow struct {
	ID          pgtype.UUID
	SeriesID    pgtype.UUID
	CourseID    pgtype.UUID
	RoomID      pgtype.UUID
	TeacherID   pgtype.UUID
	StartAt     pgtype.Timestamptz
	EndAt       pgtype.Timestamptz
	Version     int32
	CourseCode  string
	CourseName  string
	TeacherName string
}

func (q *Queries) CourseMergeGroupLockCourses(ctx context.Context, ids []pgtype.UUID) ([]CourseMergeGroupCourseLockRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, m.group_id
		FROM courses c
		LEFT JOIN course_merge_group_members m ON m.course_id = c.id
		WHERE c.id = ANY($1::uuid[])
		ORDER BY c.id
		FOR UPDATE OF c
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]CourseMergeGroupCourseLockRow, 0, len(ids))
	for rows.Next() {
		var item CourseMergeGroupCourseLockRow
		if err := rows.Scan(&item.ID, &item.MergeGroupID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (q *Queries) CourseMergeGroupCreate(ctx context.Context, name string, actorID pgtype.UUID) (CourseMergeGroupRow, error) {
	var item CourseMergeGroupRow
	err := q.db.QueryRow(ctx, `
		INSERT INTO course_merge_groups (name, created_by)
		VALUES ($1, $2)
		RETURNING id, name
	`, name, actorID).Scan(&item.ID, &item.Name)
	return item, err
}

func (q *Queries) CourseMergeGroupAssignCourse(ctx context.Context, groupID, courseID pgtype.UUID, position int16) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO course_merge_group_members (group_id, course_id, position)
		VALUES ($1, $2, $3)
	`, groupID, courseID, position)
	return err
}

func (q *Queries) CourseMergeGroupGet(ctx context.Context, id pgtype.UUID) (CourseMergeGroupRow, error) {
	var item CourseMergeGroupRow
	err := q.db.QueryRow(ctx, `
		SELECT id, name
		FROM course_merge_groups
		WHERE id = $1
	`, id).Scan(&item.ID, &item.Name)
	return item, err
}

func (q *Queries) CourseMergeGroupGetForUpdate(ctx context.Context, id pgtype.UUID) (CourseMergeGroupRow, error) {
	var item CourseMergeGroupRow
	err := q.db.QueryRow(ctx, `
		SELECT id, name
		FROM course_merge_groups
		WHERE id = $1
		FOR UPDATE
	`, id).Scan(&item.ID, &item.Name)
	return item, err
}

func (q *Queries) CourseMergeGroupUpdateName(ctx context.Context, id pgtype.UUID, name string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE course_merge_groups
		SET name = $2, updated_at = now()
		WHERE id = $1
	`, id, name)
	return err
}

func (q *Queries) CourseMergeGroupDelete(ctx context.Context, id pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `
		DELETE FROM course_merge_groups
		WHERE id = $1
	`, id)
	return err
}

func (q *Queries) CourseMergeGroupList(ctx context.Context) ([]CourseMergeGroupListRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT g.id, g.name, COUNT(m.course_id),
		       COALESCE(array_agg(c.code ORDER BY m.position) FILTER (WHERE c.id IS NOT NULL), ARRAY[]::text[])
		FROM course_merge_groups g
		LEFT JOIN course_merge_group_members m ON m.group_id = g.id
		LEFT JOIN courses c ON c.id = m.course_id
		GROUP BY g.id, g.name
		ORDER BY g.name ASC, g.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]CourseMergeGroupListRow, 0)
	for rows.Next() {
		var item CourseMergeGroupListRow
		if err := rows.Scan(&item.ID, &item.Name, &item.MemberCount, &item.CourseCodes); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (q *Queries) CourseMergeGroupMembers(ctx context.Context, id pgtype.UUID) ([]CourseMergeGroupMemberRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.code, c.name,
		       COALESCE(sub.code, ''), COALESCE(sub.name, ''),
		       c.year, c.hour, c.student_count, c.course_type, c.cycle_id,
		       c.root_course_group_id, c.legacy_course_id, c.legacy_archived
		FROM course_merge_group_members m
		JOIN courses c ON c.id = m.course_id
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		WHERE m.group_id = $1
		ORDER BY m.position ASC
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]CourseMergeGroupMemberRow, 0, 2)
	for rows.Next() {
		var item CourseMergeGroupMemberRow
		if err := rows.Scan(
			&item.ID, &item.Code, &item.Name,
			&item.SubjectCode, &item.SubjectName,
			&item.Year, &item.Hour, &item.StudentCount, &item.CourseType, &item.CycleID,
			&item.RootCourseGroupID, &item.LegacyCourseID, &item.LegacyArchived,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (q *Queries) CourseMergeGroupSessions(ctx context.Context, id pgtype.UUID) ([]CourseMergeGroupSessionRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT s.id, s.series_id, s.course_id, s.room_id, s.teacher_id,
		       s.start_at, s.end_at, s.version,
		       c.code, c.name,
		       COALESCE(NULLIF(u.full_name, ''), u.username, '')
		FROM course_merge_group_members m
		JOIN sessions s ON s.course_id = m.course_id
		JOIN courses c ON c.id = s.course_id
		JOIN users u ON u.id = s.teacher_id
		WHERE m.group_id = $1
		  AND s.deleted_at IS NULL
		ORDER BY s.start_at ASC, s.id ASC
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]CourseMergeGroupSessionRow, 0)
	for rows.Next() {
		var item CourseMergeGroupSessionRow
		if err := rows.Scan(
			&item.ID, &item.SeriesID, &item.CourseID, &item.RoomID, &item.TeacherID,
			&item.StartAt, &item.EndAt, &item.Version,
			&item.CourseCode, &item.CourseName, &item.TeacherName,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (q *Queries) CourseMergeGroupConfigsList(ctx context.Context) ([]CourseMergeGroupConfigRow, error) {
	rows, err := q.db.Query(ctx, `
		WITH group_members AS (
			SELECT g.id, g.name, g.level, g.sit_in_rule_id,
			       m.position, c.code, c.name AS course_name, c.cycle_id AS member_cycle_id
			FROM course_merge_groups g
			JOIN course_merge_group_members m ON m.group_id = g.id
			JOIN courses c ON c.id = m.course_id
		), group_data AS (
			SELECT id, name, level, sit_in_rule_id,
			       (array_agg(member_cycle_id ORDER BY position))[1] AS effective_cycle_id,
			       array_agg(code ORDER BY position) AS course_codes,
			       array_agg(course_name ORDER BY position) AS course_names
			FROM group_members
			GROUP BY id, name, level, sit_in_rule_id
			HAVING COUNT(*) = 2
		)
		SELECT d.id, d.name, d.level, d.effective_cycle_id, cy.label, d.sit_in_rule_id,
		       d.course_codes, d.course_names
		FROM group_data d
		LEFT JOIN crm_cycles cy ON cy.id = d.effective_cycle_id
		ORDER BY d.name ASC, d.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]CourseMergeGroupConfigRow, 0)
	for rows.Next() {
		var item CourseMergeGroupConfigRow
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Level, &item.CycleID, &item.CycleLabel,
			&item.SitInRuleID, &item.CourseCodes, &item.CourseNames,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (q *Queries) CourseMergeGroupLevelUpdate(ctx context.Context, id pgtype.UUID, level pgtype.Int2) error {
	_, err := q.db.Exec(ctx, `
		UPDATE course_merge_groups
		SET level = $2, updated_at = now()
		WHERE id = $1
	`, id, level)
	return err
}

func (q *Queries) CourseMergeGroupSitInRuleUpdate(ctx context.Context, id pgtype.UUID, sitInRuleID pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `
		UPDATE course_merge_groups
		SET sit_in_rule_id = $2, updated_at = now()
		WHERE id = $1
	`, id, sitInRuleID)
	return err
}

func (q *Queries) CoursesByMergeGroup(ctx context.Context, mergeGroupID pgtype.UUID) ([]SubjectCourseV2, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.code, c.name, c.subject_id, COALESCE(sub.code, ''), COALESCE(sub.name, ''),
		       c.cycle_id, COALESCE(g.level, c.level), c.root_course_group_id,
		       COALESCE(g.sit_in_rule_id, rcg.sit_in_rule_id), m.group_id
		FROM course_merge_group_members m
		JOIN courses c ON c.id = m.course_id
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		JOIN course_merge_groups g ON g.id = m.group_id
		WHERE m.group_id = $1
		  AND COALESCE(g.level, c.level) IS NOT NULL
		ORDER BY COALESCE(g.level, c.level) ASC, m.position ASC
	`, mergeGroupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SubjectCourseV2, 0)
	for rows.Next() {
		var r SubjectCourseV2
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.CycleID, &r.Level, &r.RootCourseGroupID, &r.SitInRuleID, &r.MergeGroupID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
