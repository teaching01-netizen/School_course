package scheduling

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func addLocalDays(d LocalDate, n int) LocalDate {
	t := time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n)
	return LocalDate{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

// TestBusyRanges_CancelSeriesSoftDeletesBusyRangesAndFreesSlot guards the
// invariant: canceling a series must liberate the canceled time window for the
// course's students. The canceled session's busy-range rows must be soft-deleted
// (deleted_at set) so the student_busy_ranges_no_overlap exclusion constraint
// (WHERE deleted_at IS NULL) stops blocking a rebooking at that time.
func TestBusyRanges_CancelSeriesSoftDeletesBusyRangesAndFreesSlot(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	ctx := context.Background()
	fx := seedOccupancyFixture(t, q, "reg-cancel-busy")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AddCourseStudentTx(ctx, tx, q.WithTx(tx), fx.courseID, fx.studentID, CourseStudentStatusEnrolled); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	start := futureBangkok(3, 0)
	startDate := LocalDate{Year: start.Year(), Month: start.Month(), Day: start.Day()}
	endDate := addLocalDays(startDate, 3)

	res, err := svc.CreateSeriesAndMaterialize(ctx, CreateSeriesParams{
		CourseID:        fx.courseID,
		RoomID:          fx.roomID,
		TeacherID:       fx.teacherID,
		Weekdays:        []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday},
		StartLocalTime:  Clock{Hour: 9, Minute: 0},
		DurationMinutes: 60,
		StartDate:       startDate,
		EndDate:         &endDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	ser, err := q.SeriesGetByID(ctx, res.SeriesID)
	if err != nil {
		t.Fatal(err)
	}

	pivot := addLocalDays(startDate, 2)
	if _, err := svc.CancelSeries(ctx, CancelSeriesParams{
		SeriesID:        res.SeriesID,
		Scope:           CancelScopeThisAndFuture,
		PivotDate:       &pivot,
		ExpectedVersion: ser.Version,
		NowUTC:          start.UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	// Day 0 and day 1 are retained; day 2 and day 3 are canceled. Canceled
	// sessions must carry soft-deleted (not active) busy ranges.
	var active, softDeleted int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE sbr.deleted_at IS NULL),
		       count(*) FILTER (WHERE sbr.deleted_at IS NOT NULL)
		FROM sessions s
		JOIN student_busy_ranges sbr ON sbr.session_id = s.id
		WHERE s.series_id = $1 AND s.deleted_at IS NOT NULL AND sbr.student_id = $2
	`, res.SeriesID, fx.studentID).Scan(&active, &softDeleted); err != nil {
		t.Fatal(err)
	}
	if active != 0 || softDeleted != 2 {
		t.Fatalf("canceled sessions busy ranges: active=%d soft_deleted=%d, want 0/2", active, softDeleted)
	}

	// Rebooking into a canceled slot (day 5 09:00 Bangkok, in the canceled
	// window) must now succeed for the same student.
	canceledSlot := futureBangkok(5, 9)
	created, err := svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  fx.courseID,
		TeacherID: fx.teacherID,
		RoomID:    fx.roomID,
		StartAt:   pgtype.Timestamptz{Time: canceledSlot, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: canceledSlot.Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("rebook into canceled slot failed: %v", err)
	}
	var busy int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM student_busy_ranges
		WHERE session_id = $1 AND student_id = $2 AND deleted_at IS NULL
	`, created.SessionID, fx.studentID).Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if busy != 1 {
		t.Fatalf("rebooked session busy ranges=%d, want 1", busy)
	}
}

// TestBusyRanges_RawSessionSoftDeleteFreesSlot covers the same invariant for
// the legacy-sync deactivation path, which soft-deletes sessions with a plain
// UPDATE sessions SET deleted_at = now().
func TestBusyRanges_RawSessionSoftDeleteFreesSlot(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	ctx := context.Background()
	fx := seedOccupancyFixture(t, q, "reg-raw-softdelete")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AddCourseStudentTx(ctx, tx, q.WithTx(tx), fx.courseID, fx.studentID, CourseStudentStatusEnrolled); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	slot := futureBangkok(10, 9)
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  fx.courseID,
		TeacherID: fx.teacherID,
		RoomID:    fx.roomID,
		StartAt:   pgtype.Timestamptz{Time: slot, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: slot.Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `UPDATE sessions SET deleted_at = now() WHERE id = $1`, session.ID); err != nil {
		t.Fatal(err)
	}

	var active, softDeleted int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE deleted_at IS NULL),
		       count(*) FILTER (WHERE deleted_at IS NOT NULL)
		FROM student_busy_ranges
		WHERE session_id = $1 AND student_id = $2
	`, session.ID, fx.studentID).Scan(&active, &softDeleted); err != nil {
		t.Fatal(err)
	}
	if active != 0 || softDeleted != 1 {
		t.Fatalf("raw soft-deleted session busy ranges: active=%d soft_deleted=%d, want 0/1", active, softDeleted)
	}

	rebooked, err := svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  fx.courseID,
		TeacherID: fx.teacherID,
		RoomID:    fx.roomID,
		StartAt:   pgtype.Timestamptz{Time: slot, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: slot.Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("rebook into raw soft-deleted slot failed: %v", err)
	}
	var busy int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM student_busy_ranges
		WHERE session_id = $1 AND student_id = $2 AND deleted_at IS NULL
	`, rebooked.SessionID, fx.studentID).Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if busy != 1 {
		t.Fatalf("rebooked session busy ranges=%d, want 1", busy)
	}
}
