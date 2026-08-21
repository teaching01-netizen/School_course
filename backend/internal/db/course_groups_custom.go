package db

import (
	"context"

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
