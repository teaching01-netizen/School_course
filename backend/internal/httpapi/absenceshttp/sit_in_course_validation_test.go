package absenceshttp

import "testing"

func TestResolvedSitInSelectionAllowsResolvedCrossSubjectTarget(t *testing.T) {
	result := &SitInResult{
		SitInMethod: SitInMethodPhysical,
		Priorities: []SitInPriorityResult{
			{
				SitInCourse: &SitInCourseInfo{ID: "target-rank-2", MergeGroupID: "target-merge"},
				Available:   []sessionBrief{{ID: "target-session"}},
			},
		},
	}

	if !resolvedSitInSelectionAllowsCourse(result, "target-rank-2", []string{"target-session"}) {
		t.Fatal("expected the policy-resolved cross-subject target to be accepted")
	}
	if resolvedSitInSelectionAllowsCourse(result, "unresolved-course", []string{"target-session"}) {
		t.Fatal("expected an unresolved target course to be rejected")
	}
	if resolvedSitInSelectionAllowsCourse(result, "target-rank-2", []string{"unresolved-session"}) {
		t.Fatal("expected a session outside the policy result to be rejected")
	}
}

func TestResolvedSitInSelectionAllowsMembersOfResolvedMergedTarget(t *testing.T) {
	result := &SitInResult{
		SitInMethod: SitInMethodPhysical,
		Priorities: []SitInPriorityResult{
			{
				SitInCourse: &SitInCourseInfo{ID: "target-reading", MergeGroupID: "target-merge"},
				Available:   []sessionBrief{{ID: "reading-session"}},
			},
			{
				SitInCourse: &SitInCourseInfo{ID: "target-writing", MergeGroupID: "target-merge"},
				Available:   []sessionBrief{{ID: "writing-session"}},
			},
		},
	}

	if !resolvedSitInSelectionAllowsCourse(result, "target-reading", []string{"reading-session", "writing-session"}) {
		t.Fatal("expected sessions from the resolved merged target to be accepted")
	}
}
