package absenceshttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/otp"
	"warwick-institute/internal/smartsms"
	"warwick-institute/internal/studentauth"
)

var (
	migrationsOncePending sync.Once
	migrationsErrPending  error
)

func requireTestDBPending(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

func TestParentVerificationResendCooldownIsFiveMinutes(t *testing.T) {
	if resendCooldown != 5*time.Minute {
		t.Fatalf("resendCooldown = %s, want 5m", resendCooldown)
	}
}

func migrateUpOncePending(t *testing.T, databaseURL string) {
	t.Helper()
	migrationsOncePending.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			migrationsErrPending = err
			return
		}
		defer db.Close()
		_, _ = db.Exec(`DELETE FROM crm_rows`)
		if err := goose.SetDialect("postgres"); err != nil {
			migrationsErrPending = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			migrationsErrPending = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations"))
		migrationsErrPending = goose.UpContext(ctx, db, migrationsDir)
	})
	if migrationsErrPending != nil {
		t.Fatal(migrationsErrPending)
	}
}

func newPoolPending(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

// seedParentVerificationTestData inserts the minimum rows needed for
// handleParentVerificationSend to reach StudentSubjectByWCode: a student
// (lowercase wcode), a subject, a course with subject_id, an enrolled
// course_students row, and app_settings with SmsParentEnabled=true.
func seedParentVerificationTestData(t *testing.T, dbpool *pgxpool.Pool, q *sqldb.Queries, suffix string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	studentWCode := strings.ToLower("w" + suffix)

	// Student (lowercase wcode, with parent phone for verification).
	_, err := dbpool.Exec(ctx,
		`INSERT INTO students (wcode, full_name, parent_phone) VALUES ($1, $2, $3)`,
		studentWCode, "Test Student "+suffix, "0812345678")
	if err != nil {
		t.Fatal(err)
	}

	// Subject.
	subject, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{
		Code: "SUBJ-" + suffix, Name: "Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Course (with subject_id).
	var courseID pgtype.UUID
	err = dbpool.QueryRow(ctx,
		`INSERT INTO courses (code, name, subject_id) VALUES ($1, $2, $3) RETURNING id`,
		"COURSE-"+suffix, "Course "+suffix, subject.ID,
	).Scan(&courseID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2)
	`, subject.ID, courseID); err != nil {
		t.Fatal(err)
	}

	// Enroll student in course.
	_, err = dbpool.Exec(ctx,
		`INSERT INTO course_students (course_id, student_id, status)
		 SELECT $1, s.id, 'enrolled' FROM students s WHERE s.wcode = $2`,
		courseID, studentWCode)
	if err != nil {
		t.Fatal(err)
	}

	// Ensure SmsParentEnabled = true.
	_, err = dbpool.Exec(ctx, `
		INSERT INTO app_settings (id, absence_policies)
		VALUES (true,
		        '{"notifications":{"sms_parent_enabled":true,"sms_parent_template":"Your code is {{code}}"}}'::jsonb)
		ON CONFLICT (id) DO UPDATE SET absence_policies = EXCLUDED.absence_policies`)
	if err != nil {
		t.Fatal(err)
	}

	return studentWCode
}
func lookupTokenForPendingStudent(t *testing.T, dbpool *pgxpool.Pool, wcode string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := studentauth.NewService(dbpool).Lookup(ctx, wcode)
	if err != nil {
		t.Fatalf("create lookup token for %s: %v", wcode, err)
	}
	return result.LookupToken
}

// TestHandleParentVerificationSend_UpperCaseWCode_NormalizesToLowercase
// verifies that sending an uppercase wcode to the parent verification send
// endpoint still finds the student (who is stored with lowercase wcode).
// This exercises the case-insensitive normalization required after migration
// 00066 which lowercased all wcodes in the database.
func TestHandleParentVerificationSend_UpperCaseWCode_NormalizesToLowercase(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	suffix := uuid.New().String()[:8]

	studentWCode := seedParentVerificationTestData(t, dbpool, q, suffix)
	lookupToken := lookupTokenForPendingStudent(t, dbpool, studentWCode)

	t.Setenv("OTP_HMAC_KEY", "test-hmac-key-parent-verify")
	otpSvc, err := otp.NewService(dbpool, "test-hmac-key-parent-verify")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Log:         slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Q:           q,
		DB:          dbpool,
		OTP:         otpSvc,
		OTPSender:   &smartsms.MockProvider{},
		AppOrigin:   "",
		InstituteTZ: "Asia/Bangkok",
	})

	// Call with UPPERCASE wcode — should still find the student.
	upperWCode := strings.ToUpper(studentWCode)
	body := `{"wcode":"` + upperWCode + `","lookup_token":"` + lookupToken + `"}`
	req := httptest.NewRequest("POST", "/api/v1/absences/parent-verification/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var got struct {
		Code   string `json:"code"`
		Token  string `json:"token"`
		Status string `json:"status"`
	}
	_ = json.NewDecoder(w.Body).Decode(&got)

	t.Logf("response: status=%d code=%q", w.Code, got.Code)

	// The critical assertion: the student MUST be found. Before the fix,
	// this returns "no_subjects" (404) because the uppercase wcode doesn't
	// match the lowercase wcode stored in the DB.
	if got.Code == "no_subjects" {
		t.Fatalf("BUG: uppercase wcode %q was not normalized — student not found (got %q)", upperWCode, got.Code)
	}
	if w.Code != http.StatusOK && w.Code != http.StatusAccepted {
		t.Fatalf("unexpected status %d (code=%q)", w.Code, got.Code)
	}
	if got.Token == "" {
		t.Fatal("expected a verification token in the response")
	}
}

// TestHandleParentVerificationSend_LowerCaseWCode_AlreadyNormalized verifies
// that the existing lowercase path still works after the normalization fix.
func TestHandleParentVerificationSend_LowerCaseWCode_AlreadyNormalized(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	suffix := uuid.New().String()[:8]

	studentWCode := seedParentVerificationTestData(t, dbpool, q, suffix)
	lookupToken := lookupTokenForPendingStudent(t, dbpool, studentWCode)

	t.Setenv("OTP_HMAC_KEY", "test-hmac-key-parent-verify")
	otpSvc, err := otp.NewService(dbpool, "test-hmac-key-parent-verify")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Log:         slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Q:           q,
		DB:          dbpool,
		OTP:         otpSvc,
		OTPSender:   &smartsms.MockProvider{},
		AppOrigin:   "",
		InstituteTZ: "Asia/Bangkok",
	})

	body := `{"wcode":"` + studentWCode + `","lookup_token":"` + lookupToken + `"}`
	req := httptest.NewRequest("POST", "/api/v1/absences/parent-verification/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var got struct {
		Code   string `json:"code"`
		Token  string `json:"token"`
		Status string `json:"status"`
	}
	_ = json.NewDecoder(w.Body).Decode(&got)

	t.Logf("response: status=%d code=%q", w.Code, got.Code)

	if got.Code == "no_subjects" {
		t.Fatalf("lowercase wcode %q should find student, got %q", studentWCode, got.Code)
	}
	if w.Code != http.StatusOK && w.Code != http.StatusAccepted {
		t.Fatalf("unexpected status %d (code=%q)", w.Code, got.Code)
	}
	if got.Token == "" {
		t.Fatal("expected a verification token in the response")
	}
}

// TestHandleParentVerificationSend_MixedCaseWCode_MixedWhitespace verifies
// that various edge cases of wcode formatting are handled.
func TestHandleParentVerificationSend_MixedCaseWCode_MixedWhitespace(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	suffix := uuid.New().String()[:8]

	studentWCode := seedParentVerificationTestData(t, dbpool, q, suffix)
	lookupToken := lookupTokenForPendingStudent(t, dbpool, studentWCode)

	t.Setenv("OTP_HMAC_KEY", "test-hmac-key-parent-verify")
	otpSvc, err := otp.NewService(dbpool, "test-hmac-key-parent-verify")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Log:         slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Q:           q,
		DB:          dbpool,
		OTP:         otpSvc,
		OTPSender:   &smartsms.MockProvider{},
		AppOrigin:   "",
		InstituteTZ: "Asia/Bangkok",
	})

	tests := []struct {
		name    string
		input   string
		wantHit bool
	}{
		{name: "UPPERCASE", input: strings.ToUpper(studentWCode), wantHit: true},
		{name: "leading_space", input: "  " + studentWCode, wantHit: true},
		{name: "trailing_space", input: studentWCode + "  ", wantHit: true},
		{name: "mixed_case_and_space", input: "  " + strings.ToUpper(studentWCode) + "  ", wantHit: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"wcode":"` + tt.input + `","lookup_token":"` + lookupToken + `"}`
			req := httptest.NewRequest("POST", "/api/v1/absences/parent-verification/send", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", uuid.New().String())
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			var got struct {
				Code  string `json:"code"`
				Token string `json:"token"`
			}
			_ = json.NewDecoder(w.Body).Decode(&got)

			if tt.wantHit && got.Code == "no_subjects" {
				t.Errorf("input %q should find student but got %q", tt.input, got.Code)
			}
		})
	}
}

func TestParentVerificationVerifyReplayBehavior(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	suffix := uuid.NewString()[:8]
	studentWCode := seedParentVerificationTestData(t, dbpool, q, "REPLAY"+suffix)

	const hmacKey = "test-hmac-key-parent-verify"
	t.Setenv("OTP_HMAC_KEY", hmacKey)
	otpSvc, err := otp.NewService(dbpool, hmacKey)
	if err != nil {
		t.Fatal(err)
	}
	code, token, err := otpSvc.StartSession(context.Background(), studentWCode, "0812345678")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	info, err := otpSvc.DecodeToken(token)
	if err != nil {
		t.Fatalf("DecodeToken: %v", err)
	}
	if _, err := otpSvc.VerifySession(context.Background(), token, code); err != nil {
		t.Fatalf("initial VerifySession: %v", err)
	}

	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Log:                 slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Q:                   q,
		DB:                  dbpool,
		OTP:                 otpSvc,
		StudentSelfService:  studentauth.NewService(dbpool),
		StudentCookieSecure: false,
		AppOrigin:           "",
		InstituteTZ:         "Asia/Bangkok",
	})

	countStudentSessions := func() int {
		t.Helper()
		var sessionCount int
		if err := dbpool.QueryRow(
			context.Background(),
			`SELECT count(*) FROM student_self_service_sessions WHERE verification_session_id = $1`,
			info.SessionID,
		).Scan(&sessionCount); err != nil {
			t.Fatalf("count student sessions: %v", err)
		}
		return sessionCount
	}
	replayVerify := func() *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/absences/parent-verification/verify",
			strings.NewReader(`{"token":"`+token+`","code":"`+code+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		return recorder
	}

	t.Run("verified replay reissues a student session", func(t *testing.T) {
		recorder := replayVerify()
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", recorder.Code, recorder.Body.String())
		}
		var response struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response.Status != "verified" {
			t.Fatalf("response status = %q, want verified", response.Status)
		}
		if !strings.Contains(recorder.Header().Get("Set-Cookie"), studentauth.CookieName(false)) {
			t.Fatalf("replayed verification must re-issue the student session cookie, got headers %v", recorder.Header())
		}
		// Replaying a verified (not yet consumed) verification mints exactly
		// one fresh session row — the setup verified via the service without
		// creating a session, so the only session is the replayed one.
		if got := countStudentSessions(); got != 1 {
			t.Fatalf("student sessions after verified replay = %d, want 1", got)
		}
	})

	t.Run("consumed replay is rejected without issuing a session", func(t *testing.T) {
		absenceID := seedStudentAbsenceOwnership(t, dbpool, studentWCode, "pending")
		if err := otpSvc.ConsumeSession(context.Background(), token, absenceID); err != nil {
			t.Fatalf("ConsumeSession: %v", err)
		}
		recorder := replayVerify()
		if recorder.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body = %s", recorder.Code, recorder.Body.String())
		}
		var response struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response.Code != "already_verified" {
			t.Fatalf("error code = %q, want already_verified", response.Code)
		}
		if got := countStudentSessions(); got != 1 {
			t.Fatalf("student sessions after consumed replay = %d, want 1 (no new session)", got)
		}
	})
}
