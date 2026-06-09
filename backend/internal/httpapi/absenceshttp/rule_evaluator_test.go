package absenceshttp

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func mustParsePredicate(t *testing.T, raw string) RulePredicate {
	t.Helper()
	var p RulePredicate
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("predicate parse: %v", err)
	}
	return p
}

func courseV2(id string, level int16) sqldb.SubjectCourseV2 {
	return sqldb.SubjectCourseV2{
		ID:    uuidFromString(id),
		Level: pgtype.Int2{Int16: level, Valid: true},
	}
}

func uuidFromString(s string) pgtype.UUID {
	u := pgtype.UUID{}
	copy(u.Bytes[:], []byte(s))
	u.Valid = true
	return u
}

// Test 1: Level Ladder — Level 1 returns zoom
func TestEvaluateRule_LevelLadder_Level1_ReturnsZoom(t *testing.T) {
	predicate := mustParsePredicate(t, `{
		"level_1_action": "zoom",
		"non_max_direction": "higher",
		"max_direction": "lower",
		"min_level_for_sit_lower": 2,
		"section_match": "same_section",
		"occurrence_match": "any",
		"day_match": "any",
		"last_class_excluded": false,
		"schedule_source": "target",
		"chains": [],
		"auto_assign": true,
		"requires_teacher_approval": false
	}`)

	input := EvaluateRuleInput{
		RuleType:     "level_ladder",
		Predicate:    predicate,
		StudentLevel: 1,
		AllCourses:   []sqldb.SubjectCourseV2{courseV2("10000000-0000-0000-0000-000000000001", 1)},
		MissedCount:  1,
	}

	output, err := EvaluateRule(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Eligible {
		t.Fatal("expected eligible=true")
	}
	if output.Method != "zoom" {
		t.Fatalf("expected method=zoom, got %s", output.Method)
	}
	if output.TargetCourseID != nil {
		t.Fatal("expected nil target course for zoom")
	}
}

// Test 1b: Level Ladder — Level 1 ALWAYS returns zoom, even with level_1_action="physical"
func TestEvaluateRule_LevelLadder_Level1_AlwaysZoom(t *testing.T) {
	predicate := mustParsePredicate(t, `{
		"level_1_action": "physical",
		"non_max_direction": "higher",
		"max_direction": "lower",
		"min_level_for_sit_lower": 2,
		"section_match": "same_section",
		"occurrence_match": "any",
		"day_match": "any",
		"last_class_excluded": false,
		"schedule_source": "target",
		"chains": [],
		"auto_assign": true,
		"requires_teacher_approval": false
	}`)

	input := EvaluateRuleInput{
		RuleType:     "level_ladder",
		Predicate:    predicate,
		StudentLevel: 1,
		AllCourses: []sqldb.SubjectCourseV2{
			courseV2("10000000-0000-0000-0000-000000000001", 1),
			courseV2("20000000-0000-0000-0000-000000000002", 2),
			courseV2("30000000-0000-0000-0000-000000000003", 3),
		},
		MissedCount:  1,
	}

	output, err := EvaluateRule(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Eligible {
		t.Fatal("expected eligible=true")
	}
	if output.Method != "zoom" {
		t.Fatalf("expected method=zoom for Level 1 regardless of level_1_action, got %s", output.Method)
	}
	if output.TargetCourseID != nil {
		t.Fatal("expected nil target course for zoom")
	}
}

// Test 2: Level Ladder — Level 2 sits higher
func TestEvaluateRule_LevelLadder_SitsHigher(t *testing.T) {
	c2 := "20000000-0000-0000-0000-000000000002"
	c3 := "30000000-0000-0000-0000-000000000003"

	predicate := mustParsePredicate(t, `{
		"level_1_action": "zoom",
		"non_max_direction": "higher",
		"max_direction": "lower",
		"min_level_for_sit_lower": 2,
		"section_match": "same_section",
		"occurrence_match": "any",
		"day_match": "any",
		"last_class_excluded": false,
		"schedule_source": "target",
		"chains": [],
		"auto_assign": true,
		"requires_teacher_approval": false
	}`)

	input := EvaluateRuleInput{
		RuleType:     "level_ladder",
		Predicate:    predicate,
		StudentLevel: 2,
		AllCourses: []sqldb.SubjectCourseV2{
			courseV2("10000000-0000-0000-0000-000000000001", 1),
			courseV2(c2, 2),
			courseV2(c3, 3),
		},
		MissedCount: 1,
	}

	output, err := EvaluateRule(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Eligible {
		t.Fatal("expected eligible=true")
	}
	if output.Direction != "higher" {
		t.Fatalf("expected direction=higher, got %s", output.Direction)
	}
	if output.TargetCourseID == nil {
		t.Fatal("expected target course to be set")
	}
	if *output.TargetCourseID != uuidFromString(c3) {
		t.Fatalf("expected target course %s, got %v", c3, output.TargetCourseID)
	}
}

// Test 3: Level Ladder — Already-enrolled levels are skipped in both directions
func TestEvaluateRule_LevelLadder_SkipsAlreadyEnrolledLevels(t *testing.T) {
	predicate := mustParsePredicate(t, `{
		"level_1_action": "zoom",
		"non_max_direction": "higher",
		"max_direction": "lower",
		"min_level_for_sit_lower": 2,
		"section_match": "same_section",
		"occurrence_match": "any",
		"day_match": "any",
		"last_class_excluded": false,
		"schedule_source": "target",
		"chains": [],
		"auto_assign": true,
		"requires_teacher_approval": false
	}`)

	allCourses := []sqldb.SubjectCourseV2{
		courseV2("10000000-0000-0000-0000-000000000001", 1),
		courseV2("20000000-0000-0000-0000-000000000002", 2),
		courseV2("30000000-0000-0000-0000-000000000003", 3),
		courseV2("40000000-0000-0000-0000-000000000004", 4),
		courseV2("50000000-0000-0000-0000-000000000005", 5),
	}

	t.Run("student at level 3 still sits in level 4", func(t *testing.T) {
		output, err := EvaluateRule(EvaluateRuleInput{
			RuleType:       "level_ladder",
			Predicate:      predicate,
			StudentLevel:   3,
			EnrolledLevels: []int16{3},
			AllCourses:     allCourses,
			MissedCount:    1,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !output.Eligible {
			t.Fatal("expected eligible=true")
		}
		if output.Direction != "higher" {
			t.Fatalf("expected direction=higher, got %s", output.Direction)
		}
		if output.TargetCourseID == nil {
			t.Fatal("expected target course to be set")
		}
		if *output.TargetCourseID != uuidFromString("40000000-0000-0000-0000-000000000004") {
			t.Fatalf("expected target course level 4, got %v", output.TargetCourseID)
		}
	})

	t.Run("student at level 3 skips enrolled level 4 and goes to level 5", func(t *testing.T) {
		output, err := EvaluateRule(EvaluateRuleInput{
			RuleType:       "level_ladder",
			Predicate:      predicate,
			StudentLevel:   3,
			EnrolledLevels: []int16{3, 4},
			AllCourses:     allCourses,
			MissedCount:    1,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !output.Eligible {
			t.Fatal("expected eligible=true")
		}
		if output.Direction != "higher" {
			t.Fatalf("expected direction=higher, got %s", output.Direction)
		}
		if output.TargetCourseID == nil {
			t.Fatal("expected target course to be set")
		}
		if *output.TargetCourseID != uuidFromString("50000000-0000-0000-0000-000000000005") {
			t.Fatalf("expected target course level 5, got %v", output.TargetCourseID)
		}
	})

	t.Run("student at level 5 with level 4 enrolled skips to level 3", func(t *testing.T) {
		output, err := EvaluateRule(EvaluateRuleInput{
			RuleType:       "level_ladder",
			Predicate:      predicate,
			StudentLevel:   5,
			EnrolledLevels: []int16{4, 5},
			AllCourses:     allCourses,
			MissedCount:    1,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !output.Eligible {
			t.Fatal("expected eligible=true")
		}
		if output.Direction != "lower" {
			t.Fatalf("expected direction=lower, got %s", output.Direction)
		}
		if output.TargetCourseID == nil {
			t.Fatal("expected target course to be set")
		}
		if *output.TargetCourseID != uuidFromString("30000000-0000-0000-0000-000000000003") {
			t.Fatalf("expected target course level 3, got %v", output.TargetCourseID)
		}
	})

	t.Run("student at level 3 falls back to the nearest lower non-enrolled level when the higher target is already enrolled", func(t *testing.T) {
		output, err := EvaluateRule(EvaluateRuleInput{
			RuleType:       "level_ladder",
			Predicate:      predicate,
			StudentLevel:   3,
			EnrolledLevels: []int16{3, 4},
			AllCourses: []sqldb.SubjectCourseV2{
				courseV2("10000000-0000-0000-0000-000000000001", 2),
				courseV2("20000000-0000-0000-0000-000000000002", 3),
				courseV2("30000000-0000-0000-0000-000000000003", 4),
			},
			MissedCount: 1,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !output.Eligible {
			t.Fatal("expected eligible=true")
		}
		if output.Method != "physical" {
			t.Fatalf("expected method=physical, got %s", output.Method)
		}
		if output.Direction != "lower" {
			t.Fatalf("expected direction=lower, got %s", output.Direction)
		}
		if output.TargetCourseID == nil {
			t.Fatal("expected target course to be set")
		}
		if *output.TargetCourseID != uuidFromString("10000000-0000-0000-0000-000000000001") {
			t.Fatalf("expected target course level 2, got %v", output.TargetCourseID)
		}
	})
}

// Test 3: Level Ladder — Top level sits lower
func TestEvaluateRule_LevelLadder_TopLevel_SitsLower(t *testing.T) {
	c2 := "20000000-0000-0000-0000-000000000002"
	c3 := "30000000-0000-0000-0000-000000000003"
	c5 := "50000000-0000-0000-0000-000000000005"

	predicate := mustParsePredicate(t, `{
		"level_1_action": "zoom",
		"non_max_direction": "higher",
		"max_direction": "lower",
		"min_level_for_sit_lower": 2,
		"section_match": "same_section",
		"occurrence_match": "any",
		"day_match": "any",
		"last_class_excluded": false,
		"schedule_source": "target",
		"chains": [],
		"auto_assign": true,
		"requires_teacher_approval": false
	}`)

	input := EvaluateRuleInput{
		RuleType:     "level_ladder",
		Predicate:    predicate,
		StudentLevel: 5,
		AllCourses: []sqldb.SubjectCourseV2{
			courseV2("10000000-0000-0000-0000-000000000001", 1),
			courseV2(c2, 2),
			courseV2(c3, 3),
			courseV2(c5, 5),
		},
		MissedCount: 1,
	}

	output, err := EvaluateRule(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Eligible {
		t.Fatal("expected eligible=true")
	}
	if output.Direction != "lower" {
		t.Fatalf("expected direction=lower, got %s", output.Direction)
	}
	if output.TargetCourseID == nil {
		t.Fatal("expected target course to be set")
	}
	if *output.TargetCourseID != uuidFromString(c3) {
		t.Fatalf("expected target course %s (nearest lower >=2), got %v", c3, output.TargetCourseID)
	}
}

// Test 4: Level Ladder — No target returns not eligible
func TestEvaluateRule_LevelLadder_NoTarget(t *testing.T) {
	predicate := mustParsePredicate(t, `{
		"level_1_action": "zoom",
		"non_max_direction": "higher",
		"max_direction": "lower",
		"min_level_for_sit_lower": 2,
		"section_match": "same_section",
		"occurrence_match": "any",
		"day_match": "any",
		"last_class_excluded": false,
		"schedule_source": "target",
		"chains": [],
		"auto_assign": true,
		"requires_teacher_approval": false
	}`)

	input := EvaluateRuleInput{
		RuleType:     "level_ladder",
		Predicate:    predicate,
		StudentLevel: 2,
		AllCourses: []sqldb.SubjectCourseV2{
			courseV2("10000000-0000-0000-0000-000000000001", 2),
		},
		MissedCount: 1,
	}

	output, err := EvaluateRule(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Eligible {
		t.Fatal("expected eligible=false when no target course available")
	}
	if output.Method != "none" {
		t.Fatalf("expected method=none, got %s", output.Method)
	}
}

// Test 5: Cross-Section — Single course not eligible
func TestEvaluateRule_CrossSection_NoSiblings(t *testing.T) {
	predicate := mustParsePredicate(t, `{
		"level_1_action": "zoom",
		"non_max_direction": "higher",
		"max_direction": "lower",
		"min_level_for_sit_lower": 2,
		"section_match": "cross_section",
		"occurrence_match": "same_occurrence",
		"day_match": "any",
		"last_class_excluded": false,
		"schedule_source": "target",
		"chains": [],
		"auto_assign": true,
		"requires_teacher_approval": false
	}`)

	input := EvaluateRuleInput{
		RuleType:     "cross_section",
		Predicate:    predicate,
		StudentLevel: 2,
		AllCourses: []sqldb.SubjectCourseV2{
			courseV2("10000000-0000-0000-0000-000000000001", 2),
		},
		MissedCount: 1,
	}

	output, err := EvaluateRule(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Eligible {
		t.Fatal("expected eligible=false with single course (no siblings)")
	}
	if output.Method != "none" {
		t.Fatalf("expected method=none, got %s", output.Method)
	}
}

// Test 6: Cross-Section — Multiple courses eligible
func TestEvaluateRule_CrossSection_HasSiblings(t *testing.T) {
	c1 := "10000000-0000-0000-0000-000000000001"
	c2 := "20000000-0000-0000-0000-000000000002"

	predicate := mustParsePredicate(t, `{
		"level_1_action": "zoom",
		"non_max_direction": "higher",
		"max_direction": "lower",
		"min_level_for_sit_lower": 2,
		"section_match": "cross_section",
		"occurrence_match": "same_occurrence",
		"day_match": "any",
		"last_class_excluded": false,
		"schedule_source": "target",
		"chains": [],
		"auto_assign": true,
		"requires_teacher_approval": false
	}`)

	input := EvaluateRuleInput{
		RuleType:     "cross_section",
		Predicate:    predicate,
		StudentLevel: 2,
		AllCourses: []sqldb.SubjectCourseV2{
			courseV2(c1, 2),
			courseV2(c2, 2),
		},
		MissedCount: 1,
	}

	output, err := EvaluateRule(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Eligible {
		t.Fatal("expected eligible=true with sibling courses")
	}
	if output.Method != "physical" {
		t.Fatalf("expected method=physical, got %s", output.Method)
	}
	if output.Direction != "same_section" {
		t.Fatalf("expected direction=same_section, got %s", output.Direction)
	}
}

// Test 7: Any Day Except Last — Always eligible
func TestEvaluateRule_AnyDayExceptLast(t *testing.T) {
	predicate := mustParsePredicate(t, `{
		"level_1_action": "zoom",
		"non_max_direction": "higher",
		"max_direction": "lower",
		"min_level_for_sit_lower": 2,
		"section_match": "same_section",
		"occurrence_match": "any",
		"day_match": "any_day_except_last",
		"last_class_excluded": true,
		"schedule_source": "target",
		"chains": [],
		"auto_assign": true,
		"requires_teacher_approval": false
	}`)

	input := EvaluateRuleInput{
		RuleType:     "any_day_except_last",
		Predicate:    predicate,
		StudentLevel: 2,
		AllCourses: []sqldb.SubjectCourseV2{
			courseV2("10000000-0000-0000-0000-000000000001", 2),
			courseV2("20000000-0000-0000-0000-000000000002", 2),
		},
		MissedCount: 1,
	}

	output, err := EvaluateRule(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Eligible {
		t.Fatal("expected eligible=true for any_day_except_last")
	}
	if output.Method != "physical" {
		t.Fatalf("expected method=physical, got %s", output.Method)
	}
	if output.Direction != "any_day" {
		t.Fatalf("expected direction=any_day, got %s", output.Direction)
	}
}

// Test 8: Rank Chain — Matches chain
func TestEvaluateRule_RankChain_Matches(t *testing.T) {
	c1 := "10000000-0000-0000-0000-000000000001"
	c2 := "20000000-0000-0000-0000-000000000002"

	predicate := mustParsePredicate(t, `{
		"level_1_action": "zoom",
		"non_max_direction": "higher",
		"max_direction": "lower",
		"min_level_for_sit_lower": 2,
		"section_match": "same_section",
		"occurrence_match": "any",
		"day_match": "any",
		"last_class_excluded": false,
		"schedule_source": "target",
		"chains": [{"from_rank": 1, "to_rank": 2}],
		"auto_assign": true,
		"requires_teacher_approval": false
	}`)

	input := EvaluateRuleInput{
		RuleType:     "rank_chain",
		Predicate:    predicate,
		StudentLevel: 1,
		AllCourses: []sqldb.SubjectCourseV2{
			courseV2(c1, 1),
			courseV2(c2, 2),
		},
		MissedCount: 1,
	}

	output, err := EvaluateRule(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Eligible {
		t.Fatal("expected eligible=true when chain matches")
	}
	if output.Method != "physical" {
		t.Fatalf("expected method=physical, got %s", output.Method)
	}
	if output.Direction != "chain" {
		t.Fatalf("expected direction=chain, got %s", output.Direction)
	}
	if output.TargetCourseID == nil {
		t.Fatal("expected target course to be set")
	}
	if *output.TargetCourseID != uuidFromString(c2) {
		t.Fatalf("expected target course %s, got %v", c2, output.TargetCourseID)
	}
}

// Test 9: Rank Chain — No matching chain
func TestEvaluateRule_RankChain_NoMatch(t *testing.T) {
	c1 := "10000000-0000-0000-0000-000000000001"
	c2 := "20000000-0000-0000-0000-000000000002"

	predicate := mustParsePredicate(t, `{
		"level_1_action": "zoom",
		"non_max_direction": "higher",
		"max_direction": "lower",
		"min_level_for_sit_lower": 2,
		"section_match": "same_section",
		"occurrence_match": "any",
		"day_match": "any",
		"last_class_excluded": false,
		"schedule_source": "target",
		"chains": [{"from_rank": 1, "to_rank": 3}],
		"auto_assign": true,
		"requires_teacher_approval": false
	}`)

	input := EvaluateRuleInput{
		RuleType:     "rank_chain",
		Predicate:    predicate,
		StudentLevel: 1,
		AllCourses: []sqldb.SubjectCourseV2{
			courseV2(c1, 1),
			courseV2(c2, 2),
		},
		MissedCount: 1,
	}

	output, err := EvaluateRule(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Eligible {
		t.Fatal("expected eligible=false when no chain matches")
	}
	if output.Method != "none" {
		t.Fatalf("expected method=none, got %s", output.Method)
	}
}

// Test 10: Teacher Case — Returns teacher_case
func TestEvaluateRule_TeacherCaseByCase(t *testing.T) {
	predicate := mustParsePredicate(t, `{
		"level_1_action": "zoom",
		"non_max_direction": "higher",
		"max_direction": "lower",
		"min_level_for_sit_lower": 2,
		"section_match": "same_section",
		"occurrence_match": "any",
		"day_match": "any",
		"last_class_excluded": false,
		"schedule_source": "target",
		"chains": [],
		"auto_assign": false,
		"requires_teacher_approval": true
	}`)

	input := EvaluateRuleInput{
		RuleType:     "teacher_case_by_case",
		Predicate:    predicate,
		StudentLevel: 2,
		AllCourses: []sqldb.SubjectCourseV2{
			courseV2("10000000-0000-0000-0000-000000000001", 2),
			courseV2("20000000-0000-0000-0000-000000000002", 3),
		},
		MissedCount: 1,
	}

	output, err := EvaluateRule(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Eligible {
		t.Fatal("expected eligible=true for teacher_case_by_case")
	}
	if output.Method != "teacher_case" {
		t.Fatalf("expected method=teacher_case, got %s", output.Method)
	}
}

// Test 11: Unknown rule type returns error
func TestEvaluateRule_UnknownType(t *testing.T) {
	predicate := mustParsePredicate(t, `{
		"level_1_action": "zoom",
		"non_max_direction": "higher",
		"max_direction": "lower",
		"min_level_for_sit_lower": 2,
		"section_match": "same_section",
		"occurrence_match": "any",
		"day_match": "any",
		"last_class_excluded": false,
		"schedule_source": "target",
		"chains": [],
		"auto_assign": true,
		"requires_teacher_approval": false
	}`)

	input := EvaluateRuleInput{
		RuleType:     "unknown_rule_type",
		Predicate:    predicate,
		StudentLevel: 2,
		AllCourses:   []sqldb.SubjectCourseV2{},
		MissedCount:  0,
	}

	_, err := EvaluateRule(input)
	if err == nil {
		t.Fatal("expected error for unknown rule type")
	}
}
