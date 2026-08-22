package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestStudentSubjectByWcodeMergeAnnotation covers the merged-course link on
// the student subject listing: a subject is linked to its merged course only
// when every active class of the subject belongs to the same merge group, so
// the absence form can offer the merged course as one entry.
func TestStudentSubjectByWcodeMergeAnnotation(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	// StudentCreate lowercases the wcode (LOWER(BTRIM(...))), so seed and
	// query with an already-lowercase code.
	wcode := "wmerge-" + suffix

	student, err := q.StudentCreate(ctx, StudentCreateParams{Wcode: wcode, FullName: "Merge Annotation " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	subjA, err := q.SubjectCreate(ctx, SubjectCreateParams{Code: "WMA-" + suffix, Name: "Writing " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	subjB, err := q.SubjectCreate(ctx, SubjectCreateParams{Code: "WMB-" + suffix, Name: "Reading " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	newCourse := func(code string, subjectID pgtype.UUID) pgtype.UUID {
		t.Helper()
		course, err := q.CourseCreate(ctx, CourseCreateParams{Code: code, Name: code})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := dbpool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subjectID, course.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := dbpool.Exec(ctx,
			"INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, 'enrolled')", course.ID, student.ID); err != nil {
			t.Fatal(err)
		}
		return course.ID
	}

	courseA := newCourse("WMA-C1-"+suffix, subjA.ID)
	courseA2 := newCourse("WMA-C2-"+suffix, subjA.ID)
	courseB := newCourse("WMB-C1-"+suffix, subjB.ID)

	activate := func(courseIDs ...pgtype.UUID) {
		t.Helper()
		for _, id := range courseIDs {
			if _, err := dbpool.Exec(ctx,
				"INSERT INTO subject_active_courses (subject_id, course_id) SELECT subject_id, id FROM courses WHERE id = $1",
				id); err != nil {
				t.Fatal(err)
			}
		}
	}

	rowsByCode := func() map[string]StudentSubjectRow {
		t.Helper()
		rows, err := q.StudentSubjectByWCode(ctx, wcode)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]StudentSubjectRow{}
		for _, row := range rows {
			out[row.SubjectCode] = row
		}
		return out
	}

	// No active courses: no merge link (and no active course either).
	rows := rowsByCode()
	if len(rows) != 2 {
		t.Fatalf("expected 2 subjects, got %d (%v)", len(rows), rows)
	}
	if rows["WMA-"+suffix].MergeGroupID.Valid {
		t.Fatal("subject without active courses must not carry a merge link")
	}

	// Both source courses active and merged: both subjects carry the link.
	activate(courseA, courseB)
	mergeName := "Merged WR " + suffix
	var mergeID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `
		WITH g AS (INSERT INTO course_merge_groups (name) VALUES ($1) RETURNING id)
		INSERT INTO course_merge_group_members (group_id, course_id, position)
		SELECT g.id, c.id, c.pos FROM g, (VALUES ($2::uuid, 1), ($3::uuid, 2)) AS c(id, pos)
		RETURNING group_id
	`, mergeName, courseA, courseB).Scan(&mergeID); err != nil {
		t.Fatal(err)
	}

	rows = rowsByCode()
	for _, code := range []string{"WMA-" + suffix, "WMB-" + suffix} {
		row := rows[code]
		if !row.MergeGroupID.Valid || row.MergeGroupID.Bytes != mergeID.Bytes {
			t.Fatalf("subject %s merge id = %v, want %s", code, row.MergeGroupID, mergeID)
		}
		if row.MergeGroupName.String != mergeName {
			t.Fatalf("subject %s merge name = %q, want %q", code, row.MergeGroupName.String, mergeName)
		}
	}

	// A subject with one more active class outside the merge group loses the
	// link: its actives no longer all belong to the same merged course.
	activate(courseA2)
	rows = rowsByCode()
	if rows["WMA-"+suffix].MergeGroupID.Valid {
		t.Fatal("subject with active classes inside and outside a merge group must not carry a merge link")
	}
	if !rows["WMB-"+suffix].MergeGroupID.Valid {
		t.Fatal("subject whose only active class is merged must keep the merge link")
	}
}
