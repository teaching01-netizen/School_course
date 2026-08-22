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
	CourseID           pgtype.UUID
	CourseCode         string
	CourseName         string
	CycleID            pgtype.Text
	CycleLabel         string
	IsActive           bool
	AbsenceFormVisible bool
}

func (q *Queries) ActiveCoursesList(ctx context.Context) ([]ActiveCourseSubjectRow, [][]ActiveCourseCourseRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT s.id, s.code, s.name,
		       c.id, c.code, c.name,
		       c.cycle_id, COALESCE(cy.label, ''),
		       CASE WHEN sac.course_id IS NOT NULL THEN true ELSE false END,
		       COALESCE(c.absence_form_visible, true)
		FROM subjects s
		LEFT JOIN courses c ON c.subject_id = s.id
		LEFT JOIN crm_cycles cy ON cy.id = c.cycle_id
		LEFT JOIN subject_active_courses sac ON sac.course_id = c.id AND sac.subject_id = s.id
		ORDER BY s.code ASC, c.code ASC
	`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	type flatRow struct {
		subjectID          pgtype.UUID
		subjectCode        string
		subjectName        string
		courseID           pgtype.UUID
		courseCode         pgtype.Text
		courseName         pgtype.Text
		cycleID            pgtype.Text
		cycleLabel         string
		isActive           bool
		absenceFormVisible bool
	}

	var flat []flatRow
	for rows.Next() {
		var r flatRow
		if err := rows.Scan(
			&r.subjectID, &r.subjectCode, &r.subjectName,
			&r.courseID, &r.courseCode, &r.courseName,
			&r.cycleID, &r.cycleLabel,
			&r.isActive,
			&r.absenceFormVisible,
		); err != nil {
			return nil, nil, err
		}
		flat = append(flat, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var subjects []ActiveCourseSubjectRow
	var coursesBySubject [][]ActiveCourseCourseRow
	for _, r := range flat {
		if len(subjects) == 0 || subjects[len(subjects)-1].SubjectID.Bytes != r.subjectID.Bytes {
			subjects = append(subjects, ActiveCourseSubjectRow{
				SubjectID:   r.subjectID,
				SubjectCode: r.subjectCode,
				SubjectName: r.subjectName,
			})
			coursesBySubject = append(coursesBySubject, nil)
		}
		if !r.courseID.Valid {
			continue
		}
		idx := len(subjects) - 1
		coursesBySubject[idx] = append(coursesBySubject[idx], ActiveCourseCourseRow{
			CourseID:           r.courseID,
			CourseCode:         r.courseCode.String,
			CourseName:         r.courseName.String,
			CycleID:            r.cycleID,
			CycleLabel:         r.cycleLabel,
			IsActive:           r.isActive,
			AbsenceFormVisible: r.absenceFormVisible,
		})
	}

	return subjects, coursesBySubject, nil
}

func (q *Queries) ActiveCoursesListPaginated(ctx context.Context, limit, offset int) ([]ActiveCourseSubjectRow, [][]ActiveCourseCourseRow, int64, int64, error) {
	var totalSubjects, totalCourses int64
	if err := q.db.QueryRow(ctx, `SELECT count(*) FROM subjects`).Scan(&totalSubjects); err != nil {
		return nil, nil, 0, 0, err
	}
	if err := q.db.QueryRow(ctx, `SELECT count(*) FROM courses c JOIN subjects s ON s.id = c.subject_id`).Scan(&totalCourses); err != nil {
		return nil, nil, 0, 0, err
	}

	rows, err := q.db.Query(ctx, `
		WITH paged_subjects AS (
			SELECT s.id, s.code, s.name
			FROM subjects s
			ORDER BY s.code ASC, s.id ASC
			LIMIT $1 OFFSET $2
		)
		SELECT ps.id, ps.code, ps.name,
		       c.id, c.code, c.name,
		       c.cycle_id, COALESCE(cy.label, ''),
		       CASE WHEN sac.course_id IS NOT NULL THEN true ELSE false END,
		       COALESCE(c.absence_form_visible, true)
		FROM paged_subjects ps
		LEFT JOIN LATERAL (
			SELECT c.id, c.code, c.name, c.cycle_id, c.absence_form_visible
			FROM courses c
			WHERE c.subject_id = ps.id
			ORDER BY c.code ASC NULLS LAST, c.id ASC
		) c ON true
		LEFT JOIN crm_cycles cy ON cy.id = c.cycle_id
		LEFT JOIN subject_active_courses sac ON sac.course_id = c.id AND sac.subject_id = ps.id
		ORDER BY ps.code ASC, ps.id ASC, c.code ASC NULLS LAST, c.id ASC
	`, limit, offset)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	defer rows.Close()

	var subjects []ActiveCourseSubjectRow
	var coursesBySubject [][]ActiveCourseCourseRow
	for rows.Next() {
		var subjectID pgtype.UUID
		var subjectCode, subjectName string
		var courseID pgtype.UUID
		var courseCode, courseName pgtype.Text
		var cycleID pgtype.Text
		var cycleLabel string
		var isActive bool
		var absenceFormVisible bool
		if err := rows.Scan(&subjectID, &subjectCode, &subjectName, &courseID, &courseCode, &courseName, &cycleID, &cycleLabel, &isActive, &absenceFormVisible); err != nil {
			return nil, nil, 0, 0, err
		}

		if len(subjects) == 0 || subjects[len(subjects)-1].SubjectID.Bytes != subjectID.Bytes {
			subjects = append(subjects, ActiveCourseSubjectRow{SubjectID: subjectID, SubjectCode: subjectCode, SubjectName: subjectName})
			coursesBySubject = append(coursesBySubject, nil)
		}
		if !courseID.Valid {
			continue
		}
		idx := len(subjects) - 1
		coursesBySubject[idx] = append(coursesBySubject[idx], ActiveCourseCourseRow{
			CourseID: courseID, CourseCode: courseCode.String, CourseName: courseName.String,
			CycleID: cycleID, CycleLabel: cycleLabel, IsActive: isActive,
			AbsenceFormVisible: absenceFormVisible,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, 0, 0, err
	}
	return subjects, coursesBySubject, totalSubjects, totalCourses, nil
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
	rows, err := q.db.Query(ctx, `
		SELECT s.id, s.code, s.name,
		       c.id, c.code, c.name,
		       c.cycle_id, COALESCE(cy.label, ''),
		       CASE WHEN sac.course_id IS NOT NULL THEN true ELSE false END,
		       c.absence_form_visible
		FROM subjects s
		JOIN courses c ON c.subject_id = s.id
		JOIN course_students cs ON cs.course_id = c.id
		LEFT JOIN crm_cycles cy ON cy.id = c.cycle_id
		LEFT JOIN subject_active_courses sac ON sac.course_id = c.id AND sac.subject_id = s.id
		WHERE cs.student_id = $1
		ORDER BY s.code ASC, c.code ASC
	`, studentID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	type flatRow struct {
		subjectID          pgtype.UUID
		subjectCode        string
		subjectName        string
		courseID           pgtype.UUID
		courseCode         string
		courseName         string
		cycleID            pgtype.Text
		cycleLabel         string
		isActive           bool
		absenceFormVisible bool
	}

	var flat []flatRow
	for rows.Next() {
		var r flatRow
		if err := rows.Scan(
			&r.subjectID, &r.subjectCode, &r.subjectName,
			&r.courseID, &r.courseCode, &r.courseName,
			&r.cycleID, &r.cycleLabel,
			&r.isActive,
			&r.absenceFormVisible,
		); err != nil {
			return nil, nil, err
		}
		flat = append(flat, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var subjects []ActiveCourseSubjectRow
	var coursesBySubject [][]ActiveCourseCourseRow
	for _, r := range flat {
		if len(subjects) == 0 || subjects[len(subjects)-1].SubjectID.Bytes != r.subjectID.Bytes {
			subjects = append(subjects, ActiveCourseSubjectRow{
				SubjectID:   r.subjectID,
				SubjectCode: r.subjectCode,
				SubjectName: r.subjectName,
			})
			coursesBySubject = append(coursesBySubject, nil)
		}
		idx := len(subjects) - 1
		coursesBySubject[idx] = append(coursesBySubject[idx], ActiveCourseCourseRow{
			CourseID:           r.courseID,
			CourseCode:         r.courseCode,
			CourseName:         r.courseName,
			CycleID:            r.cycleID,
			CycleLabel:         r.cycleLabel,
			IsActive:           r.isActive,
			AbsenceFormVisible: r.absenceFormVisible,
		})
	}

	return subjects, coursesBySubject, nil
}
