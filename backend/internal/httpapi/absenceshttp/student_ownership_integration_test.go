package absenceshttp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/studentauth"
)

func seedStudentAbsenceOwnership(t *testing.T, dbpool *pgxpool.Pool, wcode, status string) uuid.UUID {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var courseID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		SELECT c.id
		FROM courses c
		JOIN course_students cs ON cs.course_id = c.id
		JOIN students st ON st.id = cs.student_id
		WHERE lower(st.wcode) = lower($1)
		ORDER BY c.created_at
		LIMIT 1
	`, wcode).Scan(&courseID); err != nil {
		t.Fatalf("find course for %s: %v", wcode, err)
	}

	var absenceID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO student_absences (wcode, course_id, date_from, date_to, status, reason)
		VALUES ($1, $2, '2030-01-01', '2030-01-01', $3, 'ownership test')
		RETURNING id
	`, wcode, courseID, status).Scan(&absenceID); err != nil {
		t.Fatalf("seed absence for %s: %v", wcode, err)
	}
	return absenceID
}

func seedVerifiedStudentSession(t *testing.T, dbpool *pgxpool.Pool, wcode string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	verificationID := uuid.New()
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO student_parent_verification_sessions
			(id, wcode, parent_phone, status, verified_at, created_at, updated_at)
		VALUES ($1, $2, '+66812345678', 'verified', now(), now(), now())
	`, verificationID, wcode); err != nil {
		t.Fatalf("seed verified OTP session: %v", err)
	}

	rawToken := base64.RawURLEncoding.EncodeToString([]byte("student-session-" + uuid.NewString()))
	hash := sha256.Sum256([]byte(rawToken))
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO student_self_service_sessions
			(id, token_hash, wcode, verification_session_id, created_at, last_seen_at, expires_at, absolute_expires_at)
		VALUES ($1, $2, $3, $4, now(), now(), now() + interval '45 minutes', now() + interval '24 hours')
	`, uuid.New(), hash[:], wcode, verificationID); err != nil {
		t.Fatalf("seed student session: %v", err)
	}
	return rawToken
}

func ownershipServer(t *testing.T, dbpool *pgxpool.Pool) *server {
	t.Helper()
	return &server{
		deps: httpdeps.Deps{
			DB:                  dbpool,
			Q:                   sqldb.New(dbpool),
			StudentSelfService:  studentauth.NewService(dbpool),
			StudentCookieSecure: false,
		},
		a: httpadapter.New(nil, nil),
	}
}

func callStudentCancel(t *testing.T, s *server, absenceID uuid.UUID, rawToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/absence-self-service/absences/"+absenceID.String()+"/cancel", nil)
	req.SetPathValue("id", absenceID.String())
	req.AddCookie(&http.Cookie{Name: studentauth.CookieName(false), Value: rawToken})
	req.Header.Set("Idempotency-Key", uuid.NewString())
	recorder := httptest.NewRecorder()
	s.handleStudentAbsenceCancel(recorder, req)
	return recorder
}

func TestStudentAbsenceCancellationEnforcesOwnershipAndState(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)

	t.Run("own pending absence cancels and audits", func(t *testing.T) {
		suffix := uuid.NewString()[:8]
		wcode := seedParentVerificationTestData(t, dbpool, q, "OWN"+suffix)
		_, err := dbpool.Exec(context.Background(), `
			UPDATE app_settings
			SET absence_policies = jsonb_set(
				COALESCE(absence_policies, '{}'::jsonb),
				'{student_self_service}',
				'{"can_view_own":true,"can_cancel_own":true}'::jsonb,
				true)
			WHERE id = true`)
		if err != nil {
			t.Fatal(err)
		}
		absenceID := seedStudentAbsenceOwnership(t, dbpool, wcode, "pending")
		rawToken := seedVerifiedStudentSession(t, dbpool, wcode)

		recorder := callStudentCancel(t, ownershipServer(t, dbpool), absenceID, rawToken)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var status string
		if err := dbpool.QueryRow(context.Background(), `SELECT status FROM student_absences WHERE id = $1`, absenceID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "cancelled" {
			t.Fatalf("status = %q, want cancelled", status)
		}
		var auditCount int
		if err := dbpool.QueryRow(context.Background(), `SELECT count(*) FROM absence_audit_log WHERE absence_id = $1 AND action = 'cancelled' AND actor_role = 'student'`, absenceID).Scan(&auditCount); err != nil {
			t.Fatal(err)
		}
		if auditCount != 1 {
			t.Fatalf("student cancellation audit count = %d, want 1", auditCount)
		}
	})

	t.Run("other student absence is indistinguishable", func(t *testing.T) {
		suffixA, suffixB := uuid.NewString()[:8], uuid.NewString()[:8]
		wcodeA := seedParentVerificationTestData(t, dbpool, q, "A"+suffixA)
		wcodeB := seedParentVerificationTestData(t, dbpool, q, "B"+suffixB)
		_, err := dbpool.Exec(context.Background(), `
			UPDATE app_settings
			SET absence_policies = jsonb_set(
				COALESCE(absence_policies, '{}'::jsonb),
				'{student_self_service}',
				'{"can_view_own":true,"can_cancel_own":true}'::jsonb,
				true)
			WHERE id = true`)
		if err != nil {
			t.Fatal(err)
		}
		absenceID := seedStudentAbsenceOwnership(t, dbpool, wcodeB, "pending")
		rawToken := seedVerifiedStudentSession(t, dbpool, wcodeA)

		recorder := callStudentCancel(t, ownershipServer(t, dbpool), absenceID, rawToken)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var status string
		if err := dbpool.QueryRow(context.Background(), `SELECT status FROM student_absences WHERE id = $1`, absenceID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "pending" {
			t.Fatalf("other student's absence status = %q, want pending", status)
		}
		var auditCount int
		if err := dbpool.QueryRow(context.Background(), `SELECT count(*) FROM absence_audit_log WHERE absence_id = $1 AND action = 'cancelled' AND actor_role = 'student'`, absenceID).Scan(&auditCount); err != nil {
			t.Fatal(err)
		}
		if auditCount != 0 {
			t.Fatalf("other-student cancellation audit count = %d, want 0", auditCount)
		}
	})

	t.Run("actioned absence returns conflict", func(t *testing.T) {
		suffix := uuid.NewString()[:8]
		wcode := seedParentVerificationTestData(t, dbpool, q, "ACT"+suffix)
		_, err := dbpool.Exec(context.Background(), `
			UPDATE app_settings
			SET absence_policies = jsonb_set(
				COALESCE(absence_policies, '{}'::jsonb),
				'{student_self_service}',
				'{"can_view_own":true,"can_cancel_own":true}'::jsonb,
				true)
			WHERE id = true`)
		if err != nil {
			t.Fatal(err)
		}
		absenceID := seedStudentAbsenceOwnership(t, dbpool, wcode, "actioned")
		rawToken := seedVerifiedStudentSession(t, dbpool, wcode)

		recorder := callStudentCancel(t, ownershipServer(t, dbpool), absenceID, rawToken)
		if recorder.Code != http.StatusConflict {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("policy disabled returns forbidden", func(t *testing.T) {
		suffix := uuid.NewString()[:8]
		wcode := seedParentVerificationTestData(t, dbpool, q, "DIS"+suffix)
		_, err := dbpool.Exec(context.Background(), `
			UPDATE app_settings
			SET absence_policies = jsonb_set(
				COALESCE(absence_policies, '{}'::jsonb),
				'{student_self_service}',
				'{"can_view_own":true,"can_cancel_own":false}'::jsonb,
				true)
			WHERE id = true`)
		if err != nil {
			t.Fatal(err)
		}
		absenceID := seedStudentAbsenceOwnership(t, dbpool, wcode, "pending")
		rawToken := seedVerifiedStudentSession(t, dbpool, wcode)

		recorder := callStudentCancel(t, ownershipServer(t, dbpool), absenceID, rawToken)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestStudentAbsenceCancellationAnonymousReturnsUnauthorized(t *testing.T) {
	s := &server{a: httpadapter.New(nil, nil)}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/absence-self-service/absences/"+uuid.NewString()+"/cancel", strings.NewReader(`{}`))
	s.handleStudentAbsenceCancel(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "unauthorized" {
		t.Fatalf("error code = %v, want unauthorized", body["code"])
	}
}
