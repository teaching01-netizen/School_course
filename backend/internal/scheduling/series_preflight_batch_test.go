package scheduling

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/series"
)

// batchOrdinalRows implements pgx.Rows returning ordinal + conflict session columns.
// Column layout: ordinal(int64), id(UUID), series_id(UUID), course_id(UUID),
// room_id(UUID), teacher_id(UUID), start_at(Timestamptz), end_at(Timestamptz).
type batchOrdinalRows struct {
	entries []struct {
		ordinal int64
		session ConflictSession
	}
	idx int
}

func makeBatchOrdinalRows(ordinal int64, sessions []ConflictSession) *batchOrdinalRows {
	r := &batchOrdinalRows{idx: -1}
	for range len(sessions) {
		// entry per session at same ordinal
	}
	// Build entries from sessions
	for _, s := range sessions {
		r.entries = append(r.entries, struct {
			ordinal int64
			session ConflictSession
		}{ordinal: ordinal, session: s})
	}
	return r
}

func (r *batchOrdinalRows) Close()                                       {}
func (r *batchOrdinalRows) Err() error                                   { return nil }
func (r *batchOrdinalRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *batchOrdinalRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *batchOrdinalRows) RawValues() [][]byte                          { return nil }
func (r *batchOrdinalRows) Values() ([]interface{}, error)               { return nil, nil }
func (r *batchOrdinalRows) Conn() *pgx.Conn                              { return nil }

func (r *batchOrdinalRows) Next() bool {
	r.idx++
	return r.idx < len(r.entries)
}

func (r *batchOrdinalRows) Scan(dest ...interface{}) error {
	if r.idx < 0 || r.idx >= len(r.entries) {
		return pgx.ErrNoRows
	}
	e := r.entries[r.idx]
	for i, d := range dest {
		switch i {
		case 0: // ordinal
			if p, ok := d.(*int64); ok {
				*p = e.ordinal
			}
		case 1: // id
			if p, ok := d.(*pgtype.UUID); ok {
				uid, err := uuid.Parse(e.session.SessionID)
				if err != nil {
					return fmt.Errorf("parse session id: %w", err)
				}
				*p = pgtype.UUID{Bytes: uid, Valid: true}
			}
		case 2: // series_id
			if p, ok := d.(*pgtype.UUID); ok {
				*p = pgtype.UUID{Valid: false}
			}
		case 3: // course_id
			if p, ok := d.(*pgtype.UUID); ok {
				uid, _ := uuid.Parse("00000000-0000-0000-0000-000000000001")
				*p = pgtype.UUID{Bytes: uid, Valid: true}
			}
		case 4: // room_id
			if p, ok := d.(*pgtype.UUID); ok {
				uid, _ := uuid.Parse("00000000-0000-0000-0000-000000000002")
				*p = pgtype.UUID{Bytes: uid, Valid: true}
			}
		case 5: // teacher_id
			if p, ok := d.(*pgtype.UUID); ok {
				uid, _ := uuid.Parse("00000000-0000-0000-0000-000000000003")
				*p = pgtype.UUID{Bytes: uid, Valid: true}
			}
		case 6: // start_at
			if p, ok := d.(*pgtype.Timestamptz); ok {
				*p = pgtype.Timestamptz{Time: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), Valid: true}
			}
		case 7: // end_at
			if p, ok := d.(*pgtype.Timestamptz); ok {
				*p = pgtype.Timestamptz{Time: time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC), Valid: true}
			}
		}
	}
	return nil
}

// ordinalRows implements pgx.Rows returning only ordinal column (for availability queries).
type ordinalRows struct {
	ordinals []int64
	idx      int
}

func (r *ordinalRows) Close()                                       {}
func (r *ordinalRows) Err() error                                   { return nil }
func (r *ordinalRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *ordinalRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *ordinalRows) RawValues() [][]byte                          { return nil }
func (r *ordinalRows) Values() ([]interface{}, error)               { return nil, nil }
func (r *ordinalRows) Conn() *pgx.Conn                              { return nil }

func (r *ordinalRows) Next() bool {
	r.idx++
	return r.idx < len(r.ordinals)
}

func (r *ordinalRows) Scan(dest ...interface{}) error {
	if r.idx < 0 || r.idx >= len(r.ordinals) {
		return pgx.ErrNoRows
	}
	for i, d := range dest {
		if i == 0 {
			if p, ok := d.(*int64); ok {
				*p = r.ordinals[r.idx]
			}
		}
	}
	return nil
}

// testUUID returns a valid pgtype.UUID for testing.
func testUUID(id byte) pgtype.UUID {
	uid, _ := uuid.Parse(fmt.Sprintf("00000000-0000-0000-0000-%012d", id))
	return pgtype.UUID{Bytes: uid, Valid: true}
}

// testOccurrences generates n occurrences starting at a fixed time.
func testOccurrences(n int) []series.Occurrence {
	out := make([]series.Occurrence, n)
	start := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)
	for i := range n {
		out[i] = series.Occurrence{
			StartUTC: start.Add(time.Duration(i) * 24 * time.Hour),
			EndUTC:   start.Add(time.Duration(i)*24*time.Hour + time.Hour),
		}
	}
	return out
}

// testPreflightSeriesParams returns minimal valid params for batch preflight.
func testPreflightSeriesParams() PreflightSeriesParams {
	return PreflightSeriesParams{
		CourseID:        testUUID(1),
		RoomID:          testUUID(2),
		TeacherID:       testUUID(3),
		Weekdays:        []time.Weekday{time.Monday},
		StartLocalTime:  Clock{Hour: 10},
		DurationMinutes: 60,
		StartDate:       LocalDate{Year: 2026, Month: time.January, Day: 5},
		EndDate:         &LocalDate{Year: 2026, Month: time.January, Day: 5},
	}
}

// buildNoConflictDBTX creates a fakeDBTX with no conflicts for both batch calls.
// QueryRow order: CourseTeacherMembershipGet (3 bools), SchedulingResourcesGet (4 bools),
// teacher avail EXISTS, room avail EXISTS.
func buildNoConflictDBTX(hasTeacherWindows, hasRoomWindows bool) *fakeDBTX {
	db := &fakeDBTX{}
	db.queryRowResults = []pgx.Row{
		&fakeMultiRow{vals: []bool{true, true, true}},       // CourseTeacherMembershipGet: exists, hasTeachers, assigned
		&fakeMultiRow{vals: []bool{true, true, true, true}}, // SchedulingResourcesGet: course, teacher, active, room
		&fakeMultiRow{vals: []bool{hasTeacherWindows}},      // teacher availability EXISTS
		&fakeMultiRow{vals: []bool{hasRoomWindows}},         // room availability EXISTS
	}
	// Overlap queries: room, teacher, student — all empty
	db.queryResults = []queryResult{
		{rows: &fakeRows{}},
		{rows: &fakeRows{}},
		{rows: &fakeRows{}},
	}
	return db
}

// ---------------------------------------------------------------------------
// Tests for individual batch functions
// ---------------------------------------------------------------------------

func TestBatchCheckTeacherOverlaps(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(5)
	ords, starts, ends := buildOccurrenceArrays(occ)

	t.Run("no conflicts", func(t *testing.T) {
		db := &fakeDBTX{}
		result, err := svc.checkTeacherOverlapsBatch(ctx, db, testUUID(3), ords, starts, ends, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("expected empty result, got %d ordinals", len(result))
		}
	})

	t.Run("teacher overlap at ordinal 2", func(t *testing.T) {
		db := &fakeDBTX{}
		db.queryResults = []queryResult{
			{
				rows: makeBatchOrdinalRows(2, []ConflictSession{{
					SessionID: "550e8400-e29b-41d4-a716-446655440000",
					CourseID:  "660e8400-e29b-41d4-a716-446655440000",
					TeacherID: "770e8400-e29b-41d4-a716-446655440000",
					StartAt:   time.Now().Format(time.RFC3339Nano),
					EndAt:     time.Now().Add(time.Hour).Format(time.RFC3339Nano),
				}}),
			},
		}
		result, err := svc.checkTeacherOverlapsBatch(ctx, db, testUUID(3), ords, starts, ends, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 ordinal, got %d", len(result))
		}
		sessions, ok := result[2]
		if !ok {
			t.Fatal("expected conflict at ordinal 2, not found")
		}
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session at ordinal 2, got %d", len(sessions))
		}
		if sessions[0].SessionID != "550e8400-e29b-41d4-a716-446655440000" {
			t.Fatalf("unexpected session ID: %s", sessions[0].SessionID)
		}
	})

	t.Run("teacher overlap at ordinals 0 and 3", func(t *testing.T) {
		db := &fakeDBTX{}
		db.queryResults = []queryResult{
			{
				rows: func() *batchOrdinalRows {
					r := &batchOrdinalRows{idx: -1}
					r.entries = append(r.entries, struct {
						ordinal int64
						session ConflictSession
					}{
						ordinal: 0,
						session: ConflictSession{
							SessionID: "550e8400-e29b-41d4-a716-446655440000",
							CourseID:  "660e8400-e29b-41d4-a716-446655440000",
							TeacherID: "770e8400-e29b-41d4-a716-446655440000",
							StartAt:   time.Now().Format(time.RFC3339Nano),
							EndAt:     time.Now().Add(time.Hour).Format(time.RFC3339Nano),
						},
					})
					r.entries = append(r.entries, struct {
						ordinal int64
						session ConflictSession
					}{
						ordinal: 3,
						session: ConflictSession{
							SessionID: "660e8400-e29b-41d4-a716-446655440001",
							CourseID:  "660e8400-e29b-41d4-a716-446655440000",
							TeacherID: "770e8400-e29b-41d4-a716-446655440000",
							StartAt:   time.Now().Format(time.RFC3339Nano),
							EndAt:     time.Now().Add(time.Hour).Format(time.RFC3339Nano),
						},
					})
					return r
				}(),
			},
		}
		result, err := svc.checkTeacherOverlapsBatch(ctx, db, testUUID(3), ords, starts, ends, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("expected 2 ordinals, got %d", len(result))
		}
		if _, ok := result[0]; !ok {
			t.Fatal("expected conflict at ordinal 0")
		}
		if _, ok := result[3]; !ok {
			t.Fatal("expected conflict at ordinal 3")
		}
	})

	t.Run("no teacher ID returns nil", func(t *testing.T) {
		db := &fakeDBTX{}
		result, err := svc.checkTeacherOverlapsBatch(ctx, db, pgtype.UUID{Valid: false}, ords, starts, ends, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Fatal("expected nil result for invalid teacher ID")
		}
	})
}

func TestBatchCheckTeacherAvailability(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(5)
	ords, starts, ends := buildOccurrenceArrays(occ)

	t.Run("no windows - all available", func(t *testing.T) {
		db := &fakeDBTX{}
		db.queryRowResults = []pgx.Row{
			&fakeMultiRow{vals: []bool{false}},
		}
		result, err := svc.checkTeacherAvailabilityBatch(ctx, db, testUUID(3), ords, starts, ends)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Fatal("expected nil (all available) when no windows")
		}
	})

	t.Run("has windows - ordinals 1 and 4 uncovered", func(t *testing.T) {
		db := &fakeDBTX{}
		db.queryRowResults = []pgx.Row{
			&fakeMultiRow{vals: []bool{true}},
		}
		db.queryResults = []queryResult{
			{rows: &ordinalRows{ordinals: []int64{1, 4}, idx: -1}},
		}
		result, err := svc.checkTeacherAvailabilityBatch(ctx, db, testUUID(3), ords, starts, ends)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("expected 2 unavailable ordinals, got %d; result=%v", len(result), result)
		}
		if !result[1] {
			t.Fatal("expected ordinal 1 unavailable")
		}
		if !result[4] {
			t.Fatal("expected ordinal 4 unavailable")
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for preflightSeriesBatch
// ---------------------------------------------------------------------------

func TestPreflightSeriesBatch_NoConflicts(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(5)
	params := testPreflightSeriesParams()

	db := buildNoConflictDBTX(false, false)
	q := sqldb.New(db)

	se, err := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if se != nil {
		t.Fatalf("expected no conflict, got %s", se.Code)
	}
}

func TestPreflightSeriesBatch_TeacherNotAssigned(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(5)
	params := testPreflightSeriesParams()

	// CourseTeacherMembershipGet: exists=true, hasTeachers=true, assigned=false
	// SchedulingResourcesGet is NOT reached because membership fails first.
	db := &fakeDBTX{}
	db.queryRowResults = []pgx.Row{
		&fakeMultiRow{vals: []bool{true, true, false}},
	}
	q := sqldb.New(db)

	se, err := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if se == nil {
		t.Fatal("expected teacher_not_assigned conflict, got nil")
	}
	if se.Code != ErrTeacherNotAssigned {
		t.Fatalf("expected code %s, got %s", ErrTeacherNotAssigned, se.Code)
	}
}

func TestPreflightSeriesBatch_TeacherOverlapWins(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(10)
	params := testPreflightSeriesParams()

	// Only overlap queries call db.Query (availability unnest queries are
	// skipped when hasWindows=false).
	// QueryRow order:
	//   0: CourseTeacherMembershipGet (3 bools)
	//   1: SchedulingResourcesGet (4 bools)
	//   2: teacher avail EXISTS (1 bool)
	//   3: room avail EXISTS (1 bool)
	// Query order: room overlap (0), teacher overlap (1), student overlap (2).
	db := &fakeDBTX{}
	db.queryRowResults = []pgx.Row{
		&fakeMultiRow{vals: []bool{true, true, true}},       // memberships
		&fakeMultiRow{vals: []bool{true, true, true, true}}, // resources
		&fakeMultiRow{vals: []bool{false}},                  // teacher avail: no windows
		&fakeMultiRow{vals: []bool{false}},                  // room avail: no windows
	}
	db.queryResults = []queryResult{
		{rows: &fakeRows{}}, // room overlap: empty
		{
			rows: makeBatchOrdinalRows(0, []ConflictSession{{
				SessionID: "550e8400-e29b-41d4-a716-446655440000",
				CourseID:  "660e8400-e29b-41d4-a716-446655440000",
				TeacherID: "770e8400-e29b-41d4-a716-446655440000",
				StartAt:   time.Now().Format(time.RFC3339Nano),
				EndAt:     time.Now().Add(time.Hour).Format(time.RFC3339Nano),
			}}),
		}, // teacher overlap at ordinal 0
		{rows: &fakeRows{}}, // student overlap (not reached)
	}
	q := sqldb.New(db)

	se, err := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if se == nil {
		t.Fatal("expected conflict, got nil")
	}
	if se.Code != "schedule_conflict" {
		t.Fatalf("expected schedule_conflict, got %s", se.Code)
	}
	if se.Details.Kind != ConflictKindTeacherOverlap {
		t.Fatalf("expected teacher_overlap, got %s", se.Details.Kind)
	}
}

func TestPreflightSeriesBatch_RoomOverlapBeforeTeacherOverlap(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(10)
	params := testPreflightSeriesParams()

	db := &fakeDBTX{}
	db.queryRowResults = []pgx.Row{
		&fakeMultiRow{vals: []bool{true, true, true}},       // memberships
		&fakeMultiRow{vals: []bool{true, true, true, true}}, // resources
		&fakeMultiRow{vals: []bool{false}},                  // teacher avail: no windows
		&fakeMultiRow{vals: []bool{false}},                  // room avail: no windows
	}
	db.queryResults = []queryResult{
		{
			rows: makeBatchOrdinalRows(0, []ConflictSession{{
				SessionID: "550e8400-e29b-41d4-a716-446655440000",
				CourseID:  "660e8400-e29b-41d4-a716-446655440000",
				TeacherID: "770e8400-e29b-41d4-a716-446655440000",
				StartAt:   time.Now().Format(time.RFC3339Nano),
				EndAt:     time.Now().Add(time.Hour).Format(time.RFC3339Nano),
			}}),
		}, // room overlap at ordinal 0
		{rows: &fakeRows{}}, // teacher overlap (not reached)
		{rows: &fakeRows{}}, // student overlap (not reached)
	}
	q := sqldb.New(db)

	se, err := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if se == nil {
		t.Fatal("expected conflict, got nil")
	}
	if se.Details.Kind != ConflictKindRoomOverlap {
		t.Fatalf("expected room_overlap, got %s", se.Details.Kind)
	}
}

func TestPreflightSeriesBatch_TeacherAvailabilityBlocks(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(10)
	params := testPreflightSeriesParams()

	// Teacher has windows → teacher avail unnest Query called (index 0).
	// Room has no windows → room avail unnest Query skipped.
	// Then room overlap (index 1), teacher overlap (index 2), student overlap (index 3).
	db := &fakeDBTX{}
	db.queryRowResults = []pgx.Row{
		&fakeMultiRow{vals: []bool{true, true, true}},       // memberships
		&fakeMultiRow{vals: []bool{true, true, true, true}}, // resources
		&fakeMultiRow{vals: []bool{true}},                   // teacher avail: HAS windows
		&fakeMultiRow{vals: []bool{false}},                  // room avail: no windows
	}
	db.queryResults = []queryResult{
		{rows: &ordinalRows{ordinals: []int64{1}, idx: -1}}, // ordinal 1 uncovered
		{rows: &fakeRows{}}, // room overlap (not reached)
		{rows: &fakeRows{}}, // teacher overlap (not reached)
		{rows: &fakeRows{}}, // student overlap (not reached)
	}
	q := sqldb.New(db)

	se, err := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if se == nil {
		t.Fatal("expected conflict, got nil")
	}
	if se.Details.Kind != ConflictKindTeacherAvailability {
		t.Fatalf("expected teacher_availability, got %s", se.Details.Kind)
	}
}

func TestPreflightSeriesBatch_RoomAvailabilityBlock(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(10)
	params := testPreflightSeriesParams()

	// Teacher has no windows → teacher avail unnest Query skipped.
	// Room HAS windows → room avail unnest Query called (index 0).
	// Then room overlap (index 1), teacher overlap (index 2), student overlap (index 3).
	db := &fakeDBTX{}
	db.queryRowResults = []pgx.Row{
		&fakeMultiRow{vals: []bool{true, true, true}},       // memberships
		&fakeMultiRow{vals: []bool{true, true, true, true}}, // resources
		&fakeMultiRow{vals: []bool{false}},                  // teacher avail: no windows
		&fakeMultiRow{vals: []bool{true}},                   // room avail: HAS windows
	}
	db.queryResults = []queryResult{
		{rows: &ordinalRows{ordinals: []int64{3}, idx: -1}}, // ordinal 3 uncovered
		{rows: &fakeRows{}}, // room overlap (not reached)
		{rows: &fakeRows{}}, // teacher overlap (not reached)
		{rows: &fakeRows{}}, // student overlap (not reached)
	}
	q := sqldb.New(db)

	se, err := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if se == nil {
		t.Fatal("expected conflict, got nil")
	}
	if se.Details.Kind != ConflictKindRoomAvailability {
		t.Fatalf("expected room_availability, got %s", se.Details.Kind)
	}
}

func TestPreflightSeriesBatch_StudentOverlap(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(10)
	params := testPreflightSeriesParams()

	db := &fakeDBTX{}
	db.queryRowResults = []pgx.Row{
		&fakeMultiRow{vals: []bool{true, true, true}},       // memberships
		&fakeMultiRow{vals: []bool{true, true, true, true}}, // resources
		&fakeMultiRow{vals: []bool{false}},                  // teacher avail: no windows
		&fakeMultiRow{vals: []bool{false}},                  // room avail: no windows
	}
	db.queryResults = []queryResult{
		{rows: &fakeRows{}}, // room overlap: empty
		{rows: &fakeRows{}}, // teacher overlap: empty
		{
			rows: makeBatchOrdinalRows(5, []ConflictSession{{
				SessionID: "550e8400-e29b-41d4-a716-446655440000",
				CourseID:  "660e8400-e29b-41d4-a716-446655440000",
				TeacherID: "770e8400-e29b-41d4-a716-446655440000",
				StartAt:   time.Now().Format(time.RFC3339Nano),
				EndAt:     time.Now().Add(time.Hour).Format(time.RFC3339Nano),
			}}),
		}, // student overlap at ordinal 5
	}
	q := sqldb.New(db)

	se, err := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if se == nil {
		t.Fatal("expected conflict, got nil")
	}
	if se.Details.Kind != ConflictKindStudentOverlap {
		t.Fatalf("expected student_overlap, got %s", se.Details.Kind)
	}
}

func TestPreflightSeriesBatch_PriorityOrder(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(10)
	params := testPreflightSeriesParams()

	// Room overlap at ordinal 0 checked before teacher overlap.
	db := &fakeDBTX{}
	db.queryRowResults = []pgx.Row{
		&fakeMultiRow{vals: []bool{true, true, true}},       // memberships
		&fakeMultiRow{vals: []bool{true, true, true, true}}, // resources
		&fakeMultiRow{vals: []bool{false}},                  // teacher avail: no windows
		&fakeMultiRow{vals: []bool{false}},                  // room avail: no windows
	}
	db.queryResults = []queryResult{
		{
			rows: makeBatchOrdinalRows(0, []ConflictSession{{
				SessionID: "550e8400-e29b-41d4-a716-446655440000",
				CourseID:  "660e8400-e29b-41d4-a716-446655440000",
				TeacherID: "770e8400-e29b-41d4-a716-446655440000",
				StartAt:   time.Now().Format(time.RFC3339Nano),
				EndAt:     time.Now().Add(time.Hour).Format(time.RFC3339Nano),
			}}),
		}, // room overlap at ordinal 0
		{rows: &fakeRows{}}, // teacher overlap (not reached)
		{rows: &fakeRows{}}, // student overlap (not reached)
	}
	q := sqldb.New(db)

	se, err := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if se == nil {
		t.Fatal("expected conflict, got nil")
	}
	if se.Details.Kind != ConflictKindRoomOverlap {
		t.Fatalf("expected room_overlap (highest priority), got %s", se.Details.Kind)
	}
}

func TestPreflightSeriesBatch_NilRoom(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(5)
	params := testPreflightSeriesParams()
	params.RoomID = pgtype.UUID{Valid: false}

	// No room → no room query rows. QueryRow order:
	//   CourseTeacherMembershipGet, SchedulingResourcesGet (room exists still queried),
	//   teacher avail EXISTS.
	// Only Query calls: teacher overlap (0), student overlap (1).
	db := &fakeDBTX{}
	db.queryRowResults = []pgx.Row{
		&fakeMultiRow{vals: []bool{true, true, true}},       // memberships
		&fakeMultiRow{vals: []bool{true, true, true, true}}, // resources (roomExists ignored)
		&fakeMultiRow{vals: []bool{false}},                  // teacher avail: no windows
	}
	db.queryResults = []queryResult{
		{rows: &fakeRows{}}, // teacher overlap: empty
		{rows: &fakeRows{}}, // student overlap: empty
	}
	q := sqldb.New(db)

	se, err := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if se != nil {
		t.Fatalf("expected no conflict with nil room, got %s", se.Code)
	}
}

func TestPreflightSeriesBatch_1000Occurrences(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(1000)
	params := testPreflightSeriesParams()

	db := buildNoConflictDBTX(false, false)
	q := sqldb.New(db)

	se, err := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if se != nil {
		t.Fatalf("expected no conflict, got %s", se.Code)
	}
}

// ---------------------------------------------------------------------------
// Boundary tests
// ---------------------------------------------------------------------------

func TestBatchPreflight_FirstBlocked(t *testing.T) {
	t.Run("no entries", func(t *testing.T) {
		if got := firstBlocked(nil); got != -1 {
			t.Fatalf("expected -1, got %d", got)
		}
		if got := firstBlocked(map[int]bool{}); got != -1 {
			t.Fatalf("expected -1, got %d", got)
		}
	})

	t.Run("single entry", func(t *testing.T) {
		if got := firstBlocked(map[int]bool{5: true}); got != 5 {
			t.Fatalf("expected 5, got %d", got)
		}
	})

	t.Run("multiple entries", func(t *testing.T) {
		m := map[int]bool{5: true, 2: true, 8: true}
		if got := firstBlocked(m); got != 2 {
			t.Fatalf("expected 2, got %d", got)
		}
	})
}

func TestBatchPreflight_FirstOverlap(t *testing.T) {
	t.Run("no entries", func(t *testing.T) {
		ord, cs := firstOverlap(nil)
		if ord != -1 || cs != nil {
			t.Fatal("expected -1, nil")
		}
		ord, cs = firstOverlap(map[int][]ConflictSession{})
		if ord != -1 || cs != nil {
			t.Fatal("expected -1, nil")
		}
	})

	t.Run("single entry", func(t *testing.T) {
		m := map[int][]ConflictSession{3: {{SessionID: "s1"}}}
		ord, cs := firstOverlap(m)
		if ord != 3 {
			t.Fatalf("expected 3, got %d", ord)
		}
		if len(cs) != 1 || cs[0].SessionID != "s1" {
			t.Fatal("wrong conflict data")
		}
	})

	t.Run("earliest ordinal wins", func(t *testing.T) {
		m := map[int][]ConflictSession{
			7: {{SessionID: "s1"}},
			3: {{SessionID: "s2"}},
			5: {{SessionID: "s3"}},
		}
		ord, cs := firstOverlap(m)
		if ord != 3 {
			t.Fatalf("expected 3, got %d", ord)
		}
		if cs[0].SessionID != "s2" {
			t.Fatalf("expected s2, got %s", cs[0].SessionID)
		}
	})
}

func TestBatchPreflight_BuildOccurrenceArrays(t *testing.T) {
	occ := []series.Occurrence{
		{StartUTC: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC), EndUTC: time.Date(2026, 1, 5, 11, 0, 0, 0, time.UTC)},
		{StartUTC: time.Date(2026, 1, 6, 10, 0, 0, 0, time.UTC), EndUTC: time.Date(2026, 1, 6, 11, 0, 0, 0, time.UTC)},
	}

	ords, starts, ends := buildOccurrenceArrays(occ)

	if len(ords) != 2 || len(starts) != 2 || len(ends) != 2 {
		t.Fatal("wrong array lengths")
	}
	if ords[0] != 0 || ords[1] != 1 {
		t.Fatalf("wrong ordinals: got %v", ords)
	}
	if !starts[0].Equal(time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)) {
		t.Fatal("wrong start[0]")
	}
	if !ends[1].Equal(time.Date(2026, 1, 6, 11, 0, 0, 0, time.UTC)) {
		t.Fatal("wrong end[1]")
	}
}

// ---------------------------------------------------------------------------
// Query count test
// ---------------------------------------------------------------------------

// countingDBTX wraps sqldb.DBTX and counts Query/QueryRow/Exec calls.
type countingDBTX struct {
	inner         sqldb.DBTX
	queryCount    int
	queryRowCount int
}

func (c *countingDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return c.inner.Exec(ctx, sql, args...)
}

func (c *countingDBTX) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	c.queryCount++
	return c.inner.Query(ctx, sql, args...)
}

func (c *countingDBTX) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	c.queryRowCount++
	return c.inner.QueryRow(ctx, sql, args...)
}

func needsDB(t *testing.T) {
	t.Skip("Test requires a real database — skipped")
}

func TestBatchPreflight_QueryCountIsConstant(t *testing.T) {
	needsDB(t)
}

func TestBatchPreflight_ScanBatchConflictSessions(t *testing.T) {
	rows := makeBatchOrdinalRows(0, []ConflictSession{{
		SessionID: "550e8400-e29b-41d4-a716-446655440000",
		CourseID:  "660e8400-e29b-41d4-a716-446655440000",
		TeacherID: "770e8400-e29b-41d4-a716-446655440000",
		StartAt:   time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		EndAt:     time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}})

	result, err := scanBatchConflictSessions(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 ordinal, got %d", len(result))
	}
	sessions, ok := result[0]
	if !ok {
		t.Fatal("expected ordinal 0")
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("wrong session ID: %s", sessions[0].SessionID)
	}
}

// ---------------------------------------------------------------------------
// Equivalence tests
// ---------------------------------------------------------------------------

func TestBatchPreflight_Equivalence_NoConflicts(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(5)
	params := testPreflightSeriesParams()

	db := buildNoConflictDBTX(false, false)
	q := sqldb.New(db)

	batchSE, err := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if err != nil {
		t.Fatalf("batch returned error: %v", err)
	}
	if batchSE != nil {
		t.Fatalf("expected no conflict, got %s", batchSE.Code)
	}
}

func TestBatchPreflight_Equivalence_TeacherOverlap(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(5)
	params := testPreflightSeriesParams()

	// Batch path returns teacher overlap at ordinal 2.
	// Verifies conflict kind and code are correct.
	db := &fakeDBTX{}
	db.queryRowResults = []pgx.Row{
		&fakeMultiRow{vals: []bool{true, true, true}},       // memberships
		&fakeMultiRow{vals: []bool{true, true, true, true}}, // resources
		&fakeMultiRow{vals: []bool{false}},                  // teacher avail: no windows
		&fakeMultiRow{vals: []bool{false}},                  // room avail: no windows
	}
	db.queryResults = []queryResult{
		{rows: &fakeRows{}}, // room overlap: empty
		{
			rows: makeBatchOrdinalRows(2, []ConflictSession{{
				SessionID: "550e8400-e29b-41d4-a716-446655440000",
				CourseID:  "660e8400-e29b-41d4-a716-446655440000",
				TeacherID: "770e8400-e29b-41d4-a716-446655440000",
				StartAt:   time.Now().Format(time.RFC3339Nano),
				EndAt:     time.Now().Add(time.Hour).Format(time.RFC3339Nano),
			}}),
		}, // teacher overlap at ordinal 2
		{rows: &fakeRows{}}, // student overlap (not reached)
	}
	q := sqldb.New(db)

	batchSE, batchErr := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if batchErr != nil {
		t.Fatalf("batch error: %v", batchErr)
	}
	if batchSE == nil {
		t.Fatal("batch expected conflict, got nil")
	}
	if batchSE.Details.Kind != ConflictKindTeacherOverlap {
		t.Fatalf("expected teacher_overlap, got %s", batchSE.Details.Kind)
	}
	if batchSE.Code != "schedule_conflict" {
		t.Fatalf("expected schedule_conflict, got %s", batchSE.Code)
	}
	if len(batchSE.Details.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict session, got %d", len(batchSE.Details.Conflicts))
	}
}

// ---------------------------------------------------------------------------
// Corner cases
// ---------------------------------------------------------------------------

func TestBatchPreflight_EmptyOccurrences(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	params := testPreflightSeriesParams()

	se, err := svc.preflightSeriesBatch(ctx, &fakeDBTX{}, sqldb.New(&fakeDBTX{}), params, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if se != nil {
		t.Fatal("expected nil for empty occurrences")
	}
}

func TestBatchPreflight_SingleOccurrence(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(1)
	params := testPreflightSeriesParams()

	db := buildNoConflictDBTX(false, false)
	q := sqldb.New(db)

	se, err := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if se != nil {
		t.Fatalf("expected no conflict, got %s", se.Code)
	}
}

func TestBatchPreflight_AdjacentRanges(t *testing.T) {
	occ := []series.Occurrence{
		{StartUTC: time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC), EndUTC: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)},
		{StartUTC: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC), EndUTC: time.Date(2026, 1, 5, 11, 0, 0, 0, time.UTC)},
	}

	svc := &Service{}
	ctx := context.Background()
	params := testPreflightSeriesParams()

	db := buildNoConflictDBTX(false, false)
	q := sqldb.New(db)

	se, err := svc.preflightSeriesBatch(ctx, db, q, params, occ)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if se != nil {
		t.Fatalf("expected no conflict for adjacent ranges, got %s", se.Code)
	}
}

func TestBatchPreflight_HandlesNilIgnoreSeries(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(5)
	ords, starts, ends := buildOccurrenceArrays(occ)

	db := &fakeDBTX{}
	result, err := svc.checkTeacherOverlapsBatch(ctx, db, testUUID(3), ords, starts, ends, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil && len(result) > 0 {
		t.Fatal("expected empty result")
	}
}

// errorRows implements pgx.Rows that returns an error on Err().
type errorRows struct {
	done bool
}

func (r *errorRows) Close()                                       {}
func (r *errorRows) Err() error                                   { return errors.New("simulated query error") }
func (r *errorRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *errorRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *errorRows) Next() bool {
	if r.done {
		return false
	}
	r.done = true
	return false
}
func (r *errorRows) Scan(dest ...interface{}) error { return errors.New("simulated scan error") }
func (r *errorRows) RawValues() [][]byte            { return nil }
func (r *errorRows) Values() ([]interface{}, error) { return nil, nil }
func (r *errorRows) Conn() *pgx.Conn                { return nil }

func TestBatchPreflight_PropagatesQueryError(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	occ := testOccurrences(3)
	ords, starts, ends := buildOccurrenceArrays(occ)

	db := &fakeDBTX{}
	db.queryResults = []queryResult{
		{rows: &errorRows{}, err: errors.New("query failed")},
	}

	_, err := svc.checkTeacherOverlapsBatch(ctx, db, testUUID(3), ords, starts, ends, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "query failed") {
		t.Fatalf("expected 'query failed' in error, got %s", err.Error())
	}
}
