package absenceshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

const (
	RuleTypeLevelLadder      = "level_ladder"
	RuleTypeCrossSection     = "cross_section"
	RuleTypeAnyDayExceptLast = "any_day_except_last"
	RuleTypeRankChain        = "rank_chain"
	RuleTypeTeacherCase      = "teacher_case_by_case"

	SitInMethodNone     = "none"
	SitInMethodZoom     = "zoom"
	SitInMethodPhysical = "physical"
	SitInMethodTeacher  = "teacher_case"
)

type SitInPriorityResult struct {
	Level       int              `json:"level"`
	Label       string           `json:"label"`
	SitInCourse *SitInCourseInfo `json:"sit_in_course,omitempty"`
	Available   []sessionBrief   `json:"available_sessions,omitempty"`
	PreSelected []sessionBrief   `json:"pre_selected,omitempty"`
}

type SitInResult struct {
	SitInMethod string `json:"sit_in_method"` // "physical" or "zoom"

	// Rule metadata
	RuleName string `json:"rule_name,omitempty"`
	RuleType string `json:"rule_type,omitempty"`

	// Priority-based sit-in (multi-level)
	Priorities           []SitInPriorityResult `json:"priorities,omitempty"`
	CurrentPriorityLevel int                   `json:"current_priority_level,omitempty"`
	HasNextPriority      bool                  `json:"has_next_priority,omitempty"`

	// For physical sit-in (single-level, backward compat)
	SitInCourse   *SitInCourseInfo `json:"sit_in_course,omitempty"`
	MissedCount   int              `json:"missed_count"`
	MissedSession []sessionBrief   `json:"missed_sessions,omitempty"`
	Available     []sessionBrief   `json:"available_sessions,omitempty"`
	PreSelected   []sessionBrief   `json:"pre_selected,omitempty"`
}

type SitInCourseInfo struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	SubjectCode string `json:"subject_code,omitempty"`
	SubjectName string `json:"subject_name,omitempty"`
}

type sessionBrief struct {
	ID              string `json:"id"`
	StartAt         string `json:"start_at"`
	EndAt           string `json:"end_at"`
	MissedSessionID string `json:"missed_session_id,omitempty"`
	ClassName       string `json:"class_name,omitempty"`
	CourseName      string `json:"course_name,omitempty"`
	CourseCode      string `json:"course_code,omitempty"`
	SubjectCode     string `json:"subject_code,omitempty"`
	SubjectName     string `json:"subject_name,omitempty"`
}

type ResolverInput struct {
	StudentLevel      int16
	StudentCourseID   pgtype.UUID
	AllCourses        []sqldb.SubjectCourseV2
	AutoSitInEnabled  bool
	MissedSessions    []sqldb.SessionInRange
	AvailableSessions []sqldb.SessionInRange
}

func buildPhysicalSitInResult(
	target *sqldb.SubjectCourseV2,
	missed []sqldb.SessionInRange,
	available []sqldb.SessionInRange,
	cutoff time.Time,
) *SitInResult {
	var nonOverlapping []sqldb.SessionInRange
	for _, a := range available {
		overlaps := false
		for _, m := range missed {
			if timesOverlap(a.StartAt, a.EndAt, m.StartAt, m.EndAt) {
				overlaps = true
				break
			}
		}
		if overlaps {
			continue
		}
		if !cutoff.IsZero() && a.StartAt.Time.After(cutoff) {
			continue
		}
		nonOverlapping = append(nonOverlapping, a)
	}

	preSelectCount := len(missed)
	if preSelectCount > len(nonOverlapping) {
		preSelectCount = len(nonOverlapping)
	}
	preSelected := nonOverlapping[:preSelectCount]

	targetIDStr, _ := uuidString(target.ID)

	result := &SitInResult{
		SitInMethod: SitInMethodPhysical,
		SitInCourse: &SitInCourseInfo{
			ID:          targetIDStr,
			Code:        target.Code,
			Name:        target.Name,
			SubjectCode: target.SubjectCode,
			SubjectName: target.SubjectName,
		},
		MissedCount: len(missed),
	}

	for _, m := range missed {
		result.MissedSession = append(result.MissedSession, toSessionBrief(m))
	}
	for _, a := range nonOverlapping {
		result.Available = append(result.Available, toSessionBriefForCourse(a, target))
	}
	for _, p := range preSelected {
		result.PreSelected = append(result.PreSelected, toSessionBriefForCourse(p, target))
	}

	return result
}

type priorityInput struct {
	level     int
	label     string
	target    *sqldb.SubjectCourseV2
	missed    []sqldb.SessionInRange
	available []sqldb.SessionInRange
}

// buildPrioritySitInResults builds a SitInPriorityResult for each priority level.
// Each priority has its own target course, missed sessions, and available sessions.
// Overlap and cutoff filtering are applied per-priority.
func buildPrioritySitInResults(priorities []priorityInput, cutoff time.Time) []SitInPriorityResult {
	if len(priorities) == 0 {
		return nil
	}
	results := make([]SitInPriorityResult, 0, len(priorities))
	for _, p := range priorities {
		result := buildPhysicalSitInResult(p.target, p.missed, p.available, cutoff)
		results = append(results, SitInPriorityResult{
			Level:       p.level,
			Label:       p.label,
			SitInCourse: result.SitInCourse,
			Available:   result.Available,
			PreSelected: result.PreSelected,
		})
	}
	return results
}

// resolveSitInWithPriorities resolves sit-in using priority-based rules.
// For each priority, it finds the target course by rank/section and evaluates the rule.
func resolveSitInWithPriorities(
	ctx context.Context,
	q *sqldb.Queries,
	priorities []sqldb.SitInPriorityWithRule,
	allCourses []sqldb.SubjectCourseV2,
	missedSessions []sqldb.SessionInRange,
	studentLevel int16,
	enrolledLevels []int16,
	cutoff time.Time,
) (*SitInResult, error) {
	var inputs []priorityInput

	for _, p := range priorities {
		rulePredicate, err := parsePredicate(p.RulePredicate)
		if err != nil {
			return nil, fmt.Errorf("priority %d predicate parse: %w", p.PriorityLevel, err)
		}

		evalOutput, err := EvaluateRule(EvaluateRuleInput{
			RuleType:       p.RuleType,
			Predicate:      rulePredicate,
			StudentLevel:   studentLevel,
			EnrolledLevels: enrolledLevels,
			AllCourses:     allCourses,
			MissedCount:    len(missedSessions),
		})
		if err != nil {
			return nil, fmt.Errorf("priority %d rule evaluation: %w", p.PriorityLevel, err)
		}

		if !evalOutput.Eligible {
			continue
		}

		if evalOutput.Method != SitInMethodPhysical {
			continue
		}

		if evalOutput.TargetCourseID == nil {
			continue
		}
		targetCourseID := *evalOutput.TargetCourseID

		var targetCourse *sqldb.SubjectCourseV2
		for i := range allCourses {
			if allCourses[i].ID == targetCourseID {
				targetCourse = &allCourses[i]
				break
			}
		}
		if targetCourse == nil {
			continue
		}

		availSessions, err := q.SessionsByCourse(ctx, targetCourseID)
		if err != nil {
			return nil, fmt.Errorf("priority %d available sessions lookup: %w", p.PriorityLevel, err)
		}

		inputs = append(inputs, priorityInput{
			level:     int(p.PriorityLevel),
			label:     p.Label,
			target:    targetCourse,
			missed:    missedSessions,
			available: availSessions,
		})
	}

	if len(inputs) == 0 {
		return nil, nil
	}

	results := buildPrioritySitInResults(inputs, cutoff)
	return &SitInResult{
		SitInMethod: SitInMethodPhysical,
		Priorities:  results,
	}, nil
}

func enrolledLevelsFromCourses(courses []sqldb.StudentEnrolledCourseV2) []int16 {
	levels := make([]int16, 0, len(courses))
	seen := make(map[int16]struct{}, len(courses))
	for _, course := range courses {
		if !course.Level.Valid || course.Level.Int16 <= 0 {
			continue
		}
		if _, ok := seen[course.Level.Int16]; ok {
			continue
		}
		seen[course.Level.Int16] = struct{}{}
		levels = append(levels, course.Level.Int16)
	}
	return levels
}

// resolveSitInForCourse resolves sit-in for a specific student course block.
// Uses the MISSED course's level to determine sit-in behavior, not the student's
// highest enrolled level. Level 1 absences always yield Zoom.
func resolveSitInForCourse(ctx context.Context, q *sqldb.Queries, wcode string, courseID, subjectID pgtype.UUID, dateFrom, dateTo time.Time, satVerbalAfterPriority int) (*SitInResult, error) {
	student, err := q.StudentGetByWCode(ctx, wcode)
	if err != nil {
		return nil, fmt.Errorf("student not found: %w", err)
	}

	enrolled, err := q.StudentEnrolledCoursesBySubjectV2(ctx, student.ID, subjectID)
	if err != nil {
		return nil, fmt.Errorf("enrolled courses lookup: %w", err)
	}
	if len(enrolled) == 0 {
		return nil, fmt.Errorf("student not enrolled in any course for this subject")
	}

	for _, c := range enrolled {
		if c.CourseID == courseID && c.RootCourseGroupID.Valid {
			rootEnrolled, err := q.StudentEnrolledCoursesByRootCourseGroup(ctx, student.ID, c.RootCourseGroupID)
			if err != nil {
				return nil, fmt.Errorf("root course group enrollment lookup: %w", err)
			}
			if len(rootEnrolled) > 0 {
				enrolled = rootEnrolled
			}
			break
		}
	}

	if mapped, err := resolveMappedSatVerbalSitIn(ctx, q, subjectID, courseID, enrolled, dateFrom, dateTo, satVerbalAfterPriority); err != nil {
		return nil, err
	} else if mapped != nil {
		return mapped, nil
	}

	// Find the MISSED course's level (determines sit-in behavior)
	var missedCourse *sqldb.StudentEnrolledCourseV2
	for i := range enrolled {
		if enrolled[i].CourseID == courseID && enrolled[i].Level.Valid {
			missedCourse = &enrolled[i]
			break
		}
	}
	// Fallback: if missed course not found in enrolled, use first enrolled with a level
	if missedCourse == nil {
		for i := range enrolled {
			if enrolled[i].Level.Valid {
				missedCourse = &enrolled[i]
				break
			}
		}
	}
	if missedCourse == nil {
		return nil, fmt.Errorf("no enrolled course has a level")
	}

	if !missedCourse.RootCourseGroupID.Valid {
		return nil, nil
	}

	allCourses, err := q.CoursesByRootCourseGroup(ctx, missedCourse.RootCourseGroupID)
	if err != nil {
		return nil, fmt.Errorf("root course group lookup: %w", err)
	}

	missedSessions, err := q.SessionsByCourseInRange(ctx, courseID, dateFrom, dateTo)
	if err != nil {
		return nil, fmt.Errorf("missed sessions lookup: %w", err)
	}

	win := loadRootGroupWindowWeeks(ctx, q, missedCourse.RootCourseGroupID)
	cutoff := time.Time{}
	if win > 0 {
		cutoff = time.Now().Add(time.Duration(win) * 7 * 24 * time.Hour)
	}

	// Try priority-based resolution first
	priorities, pErr := q.SitInPrioritiesByRootCourseGroupWithRule(ctx, missedCourse.RootCourseGroupID)
	if pErr == nil && len(priorities) > 0 {
		return resolveSitInWithPriorities(ctx, q, priorities, allCourses, missedSessions, missedCourse.Level.Int16, enrolledLevelsFromCourses(enrolled), cutoff)
	}

	// Fall back to single-rule logic
	rule, err := q.SitInRuleGetByRootCourseGroup(ctx, missedCourse.RootCourseGroupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
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

	var result *SitInResult
	switch evalOutput.Method {
	case SitInMethodZoom:
		result = &SitInResult{SitInMethod: SitInMethodZoom}
	case SitInMethodTeacher:
		result = &SitInResult{SitInMethod: SitInMethodTeacher}
	case SitInMethodPhysical:
		if evalOutput.TargetCourseID == nil {
			return nil, fmt.Errorf("physical sit-in eligible but no target course")
		}
		targetCourseID := *evalOutput.TargetCourseID

		availSessions, err := q.SessionsByCourse(ctx, targetCourseID)
		if err != nil {
			return nil, fmt.Errorf("available sessions lookup: %w", err)
		}

		var targetCourse *sqldb.SubjectCourseV2
		for i := range allCourses {
			if allCourses[i].ID == targetCourseID {
				targetCourse = &allCourses[i]
				break
			}
		}
		if targetCourse == nil {
			return nil, fmt.Errorf("target course not found in course group")
		}

		result = buildPhysicalSitInResult(targetCourse, missedSessions, availSessions, cutoff)
	default:
		return nil, nil
	}

	result.RuleName = rule.Name
	result.RuleType = rule.Type
	return result, nil
}

func resolveMappedSatVerbalSitIn(
	ctx context.Context,
	q *sqldb.Queries,
	subjectID pgtype.UUID,
	courseID pgtype.UUID,
	enrolled []sqldb.StudentEnrolledCourseV2,
	dateFrom time.Time,
	dateTo time.Time,
	afterPriorityLevel int,
) (*SitInResult, error) {
	mapping, err := q.SatVerbalPolicyMappingGetActiveByCourse(ctx, courseID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("SAT Verbal mapping lookup: %w", err)
	}

	rule, err := decodeSatVerbalMappedRule(mapping.PolicyRule)
	if err != nil {
		return nil, fmt.Errorf("SAT Verbal policy rule parse: %w", err)
	}

	activeMappings, err := q.SatVerbalPolicyMappingsList(ctx)
	if err != nil {
		return nil, fmt.Errorf("SAT Verbal mapping list lookup: %w", err)
	}
	mappedCourses := make([]satVerbalMappedCourse, 0, len(activeMappings))
	allCourses := make([]sqldb.SubjectCourseV2, 0, len(activeMappings))
	for _, active := range activeMappings {
		activeRule, err := decodeSatVerbalMappedRule(active.PolicyRule)
		if err != nil {
			return nil, fmt.Errorf("SAT Verbal mapped policy rule parse: %w", err)
		}
		course := satVerbalCourseFromMapping(active)
		mappedCourses = append(mappedCourses, satVerbalMappedCourse{Rule: activeRule, Course: course})
		allCourses = append(allCourses, course)
	}

	missedSessions, err := q.SessionsByCourseInRange(ctx, courseID, dateFrom, dateTo)
	if err != nil {
		return nil, fmt.Errorf("missed sessions lookup: %w", err)
	}
	missedCourse := satVerbalCourseFromMapping(*mapping)

	settings, err := q.AppSettingsGetWithPolicies(ctx)
	if err != nil {
		return nil, fmt.Errorf("policy lookup: %w", err)
	}
	subjectIDStr, _ := uuidString(mapping.SubjectID)
	rootIDStr := ""
	if missedCourse.RootCourseGroupID.Valid {
		rootIDStr, _ = uuidString(missedCourse.RootCourseGroupID)
	}
	win := subjectWindowWeeks(settings.AbsencePolicies, subjectIDStr, rootIDStr)
	cutoff := time.Time{}
	if win > 0 {
		cutoff = time.Now().Add(time.Duration(win) * 7 * 24 * time.Hour)
	}

	return resolveSatVerbalPolicy(ctx, satVerbalResolveInput{
		Rule:               &rule,
		MappedCourses:      mappedCourses,
		MissedCourse:       missedCourse,
		Enrolled:           enrolled,
		AllCourses:         allCourses,
		MissedSessions:     missedSessions,
		Cutoff:             cutoff,
		RequestTime:        time.Now(),
		AfterPriorityLevel: afterPriorityLevel,
		LoadSessions: func(ctx context.Context, targetCourseID pgtype.UUID) ([]sqldb.SessionInRange, error) {
			return q.SessionsByCourse(ctx, targetCourseID)
		},
	})
}

func decodeSatVerbalMappedRule(raw []byte) (satVerbalCourseRule, error) {
	var rule satVerbalCourseRule
	if err := json.Unmarshal(raw, &rule); err != nil {
		return satVerbalCourseRule{}, err
	}
	if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.CourseName) == "" {
		return satVerbalCourseRule{}, fmt.Errorf("missing id or courseName")
	}
	return rule, nil
}

func satVerbalCourseFromMapping(mapping sqldb.SatVerbalPolicyCourseMapping) sqldb.SubjectCourseV2 {
	return sqldb.SubjectCourseV2{
		ID:                mapping.CourseID,
		Code:              mapping.CourseCode,
		Name:              mapping.CourseName,
		SubjectID:         mapping.SubjectID,
		SubjectCode:       mapping.SubjectCode,
		SubjectName:       mapping.SubjectName,
		CycleID:           mapping.CycleID,
		Level:             mapping.Level,
		RootCourseGroupID: mapping.RootCourseGroupID,
		SitInRuleID:       mapping.SitInRuleID,
	}
}

func satVerbalMissedCourse(courseID pgtype.UUID, allCourses []sqldb.SubjectCourseV2, enrolled []sqldb.StudentEnrolledCourseV2) (sqldb.SubjectCourseV2, bool) {
	for _, course := range allCourses {
		if course.ID == courseID {
			return course, true
		}
	}
	for _, course := range enrolled {
		if course.CourseID == courseID {
			return sqldb.SubjectCourseV2{
				ID:                course.CourseID,
				Code:              course.CourseCode,
				Name:              course.CourseName,
				SubjectID:         course.SubjectID,
				CycleID:           course.CycleID,
				Level:             course.Level,
				RootCourseGroupID: course.RootCourseGroupID,
				SitInRuleID:       course.SitInRuleID,
			}, true
		}
	}
	return sqldb.SubjectCourseV2{}, false
}

func resolveSitIn(ctx context.Context, q *sqldb.Queries, wcode string, subjectID pgtype.UUID, dateFrom, dateTo time.Time) (*SitInResult, error) {
	// 1. Find student by wcode
	student, err := q.StudentGetByWCode(ctx, wcode)
	if err != nil {
		return nil, fmt.Errorf("student not found: %w", err)
	}

	// 2. Get student's enrolled courses in this subject (v2)
	enrolled, err := q.StudentEnrolledCoursesBySubjectV2(ctx, student.ID, subjectID)
	if err != nil {
		return nil, fmt.Errorf("enrolled courses lookup: %w", err)
	}
	if len(enrolled) == 0 {
		return nil, fmt.Errorf("student not enrolled in any course for this subject")
	}

	for _, c := range enrolled {
		if c.RootCourseGroupID.Valid {
			rootEnrolled, err := q.StudentEnrolledCoursesByRootCourseGroup(ctx, student.ID, c.RootCourseGroupID)
			if err != nil {
				return nil, fmt.Errorf("root course group enrollment lookup: %w", err)
			}
			if len(rootEnrolled) > 0 {
				enrolled = rootEnrolled
			}
			break
		}
	}

	// 3. Pick main course (lowest enrolled level — for sit-in resolution we need
	//    the missed course level, not the highest)
	main := enrolled[0]
	for _, c := range enrolled {
		if c.Level.Valid && main.Level.Valid && c.Level.Int16 < main.Level.Int16 {
			main = c
		}
	}

	if !main.Level.Valid {
		return nil, fmt.Errorf("main course has no level")
	}

	// 4. Determine root course group and scope courses (all cycles for full ladder)
	var allCourses []sqldb.SubjectCourseV2
	if main.RootCourseGroupID.Valid {
		allCourses, err = q.CoursesByRootCourseGroup(ctx, main.RootCourseGroupID)
		if err != nil {
			return nil, fmt.Errorf("root course group lookup: %w", err)
		}
	} else {
		allCourses = []sqldb.SubjectCourseV2{
			{ID: main.CourseID, Code: main.CourseCode, Name: main.CourseName, SubjectID: main.SubjectID, CycleID: main.CycleID, Level: main.Level, RootCourseGroupID: pgtype.UUID{}},
		}
	}

	// 5. Load sit-in rule for this root course group
	if !main.RootCourseGroupID.Valid {
		return nil, nil
	}
	rule, err := q.SitInRuleGetByRootCourseGroup(ctx, main.RootCourseGroupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sit-in rule lookup: %w", err)
	}
	if rule == nil {
		return nil, nil
	}

	// 6. Parse predicate
	predicate, err := parsePredicate(rule.Predicate)
	if err != nil {
		return nil, fmt.Errorf("rule predicate parse: %w", err)
	}

	// 7. Get missed sessions (student's course sessions in range)
	missedSessions, err := q.SessionsByCourseInRange(ctx, main.CourseID, dateFrom, dateTo)
	if err != nil {
		return nil, fmt.Errorf("missed sessions lookup: %w", err)
	}

	// 8. Evaluate rule
	evalOutput, err := EvaluateRule(EvaluateRuleInput{
		RuleType:       rule.Type,
		Predicate:      predicate,
		StudentLevel:   main.Level.Int16,
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

	var result *SitInResult
	switch evalOutput.Method {
	case SitInMethodZoom:
		result = &SitInResult{SitInMethod: SitInMethodZoom}
	case SitInMethodTeacher:
		result = &SitInResult{SitInMethod: SitInMethodTeacher}
	case SitInMethodPhysical:
		if evalOutput.TargetCourseID == nil {
			return nil, fmt.Errorf("physical sit-in eligible but no target course")
		}
		targetCourseID := *evalOutput.TargetCourseID

		availSessions, err := q.SessionsByCourse(ctx, targetCourseID)
		if err != nil {
			return nil, fmt.Errorf("available sessions lookup: %w", err)
		}

		var targetCourse *sqldb.SubjectCourseV2
		for i := range allCourses {
			if allCourses[i].ID == targetCourseID {
				targetCourse = &allCourses[i]
				break
			}
		}
		if targetCourse == nil {
			return nil, fmt.Errorf("target course not found in course group")
		}

		win := loadRootGroupWindowWeeks(ctx, q, main.RootCourseGroupID)
		cutoff := time.Time{}
		if win > 0 {
			cutoff = time.Now().Add(time.Duration(win) * 7 * 24 * time.Hour)
		}
		result = buildPhysicalSitInResult(targetCourse, missedSessions, availSessions, cutoff)
	default:
		return nil, nil
	}

	result.RuleName = rule.Name
	result.RuleType = rule.Type
	return result, nil
}

func rootGroupWindowWeeks(policies []byte, rootCourseGroupID string) int {
	var p sqldb.AbsencePolicies
	if err := json.Unmarshal(policies, &p); err != nil {
		return 0
	}
	if p.RootCourseGroups == nil {
		return 0
	}
	policy, ok := p.RootCourseGroups[rootCourseGroupID]
	if !ok {
		return 0
	}
	return policy.SitInWindowWeeks
}

func loadRootGroupWindowWeeks(ctx context.Context, q *sqldb.Queries, rootCourseGroupID pgtype.UUID) int {
	if !rootCourseGroupID.Valid {
		return 0
	}
	settings, err := q.AppSettingsGetWithPolicies(ctx)
	if err != nil {
		return 0
	}
	id, err := uuidString(rootCourseGroupID)
	if err != nil {
		return 0
	}
	return rootGroupWindowWeeks(settings.AbsencePolicies, id)
}

func automaticSitInEnabled(ctx context.Context, q *sqldb.Queries, rootCourseGroupID pgtype.UUID) (bool, error) {
	settings, err := q.AppSettingsGetWithPolicies(ctx)
	if err != nil {
		return false, fmt.Errorf("policy lookup: %w", err)
	}

	var policies sqldb.AbsencePolicies
	if err := json.Unmarshal(settings.AbsencePolicies, &policies); err != nil {
		return false, fmt.Errorf("policy parse: %w", err)
	}

	enabled := true
	if policies.SitIn.AutoResolveEnabled != nil {
		enabled = *policies.SitIn.AutoResolveEnabled
	}
	if rootCourseGroupID.Valid {
		rootGroupID, err := uuidString(rootCourseGroupID)
		if err == nil {
			if policy, ok := policies.RootCourseGroups[rootGroupID]; ok {
				enabled = enabled && policy.AutoSitInEnabled
			}
		}
	}
	return enabled, nil
}

func toSessionBrief(s sqldb.SessionInRange) sessionBrief {
	idStr, _ := uuidString(s.ID)
	return sessionBrief{
		ID:      idStr,
		StartAt: s.StartAt.Time.Format(time.RFC3339),
		EndAt:   s.EndAt.Time.Format(time.RFC3339),
	}
}

func toSessionBriefForCourse(s sqldb.SessionInRange, c *sqldb.SubjectCourseV2) sessionBrief {
	brief := toSessionBrief(s)
	if c != nil {
		brief.ClassName = c.Name
		brief.CourseName = c.Name
		brief.CourseCode = c.Code
		brief.SubjectCode = c.SubjectCode
		brief.SubjectName = c.SubjectName
	}
	return brief
}

func timesOverlap(aStart, aEnd, bStart, bEnd pgtype.Timestamptz) bool {
	if !aStart.Valid || !aEnd.Valid || !bStart.Valid || !bEnd.Valid {
		return false
	}
	return aStart.Time.Before(bEnd.Time) && aEnd.Time.After(bStart.Time)
}

func uuidString(u pgtype.UUID) (string, error) {
	if !u.Valid {
		return "", fmt.Errorf("invalid uuid")
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16]), nil
}
