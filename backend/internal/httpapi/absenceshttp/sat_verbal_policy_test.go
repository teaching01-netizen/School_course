package absenceshttp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func satCourse(id, name string) sqldb.SubjectCourseV2 {
	return sqldb.SubjectCourseV2{
		ID:          makeUUID(id),
		Code:        "SAT-" + id[:8],
		Name:        name,
		SubjectCode: "SATV",
		SubjectName: "SAT Verbal",
	}
}

func satEnrolled(id, name string) sqldb.StudentEnrolledCourseV2 {
	return sqldb.StudentEnrolledCourseV2{
		CourseID:   makeUUID(id),
		CourseCode: "SAT-" + id[:8],
		CourseName: name,
	}
}

func mustDecodeSatVerbalPolicy(t *testing.T, raw string) []satVerbalCourseRule {
	t.Helper()
	rules, err := decodeSatVerbalPolicyRules([]byte(raw))
	if err != nil {
		t.Fatalf("decode policy: %v", err)
	}
	return rules
}

func TestResolveSatVerbalPolicy_Rank3Section3PreservesGapAndNeverOffersRank2(t *testing.T) {
	section3ID := "30000000-0000-0000-0000-000000000003"
	section1ID := "10000000-0000-0000-0000-000000000001"
	rank2ID := "20000000-0000-0000-0000-000000000002"
	rank4ID := "40000000-0000-0000-0000-000000000004"

	rules := mustDecodeSatVerbalPolicy(t, `[
		{
			"id": "rank3-sec3",
			"courseName": "SAT Verbal Rank 3-Section 3",
			"lastClassExcluded": true,
			"priorities": [
				{
					"level": 1,
					"ruleType": "cross_section",
					"label": "1st Priority: Another Rank 3 section (same lesson #)",
					"makeupTargets": [{ "section": "Section 1", "subject": "Reading" }]
				},
				{
					"level": 3,
					"ruleType": "rank_chain",
					"label": "3rd Priority: Rank 4 Reading or Writing",
					"eligibleTargets": ["SAT Verbal Reading Rank 4"]
				}
			]
		}
	]`)

	courses := []sqldb.SubjectCourseV2{
		satCourse(section3ID, "SAT Verbal Rank 3-Section 3"),
		satCourse(section1ID, "SAT Verbal Rank 3 Section 1"),
		satCourse(rank2ID, "SAT Verbal Rank 2"),
		satCourse(rank4ID, "SAT Verbal Reading Rank 4"),
	}
	missedSessions := []sqldb.SessionInRange{
		session("a3000000-0000-0000-0000-000000000002", section3ID, "2026-02-08T09:00:00Z", "2026-02-08T10:00:00Z"),
	}
	sessionsByCourse := map[pgtype.UUID][]sqldb.SessionInRange{
		makeUUID(section3ID): {
			session("a3000000-0000-0000-0000-000000000001", section3ID, "2026-02-01T09:00:00Z", "2026-02-01T10:00:00Z"),
			missedSessions[0],
			session("a3000000-0000-0000-0000-000000000003", section3ID, "2026-02-15T09:00:00Z", "2026-02-15T10:00:00Z"),
		},
		makeUUID(section1ID): {
			session("a1000000-0000-0000-0000-000000000001", section1ID, "2026-02-01T11:00:00Z", "2026-02-01T12:00:00Z"),
			session("a1000000-0000-0000-0000-000000000002", section1ID, "2026-02-08T11:00:00Z", "2026-02-08T12:00:00Z"),
			session("a1000000-0000-0000-0000-000000000003", section1ID, "2026-02-15T11:00:00Z", "2026-02-15T12:00:00Z"),
		},
		makeUUID(rank2ID): {
			session("a2000000-0000-0000-0000-000000000001", rank2ID, "2026-02-08T13:00:00Z", "2026-02-08T14:00:00Z"),
		},
		makeUUID(rank4ID): {
			session("a4000000-0000-0000-0000-000000000001", rank4ID, "2026-02-09T13:00:00Z", "2026-02-09T14:00:00Z"),
			session("a4000000-0000-0000-0000-000000000002", rank4ID, "2026-02-16T13:00:00Z", "2026-02-16T14:00:00Z"),
		},
	}

	result, err := resolveSatVerbalPolicy(context.Background(), satVerbalResolveInput{
		Policy:         rules,
		MissedCourse:   courses[0],
		Enrolled:       []sqldb.StudentEnrolledCourseV2{satEnrolled(section3ID, "SAT Verbal Rank 3-Section 3")},
		AllCourses:     courses,
		MissedSessions: missedSessions,
		Cutoff:         time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		LoadSessions: func(_ context.Context, courseID pgtype.UUID) ([]sqldb.SessionInRange, error) {
			return sessionsByCourse[courseID], nil
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if result == nil {
		t.Fatal("expected SAT Verbal result")
	}
	if len(result.Priorities) != 1 {
		t.Fatalf("expected initial reveal to expose one priority, got %d priorities", len(result.Priorities))
	}
	if result.Priorities[0].Level != 1 {
		t.Fatalf("expected initial priority level 1, got %d", result.Priorities[0].Level)
	}
	if result.Priorities[0].SitInCourse == nil || result.Priorities[0].SitInCourse.Name != "SAT Verbal Rank 3 Section 1" {
		t.Fatalf("priority 1 target = %#v, want Section 1", result.Priorities[0].SitInCourse)
	}
	if got := result.Priorities[0].Available; len(got) != 1 || got[0].ID != "a1000000-0000-0000-0000-000000000002" {
		t.Fatalf("priority 1 available = %#v, want same lesson only", got)
	}
	if !result.HasNextPriority {
		t.Fatal("expected hidden later priority to be available")
	}
}

func TestResolveSatVerbalPolicy_InitialRevealOnlyReturnsFirstAvailablePriority(t *testing.T) {
	section1ID := "10000000-0000-0000-0000-000000000001"
	section2ID := "20000000-0000-0000-0000-000000000002"
	rank2ID := "22000000-0000-0000-0000-000000000002"
	rank4ID := "40000000-0000-0000-0000-000000000004"

	rules := mustDecodeSatVerbalPolicy(t, `[
		{
			"id": "rank3-sec1",
			"courseName": "SAT Verbal Rank 3-Section 1",
			"lastClassExcluded": true,
			"priorities": [
				{
					"level": 1,
					"ruleType": "cross_section",
					"label": "1st Priority: Another Rank 3 section (same lesson #)",
					"makeupTargets": [{ "section": "Section 2", "subject": "Writing" }]
				},
				{
					"level": 2,
					"ruleType": "rank_chain",
					"label": "2nd Priority: Rank 2",
					"eligibleTargets": ["SAT Verbal Rank 2"]
				},
				{
					"level": 3,
					"ruleType": "rank_chain",
					"label": "3rd Priority: Rank 4 Reading or Writing",
					"eligibleTargets": ["SAT Verbal Reading Rank 4"]
				}
			]
		}
	]`)

	courses := []sqldb.SubjectCourseV2{
		satCourse(section1ID, "SAT Verbal Rank 3-Section 1"),
		satCourse(section2ID, "SAT Verbal Rank 3 Section 2"),
		satCourse(rank2ID, "SAT Verbal Rank 2"),
		satCourse(rank4ID, "SAT Verbal Reading Rank 4"),
	}
	missedSessions := []sqldb.SessionInRange{
		session("c1000000-0000-0000-0000-000000000002", section1ID, "2026-02-08T09:00:00Z", "2026-02-08T10:00:00Z"),
	}
	sessionsByCourse := map[pgtype.UUID][]sqldb.SessionInRange{
		makeUUID(section1ID): {
			session("c1000000-0000-0000-0000-000000000001", section1ID, "2026-02-01T09:00:00Z", "2026-02-01T10:00:00Z"),
			missedSessions[0],
			session("c1000000-0000-0000-0000-000000000003", section1ID, "2026-02-15T09:00:00Z", "2026-02-15T10:00:00Z"),
		},
		makeUUID(section2ID): {
			session("c2000000-0000-0000-0000-000000000001", section2ID, "2026-02-01T11:00:00Z", "2026-02-01T12:00:00Z"),
			session("c2000000-0000-0000-0000-000000000002", section2ID, "2026-02-08T11:00:00Z", "2026-02-08T12:00:00Z"),
			session("c2000000-0000-0000-0000-000000000003", section2ID, "2026-02-15T11:00:00Z", "2026-02-15T12:00:00Z"),
		},
		makeUUID(rank2ID): {
			session("c2200000-0000-0000-0000-000000000001", rank2ID, "2026-02-09T13:00:00Z", "2026-02-09T14:00:00Z"),
			session("c2200000-0000-0000-0000-000000000002", rank2ID, "2026-02-16T13:00:00Z", "2026-02-16T14:00:00Z"),
		},
		makeUUID(rank4ID): {
			session("c4000000-0000-0000-0000-000000000001", rank4ID, "2026-02-10T13:00:00Z", "2026-02-10T14:00:00Z"),
			session("c4000000-0000-0000-0000-000000000002", rank4ID, "2026-02-17T13:00:00Z", "2026-02-17T14:00:00Z"),
		},
	}

	result, err := resolveSatVerbalPolicy(context.Background(), satVerbalResolveInput{
		Policy:         rules,
		MissedCourse:   courses[0],
		Enrolled:       []sqldb.StudentEnrolledCourseV2{satEnrolled(section1ID, "SAT Verbal Rank 3-Section 1")},
		AllCourses:     courses,
		MissedSessions: missedSessions,
		LoadSessions: func(_ context.Context, courseID pgtype.UUID) ([]sqldb.SessionInRange, error) {
			return sessionsByCourse[courseID], nil
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if result == nil {
		t.Fatal("expected SAT Verbal result")
	}
	if len(result.Priorities) != 1 {
		t.Fatalf("initial reveal must expose exactly one priority, got %#v", result.Priorities)
	}
	if result.Priorities[0].Level != 1 {
		t.Fatalf("initial reveal level = %d, want 1", result.Priorities[0].Level)
	}
	if !result.HasNextPriority {
		t.Fatal("expected has_next_priority for hidden later levels")
	}
	if result.CurrentPriorityLevel != 1 {
		t.Fatalf("current priority level = %d, want 1", result.CurrentPriorityLevel)
	}
}

func TestResolveSatVerbalPolicy_NextRevealSkipsUnavailableLevelAndDoesNotLeakFuture(t *testing.T) {
	section3ID := "30000000-0000-0000-0000-000000000003"
	section1ID := "10000000-0000-0000-0000-000000000001"
	rank4ID := "40000000-0000-0000-0000-000000000004"
	rank5ID := "50000000-0000-0000-0000-000000000005"

	rules := mustDecodeSatVerbalPolicy(t, `[
		{
			"id": "rank3-sec3",
			"courseName": "SAT Verbal Rank 3-Section 3",
			"lastClassExcluded": true,
			"priorities": [
				{
					"level": 1,
					"ruleType": "cross_section",
					"label": "1st Priority: Another Rank 3 section (same lesson #)",
					"makeupTargets": [{ "section": "Section 1", "subject": "Reading" }]
				},
				{
					"level": 3,
					"ruleType": "rank_chain",
					"label": "3rd Priority: Rank 4 Reading or Writing",
					"eligibleTargets": ["SAT Verbal Reading Rank 4"]
				},
				{
					"level": 4,
					"ruleType": "rank_chain",
					"label": "Hidden invalid future level",
					"eligibleTargets": ["SAT Verbal Reading Rank 5"]
				}
			]
		}
	]`)

	courses := []sqldb.SubjectCourseV2{
		satCourse(section3ID, "SAT Verbal Rank 3-Section 3"),
		satCourse(section1ID, "SAT Verbal Rank 3 Section 1"),
		satCourse(rank4ID, "SAT Verbal Reading Rank 4"),
		satCourse(rank5ID, "SAT Verbal Reading Rank 5"),
	}
	missedSessions := []sqldb.SessionInRange{
		session("d3000000-0000-0000-0000-000000000002", section3ID, "2026-02-08T09:00:00Z", "2026-02-08T10:00:00Z"),
	}
	sessionsByCourse := map[pgtype.UUID][]sqldb.SessionInRange{
		makeUUID(section3ID): {
			session("d3000000-0000-0000-0000-000000000001", section3ID, "2026-02-01T09:00:00Z", "2026-02-01T10:00:00Z"),
			missedSessions[0],
			session("d3000000-0000-0000-0000-000000000003", section3ID, "2026-02-15T09:00:00Z", "2026-02-15T10:00:00Z"),
		},
		makeUUID(section1ID): {
			session("d1000000-0000-0000-0000-000000000001", section1ID, "2026-02-01T11:00:00Z", "2026-02-01T12:00:00Z"),
			session("d1000000-0000-0000-0000-000000000002", section1ID, "2026-02-08T11:00:00Z", "2026-02-08T12:00:00Z"),
			session("d1000000-0000-0000-0000-000000000003", section1ID, "2026-02-15T11:00:00Z", "2026-02-15T12:00:00Z"),
		},
		makeUUID(rank4ID): {
			session("d4000000-0000-0000-0000-000000000001", rank4ID, "2026-02-09T13:00:00Z", "2026-02-09T14:00:00Z"),
			session("d4000000-0000-0000-0000-000000000002", rank4ID, "2026-02-16T13:00:00Z", "2026-02-16T14:00:00Z"),
		},
		makeUUID(rank5ID): {
			session("d5000000-0000-0000-0000-000000000001", rank5ID, "2026-02-10T13:00:00Z", "2026-02-10T14:00:00Z"),
			session("d5000000-0000-0000-0000-000000000002", rank5ID, "2026-02-17T13:00:00Z", "2026-02-17T14:00:00Z"),
		},
	}

	result, err := resolveSatVerbalPolicy(context.Background(), satVerbalResolveInput{
		Policy:             rules,
		MissedCourse:       courses[0],
		Enrolled:           []sqldb.StudentEnrolledCourseV2{satEnrolled(section3ID, "SAT Verbal Rank 3-Section 3")},
		AllCourses:         courses,
		MissedSessions:     missedSessions,
		AfterPriorityLevel: 1,
		LoadSessions: func(_ context.Context, courseID pgtype.UUID) ([]sqldb.SessionInRange, error) {
			return sessionsByCourse[courseID], nil
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if result == nil {
		t.Fatal("expected SAT Verbal result")
	}
	if len(result.Priorities) != 1 {
		t.Fatalf("next reveal must expose exactly one priority, got %#v", result.Priorities)
	}
	if result.Priorities[0].Level != 3 {
		t.Fatalf("next reveal level = %d, want 3", result.Priorities[0].Level)
	}
	if result.Priorities[0].Label == "Hidden invalid future level" {
		t.Fatal("future priority label leaked")
	}
	if !result.HasNextPriority {
		t.Fatal("expected has_next_priority for level 4")
	}
	if result.CurrentPriorityLevel != 3 {
		t.Fatalf("current priority level = %d, want 3", result.CurrentPriorityLevel)
	}
}

func TestResolveSatVerbalPolicy_BeginnerSection3TargetsSection1SameLessonOnly(t *testing.T) {
	section3ID := "31000000-0000-0000-0000-000000000003"
	section1ID := "11000000-0000-0000-0000-000000000001"

	rules := mustDecodeSatVerbalPolicy(t, `[
		{
			"id": "sat-verbal-reading-beginner",
			"courseName": "SAT Verbal Reading Beginner",
			"lastClassExcluded": true,
			"priorities": [
				{
					"level": 1,
					"ruleType": "cross_section",
					"label": "1st Priority: Same Reading Beginner lesson in another section",
					"sectionTargets": {
						"Section 3": [{ "section": "Section 1", "subject": "Reading Beginner" }]
					}
				},
				{
					"level": 2,
					"ruleType": "cross_section",
					"label": "2nd Priority: Next available Reading Beginner section/date",
					"makeupTargets": [{ "section": "Next available", "subject": "Reading Beginner" }]
				}
			]
		}
	]`)

	courses := []sqldb.SubjectCourseV2{
		satCourse(section3ID, "SAT Verbal Reading Beginner Section 3"),
		satCourse(section1ID, "SAT Verbal Reading Beginner Section 1"),
	}
	missedSessions := []sqldb.SessionInRange{
		session("b3000000-0000-0000-0000-000000000002", section3ID, "2026-04-08T09:00:00Z", "2026-04-08T10:00:00Z"),
	}
	sessionsByCourse := map[pgtype.UUID][]sqldb.SessionInRange{
		makeUUID(section3ID): {
			session("b3000000-0000-0000-0000-000000000001", section3ID, "2026-04-01T09:00:00Z", "2026-04-01T10:00:00Z"),
			missedSessions[0],
			session("b3000000-0000-0000-0000-000000000003", section3ID, "2026-04-15T09:00:00Z", "2026-04-15T10:00:00Z"),
		},
		makeUUID(section1ID): {
			session("b1000000-0000-0000-0000-000000000001", section1ID, "2026-04-01T11:00:00Z", "2026-04-01T12:00:00Z"),
			session("b1000000-0000-0000-0000-000000000002", section1ID, "2026-04-08T11:00:00Z", "2026-04-08T12:00:00Z"),
			session("b1000000-0000-0000-0000-000000000003", section1ID, "2026-04-15T11:00:00Z", "2026-04-15T12:00:00Z"),
		},
	}

	result, err := resolveSatVerbalPolicy(context.Background(), satVerbalResolveInput{
		Policy:         rules,
		MissedCourse:   courses[0],
		Enrolled:       []sqldb.StudentEnrolledCourseV2{satEnrolled(section3ID, "SAT Verbal Reading Beginner Section 3")},
		AllCourses:     courses,
		MissedSessions: missedSessions,
		Cutoff:         time.Time{},
		LoadSessions: func(_ context.Context, courseID pgtype.UUID) ([]sqldb.SessionInRange, error) {
			return sessionsByCourse[courseID], nil
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if result == nil || len(result.Priorities) == 0 {
		t.Fatal("expected beginner priorities")
	}
	first := result.Priorities[0]
	if first.SitInCourse == nil || first.SitInCourse.Name != "SAT Verbal Reading Beginner Section 1" {
		t.Fatalf("priority 1 target = %#v, want Section 1", first.SitInCourse)
	}
	if got := first.Available; len(got) != 1 || got[0].ID != "b1000000-0000-0000-0000-000000000002" {
		t.Fatalf("priority 1 available = %#v, want same lesson section 1", got)
	}
	for _, p := range result.Priorities {
		for _, s := range p.Available {
			if s.ID == "b1000000-0000-0000-0000-000000000003" {
				t.Fatal("final class must be excluded")
			}
		}
	}
}

func TestSubjectWindowWeeksFallsBackToRootGroup(t *testing.T) {
	subjectID := "11111111-1111-1111-1111-111111111111"
	rootID := "22222222-2222-2222-2222-222222222222"
	raw, err := json.Marshal(map[string]any{
		"subjects": map[string]any{
			subjectID: map[string]any{
				"auto_sit_in_enabled": true,
				"sit_in_window_weeks": 2,
			},
		},
		"root_course_groups": map[string]any{
			rootID: map[string]any{
				"auto_sit_in_enabled": true,
				"sit_in_window_weeks": 5,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := subjectWindowWeeks(raw, subjectID, rootID); got != 2 {
		t.Fatalf("subject window = %d, want 2", got)
	}
	if got := subjectWindowWeeks(raw, "missing", rootID); got != 5 {
		t.Fatalf("fallback window = %d, want 5", got)
	}
}
