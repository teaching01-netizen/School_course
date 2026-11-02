package absenceshttp

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// Submit-time validation resolves the policy once (afterPriority=0) and must
// still accept a session the student reached via "See other times".
func TestSatVerbalSubmitValidation_AcceptsRevealedLowerPriorities(t *testing.T) {
	sec2ID := "a2000000-0000-0000-0000-000000000002"
	sec1ID := "a1000000-0000-0000-0000-000000000001"
	rank5ID := "a5000000-0000-0000-0000-000000000005"
	rank4ID := "a4000000-0000-0000-0000-000000000004"
	writing5ID := "a6000000-0000-0000-0000-000000000006"

	rules := mustDecodeSatVerbalPolicy(t, `[{
		"id": "sat-verbal-reading-beginner-sec2",
		"courseName": "SAT Verbal Reading Beginner Section 2",
		"lastClassExcluded": true,
		"priorities": [
			{"level": 1, "ruleType": "cross_section", "label": "1st", "makeupTargets": [{"section": "Section 1", "subject": "Reading Beginner"}]},
			{"level": 2, "ruleType": "rank_chain", "label": "2nd", "eligibleTargets": ["SAT Verbal Reading Rank 5"]},
			{"level": 3, "ruleType": "rank_chain", "label": "3rd", "eligibleTargets": ["SAT Verbal Reading Rank 4"]}
		]
	}]`)
	courses := []sqldb.SubjectCourseV2{
		satCourse(sec2ID, "SAT Verbal Reading Beginner Section 2"),
		satCourse(sec1ID, "SAT Verbal Reading Beginner Section 1"),
		satCourse(rank5ID, "SAT Verbal Reading Rank 5"),
		satCourse(rank4ID, "SAT Verbal Reading Rank 4"),
		satCourse(writing5ID, "SAT Verbal Writing Rank 5"),
	}
	missed := session("b2000000-0000-0000-0000-000000000002", sec2ID, "2026-02-08T02:00:00Z", "2026-02-08T04:00:00Z")
	sessionsByCourse := map[pgtype.UUID][]sqldb.SessionInRange{
		makeUUID(sec2ID): {
			session("b2000000-0000-0000-0000-000000000001", sec2ID, "2026-02-01T02:00:00Z", "2026-02-01T04:00:00Z"),
			missed,
			session("b2000000-0000-0000-0000-000000000003", sec2ID, "2026-02-15T02:00:00Z", "2026-02-15T04:00:00Z"),
		},
		makeUUID(sec1ID): {
			session("b1000000-0000-0000-0000-000000000001", sec1ID, "2026-02-02T10:00:00Z", "2026-02-02T12:00:00Z"),
			session("b1000000-0000-0000-0000-000000000002", sec1ID, "2026-02-09T10:00:00Z", "2026-02-09T12:00:00Z"),
			session("b1000000-0000-0000-0000-000000000003", sec1ID, "2026-02-16T10:00:00Z", "2026-02-16T12:00:00Z"),
		},
		makeUUID(rank5ID): {
			session("b5000000-0000-0000-0000-000000000001", rank5ID, "2026-02-10T10:00:00Z", "2026-02-10T12:00:00Z"),
			session("b5000000-0000-0000-0000-000000000002", rank5ID, "2026-02-24T10:00:00Z", "2026-02-24T12:00:00Z"),
		},
		makeUUID(rank4ID): {
			session("b4000000-0000-0000-0000-000000000001", rank4ID, "2026-02-11T10:00:00Z", "2026-02-11T12:00:00Z"),
			session("b4000000-0000-0000-0000-000000000002", rank4ID, "2026-02-25T10:00:00Z", "2026-02-25T12:00:00Z"),
		},
		makeUUID(writing5ID): {
			session("b6000000-0000-0000-0000-000000000001", writing5ID, "2026-02-12T10:00:00Z", "2026-02-12T12:00:00Z"),
			session("b6000000-0000-0000-0000-000000000002", writing5ID, "2026-02-26T10:00:00Z", "2026-02-26T12:00:00Z"),
		},
	}
	resolve := func(after int) *SitInResult {
		t.Helper()
		result, err := resolveSatVerbalPolicy(context.Background(), satVerbalResolveInput{
			Policy:             rules,
			MissedCourse:       courses[0],
			Enrolled:           []sqldb.StudentEnrolledCourseV2{satEnrolled(sec2ID, courses[0].Name)},
			AllCourses:         courses,
			MissedSessions:     []sqldb.SessionInRange{missed},
			AfterPriorityLevel: after,
			LoadSessions: func(_ context.Context, id pgtype.UUID) ([]sqldb.SessionInRange, error) {
				return sessionsByCourse[id], nil
			},
		})
		if err != nil || result == nil {
			t.Fatalf("resolve(after=%d) = %#v, %v", after, result, err)
		}
		return result
	}

	// What the student sees after one / two "See other times" taps.
	if got := resolve(1).CurrentPriorityLevel; got != 2 {
		t.Fatalf("after=1 level = %d, want 2", got)
	}
	if got := resolve(2).CurrentPriorityLevel; got != 3 {
		t.Fatalf("after=2 level = %d, want 3", got)
	}

	// What submit validation sees.
	levels, err := sitInPriorityLevels(func(after int) (*SitInResult, error) { return resolve(after), nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(levels) != 3 {
		t.Fatalf("validation walked %d levels, want 3", len(levels))
	}
	cases := []struct {
		name    string
		course  string
		session string
		want    bool
	}{
		{"P1 same lesson", sec1ID, "b1000000-0000-0000-0000-000000000002", true},
		{"P1 other lesson", sec1ID, "b1000000-0000-0000-0000-000000000003", false},
		{"P2 Rank 5", rank5ID, "b5000000-0000-0000-0000-000000000001", true},
		{"P3 Rank 4", rank4ID, "b4000000-0000-0000-0000-000000000001", true},
		{"P2 Rank 5 final day", rank5ID, "b5000000-0000-0000-0000-000000000002", false},
		{"off-chain Writing Rank 5", writing5ID, "b6000000-0000-0000-0000-000000000001", false},
	}
	for _, tc := range cases {
		if got := sitInSelectionAllowedAtAnyLevel(levels, tc.course, []pgtype.UUID{makeUUID(tc.session)}); got != tc.want {
			t.Errorf("%s: allowed = %v, want %v", tc.name, got, tc.want)
		}
	}
}
