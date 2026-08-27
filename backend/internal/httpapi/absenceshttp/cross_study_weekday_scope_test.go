package absenceshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

func TestCrossStudyWeekdayScopeAbsenceSurface(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)

	seed := seedActiveCourseFixture(t, dbpool)
	setActiveCourseRow(t, dbpool, seed.subjID, seed.courses["current"])

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(loc)
	daysUntilMonday := (int(time.Monday) - int(now.Weekday()) + 7) % 7
	if daysUntilMonday < 3 {
		daysUntilMonday += 7
	}
	mondayLocal := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 10, 0, 0, 0, loc)
	tuesdayLocal := mondayLocal.AddDate(0, 0, 1)
	startRange := time.Date(mondayLocal.Year(), mondayLocal.Month(), mondayLocal.Day(), 0, 0, 0, 0, loc).UTC()
	endRange := startRange.AddDate(0, 0, 2)

	var mondaySessionID, teacherID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		SELECT id, teacher_id
		FROM sessions
		WHERE course_id = $1
		ORDER BY start_at
		LIMIT 1
	`, seed.courses["current"]).Scan(&mondaySessionID, &teacherID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		UPDATE sessions
		SET start_at = $2, end_at = $3
		WHERE id = $1
	`, mondaySessionID, mondayLocal.UTC(), mondayLocal.UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var tuesdaySessionID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO sessions (course_id, teacher_id, start_at, end_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, seed.courses["current"], teacherID, tuesdayLocal.UTC(), tuesdayLocal.UTC().Add(time.Hour)).Scan(&tuesdaySessionID); err != nil {
		t.Fatal(err)
	}

	var mergeGroupID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO course_merge_groups (name) VALUES ($1) RETURNING id
	`, "Cross Study Scope "+seed.suffix).Scan(&mergeGroupID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO course_merge_group_members (group_id, course_id, position)
		VALUES ($1, $2, 1), ($1, $3, 2)
	`, mergeGroupID, seed.courses["current"], seed.courses["sibling"]); err != nil {
		t.Fatal(err)
	}

	var snapshotID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO crm_snapshots (status) VALUES ('ready') RETURNING id
	`).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO crm_cross_study_assignments (
			snapshot_id, wcode, source_course_id, dest_course_a_id,
			dest_course_b_id, assigned_course_id,
			dest_course_a_weekdays, dest_course_b_weekdays
		) VALUES ($1, $2, $3, $4, $5, $4, ARRAY[1]::smallint[], ARRAY[2]::smallint[])
	`, snapshotID, seed.wcode, seed.courses["old"], seed.courses["current"], seed.courses["old"]); err != nil {
		t.Fatal(err)
	}
	for week := 1; week <= 4; week++ {
		startAt := mondayLocal.UTC().AddDate(0, 0, week*7)
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO sessions (course_id, teacher_id, start_at, end_at)
			VALUES ($1, $2, $3, $4)
		`, seed.courses["current"], teacherID, startAt, startAt.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}

	addActiveCourseRow(t, dbpool, seed.subjID, seed.courses["sibling"])

	var siblingMondaySessionID, sitInSessionID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO sessions (course_id, teacher_id, start_at, end_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, seed.courses["sibling"], teacherID, mondayLocal.UTC().Add(2*time.Hour), mondayLocal.UTC().Add(3*time.Hour)).Scan(&siblingMondaySessionID); err != nil {
		t.Fatal(err)
	}
	var makeupTeacherID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO users (username, role, password_hash)
		VALUES ($1, 'Teacher', 'x')
		RETURNING id
	`, "cross-study-makeup-"+seed.suffix).Scan(&makeupTeacherID); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO sessions (course_id, teacher_id, start_at, end_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, seed.courses["old"], makeupTeacherID, mondayLocal.UTC().Add(2*time.Hour), mondayLocal.UTC().Add(3*time.Hour)).Scan(&sitInSessionID); err != nil {
		t.Fatal(err)
	}

	t.Run("student listing excludes unselected weekday", func(t *testing.T) {
		rows, err := dbpool.Query(ctx, sessionsInRangeSelectSQL(), seed.wcode, startRange, endRange)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()

		var ids []uuid.UUID
		for rows.Next() {
			var id, courseID, subjectID uuid.UUID
			var startAt, endAt time.Time
			var courseCode, courseName, subjectCode, subjectName, teacherName string
			if err := rows.Scan(&id, &startAt, &endAt, &courseID, &courseCode, &courseName, &subjectID, &subjectCode, &subjectName, &teacherName); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if len(ids) != 1 || ids[0] != mondaySessionID {
			t.Fatalf("student session listing = %v, want only Monday session %s", ids, mondaySessionID)
		}
	})

	t.Run("staff listing excludes unselected merge sibling", func(t *testing.T) {
		rows, err := dbpool.Query(ctx, sessionsInRangeStaffSelectSQL(), seed.wcode, startRange, endRange)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()

		for rows.Next() {
			var id, courseID, subjectID uuid.UUID
			var startAt, endAt time.Time
			var courseCode, courseName, subjectCode, subjectName, teacherName string
			if err := rows.Scan(&id, &startAt, &endAt, &courseID, &courseCode, &courseName, &subjectID, &subjectCode, &subjectName, &teacherName); err != nil {
				t.Fatal(err)
			}
			if id == siblingMondaySessionID {
				t.Fatalf("staff session listing included unselected merge sibling %s", id)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	})

	var previousPolicies []byte
	if err := dbpool.QueryRow(ctx, `SELECT absence_policies FROM app_settings WHERE id = true`).Scan(&previousPolicies); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `UPDATE app_settings SET absence_policies = $1 WHERE id = true`, previousPolicies)
	})
	if _, err := dbpool.Exec(ctx, `UPDATE app_settings SET absence_policies = '{}'::jsonb WHERE id = true`); err != nil {
		t.Fatal(err)
	}

	adminServer := &server{
		deps: httpdeps.Deps{
			Q:           sqldb.New(dbpool),
			DB:          dbpool,
			Log:         slog.Default(),
			InstituteTZ: "Asia/Bangkok",
			Auth:        absenceLimitFakeAuth{user: auth.AuthenticatedUser{Role: "Admin"}},
		},
		a: httpadapter.New(nil, slog.Default()),
	}
	adminBody, err := json.Marshal(map[string]any{
		"wcode":              seed.wcode,
		"subject_id":         seed.subjID.String(),
		"course_id":          seed.courses["current"].String(),
		"date_from":          mondayLocal.Format("2006-01-02"),
		"date_to":            tuesdayLocal.Format("2006-01-02"),
		"missed_session_ids": []string{tuesdaySessionID.String()},
	})
	if err != nil {
		t.Fatal(err)
	}
	adminRequest := httptest.NewRequest(http.MethodPost, "/api/v1/absences", bytes.NewReader(adminBody))
	adminRequest.Header.Set("Content-Type", "application/json")
	adminRequest.Header.Set("Idempotency-Key", uuid.NewString())
	adminResponse := httptest.NewRecorder()
	adminServer.handleAbsenceCreate(adminResponse, adminRequest)
	assertInvalidMissedSessionsResponse(t, adminResponse)

	var absenceCount int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM student_absences WHERE wcode = $1`, seed.wcode).Scan(&absenceCount); err != nil {
		t.Fatal(err)
	}
	if absenceCount != 0 {
		t.Fatalf("invalid single submission created %d absence rows", absenceCount)
	}

	adminSiblingBody, err := json.Marshal(map[string]any{
		"wcode":              seed.wcode,
		"subject_id":         seed.subjID.String(),
		"course_id":          seed.courses["current"].String(),
		"date_from":          mondayLocal.Format("2006-01-02"),
		"date_to":            mondayLocal.Format("2006-01-02"),
		"missed_session_ids": []string{siblingMondaySessionID.String()},
	})
	if err != nil {
		t.Fatal(err)
	}
	adminSiblingRequest := httptest.NewRequest(http.MethodPost, "/api/v1/absences", bytes.NewReader(adminSiblingBody))
	adminSiblingRequest.Header.Set("Content-Type", "application/json")
	adminSiblingRequest.Header.Set("Idempotency-Key", uuid.NewString())
	adminSiblingResponse := httptest.NewRecorder()
	adminServer.handleAbsenceCreate(adminSiblingResponse, adminSiblingRequest)
	assertInvalidMissedSessionsResponse(t, adminSiblingResponse)

	rawToken := seedVerifiedStudentSession(t, dbpool, seed.wcode)
	studentResponse := postSelfService(t, selfServiceMux(t, dbpool), "/api/v1/absences/batch", rawToken, map[string]any{
		"items": []map[string]any{{
			"subject_id":         seed.subjID.String(),
			"course_id":          seed.courses["current"].String(),
			"date_from":          mondayLocal.Format("2006-01-02"),
			"date_to":            tuesdayLocal.Format("2006-01-02"),
			"missed_session_ids": []string{tuesdaySessionID.String()},
		}},
	})
	assertInvalidMissedSessionsResponse(t, studentResponse)

	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM student_absences WHERE wcode = $1`, seed.wcode).Scan(&absenceCount); err != nil {
		t.Fatal(err)
	}
	if absenceCount != 0 {
		t.Fatalf("invalid batch submission created %d absence rows", absenceCount)
	}

	studentSiblingResponse := postSelfService(t, selfServiceMux(t, dbpool), "/api/v1/absences/batch", seedVerifiedStudentSession(t, dbpool, seed.wcode), map[string]any{
		"items": []map[string]any{{
			"subject_id":         seed.subjID.String(),
			"course_id":          seed.courses["current"].String(),
			"date_from":          mondayLocal.Format("2006-01-02"),
			"date_to":            mondayLocal.Format("2006-01-02"),
			"missed_session_ids": []string{siblingMondaySessionID.String()},
		}},
	})
	assertInvalidMissedSessionsResponse(t, studentSiblingResponse)

	addActiveCourseRow(t, dbpool, seed.subjID, seed.courses["old"])
	validResponse := postSelfService(t, selfServiceMux(t, dbpool), "/api/v1/absences/batch", seedVerifiedStudentSession(t, dbpool, seed.wcode), map[string]any{
		"items": []map[string]any{{
			"subject_id":         seed.subjID.String(),
			"course_id":          seed.courses["current"].String(),
			"date_from":          mondayLocal.Format("2006-01-02"),
			"date_to":            mondayLocal.Format("2006-01-02"),
			"sit_in_method":      "physical",
			"sit_in_course_id":   seed.courses["old"].String(),
			"sit_in_session_ids": []string{sitInSessionID.String()},
			"missed_session_ids": []string{mondaySessionID.String()},
		}},
	})
	if validResponse.Code != http.StatusCreated {
		t.Fatalf("valid make-up overlapping irrelevant sibling status = %d, body = %s", validResponse.Code, validResponse.Body.String())
	}
}

func assertInvalidMissedSessionsResponse(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "invalid_missed_sessions" {
		t.Fatalf("error code = %q, want invalid_missed_sessions; body = %s", body.Code, response.Body.String())
	}
}
