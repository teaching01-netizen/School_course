package db

import (
	"context"
	"slices"
	"strings"

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
	// MergeGroupName is set when this course is one of the two source courses
	// of a merged view (course_merge_groups); admins know merged classes by
	// that name, so the operations console surfaces it.
	MergeGroupName pgtype.Text
	RuleCourseID   pgtype.UUID
	RuleCourseCode pgtype.Text
}

func (q *Queries) ActiveCoursesList(ctx context.Context) ([]ActiveCourseSubjectRow, [][]ActiveCourseCourseRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT s.id, s.code, s.name,
		       c.id, c.code, c.name,
		       c.cycle_id, COALESCE(cy.label, ''),
		       COALESCE(cfg.absence_form_active, false),
		       COALESCE(cfg.absence_form_active, false),
		       mg.name, cfg.rule_course_id, rc.code
		FROM subjects s
		LEFT JOIN courses c ON c.subject_id = s.id
		LEFT JOIN crm_cycles cy ON cy.id = c.cycle_id
		LEFT JOIN course_rule_configs cfg ON cfg.course_id = c.id
		LEFT JOIN courses rc ON rc.id = cfg.rule_course_id
		LEFT JOIN course_merge_group_members mm ON mm.course_id = c.id
		LEFT JOIN course_merge_groups mg ON mg.id = mm.group_id
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
		mergeGroupName     pgtype.Text
		ruleCourseID       pgtype.UUID
		ruleCourseCode     pgtype.Text
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
			&r.mergeGroupName, &r.ruleCourseID, &r.ruleCourseCode,
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
			MergeGroupName:     r.mergeGroupName,
			RuleCourseID:       r.ruleCourseID,
			RuleCourseCode:     r.ruleCourseCode,
		})
	}

	return subjects, coursesBySubject, nil
}

// ActiveCourseStatus values understood by ActiveCoursesListPaginated and
// mirrored by the operations UI filter chips. The empty string behaves as
// StatusAll so older callers get the unfiltered list.
const (
	ActiveCourseStatusAll           = "all"
	ActiveCourseStatusConfigured    = "configured"
	ActiveCourseStatusHiddenActive  = "hidden_active"
	ActiveCourseStatusMissingActive = "missing_active"
)

type ActiveCoursesListParams struct {
	Limit  int
	Offset int
	// Search matches subject code/name, any member course's code/name, or the
	// name of a merged view containing one of the subject's courses
	// (case-insensitive substring).
	Search string
	// Status filters subjects by active-course state; see the constants above.
	Status          string
	SessionDateFrom string
	SessionDateTo   string
	InstituteTZ     string
}

// The per-subject active-course state is computed the same way for the count
// and the page queries, so the join and predicate live here once. A subject
// may have several active courses; the aggregate keeps the join at one row per
// subject regardless.
const activeCourseStateJoin = `
	LEFT JOIN LATERAL (
		SELECT count(*) AS active_count
		FROM courses c
		JOIN course_rule_configs cfg ON cfg.course_id = c.id
		WHERE c.subject_id = s.id AND cfg.absence_form_active
	) act ON true`

// Placeholders are anchored at $1/$2 so the same fragment serves both the
// count query (args: search, status) and the page query, where LIMIT/OFFSET
// take $3/$4. Search reaches beyond subjects: admins also look a class up by
// its course code/name or by the merged-view name it belongs to.
const activeCourseFilterWhere = `
	WHERE (
	    $1 = ''
	    OR s.code ILIKE '%' || $1 || '%'
	    OR s.name ILIKE '%' || $1 || '%'
	    OR EXISTS (
	        SELECT 1
	        FROM courses mc
	        LEFT JOIN course_merge_group_members mm ON mm.course_id = mc.id
	        LEFT JOIN course_merge_groups mg ON mg.id = mm.group_id
	        WHERE mc.subject_id = s.id
	          AND (
	            mc.code ILIKE '%' || $1 || '%'
	            OR mc.name ILIKE '%' || $1 || '%'
	            OR mg.name ILIKE '%' || $1 || '%'
	          )
	    )
	  )
	  AND (
	    $2 = '' OR $2 = 'all'
	    OR ($2 = 'configured' AND act.active_count > 0)
	    OR ($2 = 'missing_active' AND act.active_count = 0)
	  )`

const activeCourseSessionFilterForSubject = `
	AND (
		($3 = '' AND $4 = '')
		OR EXISTS (
			SELECT 1
			FROM courses time_course
			JOIN sessions time_session ON time_session.course_id = time_course.id
			WHERE time_course.subject_id = s.id
			  AND time_session.deleted_at IS NULL
			  AND ($3 = '' OR (time_session.start_at AT TIME ZONE $5)::date >= $3::date)
			  AND ($4 = '' OR (time_session.start_at AT TIME ZONE $5)::date <= $4::date)
		)
	)`

type ActiveCoursesStatsRow struct {
	Total         int64
	MissingActive int64
	HiddenActive  int64
}

// ActiveCoursesStats returns the institute-wide audit numbers behind the
// operations filter chips: how many subjects exist, how many have no active
// course, and how many have at least one active course that students cannot
// book because it is hidden from the absence form.
func (q *Queries) ActiveCoursesStats(ctx context.Context) (ActiveCoursesStatsRow, error) {
	var row ActiveCoursesStatsRow
	err := q.db.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE act.active_count = 0),
		       0::bigint
		FROM subjects s`+activeCourseStateJoin).Scan(
		&row.Total, &row.MissingActive, &row.HiddenActive)
	return row, err
}

func (q *Queries) ActiveCoursesListPaginated(ctx context.Context, p ActiveCoursesListParams) ([]ActiveCourseSubjectRow, [][]ActiveCourseCourseRow, int64, int64, error) {
	var totalSubjects, totalCourses int64
	instituteTZ := strings.TrimSpace(p.InstituteTZ)
	if instituteTZ == "" {
		instituteTZ = "UTC"
	}
	if err := q.db.QueryRow(ctx, `
		SELECT count(*) FROM subjects s`+activeCourseStateJoin+activeCourseFilterWhere+activeCourseSessionFilterForSubject,
		p.Search, p.Status, p.SessionDateFrom, p.SessionDateTo, instituteTZ).Scan(&totalSubjects); err != nil {
		return nil, nil, 0, 0, err
	}
	if err := q.db.QueryRow(ctx, `SELECT count(*) FROM courses c JOIN subjects s ON s.id = c.subject_id`).Scan(&totalCourses); err != nil {
		return nil, nil, 0, 0, err
	}

	rows, err := q.db.Query(ctx, `
		WITH filtered_subjects AS (
			SELECT s.id, s.code, s.name
			FROM subjects s`+activeCourseStateJoin+activeCourseFilterWhere+activeCourseSessionFilterForSubject+`
		),
		paged_subjects AS (
			SELECT id, code, name
			FROM filtered_subjects
			ORDER BY code ASC, id ASC
			LIMIT $6 OFFSET $7
		)
		SELECT ps.id, ps.code, ps.name,
		       c.id, c.code, c.name,
		       c.cycle_id, COALESCE(cy.label, ''),
		       COALESCE(cfg.absence_form_active, false),
		       COALESCE(cfg.absence_form_active, false),
		       c.merge_group_name, cfg.rule_course_id, rc.code
		FROM paged_subjects ps
		LEFT JOIN LATERAL (
			SELECT c.id, c.code, c.name, c.cycle_id, mg.name AS merge_group_name
			FROM courses c
			LEFT JOIN course_merge_group_members mm ON mm.course_id = c.id
			LEFT JOIN course_merge_groups mg ON mg.id = mm.group_id
			WHERE c.subject_id = ps.id
			  AND (
				($3 = '' AND $4 = '')
				OR EXISTS (
					SELECT 1
					FROM sessions time_session
					WHERE time_session.course_id = c.id
					  AND time_session.deleted_at IS NULL
					  AND ($3 = '' OR (time_session.start_at AT TIME ZONE $5)::time >= $3::time)
					  AND ($4 = '' OR (time_session.end_at AT TIME ZONE $5)::time <= $4::time)
				)
			  )
			ORDER BY c.code ASC NULLS LAST, c.id ASC
		) c ON true
		LEFT JOIN crm_cycles cy ON cy.id = c.cycle_id
		LEFT JOIN course_rule_configs cfg ON cfg.course_id = c.id
		LEFT JOIN courses rc ON rc.id = cfg.rule_course_id
		ORDER BY ps.code ASC, ps.id ASC, c.code ASC NULLS LAST, c.id ASC
	`, p.Search, p.Status, p.SessionDateFrom, p.SessionDateTo, instituteTZ, p.Limit, p.Offset)
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
		var mergeGroupName, ruleCourseCode pgtype.Text
		var ruleCourseID pgtype.UUID
		if err := rows.Scan(&subjectID, &subjectCode, &subjectName, &courseID, &courseCode, &courseName, &cycleID, &cycleLabel, &isActive, &absenceFormVisible, &mergeGroupName, &ruleCourseID, &ruleCourseCode); err != nil {
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
			AbsenceFormVisible: absenceFormVisible, MergeGroupName: mergeGroupName,
			RuleCourseID:   ruleCourseID,
			RuleCourseCode: ruleCourseCode,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, 0, 0, err
	}
	return subjects, coursesBySubject, totalSubjects, totalCourses, nil
}

// LockCourseBookingWrites serializes admin commands that change booking state,
// course ownership, or continuation membership before any course row locks.
func (q *Queries) LockCourseBookingWrites(ctx context.Context) error {
	return q.AdvisoryLockForText(ctx, "courses:student-booking")
}

func (q *Queries) LockSubjectCoursesForDeletion(ctx context.Context, subjectID pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `SELECT id FROM courses WHERE subject_id = $1 ORDER BY id FOR UPDATE`, subjectID)
	return err
}

type CourseBookingTarget struct {
	CourseID        pgtype.UUID
	SubjectID       pgtype.UUID
	SourceID        pgtype.UUID
	SourceSubjectID pgtype.UUID
	SourceEnabled   bool
}

func (q *Queries) CourseBookingTargets(ctx context.Context, ids []pgtype.UUID) ([]CourseBookingTarget, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.subject_id, source.id, source.subject_id, source.student_booking_enabled
		FROM course_rule_configs cfg
		JOIN courses c ON c.id = cfg.course_id
		JOIN courses source ON source.id = cfg.rule_course_id
		WHERE c.id = ANY($1::uuid[])
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []CourseBookingTarget
	for rows.Next() {
		var target CourseBookingTarget
		if err := rows.Scan(&target.CourseID, &target.SubjectID, &target.SourceID, &target.SourceSubjectID, &target.SourceEnabled); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

func (q *Queries) EnabledBookingCoursesBySubject(ctx context.Context, subjectID pgtype.UUID) ([]pgtype.UUID, error) {
	rows, err := q.db.Query(ctx, `SELECT id FROM courses WHERE subject_id = $1 AND student_booking_enabled ORDER BY id`, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []pgtype.UUID
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (q *Queries) SetCourseBookingEnabled(ctx context.Context, ids []pgtype.UUID, enabled bool) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := q.db.Exec(ctx, `
		UPDATE courses SET student_booking_enabled = $2, updated_at = now()
		WHERE id = ANY($1::uuid[]) AND student_booking_enabled <> $2
	`, ids, enabled)
	return err
}

func (q *Queries) CourseBookingAffectedIDs(ctx context.Context, sources []pgtype.UUID) ([]string, error) {
	rows, err := q.db.Query(ctx, `
		SELECT course_id::text FROM course_rule_configs
		WHERE rule_course_id = ANY($1::uuid[])
	`, sources)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, rows.Err()
}

func (q *Queries) BookingSessionCourseIDs(ctx context.Context, sessionIDs []pgtype.UUID) ([]pgtype.UUID, error) {
	var courses []pgtype.UUID
	err := q.db.QueryRow(ctx, `SELECT array_agg(DISTINCT course_id) FROM sessions WHERE id = ANY($1::uuid[])`, sessionIDs).Scan(&courses)
	return courses, err
}

// CourseIDsVisible returns the subset of ids that are currently visible in
// the student absence form — the sit-in gate: inactive classes may never be
// chosen as a sit-in target.
func (q *Queries) CourseIDsVisible(ctx context.Context, courseIDs []string) (map[string]struct{}, error) {
	rows, err := q.db.Query(ctx, bundleVisibleSelectSQL(), courseIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]struct{}, len(courseIDs))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

func (q *Queries) ActiveCoursesListByStudent(ctx context.Context, studentID pgtype.UUID) ([]ActiveCourseSubjectRow, [][]ActiveCourseCourseRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT s.id, s.code, s.name,
		       c.id, c.code, c.name,
		       c.cycle_id, COALESCE(cy.label, ''),
		       cfg.absence_form_active, cfg.absence_form_active
		FROM subjects s
		JOIN courses c ON c.subject_id = s.id
		JOIN course_students cs ON cs.course_id = c.id
		LEFT JOIN crm_cycles cy ON cy.id = c.cycle_id
		JOIN course_rule_configs cfg ON cfg.course_id = c.id
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
