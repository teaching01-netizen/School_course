package scheduling

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func TestUserStory_EmptyTeacherSetPreflightAndCreateReturnStableConflict(t *testing.T) {
	// Given
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	teacherID := createMembershipTeacher(t, ctx, q, "user-story-empty")
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{
		Name:     "R-user-story-empty-" + uuid.New().String()[:8],
		Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	emptyCourse, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: "C-USER-STORY-EMPTY-" + uuid.New().String()[:8],
		Name: "User story empty teacher set",
	})
	if err != nil {
		t.Fatal(err)
	}
	start, end := membershipFutureSlot(9 * 24 * time.Hour)

	// When
	_, preflightErr, err := svc.Preflight(ctx, PreflightParams{
		CourseID:  emptyCourse.ID,
		RoomID:    room.ID,
		TeacherID: teacherID,
		StartAt:   start,
		EndAt:     end,
	})

	// Then
	if err != nil {
		t.Fatalf("preflight returned infrastructure error: %v", err)
	}
	if preflightErr == nil {
		t.Fatal("preflight returned no conflict")
	}
	if preflightErr.Code != "course_has_no_assigned_teachers" {
		t.Fatalf("expected preflight code %q, got %q", "course_has_no_assigned_teachers", preflightErr.Code)
	}
	if preflightErr.Details.Kind != ConflictKind("course_has_no_assigned_teachers") {
		t.Fatalf("expected preflight kind %q, got %q", "course_has_no_assigned_teachers", preflightErr.Details.Kind)
	}

	seriesDate := time.Now().In(svc.loc).AddDate(0, 0, 21)
	seriesCount := 1
	_, seriesErr, err := svc.PreflightSeries(ctx, PreflightSeriesParams{
		CourseID:        emptyCourse.ID,
		RoomID:          room.ID,
		TeacherID:       teacherID,
		Weekdays:        []time.Weekday{seriesDate.Weekday()},
		StartLocalTime:  Clock{Hour: 9, Minute: 0},
		DurationMinutes: 60,
		StartDate:       LocalDate{Year: seriesDate.Year(), Month: seriesDate.Month(), Day: seriesDate.Day()},
		Count:           &seriesCount,
	})
	if err != nil {
		t.Fatalf("series preflight returned infrastructure error: %v", err)
	}
	if seriesErr == nil {
		t.Fatal("series preflight returned no conflict")
	}
	if seriesErr.Code != "course_has_no_assigned_teachers" {
		t.Fatalf("expected series preflight code %q, got %q", "course_has_no_assigned_teachers", seriesErr.Code)
	}
	if seriesErr.Details.Kind != preflightErr.Details.Kind {
		t.Fatalf("expected series preflight kind %q, got %q", preflightErr.Details.Kind, seriesErr.Details.Kind)
	}
	if seriesErr.Details.Kind != ConflictKind("course_has_no_assigned_teachers") {
		t.Fatalf("expected series preflight kind %q, got %q", "course_has_no_assigned_teachers", seriesErr.Details.Kind)
	}

	_, err = svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  emptyCourse.ID,
		RoomID:    room.ID,
		TeacherID: teacherID,
		StartAt:   start,
		EndAt:     end,
	})
	var createErr *Err
	if !errors.As(err, &createErr) {
		t.Fatalf("expected *scheduling.Err from create, got %T (%v)", err, err)
	}
	if createErr.Code != "course_has_no_assigned_teachers" {
		t.Fatalf("expected create code %q, got %q", "course_has_no_assigned_teachers", createErr.Code)
	}
	if createErr.Details.Kind != ConflictKind("course_has_no_assigned_teachers") {
		t.Fatalf("expected create kind %q, got %q", "course_has_no_assigned_teachers", createErr.Details.Kind)
	}
}
