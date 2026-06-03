package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type ActiveCourseSubjectRow struct {
	SubjectID   pgtype.UUID
	SubjectCode string
	SubjectName string
}

type ActiveCourseCourseRow struct {
	CourseID   pgtype.UUID
	CourseCode string
	CourseName string
	CycleID    pgtype.Text
	CycleLabel string
	IsActive   bool
}

func (q *Queries) ActiveCoursesList(ctx context.Context) ([]ActiveCourseSubjectRow, [][]ActiveCourseCourseRow, error) {
	subjRows, err := q.db.Query(ctx, `
		SELECT id, code, name
		FROM subjects
		ORDER BY code ASC
	`)
	if err != nil {
		return nil, nil, err
	}
	defer subjRows.Close()

	var subjects []ActiveCourseSubjectRow
	for subjRows.Next() {
		var s ActiveCourseSubjectRow
		if err := subjRows.Scan(&s.SubjectID, &s.SubjectCode, &s.SubjectName); err != nil {
			return nil, nil, err
		}
		subjects = append(subjects, s)
	}
	if err := subjRows.Err(); err != nil {
		return nil, nil, err
	}

	coursesBySubject := make([][]ActiveCourseCourseRow, len(subjects))
	for i, subj := range subjects {
		cRows, err := q.db.Query(ctx, `
			SELECT c.id, c.code, c.name, c.cycle_id, COALESCE(cy.label, ''),
			       CASE WHEN sac.course_id IS NOT NULL THEN true ELSE false END AS is_active
			FROM courses c
			LEFT JOIN crm_cycles cy ON cy.id = c.cycle_id
			LEFT JOIN subject_active_courses sac ON sac.course_id = c.id AND sac.subject_id = $1
			WHERE c.subject_id = $1
			ORDER BY c.code ASC
		`, subj.SubjectID)
		if err != nil {
			return nil, nil, err
		}

		var courses []ActiveCourseCourseRow
		for cRows.Next() {
			var c ActiveCourseCourseRow
			if err := cRows.Scan(&c.CourseID, &c.CourseCode, &c.CourseName, &c.CycleID, &c.CycleLabel, &c.IsActive); err != nil {
				cRows.Close()
				return nil, nil, err
			}
			courses = append(courses, c)
		}
		cRows.Close()
		if err := cRows.Err(); err != nil {
			return nil, nil, err
		}

		coursesBySubject[i] = courses
	}

	return subjects, coursesBySubject, nil
}

type ActiveCourseUpsertParams struct {
	SubjectID pgtype.UUID
	CourseID  pgtype.UUID
}

func (q *Queries) ActiveCourseUpsert(ctx context.Context, p ActiveCourseUpsertParams) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO subject_active_courses (subject_id, course_id)
		VALUES ($1, $2)
		ON CONFLICT (subject_id) DO UPDATE
		SET course_id = $2, updated_at = now()
	`, p.SubjectID, p.CourseID)
	return err
}

func (q *Queries) ActiveCoursesListByStudent(ctx context.Context, studentID pgtype.UUID) ([]ActiveCourseSubjectRow, [][]ActiveCourseCourseRow, error) {
	subjRows, err := q.db.Query(ctx, `
		SELECT DISTINCT s.id, s.code, s.name
		FROM subjects s
		JOIN courses c ON c.subject_id = s.id
		JOIN course_students cs ON cs.course_id = c.id
		WHERE cs.student_id = $1
		ORDER BY s.code ASC
	`, studentID)
	if err != nil {
		return nil, nil, err
	}
	defer subjRows.Close()

	var subjects []ActiveCourseSubjectRow
	for subjRows.Next() {
		var s ActiveCourseSubjectRow
		if err := subjRows.Scan(&s.SubjectID, &s.SubjectCode, &s.SubjectName); err != nil {
			return nil, nil, err
		}
		subjects = append(subjects, s)
	}
	if err := subjRows.Err(); err != nil {
		return nil, nil, err
	}

	coursesBySubject := make([][]ActiveCourseCourseRow, len(subjects))
	for i, subj := range subjects {
		cRows, err := q.db.Query(ctx, `
			SELECT c.id, c.code, c.name, c.cycle_id, COALESCE(cy.label, ''),
			       CASE WHEN sac.course_id IS NOT NULL THEN true ELSE false END AS is_active
			FROM courses c
			LEFT JOIN crm_cycles cy ON cy.id = c.cycle_id
			LEFT JOIN subject_active_courses sac ON sac.course_id = c.id AND sac.subject_id = $1
			WHERE c.subject_id = $1
			ORDER BY c.code ASC
		`, subj.SubjectID)
		if err != nil {
			return nil, nil, err
		}

		var courses []ActiveCourseCourseRow
		for cRows.Next() {
			var c ActiveCourseCourseRow
			if err := cRows.Scan(&c.CourseID, &c.CourseCode, &c.CourseName, &c.CycleID, &c.CycleLabel, &c.IsActive); err != nil {
				cRows.Close()
				return nil, nil, err
			}
			courses = append(courses, c)
		}
		cRows.Close()
		if err := cRows.Err(); err != nil {
			return nil, nil, err
		}

		coursesBySubject[i] = courses
	}

	return subjects, coursesBySubject, nil
}
