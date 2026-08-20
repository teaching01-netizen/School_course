package scheduling

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// TestUserStory_EditEntireSeriesConflictReturnsConflictingSessions guards that
// an edit-entire rejected by the database surfaces the actual conflicting
// sessions in ConflictDetails instead of an empty synthetic conflict.
func TestUserStory_EditEntireSeriesConflictReturnsConflictingSessions(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fx := seedOccupancyFixture(t, q, "reg-entire-conflict-details")
	start := futureBangkok(14, 10)
	startLocal := start.In(svc.loc)
	count := 10
	created, err := svc.CreateSeriesAndMaterialize(ctx, CreateSeriesParams{
		CourseID:        fx.courseID,
		RoomID:          fx.roomID,
		TeacherID:       fx.teacherID,
		Weekdays:        []time.Weekday{startLocal.Weekday()},
		StartLocalTime:  Clock{Hour: 10},
		DurationMinutes: 60,
		StartDate:       LocalDate{Year: startLocal.Year(), Month: startLocal.Month(), Day: startLocal.Day()},
		Count:           &count,
	})
	if err != nil {
		t.Fatal(err)
	}
	ser, err := q.SeriesGetByID(ctx, created.SeriesID)
	if err != nil {
		t.Fatal(err)
	}

	// A one-off session at the NEW time (14:00) on the first occurrence day.
	conflictAt := start.Add(4 * time.Hour) // 14:00 Bangkok, same day
	conflict, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  fx.courseID,
		RoomID:    fx.roomID,
		TeacherID: fx.teacherID,
		StartAt:   pgtype.Timestamptz{Time: conflictAt, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: conflictAt.Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.EditEntireSeriesFutureOnly(ctx, EditEntireSeriesParams{
		SeriesID:        created.SeriesID,
		ExpectedVersion: ser.Version,
		NowUTC:          time.Now().UTC(),
		CourseID:        fx.courseID,
		RoomID:          fx.roomID,
		TeacherID:       fx.teacherID,
		Weekdays:        []time.Weekday{startLocal.Weekday()},
		StartLocalTime:  Clock{Hour: 14},
		DurationMinutes: 60,
		Count:           &count,
	})
	if err == nil {
		t.Fatal("conflicting entire-series edit succeeded")
	}
	var se *Err
	if !errors.As(err, &se) {
		t.Fatalf("expected scheduling conflict error, got %T (%v)", err, err)
	}
	if se.Code != "schedule_conflict" {
		t.Fatalf("conflict code=%q, want schedule_conflict", se.Code)
	}
	if len(se.Details.Conflicts) == 0 {
		t.Fatalf("conflict details missing conflicting sessions: %+v", se.Details)
	}
	found := false
	for _, c := range se.Details.Conflicts {
		if c.SessionID == uuidStringOrEmpty(conflict.ID) {
			found = true
		}
	}
	if !found {
		t.Fatalf("conflict details do not include the blocking session %s: %+v", uuidStringOrEmpty(conflict.ID), se.Details.Conflicts)
	}
}

// TestUserStory_SplitThisAndFutureConflictReturnsConflictingSessions guards
// that a split rejected by the database surfaces the actual conflicting
// sessions instead of an opaque constraint error.
func TestUserStory_SplitThisAndFutureConflictReturnsConflictingSessions(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fx := seedOccupancyFixture(t, q, "reg-split-conflict-details")
	start := futureBangkok(14, 10)
	startLocal := start.In(svc.loc)
	count := 10
	created, err := svc.CreateSeriesAndMaterialize(ctx, CreateSeriesParams{
		CourseID:        fx.courseID,
		RoomID:          fx.roomID,
		TeacherID:       fx.teacherID,
		Weekdays:        []time.Weekday{startLocal.Weekday()},
		StartLocalTime:  Clock{Hour: 10},
		DurationMinutes: 60,
		StartDate:       LocalDate{Year: startLocal.Year(), Month: startLocal.Month(), Day: startLocal.Day()},
		Count:           &count,
	})
	if err != nil {
		t.Fatal(err)
	}
	ser, err := q.SeriesGetByID(ctx, created.SeriesID)
	if err != nil {
		t.Fatal(err)
	}
	rows := userStoryActiveSeriesRows(t, q, fx.courseID, created.SeriesID)
	if len(rows) < 2 {
		t.Fatalf("active occurrences=%d, want >=2", len(rows))
	}

	pivotLocal := rows[1].StartAt.Time.In(svc.loc)
	pivot := LocalDate{Year: pivotLocal.Year(), Month: pivotLocal.Month(), Day: pivotLocal.Day()}
	clock := Clock{Hour: 8, Minute: 30}
	duration := 60

	// A one-off session at the NEW time (08:30) on the pivot day, in the same
	// room and with the same teacher. It must not overlap the old pivot-day
	// occurrence (10:00), so it can be created, but it collides with the
	// pivot-day occurrence the split would insert.
	conflictAt := time.Date(pivotLocal.Year(), pivotLocal.Month(), pivotLocal.Day(), 8, 30, 0, 0, svc.loc).UTC()
	conflict, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  fx.courseID,
		RoomID:    fx.roomID,
		TeacherID: fx.teacherID,
		StartAt:   pgtype.Timestamptz{Time: conflictAt, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: conflictAt.Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = svc.SplitThisAndFutureTx(ctx, tx, q.WithTx(tx), SplitSeriesParams{
		SeriesID:        created.SeriesID,
		PivotDate:       pivot,
		ExpectedVersion: ser.Version,
		StartLocalTime:  &clock,
		DurationMinutes: &duration,
	})
	if err == nil {
		t.Fatal("conflicting split succeeded")
	}
	var se *Err
	if !errors.As(err, &se) {
		t.Fatalf("expected scheduling conflict error, got %T (%v)", err, err)
	}
	if se.Code != "schedule_conflict" {
		t.Fatalf("conflict code=%q, want schedule_conflict", se.Code)
	}
	if len(se.Details.Conflicts) == 0 {
		t.Fatalf("conflict details missing conflicting sessions: %+v", se.Details)
	}
	found := false
	for _, c := range se.Details.Conflicts {
		if c.SessionID == uuidStringOrEmpty(conflict.ID) {
			found = true
		}
	}
	if !found {
		t.Fatalf("conflict details do not include the blocking session %s: %+v", uuidStringOrEmpty(conflict.ID), se.Details.Conflicts)
	}
}
