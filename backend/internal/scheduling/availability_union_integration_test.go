package scheduling

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// seedAdjacentAvailabilityWindows creates two abutting teacher availability
// windows [09:00,12:00) and [12:00,15:00) Bangkok local on dayUTC's date.
// The union policy accepts a session straddling the seam (e.g. 09:30-12:30);
// a single-window containment check does not.
func seedAdjacentAvailabilityWindows(t *testing.T, ctx context.Context, q *sqldb.Queries, teacherID pgtype.UUID, dayUTC time.Time) {
	t.Helper()
	w1s := dayUTC
	w1e := w1s.Add(3 * time.Hour)
	w2s := w1e
	w2e := w2s.Add(3 * time.Hour)
	if _, err := q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: teacherID, StartAt: pgtype.Timestamptz{Time: w1s, Valid: true}, EndAt: pgtype.Timestamptz{Time: w1e, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: teacherID, StartAt: pgtype.Timestamptz{Time: w2s, Valid: true}, EndAt: pgtype.Timestamptz{Time: w2e, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
}

// TestSeriesBatchPreflight_UsesUnionAvailabilityCoverage guards that the
// series-creation batch preflight applies the same availability policy as the
// database trigger (00070): the union of all windows must cover the requested
// range. A session straddling two abutting windows is legal.
func TestSeriesBatchPreflight_UsesUnionAvailabilityCoverage(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fx := seedOccupancyFixture(t, q, "reg-avail-union-series")
	dayUTC := futureBangkok(14, 9) // 09:00 Bangkok
	seedAdjacentAvailabilityWindows(t, ctx, q, fx.teacherID, dayUTC)

	startLocal := dayUTC.In(svc.loc)
	count := 1
	res, err := svc.CreateSeriesAndMaterialize(ctx, CreateSeriesParams{
		CourseID:        fx.courseID,
		RoomID:          fx.roomID,
		TeacherID:       fx.teacherID,
		Weekdays:        []time.Weekday{startLocal.Weekday()},
		StartLocalTime:  Clock{Hour: 9, Minute: 30},
		DurationMinutes: 180,
		StartDate:       LocalDate{Year: startLocal.Year(), Month: startLocal.Month(), Day: startLocal.Day()},
		Count:           &count,
	})
	if err != nil {
		t.Fatalf("series straddling adjacent availability windows rejected: %v", err)
	}
	if res.SessionsAdded != 1 {
		t.Fatalf("sessions added=%d, want 1", res.SessionsAdded)
	}
}

// TestFindAvailableSlots_UsesUnionAvailabilityCoverage guards that the slot
// finder applies union coverage: a candidate slot straddling two abutting
// windows (e.g. 10:00-13:00 with [09:00,12:00)+[12:00,15:00)) is provisional.
func TestFindAvailableSlots_UsesUnionAvailabilityCoverage(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fx := seedOccupancyFixture(t, q, "reg-avail-union-finder")
	dayUTC := futureBangkok(14, 9)
	seedAdjacentAvailabilityWindows(t, ctx, q, fx.teacherID, dayUTC)

	// The finder reads the legacy primary-teacher projection (courses.teacher_id).
	if _, err := pool.Exec(ctx, `UPDATE courses SET teacher_id = $1 WHERE id = $2`, fx.teacherID, fx.courseID); err != nil {
		t.Fatal(err)
	}

	startLocal := dayUTC.In(svc.loc)
	date := LocalDate{Year: startLocal.Year(), Month: startLocal.Month(), Day: startLocal.Day()}
	res, err := svc.FindAvailableSlots(ctx, FindAvailableSlotsParams{
		CourseID:         fx.courseID,
		StartDate:        date,
		EndDate:          date,
		SlotDurationMins: 180,
		DayStartHour:     9,
		DayEndHour:       15,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Slots) != 4 {
		t.Fatalf("slots=%d, want 4 (9:00,10:00,11:00,12:00 starts)", len(res.Slots))
	}
	statusByStart := map[string]string{}
	for _, slot := range res.Slots {
		statusByStart[slot.StartTime] = slot.Status
	}
	// Included entirely in one window — provisional under both policies.
	if statusByStart["09:00"] != "provisional" {
		t.Fatalf("slot 09:00 status=%q, want provisional", statusByStart["09:00"])
	}
	if statusByStart["12:00"] != "provisional" {
		t.Fatalf("slot 12:00 status=%q, want provisional", statusByStart["12:00"])
	}
	// Straddles the seam 10:00-13:00 / 11:00-14:00 — needs union coverage.
	if statusByStart["10:00"] != "provisional" {
		t.Fatalf("slot 10:00 status=%q, want provisional (union coverage)", statusByStart["10:00"])
	}
	if statusByStart["11:00"] != "provisional" {
		t.Fatalf("slot 11:00 status=%q, want provisional (union coverage)", statusByStart["11:00"])
	}
}
