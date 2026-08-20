package studentauth

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type recordingTx struct {
	pgx.Tx
	query string
	args  []any
}

func (tx *recordingTx) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	tx.query = query
	tx.args = args
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func TestCreateSessionTxBindsSessionToVerifiedOTP(t *testing.T) {
	now := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	service := &Service{now: func() time.Time { return now }}
	tx := &recordingTx{}

	_, _, err := service.CreateSessionTx(context.Background(), tx, " W250389 ", uuid.New())
	if err != nil {
		t.Fatalf("CreateSessionTx returned error: %v", err)
	}
	query := strings.ToLower(tx.query)
	if !strings.Contains(query, "student_parent_verification_sessions") {
		t.Fatalf("session insert does not bind to the OTP session: %s", tx.query)
	}
	if !strings.Contains(query, "status = 'verified'") {
		t.Fatalf("session insert does not require a verified OTP session: %s", tx.query)
	}
	if len(tx.args) < 4 {
		t.Fatalf("session insert args = %d, want verification session binding", len(tx.args))
	}
}

func TestCreateSessionTxStoresOnlySessionTokenHash(t *testing.T) {
	service := &Service{now: time.Now}
	tx := &recordingTx{}

	rawToken, _, err := service.CreateSessionTx(context.Background(), tx, "w250389", uuid.New())
	if err != nil {
		t.Fatalf("CreateSessionTx returned error: %v", err)
	}
	if rawToken == "" {
		t.Fatal("CreateSessionTx returned an empty bearer token")
	}
	if len(tx.args) < 2 {
		t.Fatalf("session insert args = %d, want token hash", len(tx.args))
	}
	hash, ok := tx.args[1].([]byte)
	if !ok {
		t.Fatalf("token hash argument type = %T, want []byte", tx.args[1])
	}
	if len(hash) != sha256.Size {
		t.Fatalf("token hash length = %d, want %d", len(hash), sha256.Size)
	}
	if string(hash) == rawToken {
		t.Fatal("raw session token was passed to persistence")
	}
}

func TestSetSessionCookieUsesHttpOnlyStrictHostCookieWhenSecure(t *testing.T) {
	recorder := httptest.NewRecorder()
	now := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)

	SetSessionCookie(recorder, "opaque-token", true, now)

	cookie := recorder.Result().Cookies()[0]
	if cookie.Name != "__Host-wi-student-session" {
		t.Fatalf("cookie name = %q, want __Host-wi-student-session", cookie.Name)
	}
	if cookie.Value != "opaque-token" {
		t.Fatalf("cookie value = %q, want opaque token", cookie.Value)
	}
	if !cookie.HttpOnly || !cookie.Secure {
		t.Fatalf("cookie flags = HttpOnly:%t Secure:%t, want both true", cookie.HttpOnly, cookie.Secure)
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie SameSite = %v, want Strict", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Fatalf("cookie Path = %q, want /", cookie.Path)
	}
}
