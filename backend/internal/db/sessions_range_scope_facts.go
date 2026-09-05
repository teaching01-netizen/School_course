package db

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SessionsRangeScopeFacts extends SessionsRangeFacts with the per-request
// batched derivations that the old implementation fetched once per course:
//
//   - merge-group membership + names for every course in the fact set
//   - blocked (already-assigned) sit-in session IDs for the student
//   - absence-day counters for every absence scope in ONE query
//     (replaces N x AbsenceDayCountsForCourse/ForMergeGroup)
//
// Query-count contract: exactly two Query calls here (scopes one call, day
// counts one call) plus one blocked-sit-ins call. Conflict details ride on
// the blocked-sit-ins result, so no extra round trip is needed.
//
// NOTE: the handler still loads settings (1 QueryRow) via readAbsenceSettings
// and resolves the student row (1 QueryRow) for enrollment lookups, so the
// full endpoint stays at <= 5 round trips independent of course count.
type AbsenceScopeKey struct {
	// MergeGroup is true when the scope is a merge group, false for a course.
	MergeGroup bool
	// MergeGroupID is valid when MergeGroup is true.
	MergeGroupID pgtype.UUID
	// CourseID is valid when MergeGroup is false.
	CourseID pgtype.UUID
}

// ScopeKey returns the canonical key used to group day counts.
func (k AbsenceScopeKey) String() string {
	if k.MergeGroup {
		return "merge:" + uuidBytesString(k.MergeGroupID)
	}
	return "course:" + uuidBytesString(k.CourseID)
}

// SessionsRangeScopeFactsRow carries one absence scope touched by the fact
// set: its courses and its human-readable merge-group name ("" when none).
type SessionsRangeScopeFactsRow struct {
	Key            AbsenceScopeKey
	CourseIDs      []pgtype.UUID
	MergeGroupName string
}

// SessionsRangeScopeFacts resolves absence scopes for the given courses in
// ONE round trip: merge-group membership for all course IDs at once. Courses
// without a merge group each form their own course scope.
func (q *Queries) SessionsRangeScopeFacts(ctx context.Context, courseIDs []pgtype.UUID) ([]SessionsRangeScopeFactsRow, error) {
	if len(courseIDs) == 0 {
		return nil, nil
	}
	rows, err := q.db.Query(ctx, `
		SELECT m.course_id, g.id, g.name
		FROM course_merge_group_members m
		JOIN course_merge_groups g ON g.id = m.group_id
		WHERE m.course_id = ANY($1::uuid[])
	`, courseIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type membership struct {
		groupID   pgtype.UUID
		groupName string
	}
	byCourse := make(map[string]membership, len(courseIDs))
	for rows.Next() {
		var courseID, groupID pgtype.UUID
		var name string
		if err := rows.Scan(&courseID, &groupID, &name); err != nil {
			return nil, err
		}
		byCourse[uuidBytesString(courseID)] = membership{groupID: groupID, groupName: name}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	byScope := make(map[string]*SessionsRangeScopeFactsRow)
	order := make([]string, 0, len(courseIDs))
	for _, courseID := range courseIDs {
		key := "course:" + uuidBytesString(courseID)
		name := ""
		var scopeKey AbsenceScopeKey
		if m, ok := byCourse[uuidBytesString(courseID)]; ok {
			key = "merge:" + uuidBytesString(m.groupID)
			name = m.groupName
			scopeKey = AbsenceScopeKey{MergeGroup: true, MergeGroupID: m.groupID}
		} else {
			scopeKey = AbsenceScopeKey{CourseID: courseID}
		}
		row := byScope[key]
		if row == nil {
			row = &SessionsRangeScopeFactsRow{Key: scopeKey, MergeGroupName: name}
			byScope[key] = row
			order = append(order, key)
		}
		row.CourseIDs = append(row.CourseIDs, courseID)
	}
	out := make([]SessionsRangeScopeFactsRow, 0, len(order))
	for _, key := range order {
		out = append(out, *byScope[key])
	}
	return out, nil
}

// ScopeDayCounts maps scope key -> day counters.
type ScopeDayCounts map[string]AbsenceDayCounts

// SessionsRangeDayCountsParams binds the batched day-count query.
type SessionsRangeDayCountsParams struct {
	Wcode       string
	Scopes      []SessionsRangeScopeFactsRow
	InstituteTZ string
}

// SessionsRangeDayCounts computes total/used day counters for EVERY scope in
// ONE round trip. It unions course_days and used_days per scope and returns
// one row per scope. Complexity is O(scopes + relevant sessions + relevant
// absences), never O(courses x history).
//
// Cost statement: the all-history scan per scope is fundamental, not
// accidental — TotalCourseDays/UsedAbsenceDays are defined over all time,
// so no window bound can apply. What was accidental (seq scans from
// lower(wcode) predicates) is removed via lower(wcode) functional indexes
// (migration 00122). A cross-request cache would trade freshness for speed
// and is deliberately absent: counts must reflect concurrent submissions.
//
// used_days semantics mirror AbsenceDayCountsForCourse/ForMergeGroup exactly:
// explicit missed-session days UNION legacy date-range days, restricted to
// non-cancelled, non-special_approved absences with
// student_is_expected_at_session on the session side and merge-group
// equivalence on the absence side.
func (q *Queries) SessionsRangeDayCounts(ctx context.Context, arg SessionsRangeDayCountsParams) (ScopeDayCounts, error) {
	out := make(ScopeDayCounts, len(arg.Scopes))
	if len(arg.Scopes) == 0 {
		return out, nil
	}
	timezone := strings.TrimSpace(arg.InstituteTZ)
	if timezone == "" {
		timezone = "Asia/Bangkok"
	}
	mergeGroupIDs := make([]pgtype.UUID, 0)
	courseIDs := make([]pgtype.UUID, 0)
	for _, scope := range arg.Scopes {
		if scope.Key.MergeGroup {
			mergeGroupIDs = append(mergeGroupIDs, scope.Key.MergeGroupID)
		} else {
			courseIDs = append(courseIDs, scope.Key.CourseID)
		}
	}
	// One query, two grouped aggregations: per-course scopes and per-merge
	// scopes. A scope key prefix disambiguates the two namespaces.
	rows, err := q.db.Query(ctx, `
		WITH student_scope AS (
			SELECT id FROM students WHERE lower(wcode) = lower($1)
		), course_days AS (
			SELECT DISTINCT s.course_id AS course_id, NULL::uuid AS merge_group_id,
			       (s.start_at AT TIME ZONE $4)::date AS day
			FROM sessions s
			CROSS JOIN student_scope st
			WHERE s.deleted_at IS NULL
			  AND s.course_id = ANY($2::uuid[])
			  AND student_is_expected_at_session(st.id, s.id)
		), merge_days AS (
			SELECT DISTINCT NULL::uuid AS course_id, m.group_id AS merge_group_id,
			       (s.start_at AT TIME ZONE $4)::date AS day
			FROM sessions s
			JOIN course_merge_group_members m ON m.course_id = s.course_id
			CROSS JOIN student_scope st
			WHERE s.deleted_at IS NULL
			  AND m.group_id = ANY($3::uuid[])
			  AND student_is_expected_at_session(st.id, s.id)
		), scoped_absences AS (
			SELECT sa.id, sa.course_id, sa.merge_group_id,
			       sa.date_from, sa.date_to,
			       EXISTS (SELECT 1 FROM absence_missed_sessions ams WHERE ams.absence_id = sa.id) AS has_missed
			FROM student_absences sa
			WHERE lower(sa.wcode) = lower($1)
			  AND sa.status NOT IN ('cancelled', 'special_approved')
		), explicit_days AS (
			SELECT DISTINCT
				CASE WHEN mc.group_id IS NULL THEN s.course_id ELSE NULL END AS course_id,
				mc.group_id AS merge_group_id,
				(s.start_at AT TIME ZONE $4)::date AS day
			FROM scoped_absences sa
			JOIN absence_missed_sessions ams ON ams.absence_id = sa.id
			JOIN sessions s ON s.id = ams.session_id
			LEFT JOIN course_merge_group_members mc ON mc.course_id = s.course_id
			CROSS JOIN student_scope st
			WHERE s.deleted_at IS NULL
			  AND student_is_expected_at_session(st.id, s.id)
			  AND (
				(mc.group_id IS NOT NULL AND mc.group_id = ANY($3::uuid[]))
				OR
				(s.course_id = ANY($2::uuid[]))
			  )
			  AND (
				sa.merge_group_id IS NOT DISTINCT FROM mc.group_id
				OR (sa.merge_group_id IS NULL AND s.course_id = sa.course_id)
				OR (sa.merge_group_id IS NULL AND mc.group_id IS NOT NULL AND EXISTS (
					SELECT 1 FROM course_merge_group_members m2
					WHERE m2.group_id = mc.group_id AND m2.course_id = sa.course_id
				))
			  )
		), legacy_days AS (
			SELECT DISTINCT
				CASE WHEN cd.merge_group_id IS NULL THEN cd.course_id ELSE NULL END AS course_id,
				cd.merge_group_id AS merge_group_id,
				cd.day AS day
			FROM scoped_absences sa
			JOIN (
				SELECT s.course_id AS course_id, m.group_id AS merge_group_id,
				       (s.start_at AT TIME ZONE $4)::date AS day
				FROM sessions s
				LEFT JOIN course_merge_group_members m ON m.course_id = s.course_id
				CROSS JOIN student_scope st
				WHERE s.deleted_at IS NULL
				  AND student_is_expected_at_session(st.id, s.id)
				  AND (
					(m.group_id IS NOT NULL AND m.group_id = ANY($3::uuid[]))
					OR
					(s.course_id = ANY($2::uuid[]))
				  )
			) cd ON (
				(sa.merge_group_id IS NOT NULL AND sa.merge_group_id = cd.merge_group_id)
				OR (sa.merge_group_id IS NULL AND sa.course_id = cd.course_id)
				OR (sa.merge_group_id IS NULL AND cd.merge_group_id IS NOT NULL AND EXISTS (
					SELECT 1 FROM course_merge_group_members m2
					WHERE m2.group_id = cd.merge_group_id AND m2.course_id = sa.course_id
				))
			)
			WHERE NOT sa.has_missed
			  AND cd.day BETWEEN sa.date_from AND sa.date_to
		), used_days AS (
			SELECT course_id, merge_group_id, day FROM explicit_days
			UNION
			SELECT course_id, merge_group_id, day FROM legacy_days
		)
		SELECT 'total', 'course:' || course_id::text, count(DISTINCT day) FROM course_days GROUP BY 2
		UNION ALL
		SELECT 'total', 'merge:' || merge_group_id::text, count(DISTINCT day) FROM merge_days GROUP BY 2
		UNION ALL
		SELECT 'used', COALESCE('merge:' || merge_group_id::text, 'course:' || course_id::text), count(DISTINCT day) FROM used_days GROUP BY 2
	`, arg.Wcode, courseIDs, mergeGroupIDs, timezone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	totals := make(map[string]int32)
	used := make(map[string]int32)
	for rows.Next() {
		var section, key string
		var count int32
		if err := rows.Scan(&section, &key, &count); err != nil {
			return nil, err
		}
		if section == "used" {
			used[key] = count
		} else {
			totals[key] = count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, scope := range arg.Scopes {
		key := scope.Key.String()
		out[key] = AbsenceDayCounts{TotalCourseDays: totals[key], UsedAbsenceDays: used[key]}
	}
	return out, nil
}

// SessionsRangeBlockedSitIns loads the blocked (already-assigned) sit-in
// session IDs plus per-session conflict details for one student in ONE round
// trip. It replaces the old per-course pair
// (ActiveSitInSessionIDsForStudent + ActiveSitInSessionConflictsForStudent
// per resolveSitInForCourse call) with a single student-scoped fetch whose
// cost is O(active sit-ins of this student), independent of course count.
func (q *Queries) SessionsRangeBlockedSitIns(ctx context.Context, studentID pgtype.UUID) ([]ActiveSitInSessionConflict, error) {
	rows, err := q.db.Query(ctx, `
		SELECT asi.session_id, sa.id,
		       COALESCE(abs_subj.name, ''), sa.date_from, sa.date_to,
		       COALESCE(sit_subj.name, ''), COALESCE(sit_course.name, ''), sit_session.start_at, sit_session.end_at
		FROM absence_sit_ins asi
		JOIN student_absences sa ON sa.id = asi.absence_id
		JOIN students st ON lower(st.wcode) = lower(sa.wcode)
		JOIN sessions sit_session ON sit_session.id = asi.session_id
		JOIN courses sit_course ON sit_course.id = sit_session.course_id
		LEFT JOIN subjects sit_subj ON sit_subj.id = sit_course.subject_id
		LEFT JOIN subjects abs_subj ON abs_subj.id = sa.subject_id
		WHERE st.id = $1
		  AND sa.status <> 'cancelled'
		ORDER BY asi.session_id, sa.created_at DESC
	`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ActiveSitInSessionConflict
	// First-wins dedup per session: rows arrive newest-absence-first
	// (ORDER BY session, created DESC), mirroring
	// ActiveSitInSessionConflictsForStudent exactly.
	seen := make(map[string]struct{})
	for rows.Next() {
		var item ActiveSitInSessionConflict
		if err := rows.Scan(&item.SessionID, &item.AbsenceID, &item.AbsenceSubjectName,
			&item.AbsenceDateFrom, &item.AbsenceDateTo,
			&item.SitInSubjectName, &item.SitInCourseName,
			&item.SitInStartAt, &item.SitInEndAt); err != nil {
			return nil, err
		}
		key := uuidBytesString(item.SessionID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

var _ = pgx.ErrNoRows

func uuidBytesString(u pgtype.UUID) string {
	if !u.Valid {
		return "00000000-0000-0000-0000-000000000000"
	}
	return sprintfUUID(u.Bytes)
}

func sprintfUUID(b [16]byte) string {
	const hexd = "0123456789abcdef"
	var out [36]byte
	hex := func(v byte) (byte, byte) { return hexd[v>>4], hexd[v&0x0f] }
	out[0], out[1] = hex(b[0])
	out[2], out[3] = hex(b[1])
	out[4], out[5] = hex(b[2])
	out[6], out[7] = hex(b[3])
	out[8] = '-'
	out[9], out[10] = hex(b[4])
	out[11], out[12] = hex(b[5])
	out[13] = '-'
	out[14], out[15] = hex(b[6])
	out[16], out[17] = hex(b[7])
	out[18] = '-'
	out[19], out[20] = hex(b[8])
	out[21], out[22] = hex(b[9])
	out[23] = '-'
	out[24], out[25] = hex(b[10])
	out[26], out[27] = hex(b[11])
	out[28], out[29] = hex(b[12])
	out[30], out[31] = hex(b[13])
	out[32], out[33] = hex(b[14])
	out[34], out[35] = hex(b[15])
	return string(out[:])
}
