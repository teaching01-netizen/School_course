package absenceshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type lostSubmitACK struct {
	header http.Header
	status int
}

func (w *lostSubmitACK) Header() http.Header       { return w.header }
func (w *lostSubmitACK) WriteHeader(status int)    { w.status = status }
func (w *lostSubmitACK) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestFormalCorrespondenceLostAckAndConcurrentPhysicalSubmitReplay(t *testing.T) {
	f := seedFinalSessionFixture(t)
	peer := f
	peer.pool = newStaffPool(t, requireStaffTestDB(t))
	t.Cleanup(peer.pool.Close)
	peer.q = sqldb.New(peer.pool)
	first, second := f.staffServer(t), peer.staffServer(t)
	body := f.requestBody()
	body["sit_in_session_ids"] = []string{f.earlierID}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	key := "formal-lost-ack-" + uuid.NewString()
	request := func(url string) *http.Request {
		r, err := http.NewRequest(http.MethodPost, url+"/api/v1/absences", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key)
		return r
	}
	// The business transaction commits, but writing its HTTP body fails.
	lost := &lostSubmitACK{header: make(http.Header)}
	first.Config.Handler.ServeHTTP(lost, request(first.URL))
	if lost.status != http.StatusCreated {
		t.Fatalf("first submit status=%d", lost.status)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	type result struct {
		status int
		body   []byte
		err    error
	}
	const workers = 16
	ready, release := make(chan struct{}, workers), make(chan struct{})
	results := make(chan result, workers)
	client := &http.Client{Timeout: 25 * time.Second}
	for i := 0; i < workers; i++ {
		url := first.URL
		if i%2 != 0 {
			url = second.URL
		}
		r := request(url).WithContext(ctx)
		go func() {
			ready <- struct{}{}
			<-release
			response, err := client.Do(r)
			if err != nil {
				results <- result{err: err}
				return
			}
			defer response.Body.Close()
			data, err := io.ReadAll(response.Body)
			results <- result{response.StatusCode, data, err}
		}()
	}
	for i := 0; i < workers; i++ {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(release)
	var replay []byte
	for i := 0; i < workers; i++ {
		var got result
		select {
		case got = <-results:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		if got.err != nil || got.status != http.StatusCreated {
			t.Fatalf("replay status=%d err=%v body=%s", got.status, got.err, got.body)
		}
		if replay == nil {
			replay = got.body
		}
		if !bytes.Equal(replay, got.body) {
			t.Fatal("replicas returned different stored responses")
		}
	}
	var absences, assignments, missed int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM student_absences WHERE wcode=$1`, f.wcode).Scan(&absences); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM absence_sit_ins a JOIN student_absences s ON s.id=a.absence_id WHERE s.wcode=$1`, f.wcode).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM absence_missed_sessions a JOIN student_absences s ON s.id=a.absence_id WHERE s.wcode=$1`, f.wcode).Scan(&missed); err != nil {
		t.Fatal(err)
	}
	if absences != 1 || assignments != 1 || missed != 1 {
		t.Fatalf("after lost ACK and replays: absence/sit-in/missed=%d/%d/%d, want 1/1/1", absences, assignments, missed)
	}
	t.Log("lost response body followed by 16 concurrent replays across two HTTP handlers and independent pools; one absence, sit-in, and missed-session row")
}

// This is implementation correspondence evidence, not a Lean proof of Go.
// The catalogs have coherent IDs/groups, as required by the formal scope model
// and enforced by the policy apply endpoint's seenCourses check.
func TestFormalCorrespondenceFocusedResolverMatchesFullPolicy(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 8706618))
	rules := mustDecodeSatVerbalPolicy(t, string(satVerbalPolicyRulesJSON))
	var rule satVerbalCourseRule
	for _, item := range rules {
		if item.ID == "sat-verbal-reading-beginner-sec2" {
			rule = item
		}
	}
	if len(rule.Priorities) == 0 {
		t.Fatal("missing real policy fixture")
	}
	names := []string{
		"SAT Verbal Reading Beginner Section 2", "SAT Verbal Reading Beginner Section 1",
		"SAT Verbal Reading Rank 5", "SAT Verbal Writing Rank 5", "SAT Verbal Reading Rank 4",
	}
	compared, accepted, rejected := 0, 0, 0
	for sample := 0; sample < 128; sample++ {
		courses := make([]sqldb.SubjectCourseV2, len(names))
		mapped := make([]satVerbalMappedCourse, len(names))
		schedules := make(map[pgtype.UUID][]sqldb.SessionInRange)
		var allIDs []string
		for i, name := range names {
			id := fmt.Sprintf("a0000000-0000-0000-0000-%012d", i+1)
			courses[i] = satCourse(id, name)
			mapped[i] = satVerbalMappedCourse{Rule: satVerbalCourseRule{CourseName: name}, Course: courses[i]}
			count := 3 + rng.IntN(4)
			for n := 0; n < count; n++ {
				sessionID := fmt.Sprintf("b0000000-0000-0000-%04d-%012d", i+1, n+1)
				at := time.Date(2030, time.January, 1+n*7+rng.IntN(4), 2+rng.IntN(8), 0, 0, 0, time.UTC)
				schedules[courses[i].ID] = append(schedules[courses[i].ID], session(sessionID, id, at.Format(time.RFC3339), at.Add(2*time.Hour).Format(time.RFC3339)))
				allIDs = append(allIDs, sessionID)
			}
		}
		if sample%2 == 0 {
			group := makeUUID("c0000000-0000-0000-0000-000000000001")
			for _, i := range []int{2, 3} {
				courses[i].MergeGroupID = group
				mapped[i].Course = courses[i]
				mapped[i].Rule.CourseName = names[2]
			}
		}
		blocked := make(map[string]struct{})
		if sample%3 == 0 {
			blocked[allIDs[rng.IntN(len(allIDs))]] = struct{}{}
		}
		missed := schedules[courses[0].ID][1:2]
		if sample%4 == 0 {
			missed = schedules[courses[0].ID][0:2]
		}
		input := satVerbalResolveInput{
			Rule: &rule, MappedCourses: mapped, AllCourses: courses, MissedCourse: courses[0],
			Enrolled:       []sqldb.StudentEnrolledCourseV2{satEnrolled(courses[0].ID.String(), names[0])},
			MissedSessions: missed, IncludeAllPriorities: true, InstituteTZ: "Asia/Bangkok",
			RequestTime:            time.Date(2029, time.December, 31, 0, 0, 0, 0, time.UTC),
			BlockedSitInSessionIDs: blocked,
			LoadSessions:           func(_ context.Context, id pgtype.UUID) ([]sqldb.SessionInRange, error) { return schedules[id], nil },
		}
		if sample%5 == 0 {
			input.Cutoff = time.Date(2030, time.January, 18, 0, 0, 0, 0, time.UTC)
		}
		full, err := resolveSatVerbalPolicy(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range courses {
			input.SelectedTargetCourseID = target.ID
			focused, err := resolveSatVerbalPolicy(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			for i, id := range allIDs {
				selections := [][]string{{id}, {id, allIDs[(i+1)%len(allIDs)]}}
				for _, selected := range selections {
					want := resolvedSitInSelectionAllowsCourse(full, target.ID.String(), selected)
					got := resolvedSitInSelectionAllowsCourse(focused, target.ID.String(), selected)
					if got != want {
						t.Fatalf("sample=%d target=%s selected=%v: focused=%t full=%t", sample, target.Name, selected, got, want)
					}
					compared++
					if got {
						accepted++
					} else {
						rejected++
					}
				}
			}
		}
	}
	if accepted == 0 || rejected == 0 {
		t.Fatalf("vacuous correspondence check: accepted=%d rejected=%d", accepted, rejected)
	}
	t.Logf("128 coherent catalogs; %d selections compared; %d accepted, %d rejected", compared, accepted, rejected)
}
