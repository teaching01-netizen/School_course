package courseadmin

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sqldb "warwick-institute/internal/db"
)

func requireTestDB(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

var migrationsOnce sync.Once
var migrationsErr error

func migrateUpOnce(t *testing.T, databaseURL string) {
	t.Helper()
	migrationsOnce.Do(func() {
		// Supabase pooler / PgBouncer can break prepared statements; ensure stdlib driver uses simple protocol.
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			migrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			migrationsErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			migrationsErr = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations"))
		migrationsErr = goose.UpContext(ctx, db, migrationsDir)
	})
	if migrationsErr != nil {
		t.Fatal(migrationsErr)
	}
}

func newPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

// testFixture is a shared DB harness: a pool, a unique suffix for object names,
// and a query handle bound to the pool.
type testFixture struct {
	pool   *pgxpool.Pool
	q      *sqldb.Queries
	suffix string
}

func setupTestDB(t *testing.T) *testFixture {
	t.Helper()
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	return &testFixture{
		pool:   pool,
		q:      sqldb.New(pool),
		suffix: time.Now().UTC().Format("20060102150405.000000000") + "-" + uuid.NewString()[:8],
	}
}

func (f *testFixture) createTeacher(t *testing.T, role string) pgtype.UUID {
	t.Helper()
	id, err := f.q.AdminUserCreate(context.Background(), sqldb.AdminUserCreateParams{
		Username:     "courseadmin-teacher-" + uuid.NewString()[:8],
		Role:         role,
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *testFixture) createCourse(t *testing.T, code string) pgtype.UUID {
	t.Helper()
	course, err := f.q.CourseCreate(context.Background(), sqldb.CourseCreateParams{Code: code, Name: "Course " + code})
	if err != nil {
		t.Fatal(err)
	}
	return course.ID
}

func (f *testFixture) createRoom(t *testing.T) pgtype.UUID {
	t.Helper()
	room, err := f.q.RoomCreate(context.Background(), sqldb.RoomCreateParams{
		Name:     "room-" + uuid.NewString()[:8],
		Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return room.ID
}

// runUpdate executes UpdateCourseTx in its own transaction; on success the
// transaction commits, on any error it rolls back (as a real caller would).
func (f *testFixture) runUpdate(t *testing.T, svc *Service, cmd UpdateCourseCommand) (UpdateCourseResult, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	qtx := sqldb.New(tx)
	result, err := svc.UpdateCourseTx(ctx, qtx, cmd)
	if err != nil {
		return UpdateCourseResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return result, nil
}

func requireErrorCode(t *testing.T, err error, code string) *Error {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %s, got nil", code)
	}
	ce, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *courseadmin.Error, got %T: %v", err, err)
	}
	if ce.Code != code {
		t.Fatalf("expected error code %s, got %s (%s)", code, ce.Code, ce.Message)
	}
	return ce
}

func (f *testFixture) courseVersion(t *testing.T, courseID pgtype.UUID) int32 {
	t.Helper()
	var version int32
	if err := f.pool.QueryRow(context.Background(), `SELECT version FROM courses WHERE id = $1`, courseID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func (f *testFixture) courseTeacherIDs(t *testing.T, courseID pgtype.UUID) map[[16]byte]bool {
	t.Helper()
	rows, err := f.q.CourseTeachersList(context.Background(), courseID)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[[16]byte]bool, len(rows))
	for _, row := range rows {
		out[row.TeacherID.Bytes] = row.IsPrimary
	}
	return out
}

func assignmentsToIDs(assignments []TeacherAssignment) map[[16]byte]bool {
	out := make(map[[16]byte]bool, len(assignments))
	for _, a := range assignments {
		out[a.TeacherID.Bytes] = a.IsPrimary
	}
	return out
}

func TestUpdateCourseTx_MultipleTeachersAndPrimary(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	teacherC := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "CA-"+f.suffix)

	assignments := []TeacherAssignment{
		{TeacherID: teacherA, IsPrimary: true},
		{TeacherID: teacherB, IsPrimary: false},
		{TeacherID: teacherC, IsPrimary: false},
	}
	result, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "CA-" + f.suffix,
		Name:            "Multi-teacher course",
		Teachers:        assignments,
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if result.Version != 2 {
		t.Fatalf("expected version 2, got %d", result.Version)
	}
	if result.CourseID != courseID {
		t.Fatalf("expected course id %v, got %v", courseID, result.CourseID)
	}

	stored := f.courseTeacherIDs(t, courseID)
	if len(stored) != 3 {
		t.Fatalf("expected 3 stored teachers, got %d", len(stored))
	}
	want := assignmentsToIDs(assignments)
	for id, primary := range want {
		if gotPrimary, ok := stored[id]; !ok || gotPrimary != primary {
			t.Fatalf("teacher %v stored incorrectly: ok=%v primary=%v want=%v", id, ok, gotPrimary, primary)
		}
	}

	// courses.teacher_id mirrors the primary assignment.
	var primary pgtype.UUID
	if err := f.pool.QueryRow(context.Background(), `SELECT teacher_id FROM courses WHERE id = $1`, courseID).Scan(&primary); err != nil {
		t.Fatal(err)
	}
	if !primary.Valid || primary.Bytes != teacherA.Bytes {
		t.Fatalf("expected courses.teacher_id to mirror primary %v, got %v", teacherA, primary)
	}

	// Audit row written in the same transaction.
	var auditCount int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'course.teachers_updated' AND payload->>'course_id' = $1`,
		courseID.String()).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected 1 audit row, got %d", auditCount)
	}
}

func TestUpdateCourseTx_NoPrimary(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "CN-"+f.suffix)

	assignments := []TeacherAssignment{
		{TeacherID: teacherA, IsPrimary: false},
		{TeacherID: teacherB, IsPrimary: false},
	}
	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "CN-" + f.suffix,
		Name:            "No primary course",
		Teachers:        assignments,
	}); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	var primary pgtype.UUID
	if err := f.pool.QueryRow(context.Background(), `SELECT teacher_id FROM courses WHERE id = $1`, courseID).Scan(&primary); err != nil {
		t.Fatal(err)
	}
	if primary.Valid {
		t.Fatalf("expected NULL courses.teacher_id when no primary, got %v", primary)
	}
}

func TestUpdateCourseTx_PrimarySwapIsNotRemoval(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "CS-"+f.suffix)

	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "CS-" + f.suffix,
		Name:            "Swap primary",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	}); err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	// Swap the primary flag to teacherB with version 2; no teacher is removed,
	// so no future-session check applies.
	result, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 2,
		Code:            "CS-" + f.suffix,
		Name:            "Swap primary",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: false},
			{TeacherID: teacherB, IsPrimary: true},
		},
	})
	if err != nil {
		t.Fatalf("primary swap failed: %v", err)
	}
	if result.Version != 3 {
		t.Fatalf("expected version 3, got %d", result.Version)
	}

	var primary pgtype.UUID
	if err := f.pool.QueryRow(context.Background(), `SELECT teacher_id FROM courses WHERE id = $1`, courseID).Scan(&primary); err != nil {
		t.Fatal(err)
	}
	if !primary.Valid || primary.Bytes != teacherB.Bytes {
		t.Fatalf("expected primary %v, got %v", teacherB, primary)
	}
}

func TestUpdateCourseTx_InvalidUser(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "CI-"+f.suffix)

	unknown := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	ce := requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "CI-" + f.suffix,
		Name:            "Invalid user",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: false},
			{TeacherID: unknown, IsPrimary: false},
		},
	}), "invalid_teacher")

	teachers, ok := ce.Details["teachers"].([]map[string]any)
	if !ok {
		t.Fatalf("expected details.teachers array, got %#v", ce.Details["teachers"])
	}
	if len(teachers) != 1 {
		t.Fatalf("expected exactly 1 invalid teacher, got %d", len(teachers))
	}
	if teachers[0]["reason"] != "not_found" {
		t.Fatalf("expected reason not_found, got %v", teachers[0]["reason"])
	}
}

// mustUpdateErr asserts the update fails and returns the error for inspection.
func (f *testFixture) mustUpdateErr(t *testing.T, svc *Service, courseID pgtype.UUID, cmd UpdateCourseCommand) error {
	t.Helper()
	_, err := f.runUpdate(t, svc, cmd)
	if err == nil {
		t.Fatalf("expected update to fail")
	}
	return err
}

func TestUpdateCourseTx_WrongRole(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	admin := f.createTeacher(t, "Admin")
	teacherA := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "CR-"+f.suffix)

	ce := requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "CR-" + f.suffix,
		Name:            "Wrong role",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: false},
			{TeacherID: admin, IsPrimary: false},
		},
	}), "invalid_teacher")

	teachers, ok := ce.Details["teachers"].([]map[string]any)
	if !ok {
		t.Fatalf("expected details.teachers array, got %#v", ce.Details["teachers"])
	}
	for _, item := range teachers {
		if item["teacher_id"] == admin.String() && item["reason"] != "role_not_allowed" {
			t.Fatalf("expected admin to be role_not_allowed, got %v", item["reason"])
		}
		if item["teacher_id"] == teacherA.String() {
			t.Fatalf("valid teacher %v must not be flagged", teacherA)
		}
	}
}

func TestUpdateCourseTx_InactiveTeacher(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacher := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "CD-"+f.suffix)

	ctx := context.Background()
	if err := f.q.AdminUserDeactivate(ctx, teacher); err != nil {
		t.Fatal(err)
	}

	ce := requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "CD-" + f.suffix,
		Name:            "Inactive teacher",
		Teachers: []TeacherAssignment{
			{TeacherID: teacher, IsPrimary: false},
		},
	}), "invalid_teacher")

	teachers, ok := ce.Details["teachers"].([]map[string]any)
	if !ok {
		t.Fatalf("expected details.teachers array, got %#v", ce.Details["teachers"])
	}
	if teachers[0]["reason"] != "inactive" {
		t.Fatalf("expected reason inactive, got %v", teachers[0]["reason"])
	}
}

func TestUpdateCourseTx_InvalidExpectedVersion(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacher := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "CV-"+f.suffix)

	requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 0,
		Code:            "CV-" + f.suffix,
		Name:            "Bad version",
		Teachers: []TeacherAssignment{
			{TeacherID: teacher, IsPrimary: false},
		},
	}), "invalid_expected_version")

	// Nothing written: version still 1, no assignments.
	if v := f.courseVersion(t, courseID); v != 1 {
		t.Fatalf("expected version 1 untouched, got %d", v)
	}
	if stored := f.courseTeacherIDs(t, courseID); len(stored) != 0 {
		t.Fatalf("expected no assignments, got %d", len(stored))
	}
}

func TestUpdateCourseTx_CourseNotFound(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacher := f.createTeacher(t, "Teacher")
	requireErrorCode(t, f.mustUpdateErr(t, svc, pgtype.UUID{Bytes: uuid.New(), Valid: true}, UpdateCourseCommand{
		CourseID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
		ExpectedVersion: 1,
		Code:            "NF-" + f.suffix,
		Name:            "Missing course",
		Teachers: []TeacherAssignment{
			{TeacherID: teacher, IsPrimary: false},
		},
	}), "not_found")
}

func TestUpdateCourseTx_StaleEdit(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "CT-"+f.suffix)

	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "CT-" + f.suffix,
		Name:            "Stale test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	}); err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	// Replay with the stale expected_version 1.
	ce := requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "CT-" + f.suffix,
		Name:            "Stale test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
		},
	}), "stale_edit")

	current, ok := ce.Details["current"].(*CourseResponse)
	if !ok {
		t.Fatalf("expected details.current to be *CourseResponse, got %T", ce.Details["current"])
	}
	if current.Version != 2 {
		t.Fatalf("expected current version 2, got %d", current.Version)
	}
	if current.PrimaryTeacherID == nil || *current.PrimaryTeacherID != teacherA.String() {
		t.Fatalf("expected current primary %v, got %v", teacherA, current.PrimaryTeacherID)
	}
	if len(current.Teachers) != 2 {
		t.Fatalf("expected 2 teachers in current, got %d", len(current.Teachers))
	}

	// Stale edit must not have written anything.
	if v := f.courseVersion(t, courseID); v != 2 {
		t.Fatalf("expected version still 2, got %d", v)
	}
	if stored := f.courseTeacherIDs(t, courseID); len(stored) != 2 {
		t.Fatalf("expected assignments untouched, got %d", len(stored))
	}
}

func TestUpdateCourseTx_FutureSessionBlocksRemoval(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "CF-"+f.suffix)
	roomID := f.createRoom(t)

	ctx := context.Background()
	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "CF-" + f.suffix,
		Name:            "Future session course",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	}); err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	now := time.Now().UTC()
	futureStart := now.Add(48 * time.Hour)
	session, err := f.q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  courseID,
		RoomID:    roomID,
		TeacherID: teacherA,
		StartAt:   pgtype.Timestamptz{Time: futureStart, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: futureStart.Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Removing teacherA is blocked while they own a future session.
	ce := requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 2,
		Code:            "CF-" + f.suffix,
		Name:            "Future session course",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherB, IsPrimary: true},
		},
	}), "teacher_in_use")

	if ce.Details["teacher_id"] != teacherA.String() {
		t.Fatalf("expected blocked teacher %v, got %v", teacherA, ce.Details["teacher_id"])
	}
	if name, _ := ce.Details["teacher_name"].(string); name == "" {
		t.Fatalf("expected teacher_name in details, got %#v", ce.Details["teacher_name"])
	}
	if ce.Details["future_session_count"] != int64(1) {
		t.Fatalf("expected future_session_count 1, got %v", ce.Details["future_session_count"])
	}
	sessionIDs, ok := ce.Details["session_ids"].([]string)
	if !ok || len(sessionIDs) != 1 || sessionIDs[0] != session.ID.String() {
		t.Fatalf("expected session_ids [%v], got %#v", session.ID, ce.Details["session_ids"])
	}

	// Nothing changed: teacherA still assigned.
	stored := f.courseTeacherIDs(t, courseID)
	if !stored[teacherA.Bytes] {
		t.Fatalf("teacherA must still be assigned after blocked removal")
	}
	if v := f.courseVersion(t, courseID); v != 2 {
		t.Fatalf("expected version still 2, got %d", v)
	}
}

func TestUpdateCourseTx_HistoricalSessionsRemovalSucceeds(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "CH-"+f.suffix)
	roomID := f.createRoom(t)

	ctx := context.Background()
	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "CH-" + f.suffix,
		Name:            "Historical sessions course",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	}); err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	pastStart := time.Now().UTC().Add(-48 * time.Hour)
	if _, err := f.q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  courseID,
		RoomID:    roomID,
		TeacherID: teacherA,
		StartAt:   pgtype.Timestamptz{Time: pastStart, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: pastStart.Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	// Past sessions do not block removal.
	result, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 2,
		Code:            "CH-" + f.suffix,
		Name:            "Historical sessions course",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherB, IsPrimary: true},
		},
	})
	if err != nil {
		t.Fatalf("removal with only historical sessions should succeed: %v", err)
	}
	if result.Version != 3 {
		t.Fatalf("expected version 3, got %d", result.Version)
	}
	stored := f.courseTeacherIDs(t, courseID)
	if stored[teacherA.Bytes] {
		t.Fatalf("teacherA should be removed")
	}
	if !stored[teacherB.Bytes] {
		t.Fatalf("teacherB should remain")
	}
}

func TestUpdateCourseTx_AtomicityOnAggregateFailure(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseA := f.createCourse(t, "AA-"+f.suffix)
	// courseB occupies the code that courseA will later collide with.
	f.createCourse(t, "AB-"+f.suffix)

	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseA,
		ExpectedVersion: 1,
		Code:            "AA-" + f.suffix,
		Name:            "Atomicity course",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	}); err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	// Attempt to rename courseA to courseB's code. The teacher delete+reinsert
	// succeeds first, then CourseUpdateAggregate violates the unique code
	// constraint (23505) — the whole transaction must roll back.
	_, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseA,
		ExpectedVersion: 2,
		Code:            "AB-" + f.suffix, // collides with courseB
		Name:            "Atomicity course",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherB, IsPrimary: true},
		},
	})
	if err == nil {
		t.Fatal("expected code collision to fail the update")
	}

	// Rollback must restore the original teacher set and version. courseTeacherIDs
	// maps teacher → is_primary, so membership is checked with the comma-ok idiom
	// and the primary flag is asserted separately.
	stored := f.courseTeacherIDs(t, courseA)
	if _, ok := stored[teacherA.Bytes]; !ok {
		t.Fatalf("teacherA must still be assigned after rollback, got %v", stored)
	}
	if _, ok := stored[teacherB.Bytes]; !ok {
		t.Fatalf("teacherB must still be assigned after rollback, got %v", stored)
	}
	if !stored[teacherA.Bytes] {
		t.Fatalf("teacherA must remain primary after rollback, got %v", stored)
	}
	if v := f.courseVersion(t, courseA); v != 2 {
		t.Fatalf("expected version 2 after rollback, got %d", v)
	}
}
