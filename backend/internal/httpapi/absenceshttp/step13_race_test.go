package absenceshttp

// Step-13 deterministic race gates, part 2: same-key cross-replica,
// opposite-order batches, merge-membership serialization. All use barriers
// with generous ceilings. No sleep timing gates correctness.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/auth"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	sqldb "warwick-institute/internal/db"
)

// Step-13 item 7: same idempotency key raced across two replicas.
//
// Two concurrent HTTP staff-create requests carrying the SAME key and
// byte-identical bodies, served by two server instances over one pool (what
// two load-balanced replicas share is the database), must leave exactly ONE
// absence row. Permitted outcomes are deterministic: either the loser lands
// while the winner is in flight and gets 409, or it lands after commit and
// replays the winner bytes. No outcome may create a second row, and a later
// sequential retry with the same key must replay the winner bytes.
func TestIdempotency_SameKeyAcrossReplicasOneRow(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	q := sqldb.New(dbpool)
	wcode, subjectIDStr, courseIDStr, seedSessionIDs := seedAbsenceLimitTestData(t, q, dbpool, "RPL", 20)

	var adminUserID pgtype.UUID
	if err := dbpool.QueryRow(ctx,
		"INSERT INTO users (username, role, password_hash) VALUES ($1, $2, $3) RETURNING id",
		"idem-replica-admin-"+uuid.NewString(), "Admin", "x").Scan(&adminUserID); err != nil {
		t.Fatal(err)
	}
	adminUUID, err := uuid.FromBytes(adminUserID.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}
	fakeAuth := absenceLimitFakeAuth{user: auth.AuthenticatedUser{ID: adminUUID, Role: "Admin"}}
	newServer := func() *server {
		return &server{
			deps: httpdeps.Deps{
				Q:           q,
				DB:          dbpool,
				Log:         slog.Default(),
				InstituteTZ: "Asia/Bangkok",
				Auth:        fakeAuth,
			},
			a: httpadapter.New(fakeAuth, slog.Default()),
		}
	}

	body := map[string]any{
		"wcode":              wcode,
		"subject_id":         subjectIDStr,
		"course_id":          courseIDStr,
		"date_from":          "2026-06-03",
		"date_to":            "2026-06-03",
		"reason":             "replica race",
		"sit_in_method":      "zoom",
		"missed_session_ids": []string{seedSessionIDs[2]},
	}
	key := "idem-replica-" + uuid.NewString()

	release := make(chan struct{})
	entered := make(chan struct{}, 2)
	results := make(chan *httptest.ResponseRecorder, 2)
	post := func() {
		go func() {
			reqBody, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/staff/absences", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", key)
			entered <- struct{}{}
			<-release
			w := httptest.NewRecorder()
			newServer().handleStaffCreateAbsence(w, req)
			results <- w
		}()
	}
	post()
	post()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(30 * time.Second):
			t.Fatal("race barrier: both replica requests did not start")
		}
	}
	close(release)

	got := make([]*httptest.ResponseRecorder, 0, 2)
	for i := 0; i < 2; i++ {
		select {
		case w := <-results:
			got = append(got, w)
		case <-time.After(60 * time.Second):
			t.Fatal("race barrier: replica request did not finish")
		}
	}
	var winnerBody string
	for i, w := range got {
		if w.Code == http.StatusCreated || w.Code == http.StatusOK {
			if winnerBody == "" {
				winnerBody = w.Body.String()
			continue
		}
			if w.Body.String() != winnerBody {
				t.Fatalf("two successes with different bodies: %s vs %s", winnerBody, w.Body.String())
			}
			continue
		}
		if w.Code != http.StatusConflict {
			t.Fatalf("replica %d: got %d, want success or 409: %s", i, w.Code, w.Body.String())
		}
	}
	if winnerBody == "" {
		t.Fatalf("no replica succeeded: %d / %d", got[0].Code, got[1].Code)
	}
	countRows := func() int {
		t.Helper()
		var n int
		if err := dbpool.QueryRow(ctx, "SELECT count(*) FROM student_absences WHERE wcode = $1", wcode).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := countRows(); n != 1 {
		t.Fatalf("after same-key replica race: %d absence rows, want 1", n)
	}
	reqBody, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/staff/absences", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	replay := httptest.NewRecorder()
	newServer().handleStaffCreateAbsence(replay, req)
	if replay.Body.String() != winnerBody {
		t.Fatalf("sequential replay differs from winner: %s vs %s", replay.Body.String(), winnerBody)
	}
	if n := countRows(); n != 1 {
		t.Fatalf("after sequential replay: %d absence rows, want 1", n)
	}
}

// Step-13 item 9: opposite-order batches must not deadlock.
//
// Two concurrent batches covering the SAME two courses in OPPOSITE item
// order race for course locks. The Step-9 G1 fix (lockBatchCourseSet takes
// the full set in immutable-ID order before item 1) makes both acquire in
// the same order, so AB-BA deadlock is impossible. A deadlock would surface
// as a 40P01 error or a barrier timeout; both fail loudly here.
func TestBatch_OppositeOrderNoDeadlock(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	suffix := uuid.NewString()[:8]
	q := sqldb.New(dbpool)
	mkCourse := func(tag string) pgtype.UUID {
		var courseID pgtype.UUID
		if err := dbpool.QueryRow(ctx, "INSERT INTO courses (code, name, level) VALUES ($1, $2, 2) RETURNING id", "OPP-"+tag+"-"+suffix, "Opp "+tag).Scan(&courseID); err != nil {
			t.Fatal(err)
		}
		return courseID
	}
	courseA := mkCourse("a")
	courseB := mkCourse("b")
	uuidStr := func(id pgtype.UUID) string {
		u, err := uuid.FromBytes(id.Bytes[:])
		if err != nil {
			t.Fatal(err)
		}
		return u.String()
	}
	courseAStr, courseBStr := uuidStr(courseA), uuidStr(courseB)

	release := make(chan struct{})
	entered := make(chan struct{}, 2)
	results := make(chan error, 2)
	run := func(first, second string) {
		go func() {
			cctx, ccancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer ccancel()
			tx, err := dbpool.Begin(cctx)
			if err != nil {
				results <- err
				return
			}
			defer tx.Rollback(cctx)
			entered <- struct{}{}
			<-release
			items := []batchAbsenceCreateItem{{CourseID: first}, {CourseID: second}}
			if err := lockBatchCourseSet(cctx, q.WithTx(tx), items); err != nil {
				results <- err
				return
			}
			results <- tx.Commit(cctx)
		}()
	}
	run(courseAStr, courseBStr)
	run(courseBStr, courseAStr)
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(30 * time.Second):
			t.Fatal("race barrier: both batches did not start")
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("opposite-order batch failed: %v", err)
			}
		case <-time.After(60 * time.Second):
			t.Fatal("opposite-order batches did not finish, possible deadlock")
		}
	}
}

// Step-13 item 6, sensitivity control: merge-membership writes serialize
// with the submission course lock.
//
// lockCourseForMergeScope takes FOR UPDATE OF c on the course row. A merge
// editor assigning membership must take the same course lock first, so the
// two serialize and a submission observes pre-change or post-change scope,
// never a torn mix. This test holds the lock in tx1 as a submission would
// after F1, then proves the writer blocks until tx1 commits. The short
// probe delay below is a liveness probe only: if the writer finishes early
// the shared-lock discipline is broken and the test fails loudly; if the
// writer is merely slow to start, the test still passes after commit.
func TestMergeMembership_ChangeSerializesWithSubmissionLock(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suffix := uuid.NewString()[:8]
	q := sqldb.New(dbpool)
	var adminID, courseID pgtype.UUID
	if err := dbpool.QueryRow(ctx, "INSERT INTO users (username, role, password_hash) VALUES ($1, $2, $3) RETURNING id", "t-mrg-"+suffix, "Admin", "x").Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, "INSERT INTO courses (code, name, level) VALUES ($1, $2, 2) RETURNING id", "MRG-"+suffix, "Merge course").Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	group, err := q.CourseMergeGroupCreate(ctx, "mrg-"+suffix, adminID)
	if err != nil {
		t.Fatal(err)
	}

	tx1, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx1.Rollback(ctx)
	if err := lockCourseForMergeScope(ctx, q.WithTx(tx1), courseID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		tx2, err := dbpool.Begin(cctx)
		if err != nil {
			done <- err
			return
		}
		defer tx2.Rollback(cctx)
		qtx2 := q.WithTx(tx2)
		if _, err := qtx2.CourseMergeGroupLockCourses(cctx, []pgtype.UUID{courseID}); err != nil {
			done <- err
			return
		}
		if err := qtx2.CourseMergeGroupAssignCourse(cctx, group.ID, courseID, 1); err != nil {
			done <- err
			return
		}
		_, found, err := mergeGroupScopeForCourse(cctx, qtx2, courseID)
		if err != nil {
			done <- err
			return
		}
		if !found {
			done <- errors.New("expected merge scope after assign")
			return
		}
		done <- tx2.Commit(cctx)
	}()

	select {
	case err := <-done:
		t.Fatalf("merge writer finished while submission holds the course lock: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("merge writer failed after lock release: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("merge writer did not finish after lock release")
	}
}
