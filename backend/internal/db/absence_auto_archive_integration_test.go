package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestAutoArchiveExpiredSitInsUsesLatestAssignedDate(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	now := time.Now().UTC()
	date := func(offset int) time.Time {
		return now.AddDate(0, 0, offset)
	}

	actor, err := q.AdminUserCreate(ctx, AdminUserCreateParams{
		Username:     "absence-auto-archive-" + now.Format("20060102150405.000000000"),
		Role:         "Admin",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: fmt.Sprintf("AUTOARCH-%d", now.UnixNano()),
		Name: "Auto archive test",
	})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{
		Name:     "Auto archive test room",
		Capacity: pgtype.Int4{Int32: 20, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	createSession := func(start time.Time) pgtype.UUID {
		t.Helper()
		session, err := q.SessionCreate(ctx, SessionCreateParams{
			CourseID:  course.ID,
			RoomID:    room.ID,
			TeacherID: actor,
			StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:     pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return session.ID
	}

	createAbsence := func(name string, method string, sessionIDs ...pgtype.UUID) pgtype.UUID {
		t.Helper()
		absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
			Wcode:         name,
			CourseID:      course.ID,
			DateFrom:      pgtype.Date{Time: date(-3), Valid: true},
			DateTo:        pgtype.Date{Time: date(-3), Valid: true},
			Reason:        pgtype.Text{String: "test", Valid: true},
			SitInCourseID: course.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := dbpool.Exec(ctx, `UPDATE student_absences SET sit_in_method = $2 WHERE id = $1`, absence.ID, method); err != nil {
			t.Fatal(err)
		}
		if err := q.AbsenceSitInsCreate(ctx, absence.ID, sessionIDs); err != nil {
			t.Fatal(err)
		}
		return absence.ID
	}

	oldest := createSession(date(-3))
	middle := createSession(date(-1))
	today := createSession(date(0))
	future := createSession(date(1))

	expired := createAbsence("W-AUTO-EXPIRED", "physical", oldest, middle)
	withToday := createAbsence("W-AUTO-TODAY", "physical", oldest, today)
	withFuture := createAbsence("W-AUTO-FUTURE", "physical", oldest, future)
	withoutDate := createAbsence("W-AUTO-UNDATED", "zoom")
	physicalWithoutSession := createAbsence("W-AUTO-NO-SESSION", "physical")

	archived, err := q.AutoArchiveExpiredSitIns(ctx, "UTC", actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0] != expired {
		t.Fatalf("auto-archived IDs = %v, want only %s", archived, expired)
	}

	statusByID := make(map[pgtype.UUID]string)
	rows, err := dbpool.Query(ctx, `SELECT id, status FROM student_absences WHERE id = ANY($1::uuid[])`, []pgtype.UUID{expired, withToday, withFuture, withoutDate, physicalWithoutSession})
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id pgtype.UUID
		var status string
		if err := rows.Scan(&id, &status); err != nil {
			t.Fatal(err)
		}
		statusByID[id] = status
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if statusByID[expired] != "actioned" {
		t.Fatalf("expired status = %q, want actioned", statusByID[expired])
	}
	for _, id := range []pgtype.UUID{withToday, withFuture, withoutDate, physicalWithoutSession} {
		if statusByID[id] != "pending" {
			t.Fatalf("absence %s status = %q, want pending", id, statusByID[id])
		}
	}

	archived, err = q.AutoArchiveExpiredSitIns(ctx, "UTC", actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("second auto-archive IDs = %v, want none", archived)
	}
}
