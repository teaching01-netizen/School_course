# SMS OTP Absence Confirmation — Parent Verification via Mock SMS

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a student submits an absence form, send an SMS OTP to the parent's phone number for confirmation. The parent receives a 6-digit code and enters it to verify the absence. The SMS provider is a deterministic mock (phone-prefix routing) since the real provider does not offer an API.

**Architecture:** Add `parent_phone` column to `students` table (backfilled from `crm_rows`). Build an `sms` package with a `Provider` interface and a deterministic mock implementation. Implement HMAC-signed OTP tokens (stateless, 24-hour TTL) — no OTP database table needed. Add HTTP endpoints for OTP send/verify. Extend the absence form with a request history + OTP input step.

**Tech Stack:** PostgreSQL/goose migration, Go `net/http`/pgx/sqlc, HMAC-SHA256 (Go `crypto/hmac` + `crypto/sha256`), React 19/Vite/TypeScript/Tailwind.

---

## File Map

- `backend/db/migrations/00025_sms_otp_parent_phone.sql`: add `parent_phone` to `students`, create `absence_otp_requests` table
- `backend/db/queries/students.sql`: add `parent_phone` to all student queries
- `backend/internal/sms/provider.go`: `Provider` interface definition
- `backend/internal/sms/mock.go`: deterministic mock implementation (phone-prefix routing)
- `backend/internal/sms/mock_test.go`: mock determinism + behavior tests
- `backend/internal/otp/service.go`: HMAC token generation + verification
- `backend/internal/otp/service_test.go`: OTP service tests (generate, verify, expiry, tamper)
- `backend/internal/config/config.go`: add `OTPKey`, `SMSProvider` env vars
- `backend/internal/httpapi/absenceshttp/routes.go`: register new OTP endpoints
- `backend/internal/httpapi/absenceshttp/otp_routes.go`: OTP send/verify HTTP handlers
- `backend/internal/httpapi/absenceshttp/otp_routes_test.go`: endpoint validation tests
- `src/types/index.ts`: add `OtpRequest`, `AbsenceWithOtp` types
- `src/api/client.ts`: add OTP API helpers
- `src/pages/AbsenceForm.tsx`: add step 5 (OTP request history + code input)
- `src/pages/__tests__/AbsenceForm.test.tsx`: extend tests for OTP flow

---

### Task 1: Database Migration — parent_phone + OTP tracking

**Files:**
- Create: `backend/db/migrations/00025_sms_otp_parent_phone.sql`

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up

-- Add parent_phone to students table
ALTER TABLE students ADD COLUMN parent_phone text NULL;

-- Backfill from crm_rows where wcode matches
UPDATE students s
SET parent_phone = c.parent_phone
FROM crm_rows c
WHERE s.wcode = c.wcode
  AND c.parent_phone IS NOT NULL
  AND c.parent_phone != '';

-- Index for OTP lookups by absence_id
CREATE INDEX idx_absence_otp_requests_absence_id ON absence_otp_requests(absence_id);

-- +goose Down

DROP TABLE IF EXISTS absence_otp_requests;
ALTER TABLE students DROP COLUMN IF EXISTS parent_phone;
```

- [ ] **Step 2: Run the migration locally**

Run: `cd backend && make migrate`
Expected: migration 00025 applied successfully

- [ ] **Step 3: Verify backfill**

Run: `psql $DATABASE_URL -c "SELECT wcode, parent_phone FROM students WHERE parent_phone IS NOT NULL LIMIT 5;"`
Expected: rows with parent_phone populated from crm_rows

---

### Task 2: Update sqlc queries — parent_phone on students

**Files:**
- Modify: `backend/db/queries/students.sql`
- Run: `cd backend && make sqlc` (regenerates Go code)

- [ ] **Step 1: Update all student queries to include parent_phone**

```sql
-- name: StudentCreate :one
INSERT INTO students (wcode, full_name, notes, parent_phone)
VALUES ($1, $2, $3, $4)
RETURNING id, wcode, full_name, notes, parent_phone, created_at, updated_at;

-- name: StudentGetByID :one
SELECT id, wcode, full_name, notes, parent_phone, created_at, updated_at
FROM students
WHERE id = $1;

-- name: StudentGetByWCode :one
SELECT id, wcode, full_name, notes, parent_phone, created_at, updated_at
FROM students
WHERE wcode = $1;

-- name: StudentList :many
SELECT id, wcode, full_name, notes, parent_phone, created_at, updated_at
FROM students
ORDER BY wcode ASC;

-- name: StudentUpdate :one
UPDATE students
SET wcode = $2, full_name = $3, notes = $4, parent_phone = $5, updated_at = now()
WHERE id = $1
RETURNING id, wcode, full_name, notes, parent_phone, created_at, updated_at;

-- name: StudentUpsertNameByWCode :one
INSERT INTO students (wcode, full_name, notes, parent_phone)
VALUES ($1, $2, '', $3)
ON CONFLICT (wcode) DO UPDATE
SET full_name = EXCLUDED.full_name,
    parent_phone = COALESCE(EXCLUDED.parent_phone, students.parent_phone),
    updated_at = now()
RETURNING id, wcode, full_name, notes, parent_phone, created_at, updated_at;

-- name: StudentGetParentPhoneByWCode :one
SELECT parent_phone
FROM students
WHERE wcode = $1;
```

- [ ] **Step 2: Regenerate sqlc code**

Run: `cd backend && make sqlc`
Expected: `internal/db/models.go` and `internal/db/students.sql.go` updated with `ParentPhone pgtype.Text`

- [ ] **Step 3: Fix any compilation errors**

Run: `cd backend && go build ./...`
Expected: compiles (existing callers may need `parent_phone` param added to `StudentCreate` and `StudentUpdate` calls — search for usages and add `pgtype.Text{}` for the new param)

---

### Task 3: SMS Provider interface + deterministic mock

**Files:**
- Create: `backend/internal/sms/provider.go`
- Create: `backend/internal/sms/mock.go`
- Create: `backend/internal/sms/mock_test.go`

- [ ] **Step 1: Define the Provider interface**

```go
package sms

// SendResult holds the outcome of an SMS send attempt.
type SendResult struct {
	MessageID string
	Status    string // "sent", "failed", "timeout"
}

// Provider is the contract for SMS delivery.
type Provider interface {
	SendOTP(phone string, code string) (SendResult, error)
}
```

- [ ] **Step 2: Write the deterministic mock**

```go
package sms

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"
)

// MockProvider routes behavior by phone number prefix.
//
// Prefix rules (checked in order):
//   - +66 → success (Thailand)
//   - +44 → timeout error (UK)
//   - +1  → 500 error (US)
//   - +81 → success (Japan)
//   - any other → success
//
// The mock logs the OTP code to stdout for dev/test use.
// MessageID is deterministic: derived from phone hash.
type MockProvider struct {
	Logger *slog.Logger
}

func NewMockProvider(logger *slog.Logger) *MockProvider {
	return &MockProvider{Logger: logger}
}

func (m *MockProvider) SendOTP(phone string, code string) (SendResult, error) {
	normalized := strings.TrimSpace(phone)

	// Log the OTP for dev/test visibility
	if m.Logger != nil {
		m.Logger.Info("SMS OTP sent (mock)",
			"phone", normalized,
			"code", code,
		)
	} else {
		fmt.Printf("[SMS MOCK] phone=%s code=%s\n", normalized, code)
	}

	// Deterministic message ID from phone hash
	h := sha256.Sum256([]byte(normalized))
	msgID := fmt.Sprintf("mock-%x", h[:8])

	// Route by prefix
	switch {
	case strings.HasPrefix(normalized, "+66"):
		return SendResult{MessageID: msgID, Status: "sent"}, nil
	case strings.HasPrefix(normalized, "+44"):
		return SendResult{}, fmt.Errorf("sms timeout: provider unreachable for %s", normalized)
	case strings.HasPrefix(normalized, "+1"):
		return SendResult{}, fmt.Errorf("sms provider error 500 for %s", normalized)
	case strings.HasPrefix(normalized, "+81"):
		return SendResult{MessageID: msgID, Status: "sent"}, nil
	default:
		return SendResult{MessageID: msgID, Status: "sent"}, nil
	}
}
```

- [ ] **Step 3: Write mock tests**

```go
package sms

import (
	"strings"
	"testing"
)

func TestMockDeterminism(t *testing.T) {
	m := NewMockProvider(nil)

	// Same phone → same MessageID, same error
	r1, err1 := m.SendOTP("+66812345678", "123456")
	r2, err2 := m.SendOTP("+66812345678", "654321")

	if r1.MessageID != r2.MessageID {
		t.Errorf("expected deterministic MessageID, got %s vs %s", r1.MessageID, r2.MessageID)
	}
	if (err1 == nil) != (err2 == nil) {
		t.Errorf("expected same error pattern, got %v vs %v", err1, err2)
	}
}

func TestMockPrefixRouting(t *testing.T) {
	m := NewMockProvider(nil)

	tests := []struct {
		phone    string
		wantSent bool
	}{
		{"+66812345678", true},   // Thailand → success
		{"+447911123456", false}, // UK → timeout
		{"+12025551234", false},  // US → 500
		{"+81901234567", true},   // Japan → success
		{"+61412345678", true},   // Australia → default success
	}

	for _, tt := range tests {
		r, err := m.SendOTP(tt.phone, "123456")
		if tt.wantSent && err != nil {
			t.Errorf("phone %s: expected success, got error: %v", tt.phone, err)
		}
		if !tt.wantSent && err == nil {
			t.Errorf("phone %s: expected error, got success", tt.phone)
		}
		if tt.wantSent && r.Status != "sent" {
			t.Errorf("phone %s: expected status 'sent', got '%s'", tt.phone, r.Status)
		}
	}
}

func TestMockMessageIDFormat(t *testing.T) {
	m := NewMockProvider(nil)
	r, err := m.SendOTP("+66812345678", "123456")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.MessageID, "mock-") {
		t.Errorf("expected mock- prefix on MessageID, got %s", r.MessageID)
	}
}
```

- [ ] **Step 4: Run mock tests**

Run: `cd backend && go test ./internal/sms/ -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
cd backend && git add internal/sms/ && git commit -m "feat(sms): add Provider interface and deterministic mock"
```

---

### Task 4: OTP service — HMAC-signed tokens

**Files:**
- Create: `backend/internal/otp/service.go`
- Create: `backend/internal/otp/service_test.go`

- [ ] **Step 1: Implement the OTP service**

```go
package otp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"
)

var (
	ErrExpired  = errors.New("otp token expired")
	ErrInvalid  = errors.New("otp code invalid")
	ErrTampered = errors.New("otp token tampered")
)

// Token is the signed payload embedded in the OTP token.
type Token struct {
	AbsenceID string `json:"absence_id"`
	Phone     string `json:"phone"`
	CodeHash  string `json:"code_hash"` // SHA256 hex of the 6-digit code
	IssuedAt  int64  `json:"issued_at"`
	ExpiresAt int64  `json:"expires_at"`
}

// SignedToken is the base64-encoded token + HMAC signature.
type SignedToken struct {
	Payload   string `json:"payload"`   // base64url(JSON(Token))
	Signature string `json:"signature"` // hex(HMAC-SHA256(payload, key))
}

// Service generates and verifies HMAC-signed OTP tokens.
type Service struct {
	key []byte
	ttl time.Duration
}

// NewService creates an OTP service with the given HMAC key and TTL.
func NewService(key []byte, ttl time.Duration) *Service {
	return &Service{key: key, ttl: ttl}
}

// Generate creates a 6-digit code and returns it along with the signed token.
func (s *Service) Generate(absenceID, phone string) (code string, token SignedToken, err error) {
	code = fmt.Sprintf("%06d", rand.Intn(1000000))

	now := time.Now().UTC()
	t := Token{
		AbsenceID: absenceID,
		Phone:     phone,
		CodeHash:  hashCode(code),
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(s.ttl).Unix(),
	}

	payloadBytes, err := json.Marshal(t)
	if err != nil {
		return "", SignedToken{}, fmt.Errorf("marshal token: %w", err)
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadBytes)
	sig := hmacSign(payloadB64, s.key)

	return code, SignedToken{
		Payload:   payloadB64,
		Signature: sig,
	}, nil
}

// Verify checks the submitted code against the signed token.
// Returns the Token payload on success.
func (s *Service) Verify(st SignedToken, submittedCode string) (Token, error) {
	// Verify HMAC
	if !hmac.Equal([]byte(hmacSign(st.Payload, s.key)), []byte(st.Signature)) {
		return Token{}, ErrTampered
	}

	// Decode payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(st.Payload)
	if err != nil {
		return Token{}, ErrTampered
	}

	var t Token
	if err := json.Unmarshal(payloadBytes, &t); err != nil {
		return Token{}, ErrTampered
	}

	// Check expiry
	if time.Now().UTC().Unix() > t.ExpiresAt {
		return t, ErrExpired
	}

	// Check code
	if hashCode(submittedCode) != t.CodeHash {
		return t, ErrInvalid
	}

	return t, nil
}

func hashCode(code string) string {
	h := sha256.Sum256([]byte(code))
	return fmt.Sprintf("%x", h)
}

func hmacSign(payload string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil))
}
```

- [ ] **Step 2: Write OTP service tests**

```go
package otp

import (
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"
)

func TestGenerateAndVerify(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	svc := NewService(key, 24*time.Hour)

	code, token, err := svc.Generate("absence-123", "+66812345678")
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Errorf("expected 6-digit code, got %q", code)
	}

	// Verify with correct code
	parsed, err := svc.Verify(token, code)
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
	if parsed.AbsenceID != "absence-123" {
		t.Errorf("expected absence_id=absence-123, got %q", parsed.AbsenceID)
	}
	if parsed.Phone != "+66812345678" {
		t.Errorf("expected phone=+66812345678, got %q", parsed.Phone)
	}
}

func TestVerifyWrongCode(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	svc := NewService(key, 24*time.Hour)
	_, token, _ := svc.Generate("absence-123", "+66812345678")

	_, err := svc.Verify(token, "000000")
	if err != ErrInvalid {
		t.Errorf("expected ErrInvalid, got %v", err)
	}
}

func TestVerifyExpired(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	svc := NewService(key, 1*time.Millisecond) // 1ms TTL

	code, token, _ := svc.Generate("absence-123", "+66812345678")
	time.Sleep(5 * time.Millisecond)

	_, err := svc.Verify(token, code)
	if err != ErrExpired {
		t.Errorf("expected ErrExpired, got %v", err)
	}
}

func TestVerifyTamperedPayload(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	svc := NewService(key, 24*time.Hour)
	_, token, _ := svc.Generate("absence-123", "+66812345678")

	// Tamper with payload
	tampered := token
	tampered.Payload = base64.RawURLEncoding.EncodeToString([]byte(`{"absence_id":"evil","phone":"x","code_hash":"x","issued_at":0,"expires_at":9999999999}`))

	_, err := svc.Verify(tampered, "123456")
	if err != ErrTampered {
		t.Errorf("expected ErrTampered, got %v", err)
	}
}

func TestVerifyDifferentKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	rand.Read(key1)
	rand.Read(key2)

	svc1 := NewService(key1, 24*time.Hour)
	svc2 := NewService(key2, 24*time.Hour)

	code, token, _ := svc1.Generate("absence-123", "+66812345678")

	// Verify with wrong key
	_, err := svc2.Verify(token, code)
	if err != ErrTampered {
		t.Errorf("expected ErrTampered with different key, got %v", err)
	}
}

func TestCodeIsRandom(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	svc := NewService(key, 24*time.Hour)

	codes := map[string]bool{}
	for i := 0; i < 100; i++ {
		code, _, _ := svc.Generate("a", "+66812345678")
		if codes[code] {
			t.Errorf("duplicate code %q on run %d", code, i)
		}
		codes[code] = true
	}
}
```

- [ ] **Step 3: Run OTP tests**

Run: `cd backend && go test ./internal/otp/ -v`
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
cd backend && git add internal/otp/ && git commit -m "feat(otp): HMAC-signed token generation and verification"
```

---

### Task 5: Config additions

**Files:**
- Modify: `backend/internal/config/config.go`

- [ ] **Step 1: Add OTP and SMS config fields**

Add to the `Config` struct and `FromEnv()`:

```go
type Config struct {
	// ... existing fields ...

	OTPKey     string // HMAC key for OTP tokens (32+ bytes hex)
	OTPTTL     string // OTP token TTL, e.g. "24h"
	SMSProvider string // "mock" for deterministic mock
}

func FromEnv() (Config, error) {
	// ... existing code ...

	cfg.OTPKey = envOr("OTP_KEY", "dev-otp-key-change-in-production-32bytes")
	cfg.OTPTTL = envOr("OTP_TTL", "24h")
	cfg.SMSProvider = envOr("SMS_PROVIDER", "mock")

	// ... existing validation ...
}
```

- [ ] **Step 2: Add env vars to .env.example**

```
OTP_KEY=dev-otp-key-change-in-production-32bytes
OTP_TTL=24h
SMS_PROVIDER=mock
```

- [ ] **Step 3: Verify compilation**

Run: `cd backend && go build ./...`
Expected: compiles clean

- [ ] **Step 4: Commit**

```bash
cd backend && git add internal/config/config.go .env.example && git commit -m "feat(config): add OTP_KEY, OTP_TTL, SMS_PROVIDER env vars"
```

---

### Task 6: Wire deps — SMS provider + OTP service into Deps

**Files:**
- Modify: `backend/internal/httpapi/httpdeps/deps.go`
- Modify: `backend/cmd/server/main.go`

- [ ] **Step 1: Add SMS and OTP to Deps**

```go
package httpdeps

import (
	// ... existing imports ...
	"warwick-institute/internal/otp"
	"warwick-institute/internal/sms"
)

type Deps struct {
	// ... existing fields ...
	SMS  sms.Provider
	OTP  *otp.Service
}
```

- [ ] **Step 2: Wire in main.go**

In `cmd/server/main.go`, after existing service initialization, add:

```go
import (
	// ... existing imports ...
	"warwick-institute/internal/otp"
	"warwick-institute/internal/sms"
)

// After config is loaded:
var smsProvider sms.Provider
switch cfg.SMSProvider {
case "mock":
	smsProvider = sms.NewMockProvider(log)
default:
	smsProvider = sms.NewMockProvider(log) // fallback to mock
}

otpKey := []byte(cfg.OTPKey)
otpTTL, _ := time.ParseDuration(cfg.OTPTTL)
otpSvc := otp.NewService(otpKey, otpTTL)

// Add to deps:
deps.SMS = smsProvider
deps.OTP = otpSvc
```

- [ ] **Step 3: Verify compilation**

Run: `cd backend && go build ./cmd/server`
Expected: compiles clean

- [ ] **Step 4: Commit**

```bash
cd backend && git add internal/httpapi/httpdeps/deps.go cmd/server/main.go && git commit -m "feat(deps): wire SMS provider and OTP service into HTTP deps"
```

---

### Task 7: OTP HTTP handlers — send + verify

**Files:**
- Create: `backend/internal/httpapi/absenceshttp/otp_routes.go`
- Create: `backend/internal/httpapi/absenceshttp/otp_routes_test.go`
- Modify: `backend/internal/httpapi/absenceshttp/routes.go` (register new routes)

- [ ] **Step 1: Write OTP route handlers**

```go
package absenceshttp

import (
	"encoding/json"
	"net/http"
	"time"

	"warwick-institute/internal/httpapi/httpadapter"
)

// OTP request/response types
type otpSendRequest struct {
	AbsenceID string `json:"absence_id"`
}

type otpSendResponse struct {
	Token   string `json:"token"`   // JSON-serialized SignedToken
	Expires string `json:"expires"` // RFC3339 expiry time
	Masked  string `json:"masked"`  // masked phone: +66****5678
}

type otpVerifyRequest struct {
	Token string `json:"token"` // JSON-serialized SignedToken
	Code  string `json:"code"`  // 6-digit code
}

type otpVerifyResponse struct {
	Valid     bool   `json:"valid"`
	AbsenceID string `json:"absence_id"`
	Phone     string `json:"phone"`
}

// POST /api/v1/absences/otp/send
func (s *server) handleOtpSend(w http.ResponseWriter, r *http.Request) {
	var body otpSendRequest
	if err := s.a.DecodeJSON(r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.AbsenceID == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "missing_absence_id", "absence_id is required")
		return
	}

	// Look up absence to get wcode
	absenceID, err := s.a.ParseUUID(body.AbsenceID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_absence_id", "Invalid absence_id")
		return
	}

	absence, err := s.deps.Q.GetAbsenceByID(r.Context(), absenceID)
	if err != nil {
		s.a.WriteErr(w, http.StatusNotFound, "not_found", "Absence not found")
		return
	}

	// Look up parent phone from students table
	phone, err := s.deps.Q.StudentGetParentPhoneByWCode(r.Context(), absence.Wcode)
	if err != nil || phone.String == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "no_parent_phone", "No parent phone number on file for this student")
		return
	}

	// Generate OTP
	code, signedToken, err := s.deps.OTP.Generate(body.AbsenceID, phone.String)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "otp_gen_failed", "Failed to generate OTP")
		return
	}

	// Send via SMS provider
	result, err := s.deps.SMS.SendOTP(phone.String, code)
	if err != nil {
		s.deps.Log.Error("sms send failed", "error", err, "phone", phone.String)
		s.a.WriteErr(w, http.StatusBadGateway, "sms_failed", "Failed to send SMS")
		return
	}
	s.deps.Log.Info("otp sent", "absence_id", body.AbsenceID, "phone", phone.String, "message_id", result.MessageID)

	// Mask phone for response: +66****5678
	masked := maskPhone(phone.String)

	// Serialize token as JSON string for client storage
	tokenJSON, _ := json.Marshal(signedToken)

	s.a.WriteJSON(w, http.StatusOK, otpSendResponse{
		Token:   string(tokenJSON),
		Expires: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		Masked:  masked,
	})
}

// POST /api/v1/absences/otp/verify
func (s *server) handleOtpVerify(w http.ResponseWriter, r *http.Request) {
	var body otpVerifyRequest
	if err := s.a.DecodeJSON(r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.Token == "" || body.Code == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "missing_fields", "token and code are required")
		return
	}

	// Deserialize token
	var signedToken otp.SignedToken
	if err := json.Unmarshal([]byte(body.Token), &signedToken); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_token", "Invalid token format")
		return
	}

	// Verify
	parsed, err := s.deps.OTP.Verify(signedToken, body.Code)
	if err != nil {
		switch err {
		case otp.ErrExpired:
			s.a.WriteErr(w, http.StatusGone, "otp_expired", "OTP code has expired. Please request a new one.")
		case otp.ErrInvalid:
			s.a.WriteErr(w, http.StatusBadRequest, "otp_invalid", "Incorrect OTP code")
		case otp.ErrTampered:
			s.a.WriteErr(w, http.StatusBadRequest, "otp_tampered", "Token has been tampered with")
		default:
			s.a.WriteErr(w, http.StatusInternalServerError, "otp_error", "Verification failed")
		}
		return
	}

	s.a.WriteJSON(w, http.StatusOK, otpVerifyResponse{
		Valid:     true,
		AbsenceID: parsed.AbsenceID,
		Phone:     parsed.Phone,
	})
}

func maskPhone(phone string) string {
	if len(phone) <= 4 {
		return phone
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}
```

- [ ] **Step 2: Register routes in routes.go**

Add to the `Register` function in `routes.go`:

```go
// OTP endpoints for absence parent confirmation
mux.HandleFunc("POST /api/v1/absences/otp/send", s.handleOtpSend)
mux.HandleFunc("POST /api/v1/absences/otp/verify", s.handleOtpVerify)
```

- [ ] **Step 3: Write handler tests**

```go
package absenceshttp

import (
	"testing"
)

func TestMaskPhone(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"+66812345678", "+66****5678"},
		{"+12025551234", "+12****1234"},
		{"123", "123"},
		{"+44", "+44"},
	}

	for _, tt := range tests {
		got := maskPhone(tt.input)
		if got != tt.want {
			t.Errorf("maskPhone(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd backend && go build ./cmd/server`
Expected: compiles clean

- [ ] **Step 5: Commit**

```bash
cd backend && git add internal/httpapi/absenceshttp/otp_routes.go internal/httpapi/absenceshttp/otp_routes_test.go internal/httpapi/absenceshttp/routes.go && git commit -m "feat(http): add OTP send/verify endpoints"
```

---

### Task 8: Frontend types + API helpers

**Files:**
- Modify: `src/types/index.ts`
- Modify: `src/api/client.ts`

- [ ] **Step 1: Add OTP types to types/index.ts**

```typescript
export type OtpSendResponse = {
  token: string;
  expires: string;
  masked: string; // masked phone: +66****5678
};

export type OtpVerifyResponse = {
  valid: boolean;
  absence_id: string;
  phone: string;
};

export type OtpRequestStatus = "pending" | "verified" | "expired";

export type OtpRequestRecord = {
  id: string;
  absence_id: string;
  masked_phone: string;
  status: OtpRequestStatus;
  sent_at: string;
  expires_at: string;
  token?: string; // stored for re-verification
};
```

- [ ] **Step 2: Add API helpers to client.ts**

```typescript
export async function sendOtp(absenceId: string): Promise<OtpSendResponse> {
  return apiJson("POST", "/api/v1/absences/otp/send", { absence_id: absenceId });
}

export async function verifyOtp(token: string, code: string): Promise<OtpVerifyResponse> {
  return apiJson("POST", "/api/v1/absences/otp/verify", { token, code });
}
```

- [ ] **Step 3: Verify TypeScript compiles**

Run: `npm run typecheck`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add src/types/index.ts src/api/client.ts && git commit -m "feat(frontend): add OTP types and API helpers"
```

---

### Task 9: Absence form — add OTP step (step 5)

**Files:**
- Modify: `src/pages/AbsenceForm.tsx`

- [ ] **Step 1: Add OTP state and step 5 to the wizard**

Add to the state variables after the existing ones:

```typescript
const [otpRequests, setOtpRequests] = useState<OtpRequestRecord[]>([]);
const [otpCode, setOtpCode] = useState("");
const [otpToken, setOtpToken] = useState<string | null>(null);
const [otpSending, setOtpSending] = useState(false);
const [otpVerifying, setOtpVerifying] = useState(false);
const [otpError, setOtpError] = useState("");
const [otpVerified, setOtpVerified] = useState(false);
```

Update `STEPS`:

```typescript
const STEPS = ["Student", "Subject & Dates", "Sit-in Plan", "Confirm", "Parent OTP"];
```

- [ ] **Step 2: Add OTP request history + input UI (step 4 in zero-indexed)**

After the Confirm step block, add:

```tsx
{/* Step 5: Parent OTP */}
{step === 4 ? (
  <div className="p-4 space-y-4">
    <h2 className="text-sm font-semibold text-gray-700">Parent Confirmation</h2>
    <p className="text-xs text-gray-500">
      An SMS will be sent to your parent's phone for confirmation.
    </p>

    {/* OTP Request History */}
    {otpRequests.length > 0 && (
      <div className="space-y-2">
        <h3 className="text-xs font-medium text-gray-600">Request History</h3>
        {otpRequests.map((req) => (
          <div key={req.id} className="flex items-center justify-between rounded-sm border border-gray-100 bg-gray-50 px-3 py-2">
            <div className="text-xs">
              <span className="font-medium">{req.masked_phone}</span>
              <span className="ml-2 text-gray-500">
                {new Date(req.sent_at).toLocaleString("en-GB", { hour: "2-digit", minute: "2-digit" })}
              </span>
            </div>
            <span className={`text-xs font-medium ${
              req.status === "verified" ? "text-green-600" :
              req.status === "expired" ? "text-red-500" :
              "text-amber-600"
            }`}>
              {req.status === "verified" ? "✓ Verified" :
               req.status === "expired" ? "Expired" :
               "Pending"}
            </span>
          </div>
        ))}
      </div>
    )}

    {/* OTP Code Input */}
    {!otpVerified && (
      <div className="space-y-2">
        <div className="flex gap-2">
          <input
            type="text"
            value={otpCode}
            onChange={(e) => {
              const val = e.target.value.replace(/\D/g, "").slice(0, 6);
              setOtpCode(val);
              setOtpError("");
            }}
            placeholder="Enter 6-digit code"
            className="flex-1 rounded-sm border border-gray-300 px-3 py-2 text-sm font-mono tracking-widest"
            maxLength={6}
            inputMode="numeric"
          />
          <Button
            variant="secondary"
            size="md"
            loading={otpVerifying}
            disabled={otpCode.length !== 6}
            onClick={async () => {
              if (!otpToken) {
                setOtpError("No OTP token. Please request a new code.");
                return;
              }
              setOtpVerifying(true);
              setOtpError("");
              try {
                const res = await verifyOtp(otpToken, otpCode);
                if (res.valid) {
                  setOtpVerified(true);
                  setOtpRequests((prev) =>
                    prev.map((r) => r.token === otpToken ? { ...r, status: "verified" as const } : r)
                  );
                }
              } catch (e: any) {
                const msg = e?.message || "Verification failed";
                if (msg.includes("expired")) {
                  setOtpError("Code expired. Please request a new one.");
                  setOtpRequests((prev) =>
                    prev.map((r) => r.token === otpToken ? { ...r, status: "expired" as const } : r)
                  );
                } else {
                  setOtpError(msg);
                }
              } finally {
                setOtpVerifying(false);
              }
            }}
          >
            Verify
          </Button>
        </div>
        {otpError && <p className="text-xs text-red-600">{otpError}</p>}
      </div>
    )}

    {otpVerified && (
      <div className="rounded-sm border border-green-200 bg-green-50 p-3 text-sm text-green-800">
        ✓ Parent confirmation verified
      </div>
    )}

    {/* Send OTP Button */}
    {!otpVerified && (
      <Button
        variant="primary"
        size="md"
        loading={otpSending}
        disabled={otpSending}
        onClick={async () => {
          setOtpSending(true);
          setOtpError("");
          try {
            const res = await sendOtp(absenceRes!.id);
            const newRequest: OtpRequestRecord = {
              id: crypto.randomUUID(),
              absence_id: absenceRes!.id,
              masked_phone: res.masked,
              status: "pending",
              sent_at: new Date().toISOString(),
              expires_at: res.expires,
              token: res.token,
            };
            setOtpRequests((prev) => [...prev, newRequest]);
            setOtpToken(res.token);
            setOtpCode("");
          } catch (e: any) {
            setOtpError(e?.message || "Failed to send OTP");
          } finally {
            setOtpSending(false);
          }
        }}
      >
        {otpRequests.length === 0 ? "Send OTP to Parent" : "Resend OTP"}
      </Button>
    )}
  </div>
) : null}
```

- [ ] **Step 3: Update navigation to handle step 5**

Update the navigation buttons to account for the new step. The Submit button now appears on step 4 (zero-indexed), and the OTP step is step 4. The "Next" button on step 3 (Confirm) should proceed to step 4 (OTP), and the "Submit Absence" button should be disabled until OTP is verified:

```tsx
{/* Navigation */}
<div className="flex items-center justify-between border-t border-gray-100 px-4 py-3">
  <div>
    {step > 0 ? (
      <Button variant="secondary" size="md" onClick={() => setStep((prev) => prev - 1)}>
        <ChevronLeft className="mr-1 h-4 w-4" /> Back
      </Button>
    ) : null}
  </div>
  <div>
    {step < STEPS.length - 1 ? (
      <Button variant="primary" size="md" disabled={!canProceedFromStep(step)} onClick={handleNext}>
        {step === 1 && !sitInResult ? "Check Availability" : "Next"} <ChevronRight className="ml-1 h-4 w-4" />
      </Button>
    ) : (
      <Button variant="primary" size="lg" loading={submitting} disabled={!otpVerified} onClick={() => void handleSubmit()}>
        {submitting ? "Submitting..." : "Submit Absence"}
      </Button>
    )}
  </div>
</div>
```

- [ ] **Step 4: Verify TypeScript compiles**

Run: `npm run typecheck`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add src/pages/AbsenceForm.tsx && git commit -m "feat(absence-form): add parent OTP confirmation step"
```

---

### Task 10: Verification and review

- [ ] **Step 1: Run backend tests**

Run: `cd backend && go test ./...`
Expected: all PASS

- [ ] **Step 2: Run backend linter**

Run: `cd backend && make lint`
Expected: no errors

- [ ] **Step 3: Run frontend tests**

Run: `npm test`
Expected: all PASS

- [ ] **Step 4: Run frontend typecheck + build**

Run: `npm run typecheck && npm run build`
Expected: no errors, build succeeds

- [ ] **Step 5: Manual smoke test**

1. Start dev: `cd backend && make dev` + `npm run dev`
2. Go to absence form, enter a wcode, complete steps 1-3
3. On step 4 (Confirm), submit → step 5 (Parent OTP) appears
4. Click "Send OTP to Parent" → mock logs code to console
5. Enter code → verify → green success state
6. Click "Submit Absence" → absence created

- [ ] **Step 6: Review checklist**

- [ ] HMAC key is configurable via env var (not hardcoded)
- [ ] Mock SMS logs OTP code for dev visibility
- [ ] Phone masking works (no full number in responses)
- [ ] Expired tokens return clear error message
- [ ] Tampered tokens are rejected
- [ ] parent_phone is backfilled from crm_rows
- [ ] No OTP database table needed (stateless HMAC)
