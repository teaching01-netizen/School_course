package scheduling

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func teacherWindowMutation(ctx context.Context, t *testing.T, svc *Service, pool interface {
	Begin(context.Context) (pgx.Tx, error)
}, q *sqldb.Queries, teacherID pgtype.UUID, start, end time.Time) error {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = svc.CreateTeacherAvailabilityTx(ctx, tx, q.WithTx(tx), teacherID,
		pgtype.Timestamptz{Time: start, Valid: true}, pgtype.Timestamptz{Time: end, Valid: true})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func TestScheduleDB_AvailabilityUnionCoversSession(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	fx := seedOccupancyFixture(t, q, "availability-union")
	day := futureBangkok(21, 9)
	if _, err := q.CreateTeacherAvailability(context.Background(), sqldb.CreateTeacherAvailabilityParams{
		TeacherID: fx.teacherID, StartAt: pgtype.Timestamptz{Time: day, Valid: true}, EndAt: pgtype.Timestamptz{Time: day.Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateTeacherAvailability(context.Background(), sqldb.CreateTeacherAvailabilityParams{
		TeacherID: fx.teacherID, StartAt: pgtype.Timestamptz{Time: day.Add(time.Hour), Valid: true}, EndAt: pgtype.Timestamptz{Time: day.Add(2 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreateSession(context.Background(), CreateSessionParams{
		CourseID: fx.courseID, TeacherID: fx.teacherID, RoomID: fx.roomID,
		StartAt: pgtype.Timestamptz{Time: day.Add(30 * time.Minute), Valid: true}, EndAt: pgtype.Timestamptz{Time: day.Add(90 * time.Minute), Valid: true},
	})
	if err != nil {
		t.Fatalf("union-covered session rejected: %v", err)
	}
}

func TestScheduleDB_FirstAvailabilityWindowRejectsUncoveredFutureSession(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	fx := seedOccupancyFixture(t, q, "availability-first")
	start := futureBangkok(22, 9)
	if _, err := svc.CreateSession(context.Background(), CreateSessionParams{
		CourseID: fx.courseID, TeacherID: fx.teacherID, RoomID: fx.roomID,
		StartAt: pgtype.Timestamptz{Time: start, Valid: true}, EndAt: pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	err := teacherWindowMutation(context.Background(), t, svc, pool, q, fx.teacherID, start.Add(3*time.Hour), start.Add(4*time.Hour))
	var se *Err
	if !errors.As(err, &se) || se.Code != "availability_conflict" {
		t.Fatalf("error=%T %v, want availability_conflict", err, err)
	}
	var windows int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM teacher_availability WHERE teacher_id=$1 AND deleted_at IS NULL`, fx.teacherID).Scan(&windows); err != nil {
		t.Fatal(err)
	}
	if windows != 0 {
		t.Fatalf("tentative availability committed: windows=%d", windows)
	}
}

func TestScheduleDB_LastWindowDeleteRestoresDefaultOpen(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	fx := seedOccupancyFixture(t, q, "availability-last")
	start := futureBangkok(23, 9)
	window, err := q.CreateTeacherAvailability(context.Background(), sqldb.CreateTeacherAvailabilityParams{
		TeacherID: fx.teacherID, StartAt: pgtype.Timestamptz{Time: start, Valid: true}, EndAt: pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteTeacherAvailabilityTx(context.Background(), tx, q.WithTx(tx), fx.teacherID, window.ID); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateSession(context.Background(), CreateSessionParams{
		CourseID: fx.courseID, TeacherID: fx.teacherID, RoomID: fx.roomID,
		StartAt: pgtype.Timestamptz{Time: start.Add(3 * time.Hour), Valid: true}, EndAt: pgtype.Timestamptz{Time: start.Add(4 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("default-open session rejected after last window delete: %v", err)
	}
}

func TestScheduleDB_AvailabilityMutationIgnoresStartedHistory(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	fx := seedOccupancyFixture(t, q, "availability-history")
	past := time.Now().UTC().Add(-2 * time.Hour)
	if _, err := q.SessionCreate(context.Background(), sqldb.SessionCreateParams{
		CourseID: fx.courseID, TeacherID: fx.teacherID, RoomID: fx.roomID,
		StartAt: pgtype.Timestamptz{Time: past, Valid: true}, EndAt: pgtype.Timestamptz{Time: past.Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	future := futureBangkok(24, 12)
	if err := teacherWindowMutation(context.Background(), t, svc, pool, q, fx.teacherID, future, future.Add(time.Hour)); err != nil {
		t.Fatalf("historical session blocked availability mutation: %v", err)
	}
}

func TestScheduleDB_ConcurrentAvailabilityAndSessionCreatePreservesPolicy(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	fx := seedOccupancyFixture(t, q, "availability-race")
	start := futureBangkok(25, 9)
	left, right := runRace(t, func(ctx context.Context) error {
		return teacherWindowMutation(ctx, t, svc, pool, q, fx.teacherID, start.Add(3*time.Hour), start.Add(4*time.Hour))
	}, func(ctx context.Context) error {
		_, err := svc.CreateSession(ctx, CreateSessionParams{
			CourseID: fx.courseID, TeacherID: fx.teacherID, RoomID: fx.roomID,
			StartAt: pgtype.Timestamptz{Time: start, Valid: true}, EndAt: pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
		})
		return err
	})
	if left == nil && right == nil {
		t.Fatal("both mutually incompatible writers committed")
	}
	var uncovered int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM sessions s
		WHERE s.teacher_id=$1 AND s.deleted_at IS NULL AND s.start_at>transaction_timestamp()
		  AND EXISTS (SELECT 1 FROM teacher_availability a WHERE a.teacher_id=s.teacher_id AND a.deleted_at IS NULL)
		  AND NOT (COALESCE((SELECT range_agg(a.time_range) FROM teacher_availability a
		                    WHERE a.teacher_id=s.teacher_id AND a.deleted_at IS NULL), '{}'::tstzmultirange) @> s.time_range)`, fx.teacherID).Scan(&uncovered); err != nil {
		t.Fatal(err)
	}
	if uncovered != 0 {
		t.Fatalf("uncovered=%d", uncovered)
	}
}
