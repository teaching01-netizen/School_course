package absenceshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The absence-form visibility flag is a student-facing gate only: the student
// listing SQL must filter on it, the staff listing SQL must not.
func TestSessionsInRangeQueryAbsenceFormVisibleSplit(t *testing.T) {
	studentSQL := sessionsInRangeSelectSQL()
	if !strings.Contains(studentSQL, "c.absence_form_visible") {
		t.Fatalf("student sessions-in-range query must filter hidden courses, SQL: %s", studentSQL)
	}
	staffSQL := sessionsInRangeStaffSelectSQL()
	if strings.Contains(staffSQL, "absence_form_visible") {
		t.Fatalf("staff sessions-in-range query must not filter hidden courses, SQL: %s", staffSQL)
	}
}

func setAbsenceFormVisible(t *testing.T, dbpool *pgxpool.Pool, courseID uuid.UUID, visible bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := dbpool.Exec(ctx, `
		UPDATE courses SET absence_form_visible = $2 WHERE id = $1
	`, courseID, visible); err != nil {
		t.Fatal(err)
	}
}

// Full-chain gate: a course hidden from the absence form disappears from the
// student's session listing and both student submission endpoints reject it
// with 403, while the staff listing keeps showing it and a sibling visible
// course stays bookable.
func TestAbsenceFormHiddenCourseGate(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)

	mux := selfServiceMux(t, dbpool)
	seed := seedActiveCourseFixture(t, dbpool)
	rawToken := seedVerifiedStudentSession(t, dbpool, seed.wcode)

	t.Run("hidden_course_leaves_student_listing_but_not_staff", func(t *testing.T) {
		setAbsenceFormVisible(t, dbpool, seed.courses["current"], false)

		codes, _ := querySessionCourseCodes(t, dbpool, seed.wcode)
		if codes[seed.code("current")] {
			t.Fatalf("hidden course must not appear in the student listing, got %v", codes)
		}
		if !codes[seed.code("old")] || !codes[seed.code("sibling")] {
			t.Fatalf("visible courses must keep appearing in the student listing, got %v", codes)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rows, err := dbpool.Query(ctx, sessionsInRangeStaffSelectSQL(),
			seed.wcode, time.Now().UTC().AddDate(0, 0, -30), time.Now().UTC().AddDate(0, 0, 90))
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		staffCodes := map[string]bool{}
		for rows.Next() {
			var id, courseID, subjectID uuid.UUID
			var startAt, endAt time.Time
			var courseCode, courseName, subjectCode, subjectName, teacherName string
			if err := rows.Scan(&id, &startAt, &endAt, &courseID, &courseCode, &courseName, &subjectID, &subjectCode, &subjectName, &teacherName); err != nil {
				t.Fatal(err)
			}
			staffCodes[courseCode] = true
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if !staffCodes[seed.code("current")] {
			t.Fatalf("staff listing must still show the hidden course, got %v", staffCodes)
		}
	})

	t.Run("student_single_submit_rejected", func(t *testing.T) {
		_, localDate := pickCourseSessionDate(t, dbpool, seed.courses["current"], "Asia/Bangkok")
		recorder := postSelfService(t, mux, "/api/v1/absences", rawToken, map[string]any{
			"subject_id": seed.subjID.String(),
			"course_id":  seed.courses["current"].String(),
			"date_from":  localDate,
			"date_to":    localDate,
		})
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("hidden-course submit status = %d, want 403, body = %s", recorder.Code, recorder.Body.String())
		}
		var errBody struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &errBody); err != nil {
			t.Fatal(err)
		}
		if errBody.Code != "course_not_available" {
			t.Fatalf("hidden-course submit error code = %q, want course_not_available", errBody.Code)
		}
	})

	t.Run("student_batch_submit_rejected", func(t *testing.T) {
		_, localDate := pickCourseSessionDate(t, dbpool, seed.courses["current"], "Asia/Bangkok")
		recorder := postSelfService(t, mux, "/api/v1/absences/batch", rawToken, map[string]any{
			"items": []map[string]any{{
				"subject_id": seed.subjID.String(),
				"course_id":  seed.courses["current"].String(),
				"date_from":  localDate,
				"date_to":    localDate,
			}},
		})
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("hidden-course batch submit status = %d, want 403, body = %s", recorder.Code, recorder.Body.String())
		}
		var errBody struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &errBody); err != nil {
			t.Fatal(err)
		}
		if errBody.Code != "course_not_available" {
			t.Fatalf("hidden-course batch submit error code = %q, want course_not_available", errBody.Code)
		}
	})

	t.Run("batch_sit_in_to_inactive_course_rejected", func(t *testing.T) {
		// An inactive class may not be a sit-in target even when the absence
		// itself is for a live class of the same subject.
		setAbsenceFormVisible(t, dbpool, seed.courses["sibling"], false)
		t.Cleanup(func() { setAbsenceFormVisible(t, dbpool, seed.courses["sibling"], true) })

		ensureCourseAbsenceHeadroom(t, dbpool, seed.courses["old"], 15)
		sessionID, localDate := pickCourseSessionDate(t, dbpool, seed.courses["old"], "Asia/Bangkok")
		recorder := postSelfService(t, mux, "/api/v1/absences/batch", rawToken, map[string]any{
			"items": []map[string]any{{
				"subject_id":       seed.subjID.String(),
				"course_id":        seed.courses["old"].String(),
				"date_from":        localDate,
				"date_to":          localDate,
				"missed_session_ids": []string{sessionID},
				"sit_in_course_id": seed.courses["sibling"].String(),
			}},
		})
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("inactive sit-in submit status = %d, want 400, body = %s", recorder.Code, recorder.Body.String())
		}
		var errBody struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &errBody); err != nil {
			t.Fatal(err)
		}
		if errBody.Code != "sit_in_course_inactive" {
			t.Fatalf("inactive sit-in error code = %q, want sit_in_course_inactive", errBody.Code)
		}
	})

	t.Run("visible_sibling_still_submittable", func(t *testing.T) {
		ensureCourseAbsenceHeadroom(t, dbpool, seed.courses["sibling"])
		sessionID, localDate := pickCourseSessionDate(t, dbpool, seed.courses["sibling"], "Asia/Bangkok")
		recorder := postSelfService(t, mux, "/api/v1/absences", rawToken, map[string]any{
			"subject_id":         seed.subjID.String(),
			"course_id":          seed.courses["sibling"].String(),
			"date_from":          localDate,
			"date_to":            localDate,
			"missed_session_ids": []string{sessionID},
		})
		if recorder.Code != http.StatusCreated {
			t.Fatalf("visible-course submit status = %d, want 201, body = %s", recorder.Code, recorder.Body.String())
		}
	})
}
