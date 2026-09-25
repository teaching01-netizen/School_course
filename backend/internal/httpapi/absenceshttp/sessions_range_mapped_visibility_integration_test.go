package absenceshttp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/satverbalpolicy"
)

func TestSessionsRangeBundleMappedVisibilityAllowsOutOfScopeRank4Target(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqldb.New(pool).WithTx(tx)
	suffix := strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "sat-visibility-" + suffix,
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	subject, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{
		Code: "SAT-VIS-" + suffix,
		Name: "SAT Verbal Reading",
	})
	if err != nil {
		t.Fatal(err)
	}
	var rootID pgtype.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO root_course_groups (name) VALUES ($1) RETURNING id`, "SAT visibility root "+suffix).Scan(&rootID); err != nil {
		t.Fatal(err)
	}
	studentWcode := "wsv" + suffix
	student, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{Wcode: studentWcode, FullName: "SAT Visibility Student"})
	if err != nil {
		t.Fatal(err)
	}

	type testCourse struct {
		row    sqldb.CourseCreateRow
		active bool
	}
	makeCourse := func(code, name string, visible, active, inRoot bool) testCourse {
		t.Helper()
		course, createErr := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: code + "-" + suffix, Name: name})
		if createErr != nil {
			t.Fatal(createErr)
		}
		courseRoot := pgtype.UUID{}
		if inRoot {
			courseRoot = rootID
		}
		if _, updateErr := tx.Exec(ctx, `UPDATE courses SET subject_id = $1, root_course_group_id = $2, absence_form_visible = $3 WHERE id = $4`, subject.ID, courseRoot, visible, course.ID); updateErr != nil {
			t.Fatal(updateErr)
		}
		return testCourse{row: course, active: active}
	}
	source := makeCourse("SAT-VIS-SOURCE", "SAT Verbal Rank 3 Section 2 C3", true, true, true)
	section1 := makeCourse("SAT-VIS-SECTION1", "SAT Verbal Rank 3 Section 1 C3", true, true, false)
	rank4 := makeCourse("SAT-VIS-RANK4", "SAT Verbal Reading Rank 4", true, true, false)
	hiddenRank4 := makeCourse("SAT-VIS-HIDDEN-RANK4", "SAT Verbal Reading Rank 4", false, true, false)
	inactiveRank4 := makeCourse("SAT-VIS-INACTIVE-RANK4", "SAT Verbal Reading Rank 4", true, false, false)
	for _, course := range []testCourse{source, section1, rank4, hiddenRank4} {
		if !course.active {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, subject.ID, course.row.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: source.row.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}

	createSession := func(courseID pgtype.UUID, start time.Time) pgtype.UUID {
		t.Helper()
		session, createErr := q.SessionCreate(ctx, sqldb.SessionCreateParams{
			CourseID:  courseID,
			TeacherID: teacherID,
			StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:     pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return session.ID
	}
	utc := func(month time.Month, day, hour int) time.Time {
		return time.Date(2026, month, day, hour, 0, 0, 0, time.UTC)
	}
	missedSessionID := createSession(source.row.ID, utc(time.September, 26, 2))
	section1SessionID := createSession(section1.row.ID, utc(time.September, 27, 2))
	createSession(section1.row.ID, utc(time.October, 4, 2))
	rank4SessionID := createSession(rank4.row.ID, utc(time.September, 28, 2))
	createSession(rank4.row.ID, utc(time.October, 5, 2))
	createSession(hiddenRank4.row.ID, utc(time.September, 29, 2))
	createSession(hiddenRank4.row.ID, utc(time.October, 6, 2))
	createSession(inactiveRank4.row.ID, utc(time.September, 30, 2))
	createSession(inactiveRank4.row.ID, utc(time.October, 7, 2))

	sourceRule := satverbalpolicy.CourseRule{
		ID:                "sat-vis-source-" + suffix,
		CourseName:        "SAT Verbal Rank 3-Section 2",
		LastClassExcluded: true,
		Priorities: []satverbalpolicy.RulePriority{
			{Level: 1, RuleType: "cross_section", Label: "Another Rank 3 section", MakeupTargets: []satverbalpolicy.Target{{Section: "Section 1", Subject: "Reading"}}},
			{Level: 3, RuleType: "rank_chain", Label: "Rank 4 Reading", EligibleTargets: []string{"SAT Verbal Reading Rank 4"}},
		},
	}
	section1Rule := satverbalpolicy.CourseRule{ID: "sat-vis-section1-" + suffix, CourseName: "SAT Verbal Rank 3-Section 1"}
	insertMapping := func(course testCourse, rule satverbalpolicy.CourseRule) {
		t.Helper()
		raw, marshalErr := json.Marshal(rule)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, insertErr := tx.Exec(ctx, `
			INSERT INTO sat_verbal_policy_mappings (rule_id, course_id, merge_group_id, policy_rule, policy_hash, active)
			VALUES ($1, $2, NULL, $3::jsonb, $4, true)
		`, rule.ID, course.row.ID, string(raw), satverbalpolicy.HashPolicy(raw)); insertErr != nil {
			t.Fatal(insertErr)
		}
	}
	insertMapping(source, sourceRule)
	insertMapping(section1, section1Rule)
	for index, course := range []testCourse{rank4, hiddenRank4, inactiveRank4} {
		insertMapping(course, satverbalpolicy.CourseRule{
			ID:         fmt.Sprintf("sat-vis-rank4-%d-%s", index, suffix),
			CourseName: "SAT Verbal Reading Rank 4",
		})
	}

	now := time.Date(2026, time.September, 24, 17, 0, 0, 0, time.UTC) // Sep 25 in Bangkok.
	windowFrom := time.Date(2026, time.September, 25, 17, 0, 0, 0, time.UTC)
	windowTo := time.Date(2026, time.September, 26, 17, 0, 0, 0, time.UTC)
	bundle, err := q.SessionsRangeSitInBundleV2(ctx, sqldb.SitInBundleV2Params{
		StudentID:       student.ID,
		MissedCourseIDs: []pgtype.UUID{source.row.ID},
		PoliciesJSON:    []byte(`{}`),
		NowUTC:          now,
		Discovery: sqldb.SitInDiscoveryBounds{
			WindowFromUTC:   windowFrom,
			WindowToExclUTC: windowTo,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.ResolveFailed {
		t.Fatal("bundle load marked the sit-in resolve failed")
	}

	containsCourse := func(courses []sqldb.SubjectCourseV2, id pgtype.UUID) bool {
		for _, course := range courses {
			if course.ID == id {
				return true
			}
		}
		return false
	}
	if containsCourse(bundle.ScopeCourses, rank4.row.ID) {
		t.Fatal("directly mapped Rank 4 target unexpectedly belongs to the student's root scope")
	}
	for label, course := range map[string]testCourse{"visible": rank4, "hidden": hiddenRank4, "inactive": inactiveRank4} {
		id := uuidStringOrZero(course.row.ID)
		if containsCourse(bundle.ScopeCourses, course.row.ID) {
			t.Errorf("%s Rank 4 target unexpectedly belongs to scope", label)
		}
		foundMapping := false
		for _, mapping := range bundle.SatMappings {
			if mapping.CourseID == course.row.ID {
				foundMapping = true
				break
			}
		}
		if !foundMapping {
			t.Errorf("%s Rank 4 target missing from active SAT mappings", label)
		}
		_, isVisible := bundle.Visible[id]
		if isVisible != (label == "visible") {
			t.Errorf("%s Rank 4 target visible = %t, want %t", label, isVisible, label == "visible")
		}
		if len(bundle.Sessions[id]) == 0 {
			t.Errorf("%s mapped Rank 4 target has no loaded sessions", label)
		}
	}
	foundMissedSession := false
	for _, session := range bundle.Sessions[uuidStringOrZero(source.row.ID)] {
		if session.ID == missedSessionID {
			foundMissedSession = true
			break
		}
	}
	if !foundMissedSession {
		t.Fatal("missed Rank 3 session was not loaded into the bundle")
	}

	resolve := func(afterPriority int) *SitInResult {
		t.Helper()
		result, resolveErr := resolveSitInForCourseFromBundle(bundleSitInInputs{
			bundle:        bundle,
			policies:      []byte(`{}`),
			instituteTZ:   "Asia/Bangkok",
			afterPriority: afterPriority,
			studentFacing: true,
			now:           now,
		}, studentWcode, student.ID, source.row.ID, subject.ID, windowFrom, windowTo)
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		if result == nil {
			t.Fatal("mapped SAT policy returned no result")
		}
		return result
	}
	initial := resolve(0)
	if initial.CurrentPriorityLevel != 1 || !initial.HasNextPriority {
		t.Fatalf("Sep 25 initial priority = level %d, hasNext=%t; want level 1 with See other times", initial.CurrentPriorityLevel, initial.HasNextPriority)
	}
	foundInitialTarget := false
	section1ID := uuidStringOrZero(section1.row.ID)
	section1CandidateID := uuidStringOrZero(section1SessionID)
	for _, priority := range initial.Priorities {
		if priority.SitInCourse == nil || priority.SitInCourse.ID != section1ID {
			continue
		}
		for _, candidate := range priority.Available {
			if candidate.ID == section1CandidateID {
				foundInitialTarget = true
			}
		}
	}
	if !foundInitialTarget {
		t.Fatalf("initial Rank 3 cross-section priority did not offer its mapped session: %#v", initial.Priorities)
	}
	next := resolve(1) // See other times advances past the first priority.
	if next.CurrentPriorityLevel != 3 || next.HasNextPriority {
		t.Fatalf("next priority = level %d, hasNext=%t; want mapped Rank 4 level 3", next.CurrentPriorityLevel, next.HasNextPriority)
	}

	wantCourseID := uuidStringOrZero(rank4.row.ID)
	wantSessionID := uuidStringOrZero(rank4SessionID)
	hiddenCourseID := uuidStringOrZero(hiddenRank4.row.ID)
	inactiveCourseID := uuidStringOrZero(inactiveRank4.row.ID)
	foundSelectableTarget := false
	for _, priority := range next.Priorities {
		if priority.SitInCourse == nil {
			continue
		}
		if priority.SitInCourse.ID == hiddenCourseID || priority.SitInCourse.ID == inactiveCourseID {
			t.Errorf("invisible or inactive Rank 4 target was offered: %#v", priority.SitInCourse)
		}
		if priority.SitInCourse.ID != wantCourseID {
			continue
		}
		for _, candidate := range priority.Available {
			if candidate.ID == wantSessionID {
				foundSelectableTarget = true
			}
		}
	}
	if !foundSelectableTarget {
		t.Fatalf("Sep 26 missed session did not reveal selectable Sep 28 mapped Rank 4 session %s: %#v", wantSessionID, next.Priorities)
	}
}
