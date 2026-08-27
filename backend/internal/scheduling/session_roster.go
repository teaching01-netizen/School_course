package scheduling

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func effectiveStudentIDsForCourseTime(ctx context.Context, q *sqldb.Queries, sessionID, courseID pgtype.UUID, startAt pgtype.Timestamptz, filterOverridesNotInCourse bool) ([]pgtype.UUID, error) {
	rows, err := q.DBTX().Query(ctx, `
		SELECT candidates.student_id
		FROM (
			SELECT cs.student_id
			FROM course_students cs
			WHERE cs.course_id = $2
			UNION
			SELECT sa.student_id
			FROM session_attendance sa
			WHERE sa.session_id = $1
			  AND sa.status = 'included'
			  AND (
				NOT $4::boolean
				OR EXISTS (
					SELECT 1
					FROM course_students cs
					WHERE cs.course_id = $2
					  AND cs.student_id = sa.student_id
				)
			  )
			UNION
			SELECT st.id
			FROM students st
			JOIN crm_cross_study_assignments a
			  ON lower(a.wcode) = lower(st.wcode)
			 AND a.deleted_at IS NULL
			WHERE (
				$2 = a.dest_course_a_id
				OR $2 = a.dest_course_b_id
				OR EXISTS (
					SELECT 1
					FROM course_merge_group_members selected_member
					JOIN course_merge_group_members related_member
					  ON related_member.group_id = selected_member.group_id
					WHERE selected_member.course_id IN (a.dest_course_a_id, a.dest_course_b_id)
					  AND related_member.course_id = $2
				)
			)
		) candidates
		WHERE student_is_expected_at_course_time(candidates.student_id, $2, $3, $1)
		ORDER BY candidates.student_id
	`, sessionID, courseID, startAt, filterOverridesNotInCourse)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	studentIDs := make([]pgtype.UUID, 0)
	for rows.Next() {
		var studentID pgtype.UUID
		if err := rows.Scan(&studentID); err != nil {
			return nil, err
		}
		studentIDs = append(studentIDs, studentID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return studentIDs, nil
}

func effectiveStudentIDsForSession(ctx context.Context, q *sqldb.Queries, sessionID, courseID pgtype.UUID, filterOverridesNotInCourse bool) (*[]pgtype.UUID, bool, error) {
	session, err := q.SessionGetByID(ctx, sessionID)
	if err != nil {
		return nil, false, err
	}
	studentIDs, err := effectiveStudentIDsForCourseTime(ctx, q, sessionID, courseID, session.StartAt, filterOverridesNotInCourse)
	if err != nil {
		return nil, false, err
	}
	return &studentIDs, true, nil
}
