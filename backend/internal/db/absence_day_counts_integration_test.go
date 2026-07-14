package db

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAbsenceDayCountsForCourse(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	q := New(pool)
	suffix := strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")

	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{
		Username:     "teacher-aday-" + suffix,
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "ADAY-" + suffix, Name: "Absence Days " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	wcode := "waday-" + suffix
	student, err := q.StudentCreate(ctx, StudentCreateParams{Wcode: wcode, FullName: "Absence Day Student"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, CourseStudentAddParams{CourseID: course.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}

	createSession := func(start time.Time) pgtype.UUID {
		t.Helper()
		row, createErr := q.SessionCreate(ctx, SessionCreateParams{
			CourseID:  course.ID,
			TeacherID: teacherID,
			StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:     pgtype.Timestamptz{Time: start.Add(90 * time.Minute), Valid: true},
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return row.ID
	}
	dayOneMorning := createSession(time.Date(2026, 6, 1, 2, 0, 0, 0, time.UTC))
	dayOneAfternoon := createSession(time.Date(2026, 6, 1, 4, 0, 0, 0, time.UTC))
	createSession(time.Date(2026, 6, 1, 18, 30, 0, 0, time.UTC))
	dayThree := createSession(time.Date(2026, 6, 2, 17, 30, 0, 0, time.UTC))

	createAbsence := func(day int, sessionIDs []pgtype.UUID, status string) {
		t.Helper()
		date := pgtype.Date{Time: time.Date(2026, 6, day, 0, 0, 0, 0, time.UTC), Valid: true}
		row, createErr := q.AbsenceCreate(ctx, AbsenceCreateParams{Wcode: wcode, CourseID: course.ID, DateFrom: date, DateTo: date})
		if createErr != nil {
			t.Fatal(createErr)
		}
		if len(sessionIDs) > 0 {
			if createErr = q.AbsenceMissedSessionsCreate(ctx, row.ID, sessionIDs); createErr != nil {
				t.Fatal(createErr)
			}
		}
		if status != "" {
			if _, createErr = pool.Exec(ctx, `UPDATE student_absences SET status = $2 WHERE id = $1`, row.ID, status); createErr != nil {
				t.Fatal(createErr)
			}
		}
	}

	createAbsence(1, []pgtype.UUID{dayOneMorning}, "")
	createAbsence(1, []pgtype.UUID{dayOneAfternoon}, "")
	createAbsence(2, nil, "")
	createAbsence(3, []pgtype.UUID{dayThree}, "cancelled")
	createAbsence(3, []pgtype.UUID{dayThree}, "special_approved")

	dayOne := pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	counts, err := q.AbsenceDayCountsForCourse(ctx, AbsenceDayCountsForCourseParams{
		Wcode:               strings.ToUpper(wcode),
		CourseID:            course.ID,
		CandidateSessionIDs: []pgtype.UUID{dayOneMorning, dayOneAfternoon},
		DateFrom:            dayOne,
		DateTo:              dayOne,
		InstituteTZ:         "Asia/Bangkok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if counts.TotalCourseDays != 3 || counts.UsedAbsenceDays != 2 || counts.CandidateAbsenceDays != 1 || counts.ProjectedAbsenceDays != 2 {
		t.Fatalf("same-day counts = %+v, want total=3 used=2 candidate=1 projected=2", counts)
	}

	dayThreeDate := pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true}
	counts, err = q.AbsenceDayCountsForCourse(ctx, AbsenceDayCountsForCourseParams{
		Wcode:               wcode,
		CourseID:            course.ID,
		CandidateSessionIDs: []pgtype.UUID{dayThree},
		DateFrom:            dayThreeDate,
		DateTo:              dayThreeDate,
		InstituteTZ:         "Asia/Bangkok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if counts.CandidateAbsenceDays != 1 || counts.ProjectedAbsenceDays != 3 {
		t.Fatalf("new-day counts = %+v, want candidate=1 projected=3", counts)
	}

	dayTwoDate := pgtype.Date{Time: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Valid: true}
	counts, err = q.AbsenceDayCountsForCourse(ctx, AbsenceDayCountsForCourseParams{
		Wcode:       wcode,
		CourseID:    course.ID,
		DateFrom:    dayTwoDate,
		DateTo:      dayTwoDate,
		InstituteTZ: "Asia/Bangkok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if counts.CandidateAbsenceDays != 1 || counts.ProjectedAbsenceDays != 2 {
		t.Fatalf("date-range fallback counts = %+v, want candidate=1 projected=2", counts)
	}
}
