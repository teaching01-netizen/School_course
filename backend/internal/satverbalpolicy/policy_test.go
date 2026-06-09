package satverbalpolicy

import (
	"testing"

	sqldb "warwick-institute/internal/db"
)

func TestCourseMatchesRule_AllowsExplicitSatAndSectionAliasesOnly(t *testing.T) {
	rule := CourseRule{CourseName: "SAT Verbal Reading Beginner"}
	if !CourseMatchesRule(rule, "SAT Verbal Reading Beginner Section 3") {
		t.Fatal("expected sectioned beginner course to match beginner policy row")
	}
	if !CourseMatchesRule(CourseRule{CourseName: "SAT Verbal Rank 3-Section 3"}, "SAT Verbal Rank 3 Section 3") {
		t.Fatal("expected hyphen/space section alias to match")
	}
	if !CourseMatchesRule(CourseRule{CourseName: "SAT Verbal Reading Rank 5"}, "SAT Reading Rank 5") {
		t.Fatal("expected SAT/SAT Verbal alias to match")
	}
	if CourseMatchesRule(rule, "SAT Verbal Reading Advanced") {
		t.Fatal("did not expect adjacent but non-matching course name to match")
	}
}

func TestBuildApplyReport_GroupsRank3SectionsWithoutRequiringLevels(t *testing.T) {
	rules := []CourseRule{
		{CourseName: "SAT Verbal Rank 3-Section 1"},
		{CourseName: "SAT Verbal Rank 3-Section 2"},
		{CourseName: "SAT Verbal Believe"},
	}
	courses := []sqldb.SubjectCourseV2{
		{Name: "SAT Verbal Rank 3 Section 1", Code: "R3S1"},
		{Name: "SAT Verbal Rank 3 Section 2", Code: "R3S2"},
	}

	report := BuildApplyReport(rules, courses, "SATV")
	if len(report.MatchedCourses) != 2 {
		t.Fatalf("matched = %d, want 2", len(report.MatchedCourses))
	}
	if report.MatchedCourses[0].RootGroupName != report.MatchedCourses[1].RootGroupName {
		t.Fatalf("rank 3 sections should share root group: %#v", report.MatchedCourses)
	}
	if len(report.UnmatchedPolicyRows) != 1 || report.UnmatchedPolicyRows[0] != "SAT Verbal Believe" {
		t.Fatalf("unmatched policy rows = %#v", report.UnmatchedPolicyRows)
	}
}
