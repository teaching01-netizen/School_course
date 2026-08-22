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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/otp"
	"warwick-institute/internal/studentauth"
)

const selfServiceTestOTPHMACKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// pickCourseSessionDate returns the first session of a course and the
// institute-local date of that session, which is what a submission must use as
// its absence date range.
func pickCourseSessionDate(t *testing.T, dbpool *pgxpool.Pool, courseID uuid.UUID, tz string) (sessionID string, localDate string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var id uuid.UUID
	var start time.Time
	if err := dbpool.QueryRow(ctx, `
		SELECT id, start_at FROM sessions
		WHERE course_id = $1
		ORDER BY start_at
		LIMIT 1
	`, courseID).Scan(&id, &start); err != nil {
		t.Fatalf("pick session for course %s: %v", courseID, err)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		t.Fatal(err)
	}
	return id.String(), start.In(loc).Format("2006-01-02")
}

// ensureCourseAbsenceHeadroom gives a course enough distinct session dates
// that the absence-day limit (total days / 5, rounded down) allows a single
// absence day to be booked.
func ensureCourseAbsenceHeadroom(t *testing.T, dbpool *pgxpool.Pool, courseID uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var teacherID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO users (username, role, password_hash)
		VALUES ($1, 'Teacher', 'x')
		RETURNING id
	`, "headroom-"+uuid.NewString()[:8]).Scan(&teacherID); err != nil {
		t.Fatalf("create teacher: %v", err)
	}
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	for day := 1; day <= 15; day++ {
		start := time.Date(2027, 1, day, 9, 0, 0, 0, loc)
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO sessions (course_id, teacher_id, start_at, end_at)
			VALUES ($1, $2, $3, $4)
		`, courseID, teacherID, start, start.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
}

func selfServiceMux(t *testing.T, dbpool *pgxpool.Pool) *http.ServeMux {
	t.Helper()
	otpSvc, err := otp.NewService(dbpool, selfServiceTestOTPHMACKey)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Q:                   sqldb.New(dbpool),
		DB:                  dbpool,
		Log:                 slog.Default(),
		InstituteTZ:         "Asia/Bangkok",
		OTP:                 otpSvc,
		StudentSelfService:  studentauth.NewService(dbpool),
		StudentCookieSecure: false,
	})
	return mux
}

func postSelfService(t *testing.T, mux *http.ServeMux, path string, rawToken string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	req.AddCookie(&http.Cookie{Name: studentauth.CookieName(false), Value: rawToken})
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	return recorder
}

func assertOtpConsumed(t *testing.T, dbpool *pgxpool.Pool, wcode, absenceID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var status string
	var consumedAbsenceID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `
		SELECT status, consumed_absence_id
		FROM student_parent_verification_sessions
		WHERE lower(wcode) = lower($1)
		ORDER BY created_at DESC
		LIMIT 1
	`, wcode).Scan(&status, &consumedAbsenceID); err != nil {
		t.Fatalf("load verification session for %s: %v", wcode, err)
	}
	if status != "consumed" {
		t.Fatalf("verification session status = %q, want consumed", status)
	}
	if !consumedAbsenceID.Valid {
		t.Fatalf("verification session must record the consumed absence")
	}
	gotID, err := uuid.FromBytes(consumedAbsenceID.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}
	if gotID.String() != absenceID {
		t.Fatalf("consumed_absence_id = %s, want %s", gotID, absenceID)
	}
}

func assertStudentSessionRevoked(t *testing.T, dbpool *pgxpool.Pool, wcode string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var revokedAt pgtype.Timestamptz
	if err := dbpool.QueryRow(ctx, `
		SELECT revoked_at
		FROM student_self_service_sessions
		WHERE lower(wcode) = lower($1)
		ORDER BY created_at DESC
		LIMIT 1
	`, wcode).Scan(&revokedAt); err != nil {
		t.Fatalf("load student session for %s: %v", wcode, err)
	}
	if !revokedAt.Valid {
		t.Fatalf("student session for %s must be revoked after submission", wcode)
	}
}

// H4: submitting an absence consumes the parent OTP verification session and
// revokes the student's session, so a single verification cannot fuel an
// unlimited number of submissions.
func TestSelfServiceSubmitConsumesOtpAndRevokesSession(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)

	mux := selfServiceMux(t, dbpool)

	t.Run("batch submission", func(t *testing.T) {
		seed := seedActiveCourseFixture(t, dbpool)
		rawToken := seedVerifiedStudentSession(t, dbpool, seed.wcode)
		ensureCourseAbsenceHeadroom(t, dbpool, seed.courses["sibling"])
		sessionID, localDate := pickCourseSessionDate(t, dbpool, seed.courses["sibling"], "Asia/Bangkok")

		recorder := postSelfService(t, mux, "/api/v1/absences/batch", rawToken, map[string]any{
			"items": []map[string]any{{
				"subject_id":         seed.subjID.String(),
				"course_id":          seed.courses["sibling"].String(),
				"date_from":          localDate,
				"date_to":            localDate,
				"missed_session_ids": []string{sessionID},
			}},
		})
		if recorder.Code != http.StatusCreated {
			t.Fatalf("batch submit status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var created struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		if len(created.Items) != 1 || created.Items[0].ID == "" {
			t.Fatalf("batch response items = %#v", created.Items)
		}

		assertOtpConsumed(t, dbpool, seed.wcode, created.Items[0].ID)
		assertStudentSessionRevoked(t, dbpool, seed.wcode)

		// The cookie is still presented by the client but is no longer valid
		// server-side, so the next authenticated call is rejected.
		meReq := httptest.NewRequest(http.MethodGet, "/api/v1/absence-self-service/me", nil)
		meReq.AddCookie(&http.Cookie{Name: studentauth.CookieName(false), Value: rawToken})
		meRecorder := httptest.NewRecorder()
		mux.ServeHTTP(meRecorder, meReq)
		if meRecorder.Code != http.StatusUnauthorized {
			t.Fatalf("profile after submission status = %d, want 401", meRecorder.Code)
		}
	})

	t.Run("single-create student path", func(t *testing.T) {
		seed := seedActiveCourseFixture(t, dbpool)
		rawToken := seedVerifiedStudentSession(t, dbpool, seed.wcode)
		ensureCourseAbsenceHeadroom(t, dbpool, seed.courses["current"])
		sessionID, localDate := pickCourseSessionDate(t, dbpool, seed.courses["current"], "Asia/Bangkok")

		recorder := postSelfService(t, mux, "/api/v1/absences", rawToken, map[string]any{
			"subject_id":         seed.subjID.String(),
			"course_id":          seed.courses["current"].String(),
			"date_from":          localDate,
			"date_to":            localDate,
			"missed_session_ids": []string{sessionID},
		})
		if recorder.Code != http.StatusCreated {
			t.Fatalf("single submit status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var created struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}

		assertOtpConsumed(t, dbpool, seed.wcode, created.ID)
		assertStudentSessionRevoked(t, dbpool, seed.wcode)
	})
}

// MEDIUM-10: self-service idempotency must be partitioned per student (wcode),
// not per session, so retries across session rotations still deduplicate.
func TestStudentIdempotencyActorStablePerStudent(t *testing.T) {
	base := studentIdempotencyActor("w1234567")
	if base != studentIdempotencyActor(" W1234567 ") {
		t.Fatal("actor must be stable across whitespace and case")
	}
	if base == studentIdempotencyActor("w7654321") {
		t.Fatal("different students must map to different actors")
	}
}
