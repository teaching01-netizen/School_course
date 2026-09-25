package db

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBundleVisibilityIncludesDirectMappedCoursesAndKeepsVisibilityGates(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := New(pool).WithTx(tx)
	suffix := strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")

	subject, err := q.SubjectCreate(ctx, SubjectCreateParams{Code: "BUNDLE-VIS-" + suffix, Name: "Bundle Visibility " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	makeCourse := func(label string, visible bool) CourseCreateRow {
		t.Helper()
		course, createErr := q.CourseCreate(ctx, CourseCreateParams{Code: "BUNDLE-VIS-" + label + "-" + suffix, Name: "Bundle Visibility " + label + " " + suffix})
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, updateErr := tx.Exec(ctx, `UPDATE courses SET subject_id = $1, absence_form_visible = $2 WHERE id = $3`, subject.ID, visible, course.ID); updateErr != nil {
			t.Fatal(updateErr)
		}
		return course
	}
	scope := makeCourse("SCOPE", true)
	visibleTarget := makeCourse("VISIBLE", true)
	hiddenTarget := makeCourse("HIDDEN", false)
	inactiveTarget := makeCourse("INACTIVE", true)
	if _, err := tx.Exec(ctx, `INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2), ($1, $3), ($1, $4)`, subject.ID, scope.ID, visibleTarget.ID, hiddenTarget.ID); err != nil {
		t.Fatal(err)
	}

	out := &SitInBundleV2{
		RulesByID:    make(map[string]*SitInRule),
		RulesByRoot:  make(map[string]*SitInRule),
		ScopeCourses: []SubjectCourseV2{{ID: scope.ID}},
		SatMappings: []SatVerbalPolicyCourseMapping{
			{CourseID: visibleTarget.ID, Active: true},
			{CourseID: hiddenTarget.ID, Active: true},
			{CourseID: inactiveTarget.ID, Active: true},
			{CourseID: pgtype.UUID{}, Active: true},
		},
	}
	if err := q.loadBundleRulesAndVisible(ctx, out); err != nil {
		t.Fatal(err)
	}

	for _, course := range []CourseCreateRow{scope, visibleTarget} {
		if _, ok := out.Visible[uuidBytesString(course.ID)]; !ok {
			t.Errorf("visible course %s missing from bundle visibility", course.Code)
		}
	}
	for _, course := range []CourseCreateRow{hiddenTarget, inactiveTarget} {
		if _, ok := out.Visible[uuidBytesString(course.ID)]; ok {
			t.Errorf("invisible or inactive course %s passed the visibility probe", course.Code)
		}
	}
}
