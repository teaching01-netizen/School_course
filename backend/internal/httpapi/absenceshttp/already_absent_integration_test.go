package absenceshttp

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func TestAlreadyAbsentUsesExplicitSessionsAndLegacyFallback(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	pool := newPoolPending(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := uuid.NewString()[:8]
	teacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username: "absent-" + suffix, Role: "Teacher", PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	courseA, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "ABS-A-" + suffix, Name: "Absence A"})
	if err != nil {
		t.Fatal(err)
	}
	courseB, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "ABS-B-" + suffix, Name: "Absence B"})
	if err != nil {
		t.Fatal(err)
	}
	group, err := q.CourseMergeGroupCreate(ctx, "Absence group "+suffix, teacher)
	if err != nil {
		t.Fatal(err)
	}
	for i, course := range []pgtype.UUID{courseA.ID, courseB.ID} {
		if err := q.CourseMergeGroupAssignCourse(ctx, group.ID, course, int16(i+1)); err != nil {
			t.Fatal(err)
		}
	}

	sessions := make(map[string]pgtype.UUID)
	for _, item := range []struct {
		day    string
		course pgtype.UUID
	}{
		{"2026-09-26", courseA.ID}, {"2026-09-27", courseA.ID},
		{"2026-10-08", courseA.ID}, {"2026-10-10", courseB.ID},
		{"2026-10-13", courseA.ID}, {"2026-10-17", courseB.ID},
		{"2026-10-28", courseA.ID}, {"2026-10-31", courseB.ID},
	} {
		start, err := time.Parse(time.DateOnly, item.day)
		if err != nil {
			t.Fatal(err)
		}
		start = start.Add(2 * time.Hour) // 09:00 in Asia/Bangkok
		session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
			CourseID: item.course, TeacherID: teacher,
			StartAt: pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:   pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		sessions[item.day] = session.ID
	}
	date := func(day string) pgtype.Date {
		t.Helper()
		value, err := time.Parse(time.DateOnly, day)
		if err != nil {
			t.Fatal(err)
		}
		return pgtype.Date{Time: value, Valid: true}
	}
	createAbsence := func(wcode, from, to, status string, missed []pgtype.UUID) {
		t.Helper()
		row, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
			Wcode: wcode, CourseID: courseA.ID, DateFrom: date(from), DateTo: date(to),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE student_absences SET merge_group_id = $2, status = $3 WHERE id = $1`, row.ID, group.ID, status); err != nil {
			t.Fatal(err)
		}
		if len(missed) > 0 {
			if err := q.AbsenceMissedSessionsCreate(ctx, row.ID, missed); err != nil {
				t.Fatal(err)
			}
		}
	}
	modern := "modern-" + suffix
	legacy := "legacy-" + suffix
	cancelled := "cancelled-" + suffix
	createAbsence(modern, "2026-09-26", "2026-10-31", "pending",
		[]pgtype.UUID{sessions["2026-09-26"], sessions["2026-09-27"], sessions["2026-10-31"]})
	createAbsence(legacy, "2026-10-13", "2026-10-17", "pending", nil)
	createAbsence(cancelled, "2026-09-26", "2026-10-31", "cancelled",
		[]pgtype.UUID{sessions["2026-09-26"], sessions["2026-10-31"]})

	from := time.Date(2026, 9, 25, 17, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 31, 17, 0, 0, 0, time.UTC)
	cases := []struct {
		wcode string
		days  []string
	}{
		{modern, []string{"2026-09-26", "2026-09-27", "2026-10-31"}},
		{legacy, []string{"2026-10-13", "2026-10-17"}},
		{cancelled, nil},
	}
	for _, tc := range cases {
		want := make(map[string]bool)
		for _, day := range tc.days {
			id, err := sUUIDString(sessions[day])
			if err != nil {
				t.Fatal(err)
			}
			want[id] = true
		}
		batched, err := q.SessionsRangeAlreadyAbsent(ctx, sqldb.SessionsRangeAbsentParams{
			Wcode: tc.wcode, InstituteTZ: "Asia/Bangkok", FromUTC: from, ToExclusiveUTC: to,
		})
		if err != nil {
			t.Fatal(err)
		}
		legacyRows, err := pool.Query(ctx, sessionsAlreadyAbsentSelectSQL(), tc.wcode, "Asia/Bangkok", from, to)
		if err != nil {
			t.Fatal(err)
		}
		legacyResult := make(map[string]bool)
		for legacyRows.Next() {
			var id pgtype.UUID
			if err := legacyRows.Scan(&id); err != nil {
				legacyRows.Close()
				t.Fatal(err)
			}
			key, err := sUUIDString(id)
			if err != nil {
				legacyRows.Close()
				t.Fatal(err)
			}
			legacyResult[key] = true
		}
		err = legacyRows.Err()
		legacyRows.Close()
		if err != nil {
			t.Fatal(err)
		}
		for name, got := range map[string]map[string]bool{"batched": batched, "legacy endpoint": legacyResult} {
			if len(got) != len(want) {
				t.Errorf("%s %s: got %v, want %v", tc.wcode, name, got, want)
			}
			for day, id := range sessions {
				key, err := sUUIDString(id)
				if err != nil {
					t.Fatal(err)
				}
				if got[key] != want[key] {
					t.Errorf("%s %s %s: already_absent=%v, want %v", tc.wcode, name, day, got[key], want[key])
				}
			}
		}
	}
}
