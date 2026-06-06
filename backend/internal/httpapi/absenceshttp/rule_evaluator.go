package absenceshttp

import (
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

const RuleTypeSatVerbalPriority = "sat_verbal_priority"

type PriorityRow struct {
	MissedRank       int16  `json:"missed_rank"`
	MissedSection    string `json:"missed_section,omitempty"`
	MissedCourseID   string `json:"missed_course_id,omitempty"`
	Priority         int16  `json:"priority"` // 1, 2, or 3
	RuleType         string `json:"rule_type"` // "cross_section", "rank_chain", "any_day_except_last"
	TargetRank       *int16 `json:"target_rank,omitempty"`
	TargetSection    string `json:"target_section,omitempty"`
	TargetCourseID   string `json:"target_course_id,omitempty"`
	OccurrenceMatch  string `json:"occurrence_match,omitempty"` // "same", "any"
	DayMatch         string `json:"day_match,omitempty"` // "same_day", "any_day"
	LastClassExcluded bool `json:"last_class_excluded"`
	Label            string `json:"label,omitempty"`
}

type RulePredicate struct {
	Level1Action            string      `json:"level_1_action"`
	NonMaxDirection         string      `json:"non_max_direction"`
	MaxDirection            string      `json:"max_direction"`
	MinLevelForSitLower     int16       `json:"min_level_for_sit_lower"`
	SectionMatch            string      `json:"section_match"`
	OccurrenceMatch         string      `json:"occurrence_match"`
	DayMatch                string      `json:"day_match"`
	LastClassExcluded       bool        `json:"last_class_excluded"`
	ScheduleSource          string      `json:"schedule_source"`
	Chains                  []RankChain `json:"chains"`
	AutoAssign              bool        `json:"auto_assign"`
	RequiresTeacherApproval bool        `json:"requires_teacher_approval"`

	// sat_verbal_priority fields
	PriorityRows []PriorityRow `json:"priority_rows,omitempty"`
}

type RankChain struct {
	FromRank int16 `json:"from_rank"`
	ToRank   int16 `json:"to_rank"`
}

type EvaluateRuleInput struct {
	RuleType       string
	Predicate      RulePredicate
	StudentLevel   int16
	EnrolledLevels []int16
	AllCourses     []sqldb.SubjectCourseV2
	MissedCount    int
}

type EvaluateRuleOutput struct {
	Eligible       bool
	Method         string // "zoom", "physical", "teacher_case", "none"
	TargetCourseID *pgtype.UUID
	Direction      string // "higher", "lower", "same_section", "any_day", "chain", "priority"
	Reason         string
	PriorityRows   []PriorityRow // populated by sat_verbal_priority evaluator
}

func parsePredicate(raw []byte) (RulePredicate, error) {
	var p RulePredicate
	if err := json.Unmarshal(raw, &p); err != nil {
		return RulePredicate{}, fmt.Errorf("parse predicate: %w", err)
	}
	return p, nil
}

func EvaluateRule(input EvaluateRuleInput) (*EvaluateRuleOutput, error) {
	switch input.RuleType {
	case RuleTypeLevelLadder:
		return evaluateLevelLadder(input)
	case RuleTypeCrossSection:
		return evaluateCrossSection(input)
	case RuleTypeAnyDayExceptLast:
		return evaluateAnyDayExceptLast(input)
	case RuleTypeRankChain:
		return evaluateRankChain(input)
	case RuleTypeTeacherCase:
		return evaluateTeacherCaseByCase(input)
	case RuleTypeSatVerbalPriority:
		return evaluateSatVerbalPriority(input)
	default:
		return nil, fmt.Errorf("unknown rule type: %s", input.RuleType)
	}
}

func buildEnrolledLevelSet(input EvaluateRuleInput) map[int16]struct{} {
	enrolledLevels := make(map[int16]struct{}, len(input.EnrolledLevels)+1)
	if len(input.EnrolledLevels) > 0 {
		for _, level := range input.EnrolledLevels {
			if level > 0 {
				enrolledLevels[level] = struct{}{}
			}
		}
	}
	if input.StudentLevel > 0 {
		enrolledLevels[input.StudentLevel] = struct{}{}
	}
	return enrolledLevels
}

func evaluateLevelLadder(input EvaluateRuleInput) (*EvaluateRuleOutput, error) {
	// Level 1 always gets Zoom — no physical sit-in
	if input.StudentLevel == 1 {
		return &EvaluateRuleOutput{
			Eligible: true,
			Method:   SitInMethodZoom,
			Reason:   "Level 1 students attend via zoom",
		}, nil
	}

	maxLevel := int16(0)
	for _, c := range input.AllCourses {
		if c.Level.Valid && c.Level.Int16 > maxLevel {
			maxLevel = c.Level.Int16
		}
	}

	isTopLevel := input.StudentLevel >= maxLevel
	enrolledLevels := buildEnrolledLevelSet(input)

	if isTopLevel {
		return evaluateLevelLadderLower(input, maxLevel, enrolledLevels)
	}
	return evaluateLevelLadderHigher(input, enrolledLevels)
}

func evaluateLevelLadderHigher(input EvaluateRuleInput, enrolledLevels map[int16]struct{}) (*EvaluateRuleOutput, error) {
	var target *sqldb.SubjectCourseV2
	for i := range input.AllCourses {
		c := &input.AllCourses[i]
		if c.Level.Valid && c.Level.Int16 > input.StudentLevel {
			if _, ok := enrolledLevels[c.Level.Int16]; ok {
				continue
			}
			if target == nil || c.Level.Int16 < target.Level.Int16 {
				target = c
			}
		}
	}

	if target == nil {
		return &EvaluateRuleOutput{
			Eligible: false,
			Method:   SitInMethodNone,
			Reason:   "no higher-level course available",
		}, nil
	}

	return &EvaluateRuleOutput{
		Eligible:       true,
		Method:         SitInMethodPhysical,
		TargetCourseID: &target.ID,
		Direction:      "higher",
		Reason:         fmt.Sprintf("sit in level %d (higher from level %d)", target.Level.Int16, input.StudentLevel),
	}, nil
}

func evaluateLevelLadderLower(input EvaluateRuleInput, maxLevel int16, enrolledLevels map[int16]struct{}) (*EvaluateRuleOutput, error) {
	var target *sqldb.SubjectCourseV2
	for i := range input.AllCourses {
		c := &input.AllCourses[i]
		if c.Level.Valid && c.Level.Int16 < input.StudentLevel && c.Level.Int16 >= input.Predicate.MinLevelForSitLower {
			if _, ok := enrolledLevels[c.Level.Int16]; ok {
				continue
			}
			if target == nil || c.Level.Int16 > target.Level.Int16 {
				target = c
			}
		}
	}

	if target == nil {
		return &EvaluateRuleOutput{
			Eligible: false,
			Method:   SitInMethodNone,
			Reason:   "no suitable lower-level course available",
		}, nil
	}

	return &EvaluateRuleOutput{
		Eligible:       true,
		Method:         SitInMethodPhysical,
		TargetCourseID: &target.ID,
		Direction:      "lower",
		Reason:         fmt.Sprintf("sit in level %d (lower from top level %d)", target.Level.Int16, maxLevel),
	}, nil
}

func evaluateCrossSection(input EvaluateRuleInput) (*EvaluateRuleOutput, error) {
	if len(input.AllCourses) < 2 {
		return &EvaluateRuleOutput{
			Eligible: false,
			Method:   SitInMethodNone,
			Reason:   "no sibling courses for cross-section sit-in",
		}, nil
	}

	var target *sqldb.SubjectCourseV2
	for i := range input.AllCourses {
		c := &input.AllCourses[i]
		if c.Level.Valid && c.Level.Int16 == input.StudentLevel {
			if target == nil {
				target = c
			}
		}
	}

	if target == nil {
		return &EvaluateRuleOutput{
			Eligible: false,
			Method:   SitInMethodNone,
			Reason:   "no sibling course at same level",
		}, nil
	}

	return &EvaluateRuleOutput{
		Eligible:       true,
		Method:         SitInMethodPhysical,
		TargetCourseID: &target.ID,
		Direction:      "same_section",
		Reason:         "cross-section sit-in with sibling courses",
	}, nil
}

func evaluateAnyDayExceptLast(input EvaluateRuleInput) (*EvaluateRuleOutput, error) {
	if len(input.AllCourses) == 0 {
		return &EvaluateRuleOutput{
			Eligible: false,
			Method:   SitInMethodNone,
			Reason:   "no courses available",
		}, nil
	}

	var target *sqldb.SubjectCourseV2
	for i := range input.AllCourses {
		c := &input.AllCourses[i]
		if c.Level.Valid && c.Level.Int16 != input.StudentLevel {
			if target == nil || c.Level.Int16 > target.Level.Int16 {
				target = c
			}
		}
	}

	if target == nil {
		target = &input.AllCourses[0]
	}

	return &EvaluateRuleOutput{
		Eligible:       true,
		Method:         SitInMethodPhysical,
		TargetCourseID: &target.ID,
		Direction:      "any_day",
		Reason:         "any day except last class",
	}, nil
}

func evaluateRankChain(input EvaluateRuleInput) (*EvaluateRuleOutput, error) {
	rank := input.StudentLevel

	for _, chain := range input.Predicate.Chains {
		if rank == chain.FromRank {
			for _, c := range input.AllCourses {
				if c.Level.Valid && c.Level.Int16 == chain.ToRank {
					return &EvaluateRuleOutput{
						Eligible:       true,
						Method:         SitInMethodPhysical,
						TargetCourseID: &c.ID,
						Direction:      "chain",
						Reason:         fmt.Sprintf("rank chain %d→%d matched", chain.FromRank, chain.ToRank),
					}, nil
				}
			}
		}
	}

	return &EvaluateRuleOutput{
		Eligible: false,
		Method:   SitInMethodNone,
		Reason:   "no matching rank chain found",
	}, nil
}

func evaluateTeacherCaseByCase(input EvaluateRuleInput) (*EvaluateRuleOutput, error) {
	return &EvaluateRuleOutput{
		Eligible: true,
		Method:   SitInMethodTeacher,
		Reason:   "sit-in requires teacher approval on case-by-case basis",
	}, nil
}

func evaluateSatVerbalPriority(input EvaluateRuleInput) (*EvaluateRuleOutput, error) {
	// Match priority rows by student level (rank)
	matchedRows := make([]PriorityRow, 0)
	for _, row := range input.Predicate.PriorityRows {
		if row.MissedRank == input.StudentLevel {
			matchedRows = append(matchedRows, row)
		}
	}

	if len(matchedRows) == 0 {
		return &EvaluateRuleOutput{
			Eligible: false,
			Method:   SitInMethodNone,
			Reason:   "no sat_verbal_priority row matches this rank",
		}, nil
	}

	return &EvaluateRuleOutput{
		Eligible:     true,
		Method:       SitInMethodPhysical,
		Direction:    "priority",
		Reason:       fmt.Sprintf("matched %d priority rows for rank %d", len(matchedRows), input.StudentLevel),
		PriorityRows: matchedRows,
	}, nil
}
