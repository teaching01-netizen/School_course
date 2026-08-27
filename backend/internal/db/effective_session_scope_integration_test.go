package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestEffectiveStudentSessionScope(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	wcode := "w-effective-" + strings.ReplaceAll(suffix, ".", "")

	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{
		Username:     "effective-teacher-" + suffix,
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{
		Name:     "effective-room-" + suffix,
		Capacity: pgtype.Int4{Int32: 20, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	student, err := q.StudentCreate(ctx, StudentCreateParams{Wcode: wcode, FullName: "Effective Scope Student"})
	if err != nil {
		t.Fatal(err)
	}
	otherStudent, err := q.StudentCreate(ctx, StudentCreateParams{Wcode: wcode + "-other", FullName: "Effective Scope Other Student"})
	if err != nil {
		t.Fatal(err)
	}
	sourceCourse, err := q.CourseCreate(ctx, CourseCreateParams{Code: "EFFECTIVE-SRC-" + suffix, Name: "Effective Source"})
	if err != nil {
		t.Fatal(err)
	}
	destA, err := q.CourseCreate(ctx, CourseCreateParams{Code: "EFFECTIVE-A-" + suffix, Name: "Effective Destination A"})
	if err != nil {
		t.Fatal(err)
	}
	destAPeer, err := q.CourseCreate(ctx, CourseCreateParams{Code: "EFFECTIVE-A2-" + suffix, Name: "Effective Destination A2"})
	if err != nil {
		t.Fatal(err)
	}
	destB, err := q.CourseCreate(ctx, CourseCreateParams{Code: "EFFECTIVE-B-" + suffix, Name: "Effective Destination B"})
	if err != nil {
		t.Fatal(err)
	}
	destBPeer, err := q.CourseCreate(ctx, CourseCreateParams{Code: "EFFECTIVE-B2-" + suffix, Name: "Effective Destination B2"})
	if err != nil {
		t.Fatal(err)
	}
	otherCourse, err := q.CourseCreate(ctx, CourseCreateParams{Code: "EFFECTIVE-OTHER-" + suffix, Name: "Effective Other"})
	if err != nil {
		t.Fatal(err)
	}
	otherTeacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{
		Username:     "effective-other-teacher-" + suffix,
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherRoom, err := q.RoomCreate(ctx, RoomCreateParams{
		Name:     "effective-other-room-" + suffix,
		Capacity: pgtype.Int4{Int32: 20, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, courseID := range []pgtype.UUID{destA.ID, destAPeer.ID, destB.ID, destBPeer.ID, otherCourse.ID} {
		if err := q.CourseStudentAdd(ctx, CourseStudentAddParams{CourseID: courseID, StudentID: student.ID}); err != nil {
			t.Fatal(err)
		}
	}

	var mergeGroupID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO course_merge_groups (name) VALUES ($1) RETURNING id
	`, "Effective Merge "+suffix).Scan(&mergeGroupID); err != nil {
		t.Fatal(err)
	}
	for position, courseID := range []pgtype.UUID{destA.ID, destAPeer.ID} {
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO course_merge_group_members (group_id, course_id, position)
			VALUES ($1, $2, $3)
		`, mergeGroupID, courseID, position+1); err != nil {
			t.Fatal(err)
		}
	}
	var mergeGroupBID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO course_merge_groups (name) VALUES ($1) RETURNING id
	`, "Effective Merge B "+suffix).Scan(&mergeGroupBID); err != nil {
		t.Fatal(err)
	}
	for position, courseID := range []pgtype.UUID{destB.ID, destBPeer.ID} {
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO course_merge_group_members (group_id, course_id, position)
			VALUES ($1, $2, $3)
		`, mergeGroupBID, courseID, position+1); err != nil {
			t.Fatal(err)
		}
	}

	var snapshotID, assignmentID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO crm_snapshots (status) VALUES ('ready') RETURNING id
	`).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO crm_cross_study_assignments (
			snapshot_id, wcode, source_course_id, dest_course_a_id,
			dest_course_b_id, assigned_course_id,
			dest_course_a_weekdays, dest_course_b_weekdays
		) VALUES ($1, $2, $3, $4, $5, $4, ARRAY[1]::smallint[], ARRAY[6]::smallint[])
		RETURNING id
	`, snapshotID, wcode, sourceCourse.ID, destA.ID, destB.ID).Scan(&assignmentID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO crm_cross_study_assignments (
			snapshot_id, wcode, source_course_id, dest_course_a_id,
			dest_course_b_id, assigned_course_id,
			dest_course_a_weekdays, dest_course_b_weekdays
		) VALUES ($1, $2, $3, $4, $5, $4, ARRAY[2]::smallint[], ARRAY[7]::smallint[])
	`, snapshotID, wcode+"-other", sourceCourse.ID, destA.ID, destB.ID); err != nil {
		t.Fatal(err)
	}

	createSession := func(courseID, sessionTeacherID, sessionRoomID pgtype.UUID, start time.Time) pgtype.UUID {
		t.Helper()
		row, createErr := q.SessionCreate(ctx, SessionCreateParams{
			CourseID:  courseID,
			RoomID:    sessionRoomID,
			TeacherID: sessionTeacherID,
			StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:     pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return row.ID
	}

	aMonday := createSession(destA.ID, teacherID, room.ID, time.Date(2026, 8, 31, 2, 0, 0, 0, time.UTC))
	aTuesday := createSession(destA.ID, teacherID, room.ID, time.Date(2026, 9, 1, 2, 0, 0, 0, time.UTC))
	aPeerMonday := createSession(destAPeer.ID, teacherID, room.ID, time.Date(2026, 9, 7, 2, 0, 0, 0, time.UTC))
	bFriday := createSession(destB.ID, teacherID, room.ID, time.Date(2026, 9, 4, 2, 0, 0, 0, time.UTC))
	bSaturday := createSession(destB.ID, teacherID, room.ID, time.Date(2026, 9, 5, 2, 0, 0, 0, time.UTC))
	bPeerSaturday := createSession(destBPeer.ID, teacherID, room.ID, time.Date(2026, 9, 5, 4, 0, 0, 0, time.UTC))

	assertExpectedForStudent := func(name string, studentID pgtype.UUID, sessionID pgtype.UUID, want bool) {
		t.Helper()
		var got bool
		if err := dbpool.QueryRow(ctx, `
			SELECT student_is_expected_at_session($1, $2)
		`, studentID, sessionID).Scan(&got); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != want {
			t.Fatalf("%s expected=%v, got=%v", name, want, got)
		}
	}
	assertExpected := func(name string, sessionID pgtype.UUID, want bool) {
		t.Helper()
		assertExpectedForStudent(name, student.ID, sessionID, want)
	}

	assertExpected("destination A Monday", aMonday, true)
	assertExpected("destination A Tuesday", aTuesday, false)
	assertExpected("merged destination A2 Monday", aPeerMonday, false)
	assertExpected("destination B Friday", bFriday, false)
	assertExpected("destination B Saturday", bSaturday, true)
	assertExpected("merged destination B2 Saturday", bPeerSaturday, false)
	assertExpectedForStudent("other student Monday assignment", otherStudent.ID, aMonday, false)
	assertExpectedForStudent("other student Tuesday assignment", otherStudent.ID, aTuesday, true)

	assertActiveBusy := func(name string, sessionID pgtype.UUID, want int) {
		t.Helper()
		var got int
		if err := dbpool.QueryRow(ctx, `
			SELECT count(*) FROM student_busy_ranges
			WHERE student_id = $1 AND session_id = $2 AND deleted_at IS NULL
		`, student.ID, sessionID).Scan(&got); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != want {
			t.Fatalf("%s active busy ranges=%d, want %d", name, got, want)
		}
	}

	assertActiveBusy("weekday-scoped Tuesday", aTuesday, 0)
	assertActiveBusy("weekday-scoped Monday", aMonday, 1)
	assertActiveBusy("merge-group Monday", aPeerMonday, 0)
	assertActiveBusy("destination B Friday", bFriday, 0)
	assertActiveBusy("destination B Saturday", bSaturday, 1)
	assertActiveBusy("merge-group B Saturday", bPeerSaturday, 0)

	if _, err := dbpool.Exec(ctx, `
		INSERT INTO session_attendance (
			session_id, student_id, status, override_source, cross_study_assignment_id
		) VALUES ($1, $2, 'included', 'cross_study', $3)
	`, aTuesday, student.ID, assignmentID); err != nil {
		t.Fatal(err)
	}
	assertExpected("stale Cross Study include", aTuesday, false)
	assertActiveBusy("stale Cross Study include", aTuesday, 0)

	if _, err := dbpool.Exec(ctx, `DELETE FROM session_attendance WHERE session_id = $1 AND student_id = $2`, aTuesday, student.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO session_attendance (session_id, student_id, status, override_source)
		VALUES ($1, $2, 'included', 'manual')
	`, aTuesday, student.ID); err != nil {
		t.Fatal(err)
	}
	assertExpected("manual include", aTuesday, true)
	assertActiveBusy("manual include", aTuesday, 1)

	if _, err := dbpool.Exec(ctx, `DELETE FROM session_attendance WHERE session_id = $1 AND student_id = $2`, aTuesday, student.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO session_attendance (session_id, student_id, status, override_source)
		VALUES ($1, $2, 'excluded', 'manual')
	`, aMonday, student.ID); err != nil {
		t.Fatal(err)
	}
	assertExpected("manual exclude", aMonday, false)
	assertActiveBusy("manual exclude", aMonday, 0)
	if _, err := dbpool.Exec(ctx, `DELETE FROM session_attendance WHERE session_id = $1 AND student_id = $2`, aMonday, student.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := dbpool.Exec(ctx, `
		UPDATE sessions
		SET start_at = '2026-09-14 02:00:00+00'::timestamptz,
			end_at = '2026-09-14 03:00:00+00'::timestamptz
		WHERE id = $1
	`, aTuesday); err != nil {
		t.Fatal(err)
	}
	assertExpected("edited Tuesday to Monday", aTuesday, true)
	assertActiveBusy("edited Tuesday to Monday", aTuesday, 1)
	if _, err := dbpool.Exec(ctx, `
		UPDATE sessions
		SET start_at = '2026-09-15 02:00:00+00'::timestamptz,
			end_at = '2026-09-15 03:00:00+00'::timestamptz
		WHERE id = $1
	`, aTuesday); err != nil {
		t.Fatal(err)
	}
	assertExpected("edited Monday to Tuesday", aTuesday, false)
	assertActiveBusy("edited Monday to Tuesday", aTuesday, 0)

	otherSession := createSession(otherCourse.ID, otherTeacherID, otherRoom.ID, time.Date(2026, 9, 15, 2, 0, 0, 0, time.UTC))
	conflicts, err := q.StudentConflictsByCourse(ctx, destA.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, conflict := range conflicts {
		if conflict.CurrentSessionID == aTuesday || conflict.ConflictingSessionID == otherSession {
			t.Fatalf("unexpected conflict involving an unselected Tuesday session: %+v", conflict)
		}
	}

	ab, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:    wcode,
		CourseID: destA.ID,
		DateFrom: pgtype.Date{Time: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:   pgtype.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	counts, err := q.AbsenceDayCountsForCourse(ctx, AbsenceDayCountsForCourseParams{
		Wcode:       wcode,
		CourseID:    destA.ID,
		DateFrom:    pgtype.Date{Time: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:      pgtype.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		InstituteTZ: "Asia/Bangkok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if counts.TotalCourseDays != 1 {
		t.Fatalf("effective course day denominator=%d, want 1: %+v", counts.TotalCourseDays, counts)
	}

	mergeCounts, err := q.AbsenceDayCountsForMergeGroup(ctx, AbsenceDayCountsForMergeGroupParams{
		Wcode:        wcode,
		MergeGroupID: mergeGroupID,
		DateFrom:     pgtype.Date{Time: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), Valid: true},
		InstituteTZ:  "Asia/Bangkok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if mergeCounts.TotalCourseDays != 1 {
		t.Fatalf("effective merge-group day denominator=%d, want 1: %+v", mergeCounts.TotalCourseDays, mergeCounts)
	}

	timingRows, err := q.ValidExpectedMissedSessionTiming(ctx, ab.ID, []pgtype.UUID{aMonday, aTuesday}, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	if len(timingRows) != 1 || timingRows[0].ID != aMonday {
		t.Fatalf("expected one effective missed session for Monday, got %+v", timingRows)
	}
}
