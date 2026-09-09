package absenceshttp

// Bundle lookup helpers (pure, zero queries).

import (
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func toEnrolledV2(e sqldb.BundleEnrolledCourse) sqldb.StudentEnrolledCourseV2 {
	return sqldb.StudentEnrolledCourseV2{
		CourseID:           e.CourseID,
		CourseCode:         e.CourseCode,
		CourseName:         e.CourseName,
		SubjectID:          e.SubjectID,
		CycleID:            e.CycleID,
		Level:              e.Level,
		RootCourseGroupID:  e.RootCourseGroupID,
		SitInRuleID:        e.SitInRuleID,
		MergeGroupID:       e.MergeGroupID,
		AbsenceFormVisible: e.AbsenceFormVisible,
	}
}

func enrolledV2ForSubject(b *sqldb.SitInBundleV2, subjectID pgtype.UUID) []sqldb.StudentEnrolledCourseV2 {
	var out []sqldb.StudentEnrolledCourseV2
	for _, e := range b.Enrolled {
		if e.SubjectID == subjectID {
			out = append(out, toEnrolledV2(e))
		}
	}
	// Legacy StudentEnrolledCoursesBySubjectV2 orders by level ASC NULLS
	// LAST. The rule engine only reads levels as a set, but keep the order
	// identical so fallbacks (first-with-level) match.
	sortEnrolledByLevelNULLSLast(out)
	return out
}

func sortEnrolledByLevelNULLSLast(courses []sqldb.StudentEnrolledCourseV2) {
	sort.SliceStable(courses, func(i, j int) bool {
		vi, vj := courses[i].Level.Valid, courses[j].Level.Valid
		if vi != vj {
			return vi
		}
		if !vi {
			return courses[i].CourseCode < courses[j].CourseCode
		}
		if courses[i].Level.Int16 != courses[j].Level.Int16 {
			return courses[i].Level.Int16 < courses[j].Level.Int16
		}
		return courses[i].CourseCode < courses[j].CourseCode
	})
}

func expandEnrolledByRootForCourse(b *sqldb.SitInBundleV2, enrolled []sqldb.StudentEnrolledCourseV2, courseID pgtype.UUID) []sqldb.StudentEnrolledCourseV2 {
	// Legacy trigger: the enrolled row FOR THIS COURSE with a valid root.
	var root pgtype.UUID
	for _, c := range enrolled {
		if c.CourseID == courseID && c.RootCourseGroupID.Valid {
			root = c.RootCourseGroupID
			break
		}
	}
	if root.Valid {
		var expanded []sqldb.StudentEnrolledCourseV2
		for _, e := range b.Enrolled {
			if e.RootCourseGroupID.Valid && e.RootCourseGroupID == root {
				expanded = append(expanded, toEnrolledV2(e))
			}
		}
		if len(expanded) > 0 {
			sortEnrolledByLevelNULLSLast(expanded)
			return expanded
		}
		return enrolled
	}
	return enrolled
}

func sessionsInWindow(all []sqldb.SessionInRange, from, to time.Time) []sqldb.SessionInRange {
	out := make([]sqldb.SessionInRange, 0, len(all))
	for _, s := range all {
		if !s.StartAt.Valid {
			continue
		}
		t := s.StartAt.Time
		if (t.Equal(from) || t.After(from)) && t.Before(to) {
			out = append(out, s)
		}
	}
	return out
}

func missedCourseFromEnrolled(b *sqldb.SitInBundleV2, enrolled []sqldb.StudentEnrolledCourseV2, courseID pgtype.UUID) *sqldb.StudentEnrolledCourseV2 {
	for i := range enrolled {
		if enrolled[i].CourseID == courseID && enrolled[i].Level.Valid {
			return &enrolled[i]
		}
	}
	for i := range enrolled {
		if enrolled[i].Level.Valid {
			return &enrolled[i]
		}
	}
	return nil
}

func mergeScopeForCourseFromBundle(b *sqldb.SitInBundleV2, courseID pgtype.UUID) (sqldb.CourseMergeGroupScopeForCourseRow, bool) {
	var scope sqldb.CourseMergeGroupScopeForCourseRow
	found := false
	for _, c := range b.ScopeCourses {
		if c.ID == courseID && c.MergeGroupID.Valid {
			if !found {
				scope.ID = c.MergeGroupID
				scope.Name = b.MergeNames[uuidStringOrZero(c.MergeGroupID)]
				scope.SitInRuleID = c.SitInRuleID
				found = true
			}
			scope.CourseIDs = append(scope.CourseIDs, c.ID)
		}
	}
	// Collect sibling members from scope courses sharing the group.
	if found {
		sibs := make([]pgtype.UUID, 0)
		for _, c := range b.ScopeCourses {
			if c.MergeGroupID.Valid && c.MergeGroupID == scope.ID {
				sibs = append(sibs, c.ID)
			}
		}
		scope.CourseIDs = sibs
	}
	return scope, found
}

func coursesByRootFromBundle(b *sqldb.SitInBundleV2, root pgtype.UUID) []sqldb.SubjectCourseV2 {
	var out []sqldb.SubjectCourseV2
	for _, c := range b.ScopeCourses {
		if c.RootCourseGroupID.Valid && c.RootCourseGroupID == root && c.Level.Valid {
			out = append(out, c)
		}
	}
	// Legacy CoursesByRootCourseGroup: ORDER BY level ASC (NULLS implied
	// last); the rule engine picks first-on-tie, so order must match.
	sort.SliceStable(out, func(i, j int) bool {
		vi, vj := out[i].Level.Valid, out[j].Level.Valid
		if vi != vj {
			return vi
		}
		if !vi {
			return out[i].Code < out[j].Code
		}
		if out[i].Level.Int16 != out[j].Level.Int16 {
			return out[i].Level.Int16 < out[j].Level.Int16
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func coursesByMergeFromBundle(b *sqldb.SitInBundleV2, mergeID pgtype.UUID) []sqldb.SubjectCourseV2 {
	// Legacy CoursesByMergeGroup: ORDER BY level ASC, position ASC.
	key := uuidStringOrZero(mergeID)
	pos := make(map[string]int, len(b.MergeMembers[key]))
	for i, id := range b.MergeMembers[key] {
		pos[uuidStringOrZero(id)] = i
	}
	var out []sqldb.SubjectCourseV2
	for _, c := range b.ScopeCourses {
		if c.MergeGroupID.Valid && c.MergeGroupID == mergeID && c.Level.Valid {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Level.Int16 != out[j].Level.Int16 {
			return out[i].Level.Int16 < out[j].Level.Int16
		}
		pi, oki := pos[uuidStringOrZero(out[i].ID)]
		pj, okj := pos[uuidStringOrZero(out[j].ID)]
		if oki != okj {
			return oki
		}
		if pi != pj {
			return pi < pj
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func filterBundleVisible(b *sqldb.SitInBundleV2, courses []sqldb.SubjectCourseV2) []sqldb.SubjectCourseV2 {
	out := make([]sqldb.SubjectCourseV2, 0, len(courses))
	for _, c := range courses {
		if _, ok := b.Visible[uuidStringOrZero(c.ID)]; ok {
			out = append(out, c)
		}
	}
	return out
}

func scopeWindowWeeksFromPolicies(policies []byte, mergeScope sqldb.CourseMergeGroupScopeForCourseRow, hasMergeScope bool, root pgtype.UUID) int {
	if hasMergeScope {
		if id, err := uuidString(mergeScope.ID); err == nil {
			if weeks, ok := mergeGroupWindowWeeks(policies, id); ok {
				return weeks
			}
		}
	}
	return loadRootGroupWindowWeeksFromPolicies(policies, root)
}

func prioritiesForRoot(b *sqldb.SitInBundleV2, root pgtype.UUID) []sqldb.SitInPriorityWithRule {
	var out []sqldb.SitInPriorityWithRule
	for _, p := range b.Priorities {
		if p.RootCourseGroupID == root {
			out = append(out, p)
		}
	}
	return out
}

func ruleForScopeFromBundle(b *sqldb.SitInBundleV2, ruleID, root pgtype.UUID) (*sqldb.SitInRule, error) {
	if ruleID.Valid {
		if r, ok := b.RulesByID[uuidStringOrZero(ruleID)]; ok {
			return r, nil
		}
		return nil, pgx.ErrNoRows
	}
	if root.Valid {
		if r, ok := b.RulesByRoot[uuidStringOrZero(root)]; ok {
			return r, nil
		}
		// Mirror SitInRuleGetByRootCourseGroup ErrNoRows -> levelled fallback.
		return nil, pgx.ErrNoRows
	}
	return nil, pgx.ErrNoRows
}

func satMapForCourse(b *sqldb.SitInBundleV2, courseID pgtype.UUID) *sqldb.SatVerbalPolicyCourseMapping {
	if m := b.SatMapByCourse[uuidStringOrZero(courseID)]; m != nil {
		return m
	}
	if scope, ok := mergeScopeForCourseFromBundle(b, courseID); ok {
		for i := range b.SatMappings {
			m := &b.SatMappings[i]
			if m.MergeGroupID.Valid && m.MergeGroupID == scope.ID {
				return m
			}
		}
	}
	return nil
}

func satVerbalMappedCoursesByIDs(b *sqldb.SitInBundleV2, mapping sqldb.SatVerbalPolicyCourseMapping) ([]sqldb.SubjectCourseV2, error) {
	byID := make(map[string]sqldb.SubjectCourseV2, len(b.ScopeCourses)+len(b.SatMemberCourses))
	for _, c := range b.ScopeCourses {
		byID[uuidStringOrZero(c.ID)] = c
	}
	for _, c := range b.SatMemberCourses {
		byID[uuidStringOrZero(c.ID)] = c
	}
	memberIDs := b.MergeMembers[uuidStringOrZero(mapping.MergeGroupID)]
	if len(memberIDs) != 2 {
		return nil, fmt.Errorf("merged course target must contain exactly two source courses")
	}
	courses := make([]sqldb.SubjectCourseV2, 0, 2)
	for _, id := range memberIDs {
		c, ok := byID[uuidStringOrZero(id)]
		if !ok {
			// Legacy CourseSubjectByID miss propagates raw (pgx.ErrNoRows).
			return nil, pgx.ErrNoRows
		}
		courses = append(courses, c)
	}
	return courses, nil
}

func findCourse(courses []sqldb.SubjectCourseV2, id pgtype.UUID) *sqldb.SubjectCourseV2 {
	for i := range courses {
		if courses[i].ID == id {
			return &courses[i]
		}
	}
	return nil
}

// resolvePrioritiesFromBundle mirrors resolveSitInWithPrioritiesAndBlockedSessions
// but reads target sessions from the bundle map (zero queries).
func resolvePrioritiesFromBundle(b *sqldb.SitInBundleV2, priorities []sqldb.SitInPriorityWithRule, allCourses []sqldb.SubjectCourseV2, missed []sqldb.SessionInRange, level int16, enrolledLevels []int16, cutoff time.Time, blocked map[string]struct{}) (*SitInResult, error) {
	var inputs []priorityInput
	for _, p := range priorities {
		rulePredicate, err := parsePredicate(p.RulePredicate)
		if err != nil {
			return nil, fmt.Errorf("priority %d predicate parse: %w", p.PriorityLevel, err)
		}
		evalOutput, err := EvaluateRule(EvaluateRuleInput{
			RuleType:       p.RuleType,
			Predicate:      rulePredicate,
			StudentLevel:   level,
			EnrolledLevels: enrolledLevels,
			AllCourses:     allCourses,
			MissedCount:    len(missed),
		})
		if err != nil {
			return nil, fmt.Errorf("priority %d rule evaluation: %w", p.PriorityLevel, err)
		}
		if !evalOutput.Eligible || evalOutput.Method != SitInMethodPhysical || evalOutput.TargetCourseID == nil {
			continue
		}
		target := findCourse(allCourses, *evalOutput.TargetCourseID)
		if target == nil {
			continue
		}
		inputs = append(inputs, priorityInput{
			level:     int(p.PriorityLevel),
			label:     p.Label,
			target:    target,
			missed:    missed,
			available: b.Sessions[uuidStringOrZero(*evalOutput.TargetCourseID)],
		})
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	results := buildPrioritySitInResultsWithBlockedSessions(inputs, cutoff, blocked)
	return &SitInResult{SitInMethod: SitInMethodPhysical, Priorities: results}, nil
}

// enrichResultConflicts applies preloaded conflict details (zero queries).
func enrichResultConflicts(conflicts map[string]*sitInSessionConflictInfo, result *SitInResult) {
	if result == nil {
		return
	}
	apply := func(sessions []sessionBrief) {
		for i := range sessions {
			sessions[i].Conflict = conflicts[sessions[i].ID]
		}
	}
	apply(result.Available)
	apply(result.PreSelected)
	for i := range result.Unavailable {
		if result.Unavailable[i].Session != nil {
			result.Unavailable[i].Session.Conflict = conflicts[result.Unavailable[i].Session.ID]
		}
	}
	for i := range result.Priorities {
		apply(result.Priorities[i].Available)
		apply(result.Priorities[i].PreSelected)
		for j := range result.Priorities[i].Unavailable {
			if result.Priorities[i].Unavailable[j].Session != nil {
				result.Priorities[i].Unavailable[j].Session.Conflict = conflicts[result.Priorities[i].Unavailable[j].Session.ID]
			}
		}
	}
	for key, item := range result.SitInByMissedSession {
		apply(item.Available)
		apply(item.PreSelected)
		for i := range item.Unavailable {
			if item.Unavailable[i].Session != nil {
				item.Unavailable[i].Session.Conflict = conflicts[item.Unavailable[i].Session.ID]
			}
		}
		for i := range item.Priorities {
			apply(item.Priorities[i].Available)
			apply(item.Priorities[i].PreSelected)
			for j := range item.Priorities[i].Unavailable {
				if item.Priorities[i].Unavailable[j].Session != nil {
					item.Priorities[i].Unavailable[j].Session.Conflict = conflicts[item.Priorities[i].Unavailable[j].Session.ID]
				}
			}
		}
		result.SitInByMissedSession[key] = item
	}
}
