package absenceshttp

import (
	"fmt"
	"testing"
	"time"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/satverbalpolicy"
)

func TestResolveMappedFromBundleKeepsVisibleOutOfScopeSATTargets(t *testing.T) {
	targetFor := func(name string) []string {
		return []string{name}
	}
	priority := func(level int, ruleType, label string, eligible []string, makeup []satverbalpolicy.Target) satverbalpolicy.RulePriority {
		return satverbalpolicy.RulePriority{Level: level, RuleType: ruleType, Label: label, EligibleTargets: eligible, MakeupTargets: makeup}
	}

	cases := []struct {
		name             string
		sourceRuleName   string
		sourceCourseName string
		targetRuleName   string
		targetCourseName string
		priorities       []satverbalpolicy.RulePriority
		enrolledNames    []string
		expectedLevel    int
		afterPriority    int
		expectSeeOther   bool
		targetVisible    bool
	}{
		{
			name:             "Rank 3 See other times reveals mapped Rank 4",
			sourceRuleName:   "SAT Verbal Rank 3-Section 2",
			sourceCourseName: "SAT Verbal Rank 3 Section 2 C3",
			targetRuleName:   "SAT Verbal Reading Rank 4",
			targetCourseName: "SAT Verbal Reading Rank 4 C3",
			priorities: []satverbalpolicy.RulePriority{
				priority(1, "cross_section", "1st Priority: Another Rank 3 section (same lesson #)", nil, []satverbalpolicy.Target{{Section: "Section 1", Subject: "Reading"}}),
				priority(3, "rank_chain", "3rd Priority: Rank 4 Reading or Writing", targetFor("SAT Verbal Reading Rank 4"), nil),
			},
			enrolledNames:  []string{"SAT Verbal Rank 3 Section 2 C3"},
			expectedLevel:  3,
			afterPriority:  1,
			expectSeeOther: true,
			targetVisible:  true,
		},
		{
			name:             "Reading Beginner to Rank 5",
			sourceRuleName:   "SAT Verbal Reading Beginner Section 1",
			sourceCourseName: "SAT Verbal Reading Beginner Section 1",
			targetRuleName:   "SAT Verbal Reading Rank 5",
			targetCourseName: "SAT Verbal Reading Rank 5",
			priorities:       []satverbalpolicy.RulePriority{priority(1, "rank_chain", "Rank 5", targetFor("SAT Verbal Reading Rank 5"), nil)},
			enrolledNames:    []string{"SAT Verbal Reading Beginner Section 1"},
			expectedLevel:    1,
			targetVisible:    true,
		},
		{
			name:             "Writing Beginner to Rank 5",
			sourceRuleName:   "SAT Verbal Writing Beginner Section 1",
			sourceCourseName: "SAT Verbal Writing Beginner Section 1",
			targetRuleName:   "SAT Verbal Writing Rank 5",
			targetCourseName: "SAT Verbal Writing Rank 5",
			priorities:       []satverbalpolicy.RulePriority{priority(1, "rank_chain", "Rank 5", targetFor("SAT Verbal Writing Rank 5"), nil)},
			enrolledNames:    []string{"SAT Verbal Writing Beginner Section 1"},
			expectedLevel:    1,
			targetVisible:    true,
		},
		{
			name:             "Brush Up derives Reading Rank 5",
			sourceRuleName:   "SAT Verbal Brush Up",
			sourceCourseName: "SAT Verbal Brush Up",
			targetRuleName:   "SAT Verbal Reading Rank 5",
			targetCourseName: "SAT Verbal Reading Rank 5",
			priorities:       []satverbalpolicy.RulePriority{priority(1, "rank_chain", "Derived target", nil, nil)},
			enrolledNames:    []string{"SAT Verbal Brush Up", "SAT Verbal Reading Rank 4"},
			expectedLevel:    1,
			targetVisible:    true,
		},
		{
			name:             "Real Time Practice derives Rank 2",
			sourceRuleName:   "SAT Verbal Real Time Practice",
			sourceCourseName: "SAT Verbal Real Time Practice",
			targetRuleName:   "SAT Verbal Rank 2",
			targetCourseName: "SAT Verbal Rank 2",
			priorities:       []satverbalpolicy.RulePriority{priority(1, "rank_chain", "Derived target", nil, nil)},
			enrolledNames:    []string{"SAT Verbal Real Time Practice", "SAT Verbal Rank 3"},
			expectedLevel:    1,
			targetVisible:    true,
		},
		{
			name:             "Reading Mastery derives Rank 2",
			sourceRuleName:   "Reading Mastery",
			sourceCourseName: "Reading Mastery",
			targetRuleName:   "SAT Verbal Rank 2",
			targetCourseName: "SAT Verbal Rank 2",
			priorities:       []satverbalpolicy.RulePriority{priority(1, "rank_chain", "Derived target", nil, nil)},
			enrolledNames:    []string{"Reading Mastery", "SAT Verbal Rank 3"},
			expectedLevel:    1,
			targetVisible:    true,
		},
		{
			name:             "invisible or inactive mapped target is excluded",
			sourceRuleName:   "SAT Verbal Reading Beginner Section 1",
			sourceCourseName: "SAT Verbal Reading Beginner Section 1",
			targetRuleName:   "SAT Verbal Reading Rank 5",
			targetCourseName: "SAT Verbal Reading Rank 5",
			priorities:       []satverbalpolicy.RulePriority{priority(1, "rank_chain", "Rank 5", targetFor("SAT Verbal Reading Rank 5"), nil)},
			enrolledNames:    []string{"SAT Verbal Reading Beginner Section 1"},
			expectedLevel:    1,
			targetVisible:    false,
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := func(n int) string { return fmt.Sprintf("ed000000-0000-0000-0000-%012x", i*10+n) }
			sourceID, targetID := id(1), id(2)
			missedStart := "2026-09-26T02:00:00Z" // 09:00 Sep 26 in Bangkok.
			missed := session(id(4), sourceID, missedStart, "2026-09-26T03:00:00Z")
			future1 := session(id(5), targetID, "2026-09-28T02:00:00Z", "2026-09-28T03:00:00Z")
			future2 := session(id(6), targetID, "2026-10-05T02:00:00Z", "2026-10-05T03:00:00Z")

			sourcePolicy := satverbalpolicy.CourseRule{
				ID:                "source-" + id(7),
				CourseName:        tc.sourceRuleName,
				LastClassExcluded: tc.expectSeeOther,
				Priorities:        tc.priorities,
			}
			targetPolicy := satverbalpolicy.CourseRule{ID: "target-" + id(8), CourseName: tc.targetRuleName}
			mappings := []sqldb.SatVerbalPolicyCourseMapping{
				{RuleID: sourcePolicy.ID, CourseID: makeUUID(sourceID), CourseName: tc.sourceCourseName, PolicyRule: mustMarshalJSON(sourcePolicy), Active: true},
				{RuleID: targetPolicy.ID, CourseID: makeUUID(targetID), CourseName: tc.targetCourseName, PolicyRule: mustMarshalJSON(targetPolicy), Active: true},
			}
			bundle := &sqldb.SitInBundleV2{
				ScopeCourses: []sqldb.SubjectCourseV2{{ID: makeUUID(sourceID)}},
				SatMappings:  mappings,
				SatMapByCourse: map[string]*sqldb.SatVerbalPolicyCourseMapping{
					uuidStringOrZero(makeUUID(sourceID)): &mappings[0],
					uuidStringOrZero(makeUUID(targetID)): &mappings[1],
				},
				Visible: map[string]struct{}{
					uuidStringOrZero(makeUUID(sourceID)): {},
				},
				Sessions: map[string][]sqldb.SessionInRange{
					uuidStringOrZero(makeUUID(sourceID)): {missed},
					uuidStringOrZero(makeUUID(targetID)): {future1, future2},
				},
			}
			if tc.targetVisible {
				bundle.Visible[uuidStringOrZero(makeUUID(targetID))] = struct{}{}
			}

			enrolled := make([]sqldb.StudentEnrolledCourseV2, 0, len(tc.enrolledNames))
			for enrolledIndex, name := range tc.enrolledNames {
				courseID := sourceID
				if enrolledIndex > 0 {
					courseID = id(8 + enrolledIndex)
				}
				enrolled = append(enrolled, satEnrolled(courseID, name))
			}
			dateFrom := time.Date(2026, 9, 25, 17, 0, 0, 0, time.UTC)
			dateTo := time.Date(2026, 9, 26, 17, 0, 0, 0, time.UTC)
			resolve := func(afterPriority int) (*SitInResult, error) {
				result, done, err := resolveMappedFromBundle(bundleSitInInputs{
					bundle:        bundle,
					instituteTZ:   "Asia/Bangkok",
					studentFacing: true,
					now:           time.Date(2026, 9, 24, 17, 0, 0, 0, time.UTC), // Sep 25 in Bangkok.
					afterPriority: afterPriority,
				}, makeUUID(sourceID), enrolled, dateFrom, dateTo)
				if err != nil {
					return nil, err
				}
				if !done {
					t.Fatal("mapped SAT course did not enter mapped resolver")
				}
				return result, nil
			}

			if tc.expectSeeOther {
				initial, err := resolve(0)
				if err != nil {
					t.Fatal(err)
				}
				if initial == nil || initial.CurrentPriorityLevel != 1 || !initial.HasNextPriority {
					t.Fatalf("Sep 25 initial priority = %#v, want priority 1 with See other times", initial)
				}
			}
			result, err := resolve(tc.afterPriority)
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || result.CurrentPriorityLevel != tc.expectedLevel || len(result.Priorities) != 1 {
				t.Fatalf("resolved result = %#v, want priority level %d", result, tc.expectedLevel)
			}
			got := result.Priorities[0]
			if !tc.targetVisible {
				if got.SitInCourse != nil || len(got.Available) != 0 || len(got.Unavailable) != 0 {
					t.Fatalf("excluded target still resolved: %#v", got)
				}
				return
			}
			if got.SitInCourse == nil || got.SitInCourse.Name != tc.targetCourseName {
				t.Fatalf("mapped target = %#v, want %q", got.SitInCourse, tc.targetCourseName)
			}
			if len(got.Available) == 0 || got.Available[0].ID != uuidStringOrZero(future1.ID) {
				t.Fatalf("available = %#v, want the future mapped make-up session %s", got.Available, future1.ID)
			}
		})
	}
}
