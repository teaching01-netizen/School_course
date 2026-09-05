package absenceshttp

// Bundle-backed SAT-verbal mapped resolution (zero queries).
//
// Mirrors resolveMappedSatVerbalSitInWithBlockedSessions, but every catalog
// fact comes from SitInBundleV2: the active mapping for the missed course,
// the full active mapping list (already restricted to this student universe),
// mapped merge-member courses (from bundle scope courses), missed sessions
// (from the bundle session map), and policies (passed in, loaded once per
// request). Error strings match the old path so shadow diffs stay clean.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// resolveMappedFromBundle returns (result, done, err): done=false means no
// mapping applies, continue to rule-based resolution.
func resolveMappedFromBundle(in bundleSitInInputs, courseID pgtype.UUID, enrolled []sqldb.StudentEnrolledCourseV2, dateFrom, dateTo time.Time) (*SitInResult, bool, error) {
	b := in.bundle
	mapping := satMapForCourse(b, courseID)
	if mapping == nil {
		return nil, false, nil
	}
	rule, err := decodeSatVerbalMappedRule(mapping.PolicyRule)
	if err != nil {
		return nil, true, fmt.Errorf("SAT Verbal policy rule parse: %w", err)
	}
	mappedCourses := make([]satVerbalMappedCourse, 0, len(b.SatMappings))
	allCourses := make([]sqldb.SubjectCourseV2, 0, len(b.SatMappings))
	for _, active := range b.SatMappings {
		activeRule, err := decodeSatVerbalMappedRule(active.PolicyRule)
		if err != nil {
			return nil, true, fmt.Errorf("SAT Verbal mapped policy rule parse: %w", err)
		}
		courses, err := satVerbalMappedCoursesFromBundle(b, active)
		if err != nil {
			return nil, true, fmt.Errorf("SAT Verbal mapped course lookup: %w", err)
		}
		for _, course := range courses {
			mappedCourses = append(mappedCourses, satVerbalMappedCourse{Rule: activeRule, Course: course})
			allCourses = append(allCourses, course)
		}
	}
	missedSessions := sessionsInWindow(b.Sessions[uuidStringOrZero(courseID)], dateFrom, dateTo)
	var missedCourse sqldb.SubjectCourseV2
	missedCourseFound := false
	for _, course := range allCourses {
		if course.ID == courseID {
			missedCourse = course
			missedCourseFound = true
			break
		}
	}
	if !missedCourseFound {
		return nil, true, fmt.Errorf("SAT Verbal mapped course not found for absence course")
	}
	subjectIDStr, _ := uuidString(missedCourse.SubjectID)
	rootIDStr := ""
	if missedCourse.RootCourseGroupID.Valid {
		rootIDStr, _ = uuidString(missedCourse.RootCourseGroupID)
	}
	mergeIDStr := ""
	if mapping.MergeGroupID.Valid {
		mergeIDStr, _ = uuidString(mapping.MergeGroupID)
	} else if scope, ok := mergeScopeForCourseFromBundle(b, courseID); ok {
		mergeIDStr, _ = uuidString(scope.ID)
	}
	win := subjectWindowWeeksWithMerge(in.policies, subjectIDStr, rootIDStr, mergeIDStr)
	cutoff := time.Time{}
	if win > 0 {
		cutoff = time.Now().Add(time.Duration(win) * 7 * 24 * time.Hour)
	}
	activeAll := allCourses
	if in.studentFacing {
		activeAll = filterBundleVisible(b, allCourses)
	}
	activeSet := make(map[string]struct{}, len(activeAll))
	for _, c := range activeAll {
		if id, idErr := uuidString(c.ID); idErr == nil {
			activeSet[id] = struct{}{}
		}
	}
	activeMapped := make([]satVerbalMappedCourse, 0, len(mappedCourses))
	for _, mc := range mappedCourses {
		if id, idErr := uuidString(mc.Course.ID); idErr == nil {
			if _, ok := activeSet[id]; ok {
				activeMapped = append(activeMapped, mc)
			}
		}
	}
	result, err := resolveSatVerbalPolicy(context.Background(), satVerbalResolveInput{
		Rule:                   &rule,
		MappedCourses:          activeMapped,
		MissedCourse:           missedCourse,
		Enrolled:               enrolled,
		AllCourses:             activeAll,
		MergeGroupNames:        b.MergeNames,
		MissedSessions:         missedSessions,
		Cutoff:                 cutoff,
		RequestTime:            time.Now(),
		InstituteTZ:            in.instituteTZ,
		AfterPriorityLevel:     in.afterPriority,
		BlockedSitInSessionIDs: in.blocked,
		LoadSessions: func(_ context.Context, targetCourseID pgtype.UUID) ([]sqldb.SessionInRange, error) {
			return b.Sessions[uuidStringOrZero(targetCourseID)], nil
		},
	})
	if err != nil {
		return nil, true, err
	}
	return result, true, nil
}

// satVerbalMappedCoursesFromBundle mirrors satVerbalMappedCourses without
// queries: merge members come from bundle scope courses.
func satVerbalMappedCoursesFromBundle(b *sqldb.SitInBundleV2, mapping sqldb.SatVerbalPolicyCourseMapping) ([]sqldb.SubjectCourseV2, error) {
	if !mapping.MergeGroupID.Valid {
		return []sqldb.SubjectCourseV2{satVerbalCourseFromMapping(mapping)}, nil
	}
	return satVerbalMappedCoursesByIDs(b, mapping)
}

var _ = strings.TrimSpace
