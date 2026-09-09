package absenceshttp

// Step-9/13 barrier race test: same-student/same-sit-in-session.
//
// Two concurrent submissions for the SAME student and SAME session must not
// both hold the assignment: exactly one wins and the loser gets the
// deterministic sit_in_session_already_used conflict (decision D7). Both
// writers hold the student row lock (Step-9 F1: course-then-student order),
// so the loser observes the winner via the post-fence recheck (Step-9.4).

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func TestSubmission_SameStudentSameSessionRaceOneWinner(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suffix := uuid.NewString()[:8]
	q := sqldb.New(dbpool)

	var studentID, courseID, sessionID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1,$2) RETURNING id`, "wrace-"+suffix, "Race "+suffix).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	var teacherID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1,'Teacher','x') RETURNING id`, "t-race-"+suffix).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `INSERT INTO courses (code, name, level) VALUES ($1,$2,2) RETURNING id`, "RACE-"+suffix, "Race course").Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) VALUES ($1,$2,'enrolled')`, courseID, studentID); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	if err := dbpool.QueryRow(ctx, `INSERT INTO sessions (course_id, teacher_id, start_at, end_at) VALUES ($1,$2,$3,$4) RETURNING id`, courseID, teacherID, start, start.Add(time.Hour)).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}

	mkAbsence := func() pgtype.UUID {
		var id pgtype.UUID
		if err := dbpool.QueryRow(ctx, `INSERT INTO student_absences (wcode, course_id, date_from, date_to, reason) VALUES ((SELECT wcode FROM students WHERE id=$1),$2,'2026-10-01','2026-10-31','race') RETURNING id`, studentID, courseID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	absenceA := mkAbsence()
	absenceB := mkAbsence()

	release := make(chan struct{})
	// entered proves both contender txs began and are racing concurrently
	// (barrier, not sleep timing - Step 13 gate). The signal is sent BEFORE
	// the lock chain: the locks serialize (only one holder), so a post-lock
	// signal would deadlock the barrier. Buffer 2: both sends must complete
	// even though the main goroutine drains them after close(release).
	entered := make(chan struct{}, 2)
	results := make(chan error, 2)
	run := func(absenceID pgtype.UUID) {
		go func() {
			cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer ccancel()
			tx, err := dbpool.Begin(cctx)
			if err != nil {
				results <- err
				return
			}
			defer tx.Rollback(cctx)
			qtx := q.WithTx(tx)
			// Barrier signal BEFORE locks: both contenders serialize on the
			// course+student locks (only one can hold them), so signalling
			// after locking would deadlock the barrier itself. Signalling
			// after Begin proves both txs are genuinely concurrent before
			// either races for the lock chain.
			entered <- struct{}{}
			<-release
			// Step-9 F1 production order: course lock BEFORE student lock.
			if err := lockCourseForMergeScope(cctx, qtx, courseID); err != nil {
				results <- err
				return
			}
			if err := qtx.LockStudentForAbsenceSubmission(cctx, studentID); err != nil {
				results <- err
				return
			}
			if err := ensureSitInSessionsAvailable(cctx, qtx, studentID, []pgtype.UUID{sessionID}); err != nil {
				results <- err
				return
			}
			if err := lockSessionsForSubmission(cctx, qtx, []pgtype.UUID{sessionID}); err != nil {
				results <- err
				return
			}
			if err := ensureSitInSessionsAvailable(cctx, qtx, studentID, []pgtype.UUID{sessionID}); err != nil {
				results <- err
				return
			}
			if err := qtx.AbsenceSitInsCreateWithSnapshot(cctx, absenceID, []sqldb.SitInSnapshotInput{{SessionID: sessionID}}, "Asia/Bangkok", BuildSnapshotFromSessionRow); err != nil {
				results <- err
				return
			}
			results <- tx.Commit(cctx)
		}()
	}
	run(absenceA)
	run(absenceB)
	// Barrier: wait for both contender txs to begin racing (30s ceiling so
	// a stuck contender fails loudly instead of hanging the suite).
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(30 * time.Second):
			t.Fatal("race barrier: both contenders did not reach the locked region")
		}
	}
	close(release)

	errA := <-results
	errB := <-results
	succeeded := 0
	conflicted := 0
	for _, err := range []error{errA, errB} {
		if err == nil {
			succeeded++
			continue
		}
		if _, ok := err.(*sitInSessionAlreadyUsedError); ok {
			conflicted++
			continue
		}
		t.Fatalf("unexpected race outcome error: %v", err)
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("same-student/same-session race must yield exactly one winner + one conflict, got succeeded=%d conflicted=%d (%v / %v)", succeeded, conflicted, errA, errB)
	}
	var count int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM absence_sit_ins asi JOIN student_absences sa ON sa.id = asi.absence_id WHERE sa.wcode = (SELECT wcode FROM students WHERE id=$1) AND asi.session_id=$2 AND sa.status <> 'cancelled'`, studentID, sessionID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 live assignment row, got %d", count)
	}
}

// Step-10/13 barrier test: reassign-vs-submission on the same student/session.
//
// A staff reassign (course+student locks, session fence, post-fence recheck -
// management_routes.go handleSitInOverride) races a submission holding the
// same protocol. Exactly one writer must hold the sit-in session; the loser
// observes the winner via its post-fence recheck and aborts with the
// deterministic sit_in_session_already_used conflict. Final DB invariant:
// exactly one live assignment row for (student, session).
func TestSubmission_ReassignVsSubmissionRaceOneWinner(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suffix := uuid.NewString()[:8]
	q := sqldb.New(dbpool)

	var studentID, courseID, sessionID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1,$2) RETURNING id`, "wrear-"+suffix, "Rear "+suffix).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	var teacherID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1,'Teacher','x') RETURNING id`, "t-rear-"+suffix).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `INSERT INTO courses (code, name, level) VALUES ($1,$2,2) RETURNING id`, "REAR-"+suffix, "Rear course").Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) VALUES ($1,$2,'enrolled')`, courseID, studentID); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	if err := dbpool.QueryRow(ctx, `INSERT INTO sessions (course_id, teacher_id, start_at, end_at) VALUES ($1,$2,$3,$4) RETURNING id`, courseID, teacherID, start, start.Add(time.Hour)).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}

	mkAbsence := func() pgtype.UUID {
		var id pgtype.UUID
		if err := dbpool.QueryRow(ctx, `INSERT INTO student_absences (wcode, course_id, date_from, date_to, reason) VALUES ((SELECT wcode FROM students WHERE id=$1),$2,'2026-10-01','2026-10-31','race') RETURNING id`, studentID, courseID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	submitAbsence := mkAbsence()
	reassignAbsence := mkAbsence()

	release := make(chan struct{})
	entered := make(chan struct{}, 2)
	results := make(chan error, 2)
	// Submitter: Step-9.4 create protocol (course -> student -> pre-check ->
	// fence -> post-fence recheck -> snapshot insert).
	go func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		tx, err := dbpool.Begin(cctx)
		if err != nil {
			results <- err
			return
		}
		defer tx.Rollback(cctx)
		qtx := q.WithTx(tx)
		entered <- struct{}{}
		<-release
		if err := lockCourseForMergeScope(cctx, qtx, courseID); err != nil {
			results <- err
			return
		}
		if err := qtx.LockStudentForAbsenceSubmission(cctx, studentID); err != nil {
			results <- err
			return
		}
		if err := ensureSitInSessionsAvailable(cctx, qtx, studentID, []pgtype.UUID{sessionID}); err != nil {
			results <- err
			return
		}
		if err := lockSessionsForSubmission(cctx, qtx, []pgtype.UUID{sessionID}); err != nil {
			results <- err
			return
		}
		if err := ensureSitInSessionsAvailable(cctx, qtx, studentID, []pgtype.UUID{sessionID}); err != nil {
			results <- err
			return
		}
		if err := qtx.AbsenceSitInsCreateWithSnapshot(cctx, submitAbsence, []sqldb.SitInSnapshotInput{{SessionID: sessionID}}, "Asia/Bangkok", BuildSnapshotFromSessionRow); err != nil {
			results <- err
			return
		}
		results <- tx.Commit(cctx)
	}()
	// Reassigner: Step-10 G3 protocol (course -> student -> fence ->
	// post-fence recheck -> ReplaceWithSnapshot). Mirrors handleSitInOverride
	// post-validation section; the version-guarded AbsenceSitInUpdate is
	// subsumed here by the test-owned absence row (no concurrent status
	// writer in this schedule).
	go func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		tx, err := dbpool.Begin(cctx)
		if err != nil {
			results <- err
			return
		}
		defer tx.Rollback(cctx)
		qtx := q.WithTx(tx)
		entered <- struct{}{}
		<-release
		if err := lockCourseForMergeScope(cctx, qtx, courseID); err != nil {
			results <- err
			return
		}
		if err := qtx.LockStudentForAbsenceSubmission(cctx, studentID); err != nil {
			results <- err
			return
		}
		if err := lockSessionsForSubmission(cctx, qtx, []pgtype.UUID{sessionID}); err != nil {
			results <- err
			return
		}
		if err := ensureSitInSessionsAvailable(cctx, qtx, studentID, []pgtype.UUID{sessionID}); err != nil {
			results <- err
			return
		}
		if err := qtx.AbsenceSitInsReplaceWithSnapshot(cctx, reassignAbsence, []pgtype.UUID{sessionID}, "Asia/Bangkok", BuildSnapshotFromSessionRow); err != nil {
			results <- err
			return
		}
		results <- tx.Commit(cctx)
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(30 * time.Second):
			t.Fatal("race barrier: both contenders did not start")
		}
	}
	close(release)

	errA := <-results
	errB := <-results
	succeeded := 0
	conflicted := 0
	for _, err := range []error{errA, errB} {
		if err == nil {
			succeeded++
			continue
		}
		if _, ok := err.(*sitInSessionAlreadyUsedError); ok {
			conflicted++
			continue
		}
		t.Fatalf("unexpected race outcome error: %v", err)
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("reassign-vs-submission race must yield exactly one winner + one conflict, got succeeded=%d conflicted=%d (%v / %v)", succeeded, conflicted, errA, errB)
	}
	var count int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM absence_sit_ins asi JOIN student_absences sa ON sa.id = asi.absence_id WHERE sa.wcode = (SELECT wcode FROM students WHERE id=$1) AND asi.session_id=$2 AND sa.status <> 'cancelled'`, studentID, sessionID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 live assignment row, got %d", count)
	}
}

// Step-13 item 3: cancellation-vs-submission on the same student/session.
func TestSubmission_CancelVsSubmissionRace(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suffix := uuid.NewString()[:8]
	q := sqldb.New(dbpool)

	var studentID, courseID, sessionID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1,$2) RETURNING id`, "wcan-"+suffix, "Cancel "+suffix).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	var teacherID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1,'Teacher','x') RETURNING id`, "t-can-"+suffix).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `INSERT INTO courses (code, name, level) VALUES ($1,$2,2) RETURNING id`, "CAN-"+suffix, "Cancel course").Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) VALUES ($1,$2,'enrolled')`, courseID, studentID); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	if err := dbpool.QueryRow(ctx, `INSERT INTO sessions (course_id, teacher_id, start_at, end_at) VALUES ($1,$2,$3,$4) RETURNING id`, courseID, teacherID, start, start.Add(time.Hour)).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}

	mkAbsence := func() pgtype.UUID {
		var id pgtype.UUID
		if err := dbpool.QueryRow(ctx, `INSERT INTO student_absences (wcode, course_id, date_from, date_to, reason) VALUES ((SELECT wcode FROM students WHERE id=$1),$2,'2026-10-01','2026-10-31','race') RETURNING id`, studentID, courseID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	cancelAbsence := mkAbsence()
	submitAbsence := mkAbsence()
	if err := q.AbsenceSitInsCreateWithSnapshot(ctx, cancelAbsence, []sqldb.SitInSnapshotInput{{SessionID: sessionID}}, "Asia/Bangkok", BuildSnapshotFromSessionRow); err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	entered := make(chan struct{}, 2)
	results := make(chan error, 2)
	go func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		tx, err := dbpool.Begin(cctx)
		if err != nil {
			results <- err
			return
		}
		defer tx.Rollback(cctx)
		qtx := q.WithTx(tx)
		entered <- struct{}{}
		<-release
		if err := lockCourseForMergeScope(cctx, qtx, courseID); err != nil {
			results <- err
			return
		}
		if err := qtx.LockStudentForAbsenceSubmission(cctx, studentID); err != nil {
			results <- err
			return
		}
		if err := ensureSitInSessionsAvailable(cctx, qtx, studentID, []pgtype.UUID{sessionID}); err != nil {
			results <- err
			return
		}
		if err := lockSessionsForSubmission(cctx, qtx, []pgtype.UUID{sessionID}); err != nil {
			results <- err
			return
		}
		if err := ensureSitInSessionsAvailable(cctx, qtx, studentID, []pgtype.UUID{sessionID}); err != nil {
			results <- err
			return
		}
		if err := qtx.AbsenceSitInsCreateWithSnapshot(cctx, submitAbsence, []sqldb.SitInSnapshotInput{{SessionID: sessionID}}, "Asia/Bangkok", BuildSnapshotFromSessionRow); err != nil {
			results <- err
			return
		}
		results <- tx.Commit(cctx)
	}()
	go func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		tx, err := dbpool.Begin(cctx)
		if err != nil {
			results <- err
			return
		}
		defer tx.Rollback(cctx)
		qtx := q.WithTx(tx)
		entered <- struct{}{}
		<-release
		var version int32
		if err := tx.QueryRow(cctx, `SELECT version FROM student_absences WHERE id=$1`, cancelAbsence).Scan(&version); err != nil {
			results <- err
			return
		}
		var adminID pgtype.UUID
		if err := tx.QueryRow(cctx, `SELECT id FROM users WHERE username=$1`, "t-can-"+suffix).Scan(&adminID); err != nil {
			results <- err
			return
		}
		if _, err := qtx.AbsenceStatusUpdate(cctx, cancelAbsence, "cancelled", adminID, version); err != nil {
			results <- err
			return
		}
		if err := qtx.AbsenceSitInsReplaceWithSnapshot(cctx, cancelAbsence, nil, "Asia/Bangkok", BuildSnapshotFromSessionRow); err != nil {
			results <- err
			return
		}
		results <- tx.Commit(cctx)
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(30 * time.Second):
			t.Fatal("race barrier: both contenders did not start")
		}
	}
	close(release)

	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err == nil {
				continue
			}
			if _, ok := err.(*sitInSessionAlreadyUsedError); ok {
				continue
			}
			t.Fatalf("unexpected race outcome error: %v", err)
		case <-time.After(30 * time.Second):
			t.Fatal("race barrier: contender did not finish")
		}
	}
	var cleared int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM absence_sit_ins WHERE absence_id=$1`, cancelAbsence).Scan(&cleared); err != nil {
		t.Fatal(err)
	}
	if cleared != 0 {
		t.Fatalf("cancelled absence holds %d sit-in rows, want 0", cleared)
	}
	var status string
	if err := dbpool.QueryRow(ctx, `SELECT status FROM student_absences WHERE id=$1`, cancelAbsence).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" {
		t.Fatalf("cancel absence status = %q, want cancelled", status)
	}
}

// Step-13 item 10 / D7 second half: cross-student same-session must BOTH succeed.
//
// Decision D7 pins exclusivity as same-student/session only: there is no
// global UNIQUE(session_id) on absence_sit_ins, so two DIFFERENT students
// claiming the same sit-in session concurrently must both commit. This test
// would fail if anyone added a global unique constraint on session_id.
// Both writers hold DIFFERENT student row locks (no serialization between
// them); the shared course lock + session fence serialize only briefly.
func TestSubmission_CrossStudentSameSessionBothSucceed(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suffix := uuid.NewString()[:8]
	q := sqldb.New(dbpool)

	var courseX, sessionX, teacherX pgtype.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1,'Teacher','x') RETURNING id`, "t-x-"+suffix).Scan(&teacherX); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `INSERT INTO courses (code, name, level) VALUES ($1,$2,2) RETURNING id`, "CROSS-"+suffix, "Cross course").Scan(&courseX); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC)
	if err := dbpool.QueryRow(ctx, `INSERT INTO sessions (course_id, teacher_id, start_at, end_at) VALUES ($1,$2,$3,$4) RETURNING id`, courseX, teacherX, start, start.Add(time.Hour)).Scan(&sessionX); err != nil {
		t.Fatal(err)
	}
	mkPair := func(tag string) (pgtype.UUID, pgtype.UUID) {
		var sid, aid pgtype.UUID
		if err := dbpool.QueryRow(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1,$2) RETURNING id`, "wx-"+tag+"-"+suffix, "X "+tag+" "+suffix).Scan(&sid); err != nil {
			t.Fatal(err)
		}
		if _, err := dbpool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) VALUES ($1,$2,'enrolled')`, courseX, sid); err != nil {
			t.Fatal(err)
		}
		if err := dbpool.QueryRow(ctx, `INSERT INTO student_absences (wcode, course_id, date_from, date_to, reason) VALUES ((SELECT wcode FROM students WHERE id=$1),$2,'2026-10-01','2026-10-31','race') RETURNING id`, sid, courseX).Scan(&aid); err != nil {
			t.Fatal(err)
		}
		return sid, aid
	}
	studentA, absenceA := mkPair("a")
	studentB, absenceB := mkPair("b")

	release := make(chan struct{})
	entered := make(chan struct{}, 2)
	results := make(chan error, 2)
	run := func(studentID, absenceID pgtype.UUID) {
		go func() {
			cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer ccancel()
			tx, err := dbpool.Begin(cctx)
			if err != nil {
				results <- err
				return
			}
			defer tx.Rollback(cctx)
			qtx := q.WithTx(tx)
			entered <- struct{}{}
			<-release
			if err := lockCourseForMergeScope(cctx, qtx, courseX); err != nil {
				results <- err
				return
			}
			if err := qtx.LockStudentForAbsenceSubmission(cctx, studentID); err != nil {
				results <- err
				return
			}
			if err := ensureSitInSessionsAvailable(cctx, qtx, studentID, []pgtype.UUID{sessionX}); err != nil {
				results <- err
				return
			}
			if err := lockSessionsForSubmission(cctx, qtx, []pgtype.UUID{sessionX}); err != nil {
				results <- err
				return
			}
			if err := ensureSitInSessionsAvailable(cctx, qtx, studentID, []pgtype.UUID{sessionX}); err != nil {
				results <- err
				return
			}
			if err := qtx.AbsenceSitInsCreateWithSnapshot(cctx, absenceID, []sqldb.SitInSnapshotInput{{SessionID: sessionX}}, "Asia/Bangkok", BuildSnapshotFromSessionRow); err != nil {
				results <- err
				return
			}
			results <- tx.Commit(cctx)
		}()
	}
	run(studentA, absenceA)
	run(studentB, absenceB)
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(30 * time.Second):
			t.Fatal("race barrier: both contenders did not start")
		}
	}
	close(release)

	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("cross-student contender failed (both must succeed): %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("race barrier: contender did not finish")
		}
	}
	var count int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM absence_sit_ins WHERE session_id=$1`, sessionX).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 assignment rows (one per student), got %d", count)
	}
}
