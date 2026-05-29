package absenceshttp

import (
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type RulePredicate struct {
	Level1Action         string      `json:"level_1_action"`
	NonMaxDirection      string      `json:"non_max_direction"`
	MaxDirection         string      `json:"max_direction"`
	MinLevelForSitLower  int16       `json:"min_level_for_sit_lower"`
	SectionMatch         string      `json:"section_match"`
	OccurrenceMatch      string      `json:"occurrence_match"`
	DayMatch             string      `json:"day_match"`
	LastClassExcluded    bool        `json:"last_class_excluded"`
	ScheduleSource       string      `json:"schedule_source"`
	Chains               []RankChain `json:"chains"`
	AutoAssign           bool        `json:"auto_assign"`
	RequiresTeacherApproval bool     `json:"requires_teacher_approval"`
}

type RankChain struct {
	FromRank int16 `json:"from_rank"`
	ToRank   int16 `json:"to_rank"`
}

type EvaluateRuleInput struct {
	RuleType     string
	Predicate    RulePredicate
	StudentLevel int16
	AllCourses   []sqldb.SubjectCourseV2
	MissedCount  int
}

type EvaluateRuleOutput struct {
	Eligible       bool
	Method         string       // "zoom", "physical", "teacher_case", "none"
	TargetCourseID *pgtype.UUID
	Direction      string       // "higher", "lower", "same_section", "any_day", "chain"
	Reason         string
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
	default:
		return nil, fmt.Errorf("unknown rule type: %s", input.RuleType)
	}
}

func evaluateLevelLadder(input EvaluateRuleInput) (*EvaluateRuleOutput, error) {
	if input.StudentLevel == 1 && input.Predicate.Level1Action == SitInMethodZoom {
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

	if isTopLevel {
		return evaluateLevelLadderLower(input, maxLevel)
	}
	return evaluateLevelLadderHigher(input)
}

func evaluateLevelLadderHigher(input EvaluateRuleInput) (*EvaluateRuleOutput, error) {
	var target *sqldb.SubjectCourseV2
	for i := range input.AllCourses {
		c := &input.AllCourses[i]
		if c.Level.Valid && c.Level.Int16 > input.StudentLevel {
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

func evaluateLevelLadderLower(input EvaluateRuleInput, maxLevel int16) (*EvaluateRuleOutput, error) {
	var target *sqldb.SubjectCourseV2
	for i := range input.AllCourses {
		c := &input.AllCourses[i]
		if c.Level.Valid && c.Level.Int16 < input.StudentLevel && c.Level.Int16 >= input.Predicate.MinLevelForSitLower {
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
