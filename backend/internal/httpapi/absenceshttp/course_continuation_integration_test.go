package absenceshttp

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/coursegroups"
	sqldb "warwick-institute/internal/db"
)

// One real course split across two course IDs (same class, two teachers in
// CRM). The student is enrolled only under the configured ID; after linking,
// the split ID's sessions are offered and read the source's rules live.
func TestCourseContinuationLink(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx := context.Background()
	q := sqldb.New(dbpool)

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := dbpool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustScan := func(dst any, sql string, args ...any) {
		t.Helper()
		if err := dbpool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}

	suffix := uuid.NewString()[:8]
	wcode := "wcont" + suffix
	cycleID := "CY-CONT-" + suffix
	sourceCode, splitCode := "CONT-B-"+suffix, "CONT-TY-"+suffix
	var studentID, subjID, teacherID, rootID, sourceID, splitID, splitSessionID uuid.UUID
	mustScan(&studentID, `INSERT INTO students (wcode, full_name) VALUES ($1, 'Continuation') RETURNING id`, wcode)
	mustScan(&subjID, `INSERT INTO subjects (code, name) VALUES ($1, 'SAT Math') RETURNING id`, "CSUBJ-"+suffix)
	mustScan(&teacherID, `INSERT INTO users (username, role, password_hash) VALUES ($1, 'Teacher', 'x') RETURNING id`, "teacher-cont-"+suffix)
	mustScan(&rootID, `INSERT INTO root_course_groups (name) VALUES ($1) RETURNING id`, "SAT Math "+suffix)
	mustExec(`INSERT INTO crm_cycles (id, label) VALUES ($1, $1) ON CONFLICT (id) DO NOTHING`, cycleID)
	mustScan(&sourceID, `
		INSERT INTO courses (code, name, subject_id, cycle_id, level, root_course_group_id)
		VALUES ($1, 'SAT Math: Rank 1 C3', $2, $3, 1, $4) RETURNING id
	`, sourceCode, subjID, cycleID, rootID)
	mustScan(&splitID, `INSERT INTO courses (code, name, subject_id) VALUES ($1, 'SAT Math: Rank 1 C3', $2) RETURNING id`, splitCode, subjID)
	addActiveCourseRow(t, dbpool, subjID, sourceID)
	mustExec(`INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, 'enrolled')`, sourceID, studentID)
	for i, courseID := range []uuid.UUID{sourceID, splitID} {
		start := time.Now().UTC().AddDate(0, 0, 7+i)
		mustScan(&splitSessionID, `
			INSERT INTO sessions (course_id, teacher_id, start_at, end_at) VALUES ($1, $2, $3, $4) RETURNING id
		`, courseID, teacherID, start, start.Add(time.Hour))
	}

	if codes, _ := querySessionCourseCodes(t, dbpool, wcode); codes[splitCode] {
		t.Fatalf("unlinked split course must stay hidden, got %v", codes)
	}

	pg := func(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }
	errCode := func(err error) string {
		var domainErr *coursegroups.Error
		if errors.As(err, &domainErr) {
			return domainErr.Code
		}
		return ""
	}
	link := func(source uuid.UUID) (pgtype.UUID, error) {
		tx, err := dbpool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		result, err := coursegroups.NewService().CreateTx(ctx, q.WithTx(tx), coursegroups.CreateCommand{
			ActorID:            pg(teacherID),
			Name:               "SAT Math continuation " + suffix,
			CourseIDs:          []pgtype.UUID{pg(sourceID), pg(splitID)},
			RuleSourceCourseID: pg(source),
		})
		if err != nil {
			return pgtype.UUID{}, err
		}
		return result.GroupID, tx.Commit(ctx)
	}

	if _, err := link(uuid.New()); errCode(err) != "invalid_rule_source" {
		t.Fatalf("source outside the pair: err = %v, want invalid_rule_source", err)
	}
	mustExec(`INSERT INTO sat_verbal_policy_mappings (rule_id, course_id) VALUES ($1, $2)`, "cont-"+suffix, splitID)
	if _, err := link(sourceID); errCode(err) != "sat_verbal_course" {
		t.Fatalf("SAT Verbal course: err = %v, want sat_verbal_course", err)
	}
	mustExec(`DELETE FROM sat_verbal_policy_mappings WHERE rule_id = $1`, "cont-"+suffix)

	groupID, err := link(sourceID)
	if err != nil {
		t.Fatal(err)
	}

	codes, rows := querySessionCourseCodes(t, dbpool, wcode)
	if !codes[sourceCode] || !codes[splitCode] || rows != 2 {
		t.Fatalf("linked course must offer both IDs' sessions once each, got %v (%d rows)", codes, rows)
	}

	assertSplitConfig := func(wantLevel int32) {
		t.Helper()
		var level int32
		var cycle string
		var root uuid.UUID
		if err := dbpool.QueryRow(ctx, `
			SELECT level, cycle_id, root_course_group_id FROM course_rule_configs WHERE course_id = $1
		`, splitID).Scan(&level, &cycle, &root); err != nil {
			t.Fatal(err)
		}
		if level != wantLevel || cycle != cycleID || root != rootID {
			t.Fatalf("split config = level %d, cycle %s, root %s; want %d, %s, %s", level, cycle, root, wantLevel, cycleID, rootID)
		}
	}
	assertSplitConfig(1)
	mustExec(`UPDATE courses SET level = 2 WHERE id = $1`, sourceID)
	assertSplitConfig(2)

	mustExec(`INSERT INTO session_attendance (session_id, student_id, status) VALUES ($1, $2, 'excluded')`, splitSessionID, studentID)
	if codes, _ := querySessionCourseCodes(t, dbpool, wcode); codes[splitCode] {
		t.Fatalf("attendance exclusion must still hide the split session, got %v", codes)
	}

	var pgErr *pgconn.PgError
	if _, err := dbpool.Exec(ctx, `DELETE FROM courses WHERE id = $1`, sourceID); !errors.As(err, &pgErr) ||
		pgErr.ConstraintName != "course_merge_groups_rule_source_member_fk" {
		t.Fatalf("deleting the rule source: err = %v, want rule source FK violation", err)
	}

	mustExec(`
		INSERT INTO student_absences (wcode, course_id, date_from, date_to, merge_group_id)
		VALUES ($1, $2, current_date, current_date, $3)
	`, wcode, splitID, groupID)
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := coursegroups.NewService().DeleteTx(ctx, q.WithTx(tx), coursegroups.DeleteCommand{
		ActorID: pg(teacherID),
		GroupID: groupID,
	}); errCode(err) != "continuation_in_use" {
		t.Fatalf("unlink with absences: err = %v, want continuation_in_use", err)
	}
}

func linkContinuationPair(t *testing.T, pool *pgxpool.Pool, actorID, sourceID, splitID uuid.UUID) pgtype.UUID {
	t.Helper()
	ctx := context.Background()
	pg := func(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	result, err := coursegroups.NewService().CreateTx(ctx, sqldb.New(pool).WithTx(tx), coursegroups.CreateCommand{
		ActorID:            pg(actorID),
		Name:               "continuation " + uuid.NewString()[:8],
		CourseIDs:          []pgtype.UUID{pg(sourceID), pg(splitID)},
		RuleSourceCourseID: pg(sourceID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return result.GroupID
}

// Enrollment in either ID counts for both, a direct row is never duplicated by
// the sibling expansion, and unlinking restores independent enrollment.
func TestCourseContinuationEnrollmentAndUnlink(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	pool := newPoolPending(t, databaseURL)
	t.Cleanup(pool.Close)
	ctx := context.Background()

	suffix := uuid.NewString()[:8]
	var subjID, teacherID, sourceID, splitID, onlySource, onlySplit, both uuid.UUID
	scan := func(dst any, sql string, args ...any) {
		t.Helper()
		if err := pool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	scan(&subjID, `INSERT INTO subjects (code, name) VALUES ($1, 'SAT Math') RETURNING id`, "ESUBJ-"+suffix)
	scan(&teacherID, `INSERT INTO users (username, role, password_hash) VALUES ($1, 'Teacher', 'x') RETURNING id`, "teacher-enr-"+suffix)
	scan(&sourceID, `INSERT INTO courses (code, name, subject_id) VALUES ($1, 'Math', $2) RETURNING id`, "ENR-B-"+suffix, subjID)
	scan(&splitID, `INSERT INTO courses (code, name, subject_id) VALUES ($1, 'Math', $2) RETURNING id`, "ENR-TY-"+suffix, subjID)
	for _, dst := range []*uuid.UUID{&onlySource, &onlySplit, &both} {
		scan(dst, `INSERT INTO students (wcode, full_name) VALUES ($1, 'Enrollment') RETURNING id`, "wenr"+uuid.NewString()[:8])
	}
	enroll := func(courseID, studentID uuid.UUID) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, 'enrolled')`, courseID, studentID); err != nil {
			t.Fatal(err)
		}
	}
	enroll(sourceID, onlySource)
	enroll(splitID, onlySplit)
	enroll(sourceID, both)
	enroll(splitID, both)

	rowsFor := func(studentID uuid.UUID) map[uuid.UUID]int {
		t.Helper()
		rows, err := pool.Query(ctx, `SELECT course_id FROM effective_course_students WHERE student_id = $1`, studentID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[uuid.UUID]int{}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			out[id]++
		}
		return out
	}
	assertCourses := func(label string, studentID uuid.UUID, want ...uuid.UUID) {
		t.Helper()
		got := rowsFor(studentID)
		if len(got) != len(want) {
			t.Fatalf("%s: effective courses = %v, want %v", label, got, want)
		}
		for _, id := range want {
			if got[id] != 1 {
				t.Fatalf("%s: course %s appears %d times, want once (%v)", label, id, got[id], got)
			}
		}
	}

	assertCourses("unlinked source-only", onlySource, sourceID)
	assertCourses("unlinked split-only", onlySplit, splitID)

	groupID := linkContinuationPair(t, pool, teacherID, sourceID, splitID)
	assertCourses("linked source-only", onlySource, sourceID, splitID)
	assertCourses("linked split-only", onlySplit, sourceID, splitID)
	assertCourses("linked both direct rows", both, sourceID, splitID)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := coursegroups.NewService().DeleteTx(ctx, sqldb.New(pool).WithTx(tx), coursegroups.DeleteCommand{
		ActorID: pgtype.UUID{Bytes: teacherID, Valid: true},
		GroupID: groupID,
	}); err != nil {
		t.Fatalf("unlink without absences: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	assertCourses("unlinked again source-only", onlySource, sourceID)
	assertCourses("unlinked again split-only", onlySplit, splitID)
}

// Sit-in treats a continuation pair as one course: sessions of both IDs are
// offered, each carrying its own course ID.
func TestCourseContinuationSitInOffersBothIDs(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	pool := newPoolPending(t, databaseURL)
	t.Cleanup(pool.Close)
	ctx := context.Background()
	q := sqldb.New(pool)

	suffix := uuid.NewString()[:8]
	wcode := "wsit" + suffix
	cycleID := "CY-SIT-" + suffix
	var studentID, subjID, teacherID, ruleID, rootID, topID, sourceID, splitID uuid.UUID
	scan := func(dst any, sql string, args ...any) {
		t.Helper()
		if err := pool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	scan(&studentID, `INSERT INTO students (wcode, full_name) VALUES ($1, 'Sit In') RETURNING id`, wcode)
	scan(&subjID, `INSERT INTO subjects (code, name) VALUES ($1, 'SAT Math') RETURNING id`, "SSUBJ-"+suffix)
	scan(&teacherID, `INSERT INTO users (username, role, password_hash) VALUES ($1, 'Teacher', 'x') RETURNING id`, "teacher-sit-"+suffix)
	scan(&ruleID, `INSERT INTO sit_in_rules (name, type, predicate) VALUES ($1, 'level_ladder',
		'{"level_1_action":"zoom","non_max_direction":"sit_higher","max_direction":"sit_lower","min_level_for_sit_lower":2}'::jsonb) RETURNING id`, "ladder-sit-"+suffix)
	scan(&rootID, `INSERT INTO root_course_groups (name, sit_in_rule_id) VALUES ($1, $2) RETURNING id`, "SAT Math sit "+suffix, ruleID)
	if _, err := pool.Exec(ctx, `INSERT INTO crm_cycles (id, label) VALUES ($1, $1) ON CONFLICT (id) DO NOTHING`, cycleID); err != nil {
		t.Fatal(err)
	}
	newCourse := func(dst *uuid.UUID, code string, level any, withRoot bool) {
		t.Helper()
		if withRoot {
			scan(dst, `INSERT INTO courses (code, name, subject_id, cycle_id, level, root_course_group_id) VALUES ($1, $1, $2, $3, $4, $5) RETURNING id`,
				code, subjID, cycleID, level, rootID)
			return
		}
		scan(dst, `INSERT INTO courses (code, name, subject_id) VALUES ($1, $1, $2) RETURNING id`, code, subjID)
	}
	newCourse(&topID, "SIT-TOP-"+suffix, 3, true)
	newCourse(&sourceID, "SIT-B-"+suffix, 2, true)
	newCourse(&splitID, "SIT-TY-"+suffix, nil, false)
	if _, err := pool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, 'enrolled')`, topID, studentID); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().AddDate(0, 0, 3).Truncate(24 * time.Hour)
	addSession := func(courseID uuid.UUID, hour int) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		start := day.Add(time.Duration(hour) * time.Hour)
		scan(&id, `INSERT INTO sessions (course_id, teacher_id, start_at, end_at) VALUES ($1, $2, $3, $4) RETURNING id`,
			courseID, teacherID, start, start.Add(time.Hour))
		return id
	}
	addSession(topID, 1)
	sourceSession := addSession(sourceID, 5)
	splitSession := addSession(splitID, 8)

	linkContinuationPair(t, pool, teacherID, sourceID, splitID)

	result, err := resolveSitInForCourse(ctx, q, wcode,
		pgtype.UUID{Bytes: topID, Valid: true}, pgtype.UUID{Bytes: subjID, Valid: true},
		day, day.Add(24*time.Hour), "Asia/Bangkok", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.SitInMethod != SitInMethodPhysical {
		t.Fatalf("result = %+v, want physical sit-in", result)
	}
	courseOf := map[string]string{}
	for _, s := range result.Available {
		courseOf[s.ID] = s.CourseID
	}
	if len(result.Available) != 2 ||
		courseOf[sourceSession.String()] != sourceID.String() ||
		courseOf[splitSession.String()] != splitID.String() {
		t.Fatalf("available = %+v, want source session on %s and split session on %s", result.Available, sourceID, splitID)
	}
}

func TestBundleTargetSessionsMergesContinuationOnly(t *testing.T) {
	id := func(b byte) pgtype.UUID { return pgtype.UUID{Bytes: [16]byte{b}, Valid: true} }
	key := uuidStringOrZero
	session := func(course pgtype.UUID, hour int) sqldb.SessionInRange {
		return sqldb.SessionInRange{
			ID:       id(byte(100 + hour)),
			CourseID: course,
			StartAt:  pgtype.Timestamptz{Time: time.Unix(int64(hour)*3600, 0), Valid: true},
		}
	}
	group, a, b := id(1), id(2), id(3)
	bundle := &sqldb.SitInBundleV2{
		Continuations: map[string]struct{}{},
		MergeMembers:  map[string][]pgtype.UUID{key(group): {a, b}},
		Sessions: map[string][]sqldb.SessionInRange{
			key(a): {session(a, 9)},
			key(b): {session(b, 2)},
		},
	}
	target := &sqldb.SubjectCourseV2{ID: a, MergeGroupID: group}

	if got := bundleTargetSessions(bundle, target); len(got) != 1 || got[0].CourseID != a {
		t.Fatalf("plain merge group must stay per course, got %+v", got)
	}
	bundle.Continuations[key(group)] = struct{}{}
	got := bundleTargetSessions(bundle, target)
	if len(got) != 2 || got[0].CourseID != b || got[1].CourseID != a {
		t.Fatalf("continuation must merge both IDs ordered by start, got %+v", got)
	}
}

// Two concurrent link attempts on the same course serialize on the course
// lock: exactly one wins, the other sees the course already grouped.
func TestCourseContinuationConcurrentLinkSingleWinner(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	pool := newPoolPending(t, databaseURL)
	t.Cleanup(pool.Close)
	ctx := context.Background()

	suffix := uuid.NewString()[:8]
	var subjID, teacherID, sourceID, splitA, splitB uuid.UUID
	scan := func(dst any, sql string, args ...any) {
		t.Helper()
		if err := pool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	scan(&subjID, `INSERT INTO subjects (code, name) VALUES ($1, 'SAT Math') RETURNING id`, "RSUBJ-"+suffix)
	scan(&teacherID, `INSERT INTO users (username, role, password_hash) VALUES ($1, 'Teacher', 'x') RETURNING id`, "teacher-race-"+suffix)
	for i, dst := range []*uuid.UUID{&sourceID, &splitA, &splitB} {
		scan(dst, `INSERT INTO courses (code, name, subject_id) VALUES ($1, 'Math', $2) RETURNING id`, "RACE-"+suffix+"-"+string(rune('A'+i)), subjID)
	}

	results := make(chan error, 2)
	for _, split := range []uuid.UUID{splitA, splitB} {
		go func() {
			pg := func(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }
			tx, err := pool.Begin(ctx)
			if err != nil {
				results <- err
				return
			}
			defer tx.Rollback(ctx)
			if _, err := coursegroups.NewService().CreateTx(ctx, sqldb.New(pool).WithTx(tx), coursegroups.CreateCommand{
				ActorID:            pg(teacherID),
				Name:               "race " + split.String()[:8],
				CourseIDs:          []pgtype.UUID{pg(sourceID), pg(split)},
				RuleSourceCourseID: pg(sourceID),
			}); err != nil {
				results <- err
				return
			}
			results <- tx.Commit(ctx)
		}()
	}
	wins, grouped := 0, 0
	for range 2 {
		err := <-results
		var domainErr *coursegroups.Error
		switch {
		case err == nil:
			wins++
		case errors.As(err, &domainErr) && domainErr.Code == "course_already_grouped":
			grouped++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 || grouped != 1 {
		t.Fatalf("wins = %d, already grouped = %d; want 1 and 1", wins, grouped)
	}
}
