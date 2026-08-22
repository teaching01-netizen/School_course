package absenceshttp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/otp"
	"warwick-institute/internal/smartsms"
	"warwick-institute/internal/studentauth"
)

// seedStudentWithoutParentPhone seeds a verified-lookup-capable student with
// no parent phone, the precondition for client phone enrollment.
func seedStudentWithoutParentPhone(t *testing.T, dbpool *pgxpool.Pool, q *sqldb.Queries, suffix string) string {
	t.Helper()
	wcode := seedParentVerificationTestData(t, dbpool, q, suffix)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := dbpool.Exec(ctx, `UPDATE students SET parent_phone = NULL WHERE wcode = $1`, wcode); err != nil {
		t.Fatalf("clear parent phone: %v", err)
	}
	return wcode
}

func enrollmentTestServer(t *testing.T, dbpool *pgxpool.Pool, q *sqldb.Queries, otpSvc *otp.Service) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Log:                 slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Q:                   q,
		DB:                  dbpool,
		OTP:                 otpSvc,
		OTPSender:           &smartsms.MockProvider{},
		StudentSelfService:  studentauth.NewService(dbpool),
		StudentCookieSecure: false,
		AppOrigin:           "",
		InstituteTZ:         "Asia/Bangkok",
	})
	return mux
}

func postJSON(t *testing.T, mux *http.ServeMux, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	return recorder
}

// A student with no phone on file can enroll one from the form: the OTP goes
// to the client-provided number and the response still carries only the mask.
func TestParentVerificationSendEnrollsClientPhoneWhenNoneOnFile(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	suffix := uuid.NewString()[:8]
	wcode := seedStudentWithoutParentPhone(t, dbpool, q, "ENROLL"+suffix)
	lookupToken := lookupTokenForPendingStudent(t, dbpool, wcode)

	const hmacKey = "test-hmac-key-parent-verify"
	t.Setenv("OTP_HMAC_KEY", hmacKey)
	otpSvc, err := otp.NewService(dbpool, hmacKey)
	if err != nil {
		t.Fatal(err)
	}
	mux := enrollmentTestServer(t, dbpool, q, otpSvc)

	recorder := postJSON(t, mux, "/api/v1/absences/parent-verification/send",
		`{"lookup_token":"`+lookupToken+`","parent_phone":"0899998888"}`)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 200/202; body = %s", recorder.Code, recorder.Body.String())
	}
	var got struct {
		ParentPhone string `json:"parent_phone"`
		Token       string `json:"token"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ParentPhone != "••••8888" {
		t.Fatalf("parent_phone = %q, want masked %q", got.ParentPhone, "••••8888")
	}
	if strings.Contains(recorder.Body.String(), "+66899998888") || strings.Contains(recorder.Body.String(), "0899998888") {
		t.Fatalf("send response leaked the raw enrolled phone: %s", recorder.Body.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var sessionPhone string
	if err := dbpool.QueryRow(ctx, `
		SELECT parent_phone FROM student_parent_verification_sessions
		WHERE wcode = $1 ORDER BY created_at DESC LIMIT 1
	`, wcode).Scan(&sessionPhone); err != nil {
		t.Fatalf("load OTP session phone: %v", err)
	}
	if sessionPhone != "+66899998888" {
		t.Fatalf("OTP session phone = %q, want the normalized enrolled number", sessionPhone)
	}
}

// An enrolled (or staff-managed) phone always wins: a client-provided value
// is ignored once one exists on file.
func TestParentVerificationSendIgnoresClientPhoneWhenPhoneOnFile(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	suffix := uuid.NewString()[:8]
	// seedParentVerificationTestData stores parent_phone 0812345678.
	wcode := seedParentVerificationTestData(t, dbpool, q, "ONFILE"+suffix)
	lookupToken := lookupTokenForPendingStudent(t, dbpool, wcode)

	const hmacKey = "test-hmac-key-parent-verify"
	t.Setenv("OTP_HMAC_KEY", hmacKey)
	otpSvc, err := otp.NewService(dbpool, hmacKey)
	if err != nil {
		t.Fatal(err)
	}
	mux := enrollmentTestServer(t, dbpool, q, otpSvc)

	recorder := postJSON(t, mux, "/api/v1/absences/parent-verification/send",
		`{"lookup_token":"`+lookupToken+`","parent_phone":"0899998888"}`)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 200/202; body = %s", recorder.Code, recorder.Body.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var studentPhone string
	if err := dbpool.QueryRow(ctx, `SELECT parent_phone FROM students WHERE wcode = $1`, wcode).Scan(&studentPhone); err != nil {
		t.Fatalf("load student phone: %v", err)
	}
	if studentPhone != "0812345678" {
		t.Fatalf("student phone = %q, want the staff-managed number untouched", studentPhone)
	}
}

// The enrolled phone is persisted only after the OTP proves it is reachable,
// and only when the student has none on file.
func TestParentVerificationVerifyPersistsEnrolledPhoneOnce(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	suffix := uuid.NewString()[:8]
	wcode := seedStudentWithoutParentPhone(t, dbpool, q, "PERSIST"+suffix)

	const hmacKey = "test-hmac-key-parent-verify"
	t.Setenv("OTP_HMAC_KEY", hmacKey)
	otpSvc, err := otp.NewService(dbpool, hmacKey)
	if err != nil {
		t.Fatal(err)
	}
	mux := enrollmentTestServer(t, dbpool, q, otpSvc)

	code, token, err := otpSvc.StartSession(context.Background(), wcode, "+66899998888")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	recorder := postJSON(t, mux, "/api/v1/absences/parent-verification/verify",
		`{"token":"`+token+`","code":"`+code+`"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", recorder.Code, recorder.Body.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var persisted string
	if err := dbpool.QueryRow(ctx, `SELECT parent_phone FROM students WHERE wcode = $1`, wcode).Scan(&persisted); err != nil {
		t.Fatalf("load student phone after verify: %v", err)
	}
	if persisted != "+66899998888" {
		t.Fatalf("persisted phone = %q, want the OTP-verified enrolled number", persisted)
	}

	// A staff-managed number can never be displaced by a later enrollment.
	if _, err := dbpool.Exec(ctx, `UPDATE students SET parent_phone = '+66811112222' WHERE wcode = $1`, wcode); err != nil {
		t.Fatalf("install staff-managed phone: %v", err)
	}
	code2, token2, err := otpSvc.StartSession(context.Background(), wcode, "+66899998888")
	if err != nil {
		t.Fatalf("second StartSession: %v", err)
	}
	recorder = postJSON(t, mux, "/api/v1/absences/parent-verification/verify",
		`{"token":"`+token2+`","code":"`+code2+`"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("second verify status = %d, want 200; body = %s", recorder.Code, recorder.Body.String())
	}
	if err := dbpool.QueryRow(ctx, `SELECT parent_phone FROM students WHERE wcode = $1`, wcode).Scan(&persisted); err != nil {
		t.Fatalf("reload student phone: %v", err)
	}
	if persisted != "+66811112222" {
		t.Fatalf("phone after second enrollment = %q, want the staff-managed number untouched", persisted)
	}
}

// A form-provided nickname fills an empty student record once and rides along
// as the absence snapshot; once set it can never be replaced from the form.
func TestBatchSubmissionNicknameFillsEmptyRecordAndSnapshots(t *testing.T) {
	fixture := newPublicAbsenceContractFixture(t)

	request := batchAbsenceCreateRequest{
		Nickname: contractStringPtr("Bird"),
		Items:    []batchAbsenceCreateItem{fixture.validItem(0)},
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal batch request: %v", err)
	}
	recorder := fixture.submitBatch(body, uuid.NewString())
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", recorder.Code, recorder.Body.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var studentNickname *string
	if err := fixture.pool.QueryRow(ctx, `SELECT nickname FROM students WHERE wcode = $1`, fixture.wcode).Scan(&studentNickname); err != nil {
		t.Fatalf("load student nickname: %v", err)
	}
	if studentNickname == nil || *studentNickname != "Bird" {
		t.Fatalf("students.nickname = %v, want Bird", studentNickname)
	}
	var absenceNickname *string
	if err := fixture.pool.QueryRow(ctx, `
		SELECT student_nickname FROM student_absences WHERE wcode = $1 LIMIT 1
	`, fixture.wcode).Scan(&absenceNickname); err != nil {
		t.Fatalf("load absence nickname snapshot: %v", err)
	}
	if absenceNickname == nil || *absenceNickname != "Bird" {
		t.Fatalf("absence snapshot student_nickname = %v, want Bird", absenceNickname)
	}

	// The nickname now exists, so a second submission cannot replace it.
	fixture.reverify(t)
	second, err := json.Marshal(batchAbsenceCreateRequest{
		Nickname: contractStringPtr("Hacker"),
		Items:    []batchAbsenceCreateItem{fixture.validItem(1)},
	})
	if err != nil {
		t.Fatalf("marshal second batch request: %v", err)
	}
	recorder = fixture.submitBatch(second, uuid.NewString())
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", recorder.Code, recorder.Body.String())
	}
	if got := decodePublicContractError(t, recorder); got.Code != "bad_nickname" {
		t.Fatalf("error code = %q, want bad_nickname", got.Code)
	}
	if err := fixture.pool.QueryRow(ctx, `SELECT nickname FROM students WHERE wcode = $1`, fixture.wcode).Scan(&studentNickname); err != nil {
		t.Fatalf("reload student nickname: %v", err)
	}
	if studentNickname == nil || *studentNickname != "Bird" {
		t.Fatalf("students.nickname after rejected replay = %v, want Bird", studentNickname)
	}
}
