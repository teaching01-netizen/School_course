package absenceshttp

// Bundle-backed sit-in resolution (zero queries).
//
// This mirrors resolveSitInForCourse selection semantics exactly, but reads
// every fact from SitInBundleV2 + the preloaded blocked set instead of
// issuing per-course queries. The pure option builders
// (buildPhysicalSitInResultWithBlockedSessions, resolveSatVerbalPolicy with a
// map-backed LoadSessions) are reused unchanged, so option-level behavior is
// shared, not duplicated.

import (
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// bundleSitInInputs carries the runtime inputs that vary per request.
type bundleSitInInputs struct {
	bundle        *sqldb.SitInBundleV2
	policies      []byte
	blocked       map[string]struct{}
	conflicts     map[string]*sitInSessionConflictInfo
	instituteTZ   string
	afterPriority int
	studentFacing bool
}

// resolveSitInForCourseFromBundle mirrors resolveSitInForCourse using only
// bundle facts. Returns (nil, nil) when no sit-in applies.
func resolveSitInForCourseFromBundle(in bundleSitInInputs, wcode string, studentID pgtype.UUID, courseID, subjectID pgtype.UUID, dateFrom, dateTo time.Time) (*SitInResult, error) {
	b := in.bundle
	enrolled := enrolledV2ForSubject(b, subjectID)
	if len(enrolled) == 0 {
		return nil, nil
	}
	enrolled = expandEnrolledByRootForCourse(b, enrolled, courseID)
	missedSessions := sessionsInWindow(b.Sessions[uuidStringOrZero(courseID)], dateFrom, dateTo)
	// SAT-verbal mapped path first (mirrors old order). The bundle carries
	// only mappings touching this student universe; resolution itself is
	// pure given the map-backed session loader.
	if mapped, done, err := resolveMappedFromBundle(in, courseID, enrolled, dateFrom, dateTo); done {
		return mapped, err
	}
	// Missed-course level determines behavior (old: missedCourse lookup with
	// first-enrolled-with-level fallback).
	missedCourse := missedCourseFromEnrolled(b, enrolled, courseID)
	if missedCourse == nil {
		return nil, errors.New("no enrolled course has a level")
	}
	mergeScope, hasMergeScope := mergeScopeForCourseFromBundle(b, courseID)
	if !missedCourse.RootCourseGroupID.Valid && !hasMergeScope {
		return nil, nil
	}
	var allCourses []sqldb.SubjectCourseV2
	if missedCourse.RootCourseGroupID.Valid {
		allCourses = coursesByRootFromBundle(b, missedCourse.RootCourseGroupID)
	} else {
		allCourses = coursesByMergeFromBundle(b, mergeScope.ID)
	}
	if in.studentFacing {
		allCourses = filterBundleVisible(b, allCourses)
	}
	win := scopeWindowWeeksFromPolicies(in.policies, mergeScope, hasMergeScope, missedCourse.RootCourseGroupID)
	cutoff := time.Time{}
	if win > 0 {
		cutoff = time.Now().Add(time.Duration(win) * 7 * 24 * time.Hour)
	}
	// Priority-based resolution first (old: root-group + no merge rule).
	if missedCourse.RootCourseGroupID.Valid && (!hasMergeScope || !mergeScope.SitInRuleID.Valid) {
		priorities := prioritiesForRoot(b, missedCourse.RootCourseGroupID)
		if len(priorities) > 0 {
			result, err := resolvePrioritiesFromBundle(b, priorities, allCourses, missedSessions, missedCourse.Level.Int16, enrolledLevelsFromCourses(enrolled), cutoff, in.blocked)
			if err != nil {
				return nil, err
			}
			enrichResultConflicts(in.conflicts, result)
			return result, nil
		}
	}
	rule, err := ruleForScopeFromBundle(b, missedCourse.SitInRuleID, missedCourse.RootCourseGroupID)
	rule, err = sitInRuleWithLevelledRootFallback(rule, err, missedCourse.SitInRuleID, missedCourse.RootCourseGroupID, allCourses)
	if err != nil {
		return nil, fmt.Errorf("sit-in rule lookup: %w", err)
	}
	if rule == nil {
		return nil, nil
	}
	predicate, err := parsePredicate(rule.Predicate)
	if err != nil {
		return nil, fmt.Errorf("rule predicate parse: %w", err)
	}
	evalOutput, err := EvaluateRule(EvaluateRuleInput{
		RuleType:       rule.Type,
		Predicate:      predicate,
		StudentLevel:   missedCourse.Level.Int16,
		EnrolledLevels: enrolledLevelsFromCourses(enrolled),
		AllCourses:     allCourses,
		MissedCount:    len(missedSessions),
	})
	if err != nil {
		return nil, fmt.Errorf("rule evaluation: %w", err)
	}
	if !evalOutput.Eligible {
		return nil, nil
	}
	switch evalOutput.Method {
	case SitInMethodZoom:
		result := &SitInResult{SitInMethod: SitInMethodZoom, RuleName: rule.Name, RuleType: rule.Type}
		enrichResultConflicts(in.conflicts, result)
		return result, nil
	case SitInMethodTeacher:
		result := &SitInResult{SitInMethod: SitInMethodTeacher, RuleName: rule.Name, RuleType: rule.Type}
		enrichResultConflicts(in.conflicts, result)
		return result, nil
	case SitInMethodPhysical:
		if evalOutput.TargetCourseID == nil {
			return nil, errors.New("physical sit-in eligible but no target course")
		}
		targetCourse := findCourse(allCourses, *evalOutput.TargetCourseID)
		if targetCourse == nil {
			return nil, errors.New("target course not found in course group")
		}
		avail := b.Sessions[uuidStringOrZero(*evalOutput.TargetCourseID)]
		result := buildPhysicalSitInResultWithBlockedSessions(targetCourse, missedSessions, avail, cutoff, in.blocked)
		result.RuleName = rule.Name
		result.RuleType = rule.Type
		enrichResultConflicts(in.conflicts, result)
		return result, nil
	default:
		return nil, nil
	}
}

var _ = pgx.ErrNoRows
