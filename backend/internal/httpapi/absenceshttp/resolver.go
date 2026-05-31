package absenceshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type SitInResult struct {
	SitInMethod string `json:"sit_in_method"` // "physical" or "zoom"

	// Rule metadata
	RuleName string `json:"rule_name,omitempty"`
	RuleType string `json:"rule_type,omitempty"`

	// For physical sit-in
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
	ID          string `json:"id"`
	StartAt     string `json:"start_at"`
	EndAt       string `json:"end_at"`
	ClassName   string `json:"class_name,omitempty"`
	CourseName  string `json:"course_name,omitempty"`
	CourseCode  string `json:"course_code,omitempty"`
	SubjectCode string `json:"subject_code,omitempty"`
	SubjectName string `json:"subject_name,omitempty"`
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
		if !overlaps {
			nonOverlapping = append(nonOverlapping, a)
		}
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

// resolveSitInForCourse resolves sit-in for a specific student course block.
// Uses the student's highest enrolled level for the ladder target, but missed
// sessions come from the given courseID. Loads all ladder levels (any cycle)
// so mixed-cycle enrollments resolve correctly.
func resolveSitInForCourse(ctx context.Context, q *sqldb.Queries, wcode string, courseID, subjectID pgtype.UUID, dateFrom, dateTo time.Time) (*SitInResult, error) {
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

	// Find highest enrolled level (determines ladder target)
	var highest *sqldb.StudentEnrolledCourseV2
	for i := range enrolled {
		if !enrolled[i].Level.Valid {
			continue
		}
		if highest == nil || enrolled[i].Level.Int16 > highest.Level.Int16 {
			highest = &enrolled[i]
		}
	}
	if highest == nil {
		return nil, fmt.Errorf("no enrolled course has a level")
	}

	if !highest.RootCourseGroupID.Valid {
		return nil, nil
	}

	allCourses, err := q.CoursesByRootCourseGroup(ctx, highest.RootCourseGroupID)
	if err != nil {
		return nil, fmt.Errorf("root course group lookup: %w", err)
	}

	rule, err := q.SitInRuleGetByRootCourseGroup(ctx, highest.RootCourseGroupID)
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

	missedSessions, err := q.SessionsByCourseInRange(ctx, courseID, dateFrom, dateTo)
	if err != nil {
		return nil, fmt.Errorf("missed sessions lookup: %w", err)
	}

	evalOutput, err := EvaluateRule(EvaluateRuleInput{
		RuleType:     rule.Type,
		Predicate:    predicate,
		StudentLevel: highest.Level.Int16,
		AllCourses:   allCourses,
		MissedCount:  len(missedSessions),
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

		availSessions, err := q.SessionsByCourseInRange(ctx, targetCourseID, dateFrom, dateTo)
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

		result = buildPhysicalSitInResult(targetCourse, missedSessions, availSessions)
	default:
		return nil, nil
	}

	result.RuleName = rule.Name
	result.RuleType = rule.Type
	return result, nil
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

	// 3. Pick main course (highest level)
	main := enrolled[0]
	for _, c := range enrolled {
		if c.Level.Valid && main.Level.Valid && c.Level.Int16 > main.Level.Int16 {
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
		RuleType:     rule.Type,
		Predicate:    predicate,
		StudentLevel: main.Level.Int16,
		AllCourses:   allCourses,
		MissedCount:  len(missedSessions),
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

		availSessions, err := q.SessionsByCourseInRange(ctx, targetCourseID, dateFrom, dateTo)
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

		result = buildPhysicalSitInResult(targetCourse, missedSessions, availSessions)
	default:
		return nil, nil
	}

	result.RuleName = rule.Name
	result.RuleType = rule.Type
	return result, nil
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
