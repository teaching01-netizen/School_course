package scheduling

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func userStoryActiveSeriesRows(t *testing.T, q *sqldb.Queries, courseID, seriesID pgtype.UUID) []sqldb.SessionListActiveByCourseRow {
	t.Helper()
	rows, err := q.SessionListActiveByCourse(context.Background(), courseID)
	if err != nil {
		t.Fatal(err)
	}
	filtered := make([]sqldb.SessionListActiveByCourseRow, 0, len(rows))
	for _, row := range rows {
		if row.SeriesID.Valid && row.SeriesID.Bytes == seriesID.Bytes {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func userStorySessionIDs(rows []sqldb.SessionListActiveByCourseRow) map[[16]byte]struct{} {
	ids := make(map[[16]byte]struct{}, len(rows))
	for _, row := range rows {
		ids[row.ID.Bytes] = struct{}{}
	}
	return ids
}

func TestUserStory_EditEntireSeriesConflictRollsBackDefinitionAndOccurrences(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fx := seedOccupancyFixture(t, q, "user-story-entire-edit")
	replacementRoom, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{
		Name:     "user-story-entire-edit-replacement-" + uuid.NewString()[:8],
		Capacity: pgtype.Int4{Int32: 20, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
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
	beforeSeries, err := q.SeriesGetByID(ctx, created.SeriesID)
	if err != nil {
		t.Fatal(err)
	}
	beforeRows := userStoryActiveSeriesRows(t, q, fx.courseID, created.SeriesID)
	if len(beforeRows) != 10 {
		t.Fatalf("active occurrences before edit=%d, want 10", len(beforeRows))
	}
	beforeIDs := userStorySessionIDs(beforeRows)

	conflictStart := start.Add(4 * time.Hour)
	if _, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  fx.courseID,
		RoomID:    replacementRoom.ID,
		TeacherID: fx.teacherID,
		StartAt:   pgtype.Timestamptz{Time: conflictStart, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: conflictStart.Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	_, err = svc.EditEntireSeriesFutureOnly(ctx, EditEntireSeriesParams{
		SeriesID:        created.SeriesID,
		ExpectedVersion: beforeSeries.Version,
		NowUTC:          time.Now().UTC(),
		CourseID:        fx.courseID,
		RoomID:          replacementRoom.ID,
		TeacherID:       fx.teacherID,
		Weekdays:        []time.Weekday{startLocal.Weekday()},
		StartLocalTime:  Clock{Hour: 14},
		DurationMinutes: 60,
		Count:           &count,
	})
	if err == nil {
		t.Fatal("conflicting entire-series edit succeeded")
	}
	var schedulingErr *Err
	if !errors.As(err, &schedulingErr) {
		t.Fatalf("expected scheduling conflict, got %T (%v)", err, err)
	}
	if schedulingErr.Code != "schedule_conflict" {
		t.Fatalf("conflict code=%q, want schedule_conflict", schedulingErr.Code)
	}

	afterSeries, err := q.SeriesGetByID(ctx, created.SeriesID)
	if err != nil {
		t.Fatal(err)
	}
	if afterSeries.Version != beforeSeries.Version ||
		afterSeries.RoomID.Bytes != beforeSeries.RoomID.Bytes ||
		afterSeries.DurationMinutes != beforeSeries.DurationMinutes ||
		afterSeries.StartLocalTime.Microseconds != beforeSeries.StartLocalTime.Microseconds ||
		!reflect.DeepEqual(afterSeries.Weekdays, beforeSeries.Weekdays) {
		t.Fatalf("series definition changed after conflict: before=%+v after=%+v", beforeSeries, afterSeries)
	}
	afterRows := userStoryActiveSeriesRows(t, q, fx.courseID, created.SeriesID)
	if len(afterRows) != 10 || !reflect.DeepEqual(userStorySessionIDs(afterRows), beforeIDs) {
		t.Fatalf("occurrences changed after conflict: before=%v after=%v", beforeIDs, userStorySessionIDs(afterRows))
	}

	newWeekday := (startLocal.Weekday() + 1) % 7
	updated, err := svc.EditEntireSeriesFutureOnly(ctx, EditEntireSeriesParams{
		SeriesID:        created.SeriesID,
		ExpectedVersion: beforeSeries.Version,
		NowUTC:          time.Now().UTC(),
		CourseID:        fx.courseID,
		RoomID:          replacementRoom.ID,
		TeacherID:       fx.teacherID,
		Weekdays:        []time.Weekday{newWeekday},
		StartLocalTime:  Clock{Hour: 12},
		DurationMinutes: 90,
		Count:           &count,
	})
	if err != nil {
		t.Fatalf("valid entire-series replacement failed: %v", err)
	}
	if updated.SessionsCanceled != 10 || updated.SessionsAdded != 10 {
		t.Fatalf("replacement counts=(canceled=%d,added=%d), want (10,10)", updated.SessionsCanceled, updated.SessionsAdded)
	}
	afterSeries, err = q.SeriesGetByID(ctx, created.SeriesID)
	if err != nil {
		t.Fatal(err)
	}
	if afterSeries.Version != beforeSeries.Version+1 ||
		afterSeries.RoomID.Bytes != replacementRoom.ID.Bytes ||
		afterSeries.DurationMinutes != 90 ||
		afterSeries.StartLocalTime.Microseconds != 12*60*60*1_000_000 ||
		len(afterSeries.Weekdays) != 1 ||
		time.Weekday(afterSeries.Weekdays[0]) != newWeekday {
		t.Fatalf("replacement definition=%+v", afterSeries)
	}
	afterRows = userStoryActiveSeriesRows(t, q, fx.courseID, created.SeriesID)
	if len(afterRows) != 10 {
		t.Fatalf("active occurrences after replacement=%d, want 10", len(afterRows))
	}
	for _, row := range afterRows {
		local := row.StartAt.Time.In(svc.loc)
		if row.RoomID.Bytes != replacementRoom.ID.Bytes ||
			local.Weekday() != newWeekday ||
			local.Hour() != 12 ||
			row.EndAt.Time.Sub(row.StartAt.Time) != 90*time.Minute {
			t.Fatalf("replacement occurrence=%+v, want room=%s weekday=%s 12:00 duration=90m", row, replacementRoom.ID, newWeekday)
		}
	}
}

func TestUserStory_CancelSeriesScopesPreserveHistoryAndSoftDeleteFuture(t *testing.T) {
	tests := []struct {
		name  string
		scope CancelScope
	}{
		{name: "this and future", scope: CancelScopeThisAndFuture},
		{name: "entire series future only", scope: CancelScopeEntireSeriesFutureOnly},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			databaseURL := requireTestDB(t)
			migrateUpOnce(t, databaseURL)
			dbpool := newPool(t, databaseURL)
			t.Cleanup(dbpool.Close)
			q := sqldb.New(dbpool)
			svc := newTestService(t, dbpool)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			fx := seedOccupancyFixture(t, q, "user-story-cancel")
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
			rows := userStoryActiveSeriesRows(t, q, fx.courseID, created.SeriesID)
			if len(rows) != 10 {
				t.Fatalf("active occurrences before cancel=%d, want 10", len(rows))
			}
			nowUTC := time.Now().UTC()
			for i, row := range rows[:4] {
				pastStart := nowUTC.Add(-time.Duration(5-i) * 24 * time.Hour)
				if _, err := dbpool.Exec(ctx, `UPDATE sessions SET start_at=$2, end_at=$3 WHERE id=$1`,
					row.ID, pastStart, pastStart.Add(time.Hour)); err != nil {
					t.Fatal(err)
				}
			}
			pivotLocal := rows[4].StartAt.Time.In(svc.loc)
			pivot := LocalDate{Year: pivotLocal.Year(), Month: pivotLocal.Month(), Day: pivotLocal.Day()}
			cutoff := nowUTC
			params := CancelSeriesParams{
				SeriesID:        created.SeriesID,
				Scope:           test.scope,
				ExpectedVersion: 1,
				NowUTC:          nowUTC,
			}
			if test.scope == CancelScopeThisAndFuture {
				params.PivotDate = &pivot
				cutoff = time.Date(pivot.Year, pivot.Month, pivot.Day, 10, 0, 0, 0, svc.loc).UTC()
			}

			result, err := svc.CancelSeries(ctx, params)
			if err != nil {
				t.Fatalf("cancel failed: %v", err)
			}
			if result.SessionsCanceled != 6 {
				t.Fatalf("sessions canceled=%d, want 6", result.SessionsCanceled)
			}

			var total, active, deleted, historyActive, futureDeleted int
			if err := dbpool.QueryRow(ctx, `
				SELECT count(*),
				       count(*) FILTER (WHERE deleted_at IS NULL),
				       count(*) FILTER (WHERE deleted_at IS NOT NULL),
				       count(*) FILTER (WHERE deleted_at IS NULL AND start_at < $2),
				       count(*) FILTER (WHERE deleted_at IS NOT NULL AND start_at >= $2)
				FROM sessions
				WHERE course_id = $1 AND series_id = $3
			`, fx.courseID, cutoff, created.SeriesID).Scan(&total, &active, &deleted, &historyActive, &futureDeleted); err != nil {
				t.Fatal(err)
			}
			if total != 10 || active != 4 || deleted != 6 || historyActive != 4 || futureDeleted != 6 {
				t.Fatalf("cancellation rows=(total=%d,active=%d,deleted=%d,history_active=%d,future_deleted=%d), want (10,4,6,4,6)", total, active, deleted, historyActive, futureDeleted)
			}
			var changeCount, impactRunCount int
			if err := dbpool.QueryRow(ctx, `
				SELECT count(*) FROM session_changes
				WHERE session_id IN (SELECT id FROM sessions WHERE course_id=$1 AND series_id=$2 AND deleted_at IS NOT NULL)
				  AND change_source='series_cancel'`, fx.courseID, created.SeriesID).Scan(&changeCount); err != nil {
				t.Fatal(err)
			}
			if err := dbpool.QueryRow(ctx, `
				SELECT count(*) FROM session_change_impact_runs
				WHERE session_change_id IN (
					SELECT id FROM session_changes
					WHERE session_id IN (SELECT id FROM sessions WHERE course_id=$1 AND series_id=$2 AND deleted_at IS NOT NULL)
					  AND change_source='series_cancel'
				)`, fx.courseID, created.SeriesID).Scan(&impactRunCount); err != nil {
				t.Fatal(err)
			}
			if changeCount != 6 || impactRunCount != 6 {
				t.Fatalf("series cancellation impact rows = changes %d/runs %d, want 6/6", changeCount, impactRunCount)
			}
		})
	}
}
