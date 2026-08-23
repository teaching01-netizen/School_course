package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// courseTestSuffix makes codes unique per test binary run so the unique code
// constraint never collides across repeated runs on one database.
func courseTestSuffix() string {
	return time.Now().UTC().Format("20060102150405.000000000")
}

func setCourseArchived(t *testing.T, dbpool *pgxpool.Pool, id pgtype.UUID, archived bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := dbpool.Exec(ctx, `UPDATE courses SET legacy_archived = $2 WHERE id = $1`, id, archived)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCourseOverview_LiveVsArchived(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := courseTestSuffix()
	liveCourse, err := q.CourseCreate(ctx, CourseCreateParams{Code: "LIVE-" + suffix, Name: "Live course"})
	if err != nil {
		t.Fatal(err)
	}
	archivedCourse, err := q.CourseCreate(ctx, CourseCreateParams{Code: "ARCH-" + suffix, Name: "Archived course"})
	if err != nil {
		t.Fatal(err)
	}
	setCourseArchived(t, dbpool, archivedCourse.ID, true)

	// Default (live) view must exclude archived courses.
	liveItems, err := q.CourseOverview(ctx, CourseOverviewParams{Archived: false})
	if err != nil {
		t.Fatal(err)
	}
	if findCourseOverviewRow(liveItems, archivedCourse.ID) != nil {
		t.Fatalf("archived course must not appear in live view")
	}
	if findCourseOverviewRow(liveItems, liveCourse.ID) == nil {
		t.Fatalf("live course missing from live view")
	}

	// Archived view must only return archived courses.
	archivedItems, err := q.CourseOverview(ctx, CourseOverviewParams{Archived: true})
	if err != nil {
		t.Fatal(err)
	}
	if findCourseOverviewRow(archivedItems, liveCourse.ID) != nil {
		t.Fatalf("live course must not appear in archived view")
	}
	if findCourseOverviewRow(archivedItems, archivedCourse.ID) == nil {
		t.Fatalf("archived course missing from archived view")
	}
}

func TestCourseOverview_TypeFilter(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := courseTestSuffix()
	privateCourse, _ := q.CourseCreate(ctx, CourseCreateParams{Code: "TPRIV-" + suffix, Name: "Private type"})
	generalCourse, _ := q.CourseCreate(ctx, CourseCreateParams{Code: "TGEN-" + suffix, Name: "General type"})
	groupCourse, _ := q.CourseCreate(ctx, CourseCreateParams{Code: "TGRP-" + suffix, Name: "Group type"})
	typedNull, _ := q.CourseCreate(ctx, CourseCreateParams{Code: "TNULL-" + suffix, Name: "No type"})
	for _, spec := range []struct {
		id  pgtype.UUID
		typ *string
	}{
		{privateCourse.ID, strPtr("Private")},
		{generalCourse.ID, strPtr("General")},
		{groupCourse.ID, strPtr("Group")},
		{typedNull.ID, nil},
	} {
		_, err := dbpool.Exec(ctx, `UPDATE courses SET course_type = $2 WHERE id = $1`, spec.id, spec.typ)
		if err != nil {
			t.Fatal(err)
		}
	}

	privateItems, err := q.CourseOverview(ctx, CourseOverviewParams{Archived: false, CourseType: "private"})
	if err != nil {
		t.Fatal(err)
	}
	if findCourseOverviewRow(privateItems, privateCourse.ID) == nil {
		t.Fatalf("private filter must include Private course")
	}
	if findCourseOverviewRow(privateItems, generalCourse.ID) != nil || findCourseOverviewRow(privateItems, groupCourse.ID) != nil || findCourseOverviewRow(privateItems, typedNull.ID) != nil {
		t.Fatalf("private filter must exclude General/Group/untyped courses")
	}

	// The legacy site writes 'General'; the native app writes 'Group'. Both are
	// the same user-facing "general" bucket.
	generalItems, err := q.CourseOverview(ctx, CourseOverviewParams{Archived: false, CourseType: "general"})
	if err != nil {
		t.Fatal(err)
	}
	if findCourseOverviewRow(generalItems, generalCourse.ID) == nil || findCourseOverviewRow(generalItems, groupCourse.ID) == nil {
		t.Fatalf("general filter must include General and Group courses")
	}
	if findCourseOverviewRow(generalItems, privateCourse.ID) != nil || findCourseOverviewRow(generalItems, typedNull.ID) != nil {
		t.Fatalf("general filter must exclude Private/untyped courses")
	}
}

func TestCourseOverview_Search(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := courseTestSuffix()
	uid := uuid.New().String()[:8]
	teacher, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "tsearch-" + uid, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `UPDATE users SET full_name = 'Searchable Teacher' WHERE id = $1`, teacher); err != nil {
		t.Fatal(err)
	}
	subject, err := q.SubjectCreate(ctx, SubjectCreateParams{Code: "S" + uid, Name: "Searchable Subject"})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "SEARCHCODE-" + suffix, Name: "Unique Searchable Name"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `UPDATE courses SET teacher_id = $2, subject_id = $3 WHERE id = $1`, course.ID, teacher, subject.ID); err != nil {
		t.Fatal(err)
	}

	// A second teacher only in the course_teachers set (non-primary): searching
	// by them must still surface the course.
	member, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "tmember-" + uuid.New().String()[:8], Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `UPDATE users SET full_name = 'Roster Member Teacher' WHERE id = $1`, member); err != nil {
		t.Fatal(err)
	}
	if err := q.CourseTeacherInsert(ctx, CourseTeacherInsertParams{CourseID: course.ID, TeacherID: member, IsPrimary: false}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		query string
	}{
		{"code", "SEARCHCODE"},
		{"name", "Unique Searchable"},
		{"course number", suffix},
		{"course id", course.ID.String()[:8]},
		{"subject code", subject.Code},
		{"subject name", "Searchable Subject"},
		{"primary teacher username", "tsearch-" + uid},
		{"primary teacher full name", "Searchable Teacher"},
		{"set member username", "tmember-"},
		{"set member full name", "Roster Member Teacher"},
	}
	for _, tc := range cases {
		items, err := q.CourseOverview(ctx, CourseOverviewParams{Archived: false, Q: tc.query})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if findCourseOverviewRow(items, course.ID) == nil {
			t.Fatalf("search %q (%s) must match the course", tc.query, tc.name)
		}
	}

	// A search term in neither field must not match.
	items, err := q.CourseOverview(ctx, CourseOverviewParams{Archived: false, Q: "zzz-no-such-term-" + suffix})
	if err != nil {
		t.Fatal(err)
	}
	if findCourseOverviewRow(items, course.ID) != nil {
		t.Fatalf("unrelated search must not match the course")
	}
}

func TestCourseOverview_TeacherFilter(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := courseTestSuffix()
	teacher, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "tfilter-" + uuid.New().String()[:8], Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	withTeacher, _ := q.CourseCreate(ctx, CourseCreateParams{Code: "WT-" + suffix, Name: "With teacher"})
	withoutTeacher, _ := q.CourseCreate(ctx, CourseCreateParams{Code: "NT-" + suffix, Name: "No teacher"})
	if _, err := dbpool.Exec(ctx, `UPDATE courses SET teacher_id = $2 WHERE id = $1`, withTeacher.ID, teacher); err != nil {
		t.Fatal(err)
	}

	// Primary-teacher uuid filter.
	items, err := q.CourseOverview(ctx, CourseOverviewParams{Archived: false, TeacherID: teacher.String()})
	if err != nil {
		t.Fatal(err)
	}
	if findCourseOverviewRow(items, withTeacher.ID) == nil || findCourseOverviewRow(items, withoutTeacher.ID) != nil {
		t.Fatalf("teacher uuid filter must match only the primary-teacher course")
	}

	// The "none" sentinel returns courses with no primary teacher.
	noneItems, err := q.CourseOverview(ctx, CourseOverviewParams{Archived: false, TeacherID: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if findCourseOverviewRow(noneItems, withoutTeacher.ID) == nil || findCourseOverviewRow(noneItems, withTeacher.ID) != nil {
		t.Fatalf("teacher=none filter must return only teacher-less courses")
	}
}

func TestCourseOverview_PaginationAndCount(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := courseTestSuffix()
	for i := 0; i < 5; i++ {
		course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "PG-" + suffix + "-" + string(rune('A'+i)), Name: "Page course"})
		if err != nil {
			t.Fatal(err)
		}
		_ = course
	}

	// Scope the assertions to this run's five rows with the shared Q suffix
	// (the scratch DB accumulates courses from the whole package).
	total, err := q.CourseOverviewCount(ctx, CourseOverviewParams{Archived: false, Q: suffix})
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Fatalf("expected count 5 for the scoped rows, got %d", total)
	}
	page1, err := q.CourseOverview(ctx, CourseOverviewParams{Archived: false, Q: suffix, Limit: 2, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 rows on page 1, got %d", len(page1))
	}
	// course_no DESC: newest created courses are the highest course_no.
	if page1[0].CourseNo < page1[1].CourseNo {
		t.Fatalf("expected DESC order, got %d then %d", page1[0].CourseNo, page1[1].CourseNo)
	}
	lastPage, err := q.CourseOverview(ctx, CourseOverviewParams{Archived: false, Q: suffix, Limit: 2, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(lastPage) != 1 {
		t.Fatalf("expected 1 row on the last page, got %d", len(lastPage))
	}
	bareItems, err := q.CourseOverview(ctx, CourseOverviewParams{Archived: false, Q: suffix})
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(bareItems)) != total {
		t.Fatalf("bare list length %d must equal count %d", len(bareItems), total)
	}
}

func TestCourseOverview_SessionTimeFilter(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := courseTestSuffix()
	matching, err := q.CourseCreate(ctx, CourseCreateParams{Code: "TIME-MATCH-" + suffix, Name: "Matching session"})
	if err != nil {
		t.Fatal(err)
	}
	outside, err := q.CourseCreate(ctx, CourseCreateParams{Code: "TIME-OUTSIDE-" + suffix, Name: "Outside session"})
	if err != nil {
		t.Fatal(err)
	}
	crossing, err := q.CourseCreate(ctx, CourseCreateParams{Code: "TIME-CROSSING-" + suffix, Name: "Crossing session"})
	if err != nil {
		t.Fatal(err)
	}

	createSession := func(courseID pgtype.UUID, startHour, endHour int) {
		t.Helper()
		_, err := q.SessionCreate(ctx, SessionCreateParams{
			CourseID: courseID,
			StartAt:  pgtype.Timestamptz{Time: time.Date(2026, 6, 1, startHour, 0, 0, 0, time.UTC), Valid: true},
			EndAt:    pgtype.Timestamptz{Time: time.Date(2026, 6, 1, endHour, 0, 0, 0, time.UTC), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	createSession(matching.ID, 2, 4)
	createSession(outside.ID, 6, 8)
	createSession(crossing.ID, 1, 5)

	params := CourseOverviewParams{
		Archived:    false,
		Q:           suffix,
		SessionFrom: "09:00",
		SessionTo:   "11:00",
		InstituteTZ: "Asia/Bangkok",
	}
	items, err := q.CourseOverview(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if findCourseOverviewRow(items, matching.ID) == nil {
		t.Fatalf("matching session course missing from filtered overview")
	}
	if findCourseOverviewRow(items, outside.ID) != nil || findCourseOverviewRow(items, crossing.ID) != nil {
		t.Fatalf("time filter must require a fully contained matching session")
	}
	total, err := q.CourseOverviewCount(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("filtered count = %d, want 1", total)
	}
}

func TestCourseOverview_AllowsNullTeacherAndSubject(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Legacy course rows created via CourseCreate intentionally leave teacher_id and subject_id NULL.
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "NULLJOIN-" + courseTestSuffix(), Name: ""})
	if err != nil {
		t.Fatal(err)
	}

	items, err := q.CourseOverview(ctx, CourseOverviewParams{Archived: false})
	if err != nil {
		t.Fatal(err)
	}

	var found *CourseOverviewRow
	for i := range items {
		if items[i].ID == course.ID {
			found = &items[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected course %v in overview", course.ID)
	}
	if found.TeacherName != "" {
		t.Fatalf("expected TeacherName empty string for NULL teacher_id, got %q", found.TeacherName)
	}
	if found.SubjectCode != "" || found.SubjectName != "" {
		t.Fatalf("expected SubjectCode/SubjectName empty strings for NULL subject_id, got %q / %q", found.SubjectCode, found.SubjectName)
	}
}

func TestCourseOverview_StudentCountUsesEnrolledRoster(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := courseTestSuffix()
	course, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "COUNT-" + suffix,
		Name: "Roster Count Test",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = dbpool.Exec(ctx, `UPDATE courses SET student_count = 100 WHERE id = $1`, course.ID)
	if err != nil {
		t.Fatal(err)
	}

	items, err := q.CourseOverview(ctx, CourseOverviewParams{Archived: false})
	if err != nil {
		t.Fatal(err)
	}

	found := findCourseOverviewRow(items, course.ID)
	if found == nil {
		t.Fatalf("expected course %v in overview", course.ID)
	}
	if !found.StudentCount.Valid || found.StudentCount.Int32 != 0 {
		t.Fatalf("expected empty roster to report student_count 0, got valid=%v value=%d", found.StudentCount.Valid, found.StudentCount.Int32)
	}

	student, err := q.StudentCreate(ctx, StudentCreateParams{
		Wcode:    "WCOUNT-" + suffix,
		FullName: "Count Student",
		Notes:    "",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = q.CourseStudentAdd(ctx, CourseStudentAddParams{
		CourseID:  course.ID,
		StudentID: student.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	items, err = q.CourseOverview(ctx, CourseOverviewParams{Archived: false})
	if err != nil {
		t.Fatal(err)
	}
	found = findCourseOverviewRow(items, course.ID)
	if found == nil {
		t.Fatalf("expected course %v in overview after adding student", course.ID)
	}
	if !found.StudentCount.Valid || found.StudentCount.Int32 != 1 {
		t.Fatalf("expected enrolled roster to report student_count 1, got valid=%v value=%d", found.StudentCount.Valid, found.StudentCount.Int32)
	}
}

func strPtr(s string) *string { return &s }

func findCourseOverviewRow(items []CourseOverviewRow, id pgtype.UUID) *CourseOverviewRow {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}
