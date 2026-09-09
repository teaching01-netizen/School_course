package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SitInBundleFacts gathers every sit-in rule input for one student in ONE
// round trip, replacing the old per-course fan-out:
//
//	old per missed course: StudentEnrolledCoursesBySubjectV2 +
//	  [StudentEnrolledCoursesByRootCourseGroup] + CoursesByRootCourseGroup /
//	  CoursesByMergeGroup + SitInPrioritiesByRootCourseGroupWithRule +
//	  SessionsByCourseInRange (missed) + SessionsByCourse (per target) +
//	  SatVerbal mappings fan-out + AppSettingsGetWithPolicies +
//	  CourseIDsVisible.
//
// The bundle loads the student-scoped universe once: enrollments, the full
// sit-in rule/priority/mapping catalog restricted to the student courses and
// their scopes, and every session of every scope course (range + unbounded
// targets share one row set; the caller slices by instant in Go, which is
// exact because instants compare independent of time zone).
//
// Complexity: O(student courses + scope courses + their sessions + catalog
// rows touching those scopes). Independent of total courses/sessions in the
// database.
type SitInBundleFactsParams struct {
	StudentID pgtype.UUID
	// MissedCourseIDs are the distinct enrolled course IDs in the fact set.
	MissedCourseIDs []pgtype.UUID
}

type BundleEnrolledCourse struct {
	CourseID           pgtype.UUID
	CourseCode         string
	CourseName         string
	SubjectID          pgtype.UUID
	CycleID            pgtype.Text
	Level              pgtype.Int2
	RootCourseGroupID  pgtype.UUID
	SitInRuleID        pgtype.UUID
	MergeGroupID       pgtype.UUID
	AbsenceFormVisible bool
}

type BundleScopeCourse struct {
	SubjectCourseV2
}

type BundlePriorityWithRule struct {
	SitInPriorityWithRule
}

type SitInBundleFacts struct {
	Enrolled     []BundleEnrolledCourse
	ScopeCourses []SubjectCourseV2
	Priorities   []SitInPriorityWithRule
	SatMappings  []SatVerbalPolicyCourseMapping
	// satMappingsLoaded marks the tag-2 union arm as drained (even when
	// zero rows arrived), so loadBundleSatMappings skips its backfill
	// query on the V2 path. V1 never sets it: the backfill always runs.
	satMappingsLoaded bool
	// PrioritiesFailed records a priorities-only batch failure so the V2
	// orchestrator can degrade to nil WITHOUT ResolveFailed (legacy
	// single-rule fallback). Distinct from transport failures that fail
	// the whole mid-batch: those return an error AND leave this unset,
	// so the caller sets ResolveFailed.
	PrioritiesFailed bool
	MergeNames       map[string]string
	VisibleCourseIDs map[string]struct{}
	SessionsByCourse map[string][]SessionInRange
}

// SessionsRangeSitInBundle loads all rule inputs in a FIXED number of round
// trips independent of course count (see SessionsRangeSitInBundleV2 for the
// consolidated entry point). Kept for compatibility; new code prefers V2.
func (q *Queries) SessionsRangeSitInBundle(ctx context.Context, arg SitInBundleFactsParams) (*SitInBundleFacts, error) {
	out := &SitInBundleFacts{
		MergeNames:       make(map[string]string),
		VisibleCourseIDs: make(map[string]struct{}),
		SessionsByCourse: make(map[string][]SessionInRange),
	}
	if err := q.loadBundleEnrolled(ctx, arg.StudentID, out); err != nil {
		return nil, err
	}
	if err := q.loadBundleScopeCourses(ctx, arg, out); err != nil {
		return nil, err
	}
	if err := q.loadBundlePriorities(ctx, out); err != nil {
		return nil, err
	}
	if err := q.loadBundleSatMappings(ctx, out); err != nil {
		return nil, err
	}
	if err := loadBundleSatMappingNamesStandalone(ctx, q.db, out); err != nil {
		return nil, err
	}
	if err := q.loadBundleSessions(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queries) loadBundleEnrolled(ctx context.Context, studentID pgtype.UUID, out *SitInBundleFacts) error {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.code, c.name, c.subject_id, c.cycle_id, COALESCE(mgg.level, c.level), c.root_course_group_id,
		       COALESCE(mgg.sit_in_rule_id, rcg.sit_in_rule_id), mgm.group_id, c.absence_form_visible
		FROM course_students cs
		JOIN courses c ON c.id = cs.course_id
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		LEFT JOIN course_merge_group_members mgm ON mgm.course_id = c.id
		LEFT JOIN course_merge_groups mgg ON mgg.id = mgm.group_id
		WHERE cs.student_id = $1 AND cs.status = 'enrolled'
		ORDER BY c.code ASC
	`, studentID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r BundleEnrolledCourse
		if err := rows.Scan(&r.CourseID, &r.CourseCode, &r.CourseName, &r.SubjectID, &r.CycleID, &r.Level, &r.RootCourseGroupID, &r.SitInRuleID, &r.MergeGroupID, &r.AbsenceFormVisible); err != nil {
			return err
		}
		out.Enrolled = append(out.Enrolled, r)
	}
	return rows.Err()
}

// loadBundleEnrolledAndScope is implemented in sessions_range_bundle_union.go
// (single round trip); this wrapper is kept for readability.
func (q *Queries) loadBundleEnrolledAndScope(ctx context.Context, arg SitInBundleFactsParams, out *SitInBundleFacts) error {
	return q.loadBundleEnrolledAndScopeUnion(ctx, arg, out)
}

func (q *Queries) loadBundleScopeCourses(ctx context.Context, arg SitInBundleFactsParams, out *SitInBundleFacts) error {
	// Scope courses = union of: root-group siblings of enrolled courses,
	// merge-group siblings of enrolled courses, and merge members of missed
	// courses. One query, no per-course loop.
	rows, err := q.db.Query(ctx, `
		WITH enrolled AS (
			SELECT c.id, c.root_course_group_id
			FROM course_students cs
			JOIN courses c ON c.id = cs.course_id
			WHERE cs.student_id = $1 AND cs.status = 'enrolled'
		), missed AS (
			SELECT unnest($2::uuid[]) AS id
		), root_scopes AS (
			SELECT DISTINCT c.root_course_group_id AS id FROM courses c
			JOIN enrolled e ON e.root_course_group_id = c.root_course_group_id
			WHERE c.root_course_group_id IS NOT NULL
			UNION
			SELECT DISTINCT c.root_course_group_id FROM courses c
			JOIN missed m ON m.id = c.id
			WHERE c.root_course_group_id IS NOT NULL
		), merge_scopes AS (
			SELECT DISTINCT mgm.group_id AS id
			FROM course_merge_group_members mgm
			JOIN enrolled e ON e.id = mgm.course_id
			UNION
			SELECT DISTINCT mgm.group_id
			FROM course_merge_group_members mgm
			JOIN missed m ON m.id = mgm.course_id
		)
		SELECT DISTINCT c.id, c.code, c.name, c.subject_id, COALESCE(sub.code, ''), COALESCE(sub.name, ''),
		       c.cycle_id, COALESCE(mgg.level, c.level), c.root_course_group_id,
		       COALESCE(mgg.sit_in_rule_id, rcg.sit_in_rule_id), mgm.group_id,
		       COALESCE(g.name, '') AS merge_group_name
		FROM courses c
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		LEFT JOIN course_merge_group_members mgm ON mgm.course_id = c.id
		LEFT JOIN course_merge_groups mgg ON mgg.id = mgm.group_id
		LEFT JOIN course_merge_groups g ON g.id = mgm.group_id
		WHERE (c.root_course_group_id IN (SELECT id FROM root_scopes))
		   OR (mgm.group_id IN (SELECT id FROM merge_scopes))
		   OR (c.id IN (SELECT id FROM missed))
		ORDER BY c.code ASC
	`, arg.StudentID, arg.MissedCourseIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r SubjectCourseV2
		var mergeName string
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.CycleID, &r.Level, &r.RootCourseGroupID, &r.SitInRuleID, &r.MergeGroupID, &mergeName); err != nil {
			return err
		}
		out.ScopeCourses = append(out.ScopeCourses, r)
		if r.MergeGroupID.Valid {
			out.MergeNames[uuidBytesString(r.MergeGroupID)] = mergeName
		}
	}
	return rows.Err()
}

func (q *Queries) loadBundlePriorities(ctx context.Context, out *SitInBundleFacts) error {
	// Priorities for every distinct root group in the scope universe, in ONE
	// query (ANY over the distinct group IDs). This replaces the old
	// per-course SitInPrioritiesByRootCourseGroupWithRule loop. SQL text,
	// input derivation, and row scan are shared with the trip-A mid-batch
	// (bundlePrioritiesSQL / bundlePriorityRootGroups / scanBundlePriorities)
	// so the standalone path and the batch drain execute identical work.
	groups := bundlePriorityRootGroups(out.ScopeCourses)
	if len(groups) == 0 {
		return nil
	}
	rows, err := q.db.Query(ctx, bundlePrioritiesSQL(), groups)
	if err != nil {
		return err
	}
	return scanBundlePriorities(rows, out)
}

func (q *Queries) loadBundleSatMappings(ctx context.Context, out *SitInBundleFacts) error {
	// Mappings ride the enrolled+scope UNION ALL (tag 2) when the caller
	// ran it (V2 always does; out.SatMappings non-nil afterwards — note:
	// nil vs empty matters, an active-mappings count of zero still leaves
	// a non-nil empty slice... it does not: appends to a nil slice stay
	// nil when zero rows arrive. So the union path MUST mark the load as
	// done explicitly — see satMappingsLoaded flag — otherwise a
	// zero-mapping world re-queries here every request. The flag check
	// below, not len(), is the source of truth.
	if !out.satMappingsLoaded {
		mappings, err := q.SatVerbalPolicyMappingsList(ctx)
		if err != nil {
			return err
		}
		// Legacy resolves against ALL active mappings (missed-course
		// lookup is global, and every mapping contributes targets).
		// Filtering by student universe would drop sit-in targets the
		// old path offers.
		out.SatMappings = append(out.SatMappings, mappings...)
		out.satMappingsLoaded = true
	}
	// Merge names for mapped merge groups missing from the scope
	// universe are NOT loaded here: on the V2 path they ride the trip-A
	// mid-batch (4th arm via bundleMissingMergeNameIDs +
	// scanBundleMergeNames — same SQL, derivation, and scan as the
	// standalone helper below). The V1 compat path
	// (SessionsRangeSitInBundle, which never runs trip-A) calls
	// loadBundleSatMappingNamesStandalone explicitly.
	// Visibility is resolved by the V2 assembler after SAT member courses
	// load (mapped members outside the scope universe need it too).
	return nil
}

// loadBundleSatMappingNamesStandalone is the merge-names probe for
// the V1 compat path (SessionsRangeSitInBundle): same SQL, derivation,
// and scan as the trip-A names arm (bundleMergeNamesSQL /
// bundleMissingMergeNameIDs / scanBundleMergeNames). It runs only when
// the names arm did not already fuse them (namesFused=false on V1).
func loadBundleSatMappingNamesStandalone(ctx context.Context, db DBTX, out *SitInBundleFacts) error {
	missing := make([]pgtype.UUID, 0)
	for _, m := range out.SatMappings {
		if m.MergeGroupID.Valid {
			if _, ok := out.MergeNames[uuidBytesString(m.MergeGroupID)]; !ok {
				missing = append(missing, m.MergeGroupID)
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	rows, err := db.Query(ctx, bundleMergeNamesSQL(), missing)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id pgtype.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		out.MergeNames[uuidBytesString(id)] = name
	}
	return rows.Err()
}

// bundleVisibleCourseIDs collects the scope + SAT-member course ID strings
// the visibility probe covers. Empty = probe not needed (all-visible vacuous).
func bundleVisibleCourseIDs(scopeCourses, satMembers []SubjectCourseV2) []string {
	ids := make([]string, 0, len(scopeCourses)+len(satMembers))
	for _, c := range scopeCourses {
		ids = append(ids, uuidBytesString(c.ID))
	}
	for _, c := range satMembers {
		ids = append(ids, uuidBytesString(c.ID))
	}
	return ids
}

// bundleVisibleSelectSQL is the visibility probe shared by the direct and
// batched loaders: one statement, same predicate, same row shape.
func bundleVisibleSelectSQL() string {
	return `SELECT c.id::text
		 FROM courses c
		 WHERE c.id = ANY($1::uuid[])
		   AND c.absence_form_visible
		   AND EXISTS (
			 SELECT 1 FROM subject_active_courses sac
			 WHERE sac.subject_id = c.subject_id AND sac.course_id = c.id
		   )`
}

// scanBundleVisibleResult drains one queued visibility result set.
// A probe failure degrades to all-visible (legacy per-course resolve would
// log-and-skip only that course; a shared probe cannot express that, and a
// failing trivial PK query implies wider DB trouble that surfaces on the
// next query anyway).
func scanBundleVisibleResult(rows pgx.Rows, ids []string) map[string]struct{} {
	defer rows.Close()
	out := make(map[string]struct{}, len(ids))
	ok := true
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			ok = false
			break
		}
		out[id] = struct{}{}
	}
	if rows.Err() != nil {
		ok = false
	}
	if !ok {
		all := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			all[id] = struct{}{}
		}
		return all
	}
	return out
}

// LoadBundleVisible resolves student-form visibility for scope + SAT member
// courses in one query. A visibility-probe failure degrades to all-visible
// (legacy per-course resolve would log-and-skip only that course; a shared
// probe cannot express that, and a failing trivial PK query implies wider
// DB trouble that surfaces on the next query anyway).
func (q *Queries) LoadBundleVisible(ctx context.Context, scopeCourses, satMembers []SubjectCourseV2) (map[string]struct{}, error) {
	ids := make([]string, 0, len(scopeCourses)+len(satMembers))
	for _, c := range scopeCourses {
		ids = append(ids, uuidBytesString(c.ID))
	}
	for _, c := range satMembers {
		ids = append(ids, uuidBytesString(c.ID))
	}
	if len(ids) == 0 {
		return map[string]struct{}{}, nil
	}
	if visible, err := q.CourseIDsVisible(ctx, ids); err == nil {
		return visible, nil
	}
	all := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		all[id] = struct{}{}
	}
	return all, nil
}

func (q *Queries) loadBundleSessions(ctx context.Context, out *SitInBundleFacts) error {
	if len(out.ScopeCourses) == 0 {
		return nil
	}
	ids := make([]pgtype.UUID, 0, len(out.ScopeCourses))
	for _, c := range out.ScopeCourses {
		ids = append(ids, c.ID)
	}
	rows, err := q.db.Query(ctx, `
		SELECT id, course_id, room_id, start_at, end_at
		FROM sessions
		WHERE course_id = ANY($1::uuid[])
		  AND deleted_at IS NULL
		ORDER BY course_id, start_at ASC
	`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r SessionInRange
		if err := rows.Scan(&r.ID, &r.CourseID, &r.RoomID, &r.StartAt, &r.EndAt); err != nil {
			return err
		}
		out.SessionsByCourse[uuidBytesString(r.CourseID)] = append(out.SessionsByCourse[uuidBytesString(r.CourseID)], r)
	}
	return rows.Err()
}
