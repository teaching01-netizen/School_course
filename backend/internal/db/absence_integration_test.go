package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestStudentSubjectByWCode_ActiveCourseFilter(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	wcode := "WACTIVE-" + suffix

	// Create subject
	subj, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "ACT-SUBJ-" + suffix,
		Name: "Active Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create student
	student, err := q.StudentCreate(ctx, StudentCreateParams{
		Wcode:    wcode,
		FullName: "Active Student " + suffix,
		Notes:    "",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create an active course in this subject
	activeCourse, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "ACT-CRS-" + suffix,
		Name: "Active Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Set subject
	if _, err := dbpool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subj.ID, activeCourse.ID); err != nil {
		t.Fatal(err)
	}
	// Enroll student
	if err := q.CourseStudentAdd(ctx, CourseStudentAddParams{CourseID: activeCourse.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}

	// Also create a course that will be hard-deleted from the same subject.
	deletedCourse, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "DEL-CRS-" + suffix,
		Name: "Deleted Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subj.ID, deletedCourse.ID); err != nil {
		t.Fatal(err)
	}

	t.Run("returns_only_active_course_subjects", func(t *testing.T) {
		rows, err := q.StudentSubjectByWCode(ctx, wcode)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 subject, got %d", len(rows))
		}
		if rows[0].SubjectCode != "ACT-SUBJ-"+suffix {
			t.Errorf("expected subject code ACT-SUBJ-%s, got %s", suffix, rows[0].SubjectCode)
		}
		if !rows[0].ActiveCourseID.Valid {
			t.Fatal("expected active_course_id to be set")
		}
		if rows[0].ActiveCourseID.Bytes != activeCourse.ID.Bytes {
			t.Errorf("expected active_course_id to match active course")
		}
	})

	t.Run("hard_delete_removes_enrollments_for_students_with_no_remaining_courses", func(t *testing.T) {
		// Create a second student enrolled only in the deleted course.
		noCourseWcode := "WNONE-" + suffix
		noCourseStudent, err := q.StudentCreate(ctx, StudentCreateParams{
			Wcode:    noCourseWcode,
			FullName: "No Course Student " + suffix,
			Notes:    "",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := q.CourseStudentAdd(ctx, CourseStudentAddParams{CourseID: deletedCourse.ID, StudentID: noCourseStudent.ID}); err != nil {
			t.Fatal(err)
		}
		if err := q.CourseDelete(ctx, deletedCourse.ID); err != nil {
			t.Fatal(err)
		}

		rows, err := q.StudentSubjectByWCode(ctx, noCourseWcode)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected 0 subjects for student with no active courses, got %d", len(rows))
		}
	})
}

func TestAbsenceDaysInRange_UsesSubjectAndStudentFallback(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	subj, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "ABS-SUBJ-" + suffix,
		Name: "Absence Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	course, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "ABS-COURSE-" + suffix,
		Name: "Absence Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subj.ID, course.ID); err != nil {
		t.Fatal(err)
	}

	student, err := q.StudentCreate(ctx, StudentCreateParams{
		Wcode:    "WABS-" + suffix,
		FullName: "Absence Student " + suffix,
		Notes:    "",
	})
	if err != nil {
		t.Fatal(err)
	}

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:         student.Wcode,
		CourseID:      course.ID,
		DateFrom:      pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:        pgtype.Date{Time: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:        pgtype.Text{String: "travel", Valid: true},
		SitInCourseID: pgtype.UUID{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, "UPDATE student_absences SET subject_id = $1 WHERE id = $2", subj.ID, absence.ID); err != nil {
		t.Fatal(err)
	}

	rows, err := q.AbsenceDaysInRange(
		ctx,
		time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	var row *AbsenceDayInRangeRow
	for i := range rows {
		if rows[i].Wcode == student.Wcode {
			row = &rows[i]
			break
		}
	}
	if row == nil {
		t.Fatalf("expected to find absence row for wcode %q", student.Wcode)
	}
	if !row.ID.Valid {
		t.Fatal("expected absence id to be set")
	}
	if row.Wcode != student.Wcode {
		t.Fatalf("expected wcode %q, got %q", student.Wcode, row.Wcode)
	}
	if row.Status != "pending" {
		t.Fatalf("expected status pending, got %q", row.Status)
	}
	if !row.StudentName.Valid || row.StudentName.String != student.FullName {
		t.Fatalf("expected student_name %q, got %v", student.FullName, row.StudentName)
	}
	if !row.SubjectCode.Valid || row.SubjectCode.String != subj.Code {
		t.Fatalf("expected subject_code %q, got %v", subj.Code, row.SubjectCode)
	}
	if got := row.DateFrom.Time.Format("2006-01-02"); got != "2026-06-01" {
		t.Fatalf("expected date_from 2026-06-01, got %s", got)
	}
	if got := row.DateTo.Time.Format("2006-01-02"); got != "2026-06-06" {
		t.Fatalf("expected date_to 2026-06-06, got %s", got)
	}
}

func TestCalendarQueriesExposeReadableSubjectNames(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	leaveSubj, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "CAL-LEAVE-" + suffix,
		Name: "Calendar Leave Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	leaveCourse, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "CAL-LEAVE-COURSE-" + suffix,
		Name: "Calendar Leave Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", leaveSubj.ID, leaveCourse.ID); err != nil {
		t.Fatal(err)
	}

	sitSubj, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "CAL-SIT-" + suffix,
		Name: "Calendar Sit Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	sitCourse, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "CAL-SIT-COURSE-" + suffix,
		Name: "Calendar Sit Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", sitSubj.ID, sitCourse.ID); err != nil {
		t.Fatal(err)
	}

	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "Room-" + suffix, Capacity: pgtype.Int4{Int32: 18, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	student, err := q.StudentCreate(ctx, StudentCreateParams{
		Wcode:    "WCAL-" + suffix,
		FullName: "Calendar Student " + suffix,
		Notes:    "",
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := q.SessionCreate(ctx, SessionCreateParams{
		SeriesID:  pgtype.UUID{},
		CourseID:  leaveCourse.ID,
		RoomID:    room.ID,
		TeacherID: teacherID,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 6, 2, 10, 30, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:         student.Wcode,
		CourseID:      leaveCourse.ID,
		DateFrom:      pgtype.Date{Time: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:        pgtype.Date{Time: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:        pgtype.Text{String: "travel", Valid: true},
		SitInCourseID: sitCourse.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, "UPDATE student_absences SET subject_id = $1 WHERE id = $2", leaveSubj.ID, absence.ID); err != nil {
		t.Fatal(err)
	}

	calendarSessions, err := q.CalendarSessionsInRange(ctx, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var foundSession *CalendarSessionRow
	for i := range calendarSessions {
		if calendarSessions[i].CourseID.Valid && calendarSessions[i].CourseID.Bytes == leaveCourse.ID.Bytes {
			foundSession = &calendarSessions[i]
			break
		}
	}
	if foundSession == nil {
		t.Fatalf("expected to find calendar session for course %v", leaveCourse.ID)
	}
	if !foundSession.SubjectName.Valid || foundSession.SubjectName.String != leaveSubj.Name {
		t.Fatalf("expected subject_name %q, got %v", leaveSubj.Name, foundSession.SubjectName)
	}

	calendarAbsences, err := q.AbsenceDaysInRange(ctx, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var foundAbsence *AbsenceDayInRangeRow
	for i := range calendarAbsences {
		if calendarAbsences[i].Wcode == student.Wcode {
			foundAbsence = &calendarAbsences[i]
			break
		}
	}
	if foundAbsence == nil {
		t.Fatalf("expected to find calendar absence for wcode %q", student.Wcode)
	}
	if !foundAbsence.SubjectName.Valid || foundAbsence.SubjectName.String != leaveSubj.Name {
		t.Fatalf("expected subject_name %q, got %v", leaveSubj.Name, foundAbsence.SubjectName)
	}
	if !foundAbsence.SitInCourseName.Valid || foundAbsence.SitInCourseName.String != sitCourse.Name {
		t.Fatalf("expected sit_in_course_name %q, got %v", sitCourse.Name, foundAbsence.SitInCourseName)
	}
	if !foundAbsence.SitInSubjectName.Valid || foundAbsence.SitInSubjectName.String != sitSubj.Name {
		t.Fatalf("expected sit_in_subject_name %q, got %v", sitSubj.Name, foundAbsence.SitInSubjectName)
	}
	if !foundAbsence.StudentName.Valid || foundAbsence.StudentName.String != student.FullName {
		t.Fatalf("expected student_name %q, got %v", student.FullName, foundAbsence.StudentName)
	}
	if !session.ID.Valid {
		t.Fatal("expected session id to be set")
	}
}
