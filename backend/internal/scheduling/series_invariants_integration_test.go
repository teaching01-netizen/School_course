package scheduling

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func TestScheduleDB_ConcurrentSeriesCancelAndAttachedCreatePreservesDefinition(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	fx := seedOccupancyFixture(t, q, "series-cancel-attach")
	first := futureBangkok(30, 10)
	count := 4
	created, err := svc.CreateSeriesAndMaterialize(context.Background(), CreateSeriesParams{
		CourseID: fx.courseID, RoomID: fx.roomID, TeacherID: fx.teacherID,
		Weekdays: []time.Weekday{first.Weekday()}, StartLocalTime: Clock{Hour: first.Hour()}, DurationMinutes: 60,
		StartDate: LocalDate{Year: first.Year(), Month: first.Month(), Day: first.Day()}, Count: &count,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := q.SessionListActiveByCourse(context.Background(), fx.courseID)
	if err != nil {
		t.Fatal(err)
	}
	var firstSessionID pgtype.UUID
	var attachStart, attachEnd pgtype.Timestamptz
	for _, row := range rows {
		if row.SeriesID.Valid && row.SeriesID.Bytes == created.SeriesID.Bytes {
			firstSessionID = row.ID
			attachStart, attachEnd = row.StartAt, row.EndAt
			break
		}
	}
	if !firstSessionID.Valid {
		t.Fatal("first materialized occurrence not found")
	}
	if _, err := pool.Exec(context.Background(), `UPDATE sessions SET deleted_at=now() WHERE id=$1`, firstSessionID); err != nil {
		t.Fatal(err)
	}

	left, right := runRace(t, func(ctx context.Context) error {
		_, err := svc.CancelSeries(ctx, CancelSeriesParams{
			SeriesID: created.SeriesID, Scope: CancelScopeEntireSeriesFutureOnly, ExpectedVersion: 1, NowUTC: time.Now().UTC(),
		})
		return err
	}, func(ctx context.Context) error {
		seriesID := created.SeriesID
		_, err := svc.CreateSession(ctx, CreateSessionParams{
			SeriesID: &seriesID, CourseID: fx.courseID, RoomID: fx.roomID, TeacherID: fx.teacherID,
			StartAt: attachStart, EndAt: attachEnd,
		})
		return err
	})
	if left != nil && right != nil {
		t.Logf("both writers rejected safely: cancel=%v attach=%v", left, right)
	}
	definition, err := q.SeriesGetByID(context.Background(), created.SeriesID)
	if err != nil {
		t.Fatal(err)
	}
	active, err := q.SessionListActiveByCourse(context.Background(), fx.courseID)
	if err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation(definition.InstituteTz)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range active {
		if !row.SeriesID.Valid || row.SeriesID.Bytes != created.SeriesID.Bytes {
			continue
		}
		if definition.EndDate.Valid {
			end := definition.EndDate.Time.UTC().Format("2006-01-02")
			if row.StartAt.Time.In(loc).Format("2006-01-02") > end {
				t.Fatalf("active session %s lies beyond canceled series end %s", row.StartAt.Time, end)
			}
		}
	}
}
