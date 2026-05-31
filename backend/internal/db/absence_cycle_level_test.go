package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCourseCycleLevel(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	subj, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "SUBJ-CL-" + suffix,
		Name: "Subject CL " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	cycle, err := q.CrmCycleUpsert(ctx, CrmCycleUpsertParams{
		ID:    "cy2026a-" + suffix,
		Label: "Cycle 2026 A " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	c1, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "C1-CL-" + suffix,
		Name: "Course 1 CL " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	c2, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "C2-CL-" + suffix,
		Name: "Course 2 CL " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	c3, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "C3-CL-" + suffix,
		Name: "Course 3 CL " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []pgtype.UUID{c1.ID, c2.ID, c3.ID} {
		_, err = dbpool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subj.ID, c)
		if err != nil {
			t.Fatal(err)
		}
	}

	t.Run("MigrationColumnsExist", func(t *testing.T) {
		rows, err := dbpool.Query(ctx, `
			SELECT column_name
			FROM information_schema.columns
			WHERE table_name = 'courses'
			  AND column_name IN ('cycle_id', 'level', 'course_level', 'level_order')
			ORDER BY column_name
		`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var found []string
		for rows.Next() {
			var col string
			if err := rows.Scan(&col); err != nil {
				t.Fatal(err)
			}
			found = append(found, col)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}

		if !contains(found, "cycle_id") {
			t.Error("expected column cycle_id to exist")
		}
		if !contains(found, "level") {
			t.Error("expected column level to exist")
		}
		if contains(found, "course_level") {
			t.Error("expected column course_level to be removed")
		}
		if contains(found, "level_order") {
			t.Error("expected column level_order to be removed")
		}
	})

	t.Run("SetLevelAndCycle", func(t *testing.T) {
		err := q.CourseLevelUpdateV2(ctx, c1.ID,
			pgtype.Text{String: cycle.ID, Valid: true},
			pgtype.Int2{Int16: 1, Valid: true},
		)
		if err != nil {
			t.Fatal(err)
		}

		list, err := q.CourseLevelsListV2(ctx)
		if err != nil {
			t.Fatal(err)
		}

		var found *CourseLevelRowV2
		for i := range list {
			if list[i].ID == c1.ID {
				found = &list[i]
				break
			}
		}
		if found == nil {
			t.Fatal("course not found in CourseLevelsListV2")
		}
		if found.CycleID.String != cycle.ID || !found.CycleID.Valid {
			t.Errorf("expected cycle_id %s, got %v", cycle.ID, found.CycleID)
		}
		if !found.CycleLabel.Valid {
			t.Errorf("expected cycle_label to be set")
		} else if found.CycleLabel.String != cycle.Label {
			t.Errorf("expected cycle_label %s, got %s", cycle.Label, found.CycleLabel.String)
		}
		if found.Level.Int16 != 1 || !found.Level.Valid {
			t.Errorf("expected level 1, got %v", found.Level)
		}
	})

	t.Run("UniqueConstraintEnforced", func(t *testing.T) {
		err := q.CourseLevelUpdateV2(ctx, c2.ID,
			pgtype.Text{String: cycle.ID, Valid: true},
			pgtype.Int2{Int16: 1, Valid: true},
		)
		if err == nil {
			t.Fatal("expected unique constraint violation, got nil")
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code != "23505" {
				t.Errorf("expected error code 23505, got %s", pgErr.Code)
			}
		} else {
			t.Fatalf("expected unique constraint violation (PgError), got: %T %v", err, err)
		}
	})

	t.Run("CoursesBySubjectAndCycle", func(t *testing.T) {
		// Set c3 to same subject+cycle but level 2
		err := q.CourseLevelUpdateV2(ctx, c3.ID,
			pgtype.Text{String: cycle.ID, Valid: true},
			pgtype.Int2{Int16: 2, Valid: true},
		)
		if err != nil {
			t.Fatal(err)
		}

		courses, err := q.CoursesBySubjectAndCycle(ctx, subj.ID,
			pgtype.Text{String: cycle.ID, Valid: true},
		)
		if err != nil {
			t.Fatal(err)
		}

		if len(courses) != 2 {
			t.Fatalf("expected 2 courses, got %d", len(courses))
		}

		// Should be ordered by level ASC
		if courses[0].Level.Int16 != 1 {
			t.Errorf("expected first course level 1, got %d", courses[0].Level.Int16)
		}
		if courses[1].Level.Int16 != 2 {
			t.Errorf("expected second course level 2, got %d", courses[1].Level.Int16)
		}
	})

	t.Run("SubjectAndCycleFromCourse", func(t *testing.T) {
		result, err := q.SubjectAndCycleFromCourse(ctx, c1.ID)
		if err != nil {
			t.Fatal(err)
		}
		if result.SubjectID != subj.ID {
			t.Error("subject_id mismatch")
		}
		if result.CycleID.String != cycle.ID || !result.CycleID.Valid {
			t.Errorf("expected cycle_id %s, got %v", cycle.ID, result.CycleID)
		}
	})

	t.Run("StudentEnrolledCoursesBySubjectV2", func(t *testing.T) {
		student, err := q.StudentCreate(ctx, StudentCreateParams{
			Wcode:    "WCODE-CL-" + suffix,
			FullName: "Student CL " + suffix,
			Notes:    "",
		})
		if err != nil {
			t.Fatal(err)
		}

		err = q.CourseStudentAdd(ctx, CourseStudentAddParams{
			CourseID:  c1.ID,
			StudentID: student.ID,
		})
		if err != nil {
			t.Fatal(err)
		}

		enrolled, err := q.StudentEnrolledCoursesBySubjectV2(ctx, student.ID, subj.ID)
		if err != nil {
			t.Fatal(err)
		}

		if len(enrolled) == 0 {
			t.Fatal("expected at least 1 enrolled course")
		}

		var found bool
		for _, e := range enrolled {
			if e.CourseID == c1.ID {
				found = true
				if e.CycleID.String != cycle.ID || !e.CycleID.Valid {
					t.Errorf("expected cycle_id %s, got %v", cycle.ID, e.CycleID)
				}
				if e.Level.Int16 != 1 || !e.Level.Valid {
					t.Errorf("expected level 1, got %v", e.Level)
				}
				break
			}
		}
		if !found {
			t.Error("course1 not found in enrolled courses")
		}
	})

	t.Run("Level1AndAbove", func(t *testing.T) {
		// level is smallint, any positive int should work
		// course2 still has no level set (from the unique constraint failure above)
		// Set a level that didn't conflict (different cycle)
		cycle2, err := q.CrmCycleUpsert(ctx, CrmCycleUpsertParams{
			ID:    "cy2026b-" + suffix,
			Label: "Cycle 2026 B " + suffix,
		})
		if err != nil {
			t.Fatal(err)
		}

		err = q.CourseLevelUpdateV2(ctx, c2.ID,
			pgtype.Text{String: cycle2.ID, Valid: true},
			pgtype.Int2{Int16: 5, Valid: true},
		)
		if err != nil {
			t.Fatalf("expected level 5 to be accepted, got: %v", err)
		}
	})
}

func TestCoursesByRootCourseGroupAndCycle(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	subj, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "SUBJ-RCG-" + suffix,
		Name: "Subject RCG " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	cycleA, err := q.CrmCycleUpsert(ctx, CrmCycleUpsertParams{
		ID:    "cy-rcg-a-" + suffix,
		Label: "Cycle RCG A " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	cycleB, err := q.CrmCycleUpsert(ctx, CrmCycleUpsertParams{
		ID:    "cy-rcg-b-" + suffix,
		Label: "Cycle RCG B " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	var rootID pgtype.UUID
	if err := dbpool.QueryRow(ctx, "INSERT INTO root_course_groups (name) VALUES ($1) RETURNING id", "Root RCG "+suffix).Scan(&rootID); err != nil {
		t.Fatal(err)
	}

	type namedCourse struct {
		CourseCreateRow
		cycleID pgtype.Text
	}
	var courses []namedCourse
	for i, code := range []string{"A", "B"} {
		course, err := q.CourseCreate(ctx, CourseCreateParams{
			Code: "C-" + code + "-RCG-" + suffix,
			Name: "Course " + code + " RCG " + suffix,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := dbpool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subj.ID, course.ID); err != nil {
			t.Fatal(err)
		}
		if err := q.CourseUpdateRootCourseGroup(ctx, course.ID, rootID); err != nil {
			t.Fatal(err)
		}
		cycleID := pgtype.Text{String: cycleA.ID, Valid: true}
		if i == 1 {
			cycleID = pgtype.Text{String: cycleB.ID, Valid: true}
		}
		if err := q.CourseLevelUpdateV2(ctx, course.ID, cycleID, pgtype.Int2{Int16: int16(i + 1), Valid: true}); err != nil {
			t.Fatal(err)
		}
		courses = append(courses, namedCourse{CourseCreateRow: course, cycleID: cycleID})
	}

	t.Run("filters by cycle A", func(t *testing.T) {
		found, err := q.CoursesByRootCourseGroupAndCycle(ctx, rootID, pgtype.Text{String: cycleA.ID, Valid: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 1 {
			t.Fatalf("expected 1 course in cycle A, got %d", len(found))
		}
		if found[0].ID != courses[0].ID {
			t.Fatalf("expected course A, got %v", found[0].ID)
		}
	})

	t.Run("filters by cycle B", func(t *testing.T) {
		found, err := q.CoursesByRootCourseGroupAndCycle(ctx, rootID, pgtype.Text{String: cycleB.ID, Valid: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 1 {
			t.Fatalf("expected 1 course in cycle B, got %d", len(found))
		}
		if found[0].ID != courses[1].ID {
			t.Fatalf("expected course B, got %v", found[0].ID)
		}
	})

	t.Run("returns all when cycle not set", func(t *testing.T) {
		found, err := q.CoursesByRootCourseGroupAndCycle(ctx, rootID, pgtype.Text{Valid: false})
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 2 {
			t.Fatalf("expected both courses when cycle not set, got %d", len(found))
		}
	})
}

func contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}
