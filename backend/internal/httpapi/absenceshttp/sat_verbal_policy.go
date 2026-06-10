package absenceshttp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/satverbalpolicy"
)

type satVerbalCourseRule = satverbalpolicy.CourseRule

type satVerbalMappedCourse struct {
	Rule   satVerbalCourseRule
	Course sqldb.SubjectCourseV2
}

type satVerbalMissedLessonSlot struct {
	Index  int
	Missed sqldb.SessionInRange
}

type satVerbalResolveInput struct {
	Rule               *satVerbalCourseRule
	Policy             []satVerbalCourseRule
	MappedCourses      []satVerbalMappedCourse
	MissedCourse       sqldb.SubjectCourseV2
	Enrolled           []sqldb.StudentEnrolledCourseV2
	AllCourses         []sqldb.SubjectCourseV2
	MissedSessions     []sqldb.SessionInRange
	Cutoff             time.Time
	RequestTime        time.Time
	AfterPriorityLevel int
	LoadSessions       func(context.Context, pgtype.UUID) ([]sqldb.SessionInRange, error)
}

func decodeSatVerbalPolicyRules(raw []byte) ([]satVerbalCourseRule, error) {
	return satverbalpolicy.DecodeRules(raw)
}

func resolveSatVerbalPolicy(ctx context.Context, input satVerbalResolveInput) (*SitInResult, error) {
	if input.LoadSessions == nil {
		return nil, fmt.Errorf("SAT Verbal session loader is required")
	}
	rule := input.Rule
	if rule == nil {
		rule = satverbalpolicy.MatchingRule(input.Policy, input.MissedCourse.Name)
	}
	if rule == nil {
		return nil, nil
	}

	missedCourseSessions, err := input.LoadSessions(ctx, input.MissedCourse.ID)
	if err != nil {
		return nil, fmt.Errorf("missed course sessions lookup: %w", err)
	}
	missedLessonSlots := missedLessonSlotsForMissedSessions(missedCourseSessions, input.MissedSessions)
	notBefore := requestDateLowerBound(input.RequestTime)
	offered := make(map[pgtype.UUID]struct{})
	var priorities []SitInPriorityResult

	for _, priority := range rule.Priorities {
		targets := satVerbalPriorityTargets(*rule, priority, input.MissedCourse, input.Enrolled, input.AllCourses, input.MappedCourses)
		sameLessonOnly := priority.RuleType == RuleTypeCrossSection && !strings.Contains(strings.ToLower(priority.Label), "next available")
		priorityHadResult := false
		for _, target := range targets {
			targetSessions, err := input.LoadSessions(ctx, target.ID)
			if err != nil {
				return nil, fmt.Errorf("target course sessions lookup: %w", err)
			}
			available := satVerbalAvailableSessions(targetSessions, input.MissedSessions, missedLessonSlots, sameLessonOnly, notBefore, input.Cutoff, offered)
			if len(available) == 0 {
				continue
			}
			for _, session := range available {
				offered[session.ID] = struct{}{}
			}
			priorities = append(priorities, satVerbalPriorityResult(priority.Level, priority.Label, &target, available, len(input.MissedSessions)))
			priorityHadResult = true
		}
		if !priorityHadResult {
			priorities = append(priorities, satVerbalPriorityResult(priority.Level, priority.Label, nil, nil, len(input.MissedSessions)))
		}
	}

	if len(priorities) == 0 {
		return nil, nil
	}
	visiblePriorities, currentLevel, hasNext := satVerbalVisiblePriority(priorities, input.AfterPriorityLevel)
	if len(visiblePriorities) == 0 {
		return nil, nil
	}

	result := &SitInResult{
		SitInMethod:          SitInMethodPhysical,
		RuleName:             "SAT Verbal Policy",
		RuleType:             "sat_verbal_policy",
		Priorities:           visiblePriorities,
		CurrentPriorityLevel: currentLevel,
		HasNextPriority:      hasNext,
		MissedCount:          len(input.MissedSessions),
	}
	for _, missed := range input.MissedSessions {
		result.MissedSession = append(result.MissedSession, toSessionBrief(missed))
	}
	return result, nil
}

func satVerbalVisiblePriority(priorities []SitInPriorityResult, afterLevel int) ([]SitInPriorityResult, int, bool) {
	levels := make([]int, 0, len(priorities))
	seen := make(map[int]struct{}, len(priorities))
	for _, priority := range priorities {
		if priority.Level <= afterLevel {
			continue
		}
		if _, ok := seen[priority.Level]; ok {
			continue
		}
		seen[priority.Level] = struct{}{}
		levels = append(levels, priority.Level)
	}
	sort.Ints(levels)
	if len(levels) == 0 {
		return nil, 0, false
	}
	currentLevel := levels[0]
	visible := make([]SitInPriorityResult, 0, len(priorities))
	for _, priority := range priorities {
		if priority.Level == currentLevel {
			visible = append(visible, priority)
		}
	}
	return visible, currentLevel, len(levels) > 1
}

func satVerbalPriorityResult(level int, label string, target *sqldb.SubjectCourseV2, available []sqldb.SessionInRange, missedCount int) SitInPriorityResult {
	out := SitInPriorityResult{
		Level: level,
		Label: label,
	}
	if target != nil {
		targetIDStr, _ := uuidString(target.ID)
		out.SitInCourse = &SitInCourseInfo{
			ID:          targetIDStr,
			Code:        target.Code,
			Name:        target.Name,
			SubjectCode: target.SubjectCode,
			SubjectName: target.SubjectName,
		}
	}
	for _, session := range available {
		out.Available = append(out.Available, toSessionBriefForCourse(session, target))
	}
	preSelectCount := missedCount
	if preSelectCount > len(available) {
		preSelectCount = len(available)
	}
	for _, session := range available[:preSelectCount] {
		out.PreSelected = append(out.PreSelected, toSessionBriefForCourse(session, target))
	}
	return out
}

func satVerbalPriorityTargets(
	rule satverbalpolicy.CourseRule,
	priority satverbalpolicy.RulePriority,
	missed sqldb.SubjectCourseV2,
	enrolled []sqldb.StudentEnrolledCourseV2,
	allCourses []sqldb.SubjectCourseV2,
	mappedCourses []satVerbalMappedCourse,
) []sqldb.SubjectCourseV2 {
	if len(mappedCourses) > 0 {
		return satVerbalPriorityMappedTargets(rule, priority, missed, enrolled, mappedCourses)
	}
	switch priority.RuleType {
	case RuleTypeCrossSection:
		return satVerbalCrossSectionTargets(rule, priority, missed, allCourses)
	default:
		targetNames := priority.EligibleTargets
		if derived := satVerbalDerivedTargetNames(rule.CourseName, priority, missed.Name, enrolled); len(derived) > 0 {
			targetNames = derived
		}
		return satVerbalCoursesByTargetNames(targetNames, missed.ID, allCourses)
	}
}

func satVerbalPriorityMappedTargets(
	rule satverbalpolicy.CourseRule,
	priority satverbalpolicy.RulePriority,
	missed sqldb.SubjectCourseV2,
	enrolled []sqldb.StudentEnrolledCourseV2,
	mappedCourses []satVerbalMappedCourse,
) []sqldb.SubjectCourseV2 {
	switch priority.RuleType {
	case RuleTypeCrossSection:
		return satVerbalMappedCrossSectionTargets(rule, priority, missed, mappedCourses)
	default:
		targetNames := priority.EligibleTargets
		if derived := satVerbalDerivedTargetNames(rule.CourseName, priority, missed.Name, enrolled); len(derived) > 0 {
			targetNames = derived
		}
		return satVerbalMappedCoursesByTargetNames(targetNames, missed.ID, mappedCourses)
	}
}

func satVerbalMappedCrossSectionTargets(
	rule satverbalpolicy.CourseRule,
	priority satverbalpolicy.RulePriority,
	missed sqldb.SubjectCourseV2,
	mappedCourses []satVerbalMappedCourse,
) []sqldb.SubjectCourseV2 {
	missedSection := satverbalpolicy.DisplaySection(rule.CourseName)
	targets := priority.MakeupTargets
	if len(priority.SectionTargets) > 0 && missedSection != "" {
		if sectionTargets, ok := priority.SectionTargets[missedSection]; ok {
			targets = sectionTargets
		}
	}

	family := satverbalpolicy.FamilyName(rule.CourseName)
	var out []sqldb.SubjectCourseV2
	for _, target := range targets {
		if strings.EqualFold(strings.TrimSpace(target.Section), "Next available") {
			for _, mapped := range mappedCourses {
				if mapped.Course.ID == missed.ID {
					continue
				}
				if satverbalpolicy.FamilyName(mapped.Rule.CourseName) == family {
					out = append(out, mapped.Course)
				}
			}
			continue
		}
		targetSection := strings.ToLower(strings.TrimSpace(target.Section))
		for _, mapped := range mappedCourses {
			if mapped.Course.ID == missed.ID {
				continue
			}
			if satverbalpolicy.FamilyName(mapped.Rule.CourseName) != family {
				continue
			}
			if satverbalpolicy.ExtractSection(mapped.Rule.CourseName) == targetSection {
				out = append(out, mapped.Course)
			}
		}
	}
	return uniqueCourses(out)
}

func satVerbalCrossSectionTargets(
	rule satverbalpolicy.CourseRule,
	priority satverbalpolicy.RulePriority,
	missed sqldb.SubjectCourseV2,
	allCourses []sqldb.SubjectCourseV2,
) []sqldb.SubjectCourseV2 {
	missedSection := satverbalpolicy.DisplaySection(missed.Name)
	targets := priority.MakeupTargets
	if len(priority.SectionTargets) > 0 && missedSection != "" {
		if sectionTargets, ok := priority.SectionTargets[missedSection]; ok {
			targets = sectionTargets
		}
	}

	family := satverbalpolicy.FamilyName(missed.Name)
	var out []sqldb.SubjectCourseV2
	for _, target := range targets {
		if strings.EqualFold(strings.TrimSpace(target.Section), "Next available") {
			for _, course := range allCourses {
				if course.ID == missed.ID {
					continue
				}
				if satverbalpolicy.FamilyName(course.Name) == family {
					out = append(out, course)
				}
			}
			continue
		}
		targetSection := strings.ToLower(strings.TrimSpace(target.Section))
		for _, course := range allCourses {
			if course.ID == missed.ID {
				continue
			}
			if satverbalpolicy.FamilyName(course.Name) != family {
				continue
			}
			if satverbalpolicy.ExtractSection(course.Name) == targetSection {
				out = append(out, course)
			}
		}
	}
	return uniqueCourses(out)
}

func satVerbalDerivedTargetNames(ruleCourseName string, priority satverbalpolicy.RulePriority, missedCourseName string, enrolled []sqldb.StudentEnrolledCourseV2) []string {
	ruleName := satverbalpolicy.NormalizeName(ruleCourseName)
	if ruleName == "sat verbal brush up" {
		if priority.Level != 1 {
			return nil
		}
		main := satVerbalMainCourse(enrolled, missedCourseName)
		mainName := satverbalpolicy.NormalizeName(main)
		switch {
		case strings.Contains(mainName, "reading rank 4"):
			return []string{"SAT Verbal Reading Rank 5", "SAT Reading Rank 5"}
		case strings.Contains(mainName, "writing rank 4"):
			return []string{"SAT Verbal Writing Rank 5", "SAT Writing Rank 5"}
		case strings.Contains(mainName, "reading rank 5"):
			return []string{"SAT Verbal Reading Rank 4", "SAT Reading Rank 4"}
		case strings.Contains(mainName, "writing rank 5"):
			return []string{"SAT Verbal Writing Rank 4", "SAT Writing Rank 4"}
		}
		return nil
	}
	if ruleName == "sat verbal real time practice" {
		switch satVerbalRank(satVerbalMainCourse(enrolled, missedCourseName)) {
		case 3:
			return []string{"SAT Verbal Rank 2"}
		case 2:
			return []string{"SAT Verbal Rank 1"}
		case 1:
			return []string{"SAT Verbal Rank 2"}
		}
		return nil
	}
	switch ruleName {
	case "reading mastery", "writing wisdom", "sat verbal knock out", "sat verbal intensive", "sat verbal believe":
		for _, course := range enrolled {
			rank := satVerbalRank(course.CourseName)
			if rank == 3 {
				return []string{"SAT Verbal Rank 2"}
			}
			if rank == 2 {
				return []string{"SAT Verbal Rank 3"}
			}
		}
	}
	return nil
}

func satVerbalMainCourse(enrolled []sqldb.StudentEnrolledCourseV2, missedCourseName string) string {
	for _, course := range enrolled {
		if !satverbalpolicy.CourseMatchesRule(satverbalpolicy.CourseRule{CourseName: missedCourseName}, course.CourseName) {
			return course.CourseName
		}
	}
	if len(enrolled) > 0 {
		return enrolled[0].CourseName
	}
	return missedCourseName
}

func satVerbalRank(name string) int {
	n := satverbalpolicy.NormalizeName(name)
	for _, rank := range []int{5, 4, 3, 2, 1} {
		if strings.Contains(n, fmt.Sprintf("rank %d", rank)) {
			return rank
		}
	}
	return 0
}

func satVerbalCoursesByTargetNames(targetNames []string, missedID pgtype.UUID, allCourses []sqldb.SubjectCourseV2) []sqldb.SubjectCourseV2 {
	var out []sqldb.SubjectCourseV2
	for _, targetName := range targetNames {
		targetAliases := satverbalpolicy.NameAliases(targetName)
		targetFamily := satverbalpolicy.FamilyName(targetName)
		targetSection := satverbalpolicy.ExtractSection(targetName)
		for _, course := range allCourses {
			if course.ID == missedID {
				continue
			}
			courseAliases := satverbalpolicy.NameAliases(course.Name)
			matches := false
			for _, ca := range courseAliases {
				for _, ta := range targetAliases {
					if ca == ta {
						matches = true
						break
					}
				}
				if matches {
					break
				}
			}
			if !matches && targetSection == "" && targetFamily != "" && satverbalpolicy.FamilyName(course.Name) == targetFamily {
				matches = true
			}
			if matches {
				out = append(out, course)
			}
		}
	}
	return uniqueCourses(out)
}

func satVerbalMappedCoursesByTargetNames(targetNames []string, missedID pgtype.UUID, mappedCourses []satVerbalMappedCourse) []sqldb.SubjectCourseV2 {
	var out []sqldb.SubjectCourseV2
	for _, targetName := range targetNames {
		targetFamily := satverbalpolicy.FamilyName(targetName)
		targetSection := satverbalpolicy.ExtractSection(targetName)
		for _, mapped := range mappedCourses {
			if mapped.Course.ID == missedID {
				continue
			}
			if satverbalpolicy.CourseMatchesRule(mapped.Rule, targetName) {
				out = append(out, mapped.Course)
				continue
			}
			if targetSection == "" && targetFamily != "" && satverbalpolicy.FamilyName(mapped.Rule.CourseName) == targetFamily {
				out = append(out, mapped.Course)
			}
		}
	}
	return uniqueCourses(out)
}

func satVerbalAvailableSessions(
	targetSessions []sqldb.SessionInRange,
	missedSessions []sqldb.SessionInRange,
	missedLessonSlots []satVerbalMissedLessonSlot,
	sameLessonOnly bool,
	notBefore time.Time,
	cutoff time.Time,
	offered map[pgtype.UUID]struct{},
) []sqldb.SessionInRange {
	sessions := sortedSessions(targetSessions)
	finalID := pgtype.UUID{}
	if len(sessions) > 0 {
		finalID = sessions[len(sessions)-1].ID
	}
	var out []sqldb.SessionInRange
	if sameLessonOnly {
		for _, slot := range missedLessonSlots {
			if slot.Index < 0 || slot.Index >= len(sessions) {
				continue
			}
			session := sessions[slot.Index]
			if satVerbalSessionAllowed(session, finalID, missedSessions, notBefore, cutoff, offered) {
				out = append(out, session)
			}
		}
		return out
	}
	for _, session := range sessions {
		if satVerbalSessionAllowed(session, finalID, missedSessions, notBefore, cutoff, offered) {
			out = append(out, session)
		}
	}
	return out
}

func satVerbalSessionAllowed(
	session sqldb.SessionInRange,
	finalID pgtype.UUID,
	missedSessions []sqldb.SessionInRange,
	notBefore time.Time,
	cutoff time.Time,
	offered map[pgtype.UUID]struct{},
) bool {
	if finalID.Valid && session.ID == finalID {
		return false
	}
	if _, ok := offered[session.ID]; ok {
		return false
	}
	if !notBefore.IsZero() && session.StartAt.Time.Before(notBefore) {
		return false
	}
	if !cutoff.IsZero() && session.StartAt.Time.After(cutoff) {
		return false
	}
	for _, missed := range missedSessions {
		if timesOverlap(session.StartAt, session.EndAt, missed.StartAt, missed.EndAt) {
			return false
		}
	}
	return true
}

func missedLessonSlotsForMissedSessions(allCourseSessions []sqldb.SessionInRange, missedSessions []sqldb.SessionInRange) []satVerbalMissedLessonSlot {
	sessions := sortedSessions(allCourseSessions)
	indexByID := make(map[pgtype.UUID]int, len(sessions))
	for i, session := range sessions {
		indexByID[session.ID] = i
	}
	var slots []satVerbalMissedLessonSlot
	for _, missed := range missedSessions {
		if idx, ok := indexByID[missed.ID]; ok {
			slots = append(slots, satVerbalMissedLessonSlot{Index: idx, Missed: missed})
		}
	}
	return slots
}

func requestDateLowerBound(requestTime time.Time) time.Time {
	start := requestTime
	if start.IsZero() {
		return time.Time{}
	}
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
}

func sortedSessions(sessions []sqldb.SessionInRange) []sqldb.SessionInRange {
	out := append([]sqldb.SessionInRange(nil), sessions...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartAt.Time.Before(out[j].StartAt.Time)
	})
	return out
}

func subjectWindowWeeks(policies []byte, subjectID, fallbackRootCourseGroupID string) int {
	var p sqldb.AbsencePolicies
	if err := json.Unmarshal(policies, &p); err != nil {
		return 0
	}
	if p.Subjects != nil {
		if policy, ok := p.Subjects[subjectID]; ok && policy.SitInWindowWeeks > 0 {
			return policy.SitInWindowWeeks
		}
	}
	if fallbackRootCourseGroupID != "" {
		return rootGroupWindowWeeks(policies, fallbackRootCourseGroupID)
	}
	return 0
}

func uniqueCourses(courses []sqldb.SubjectCourseV2) []sqldb.SubjectCourseV2 {
	seen := make(map[pgtype.UUID]struct{}, len(courses))
	out := make([]sqldb.SubjectCourseV2, 0, len(courses))
	for _, course := range courses {
		if _, ok := seen[course.ID]; ok {
			continue
		}
		seen[course.ID] = struct{}{}
		out = append(out, course)
	}
	return out
}
