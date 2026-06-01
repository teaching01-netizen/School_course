package smartsms

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// testCfg returns a Config suitable for httptest (localhost bypass).
func testCfg(baseURL string, extra ...string) Config {
	cfg := Config{BaseURL: baseURL, Username: "u", Password: "p", skipBaseURLValidation: true}
	if len(extra) >= 1 {
		cfg.Username = extra[0]
	}
	if len(extra) >= 2 {
		cfg.Password = extra[1]
	}
	return cfg
}

// ---------------------------------------------------------------------------
// Config tests
// ---------------------------------------------------------------------------

func TestNew_RequiresUsername(t *testing.T) {
	_, err := New(Config{Password: "pass", skipBaseURLValidation: true})
	if err == nil {
		t.Fatal("expected error for empty username")
	}
}

func TestNew_RequiresPassword(t *testing.T) {
	_, err := New(Config{Username: "user", skipBaseURLValidation: true})
	if err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestNew_Defaults(t *testing.T) {
	c, err := New(Config{Username: "u", Password: "p", skipBaseURLValidation: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.baseURL != defaultBaseURL {
		t.Fatalf("baseURL = %q, want %q", c.baseURL, defaultBaseURL)
	}
	if c.sender != defaultSender {
		t.Fatalf("sender = %q, want %q", c.sender, defaultSender)
	}
	if c.label != defaultLabel {
		t.Fatalf("label = %q, want %q", c.label, defaultLabel)
	}
	if c.httpClient.Timeout != defaultTimeout {
		t.Fatalf("timeout = %v, want %v", c.httpClient.Timeout, defaultTimeout)
	}
}

func TestNew_RejectsLocalhost(t *testing.T) {
	_, err := New(Config{BaseURL: "http://localhost:8080", Username: "u", Password: "p"})
	if err == nil {
		t.Fatal("expected error for localhost base URL")
	}
}

func TestNew_RejectsNoScheme(t *testing.T) {
	_, err := New(Config{BaseURL: "smcp.sc4msg.com", Username: "u", Password: "p"})
	if err == nil {
		t.Fatal("expected error for base URL without scheme")
	}
}

// ---------------------------------------------------------------------------
// CSRF extraction tests
// ---------------------------------------------------------------------------

func TestExtractCSRF_StandardInput(t *testing.T) {
	html := `<input type="hidden" name="_token" value="abc123">`
	got := extractCSRF(html)
	if got != "abc123" {
		t.Fatalf("extractCSRF = %q, want %q", got, "abc123")
	}
}

func TestExtractCSRF_ValueBeforeName(t *testing.T) {
	html := `<input value="xyz789" name="_token" type="hidden">`
	got := extractCSRF(html)
	if got != "xyz789" {
		t.Fatalf("extractCSRF = %q, want %q", got, "xyz789")
	}
}

func TestExtractCSRF_SingleQuotes(t *testing.T) {
	html := `<input type='hidden' name='_token' value='single-quoted'>`
	got := extractCSRF(html)
	if got != "single-quoted" {
		t.Fatalf("extractCSRF = %q, want %q", got, "single-quoted")
	}
}

func TestExtractCSRF_NoToken(t *testing.T) {
	html := `<html><body>no token here</body></html>`
	got := extractCSRF(html)
	if got != "" {
		t.Fatalf("extractCSRF = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// Phone normalization tests
// ---------------------------------------------------------------------------

func TestNormalizeMobile_ThaiFormat(t *testing.T) {
	got := normalizeMobile("0812345678")
	if got != "0812345678" {
		t.Fatalf("normalizeMobile = %q, want %q", got, "0812345678")
	}
}

func TestNormalizeMobile_WithCountryCode(t *testing.T) {
	got := normalizeMobile("+66812345678")
	if got != "0812345678" {
		t.Fatalf("normalizeMobile = %q, want %q", got, "0812345678")
	}
}

func TestNormalizeMobile_With0066(t *testing.T) {
	got := normalizeMobile("0066812345678")
	if got != "0812345678" {
		t.Fatalf("normalizeMobile = %q, want %q", got, "0812345678")
	}
}

func TestNormalizeMobile_Bare66(t *testing.T) {
	got := normalizeMobile("66812345678")
	if got != "0812345678" {
		t.Fatalf("normalizeMobile = %q, want %q", got, "0812345678")
	}
}

func TestNormalizeMobile_WithHyphens(t *testing.T) {
	got := normalizeMobile("081-234-5678")
	if got != "0812345678" {
		t.Fatalf("normalizeMobile = %q, want %q", got, "0812345678")
	}
}

func TestNormalizeMobile_WithParentheses(t *testing.T) {
	got := normalizeMobile("(081) 234 5678")
	if got != "0812345678" {
		t.Fatalf("normalizeMobile = %q, want %q", got, "0812345678")
	}
}

func TestNormalizeMobile_Empty(t *testing.T) {
	got := normalizeMobile("")
	if got != "" {
		t.Fatalf("normalizeMobile = %q, want empty", got)
	}
}

func TestNormalizeMobile_InvalidLength(t *testing.T) {
	got := normalizeMobile("081234")
	if got != "" {
		t.Fatalf("normalizeMobile = %q, want empty for short number", got)
	}
}

func TestNormalizeMobiles_DropsInvalid(t *testing.T) {
	valid, dropped := normalizeMobiles([]string{"0812345678", "abc", "+66898765432", "short"})
	if len(valid) != 2 {
		t.Fatalf("valid = %v, want 2 numbers", valid)
	}
	if len(dropped) != 2 {
		t.Fatalf("dropped = %v, want 2 numbers", dropped)
	}
	if dropped[0] != "abc" || dropped[1] != "short" {
		t.Fatalf("dropped = %v, want [abc short]", dropped)
	}
}

// ---------------------------------------------------------------------------
// isLoginPage tests
// ---------------------------------------------------------------------------

func TestIsLoginPage_DetectsLogin(t *testing.T) {
	html := `<html><body><form><input name="email" type="text"><input type="password"></form></body></html>`
	if !isLoginPage(html) {
		t.Fatal("expected isLoginPage to return true")
	}
}

func TestIsLoginPage_DetectsNonLogin(t *testing.T) {
	html := `<html><body><h1>Dashboard</h1></body></html>`
	if isLoginPage(html) {
		t.Fatal("expected isLoginPage to return false")
	}
}

// ---------------------------------------------------------------------------
// Login tests
// ---------------------------------------------------------------------------

func TestLogin_Success(t *testing.T) {
	const csrfToken = "test-csrf-token-abc"
	var loginCalled bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><body><form><input type="hidden" name="_token" value="` + csrfToken + `"></form></body></html>`))
		case "/login":
			loginCalled = true
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse form: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if r.FormValue("email") != "user@test.com" {
				t.Errorf("email = %q, want %q", r.FormValue("email"), "user@test.com")
			}
			if r.FormValue("password") != "secret" {
				t.Errorf("password = %q, want %q", r.FormValue("password"), "secret")
			}
			if r.FormValue("_token") != csrfToken {
				t.Errorf("_token = %q, want %q", r.FormValue("_token"), csrfToken)
			}
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL, "user@test.com", "secret"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !c.LoggedIn() {
		t.Fatal("expected LoggedIn() = true after login")
	}
	if !loginCalled {
		t.Fatal("expected /login to be called")
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><body><form><input type="hidden" name="_token" value="tok"></form></body></html>`))
		case "/login":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><body><form><input name="email" type="text"><input type="password"></form></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL, "bad", "creds"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.Login(context.Background()); err == nil {
		t.Fatal("expected error for invalid credentials")
	}
}

func TestLogin_PostLoginCSRFMissing(t *testing.T) {
	var getCnt int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			getCnt++
			w.Header().Set("Content-Type", "text/html")
			if getCnt == 1 {
				w.Write([]byte(`<form><input name="_token" value="initial"></form>`))
			} else {
				// Post-login: page has no CSRF token
				w.Write([]byte(`<html><body>no form</body></html>`))
			}
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = c.Login(context.Background())
	if err == nil {
		t.Fatal("expected error when post-login CSRF extraction fails")
	}
	if !strings.Contains(err.Error(), "CSRF token") {
		t.Fatalf("error should mention CSRF token, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SendSMS tests
// ---------------------------------------------------------------------------

func TestSendSMS_3StepFlow(t *testing.T) {
	var calls []string
	const csrfToken = "send-csrf"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="` + csrfToken + `"></form>`))
		case "/login":
			calls = append(calls, "login")
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			calls = append(calls, "previewData")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":               true,
				"preview_id":            "prev-001",
				"number_of_used_credit": 3,
				"correct":               2,
				"error_duplicate":       1,
			})
		case "/dataset/confirmSend":
			calls = append(calls, "confirmSend")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/send/confirmSendSMS":
			calls = append(calls, "confirmSendSMS")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
		_ = body
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN001",
		Campaign:   "Test Campaign",
		Message:    "Hello from test",
		Mobiles:    []string{"0812345678", "+66898765432"},
	})
	if err != nil {
		t.Fatalf("SendSMS: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected Success = true")
	}
	if resp.CreditsUsed != 3 {
		t.Fatalf("CreditsUsed = %d, want 3", resp.CreditsUsed)
	}
	if resp.CorrectCount != 2 {
		t.Fatalf("CorrectCount = %d, want 2", resp.CorrectCount)
	}
	if resp.ErrorCounts["error_duplicate"] != 1 {
		t.Fatalf("ErrorCounts[error_duplicate] = %d, want 1", resp.ErrorCounts["error_duplicate"])
	}

	expected := []string{"login", "previewData", "confirmSend", "confirmSendSMS"}
	if len(calls) != len(expected) {
		t.Fatalf("calls = %v, want %v", calls, expected)
	}
	for i, want := range expected {
		if calls[i] != want {
			t.Fatalf("calls[%d] = %q, want %q", i, calls[i], want)
		}
	}
}

func TestSendSMS_Step1Fails(t *testing.T) {
	var step2Called, step3Called bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"correct": 0,
			})
		case "/dataset/confirmSend":
			step2Called = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/send/confirmSendSMS":
			step3Called = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN002",
		Campaign:   "Fail Test",
		Message:    "This should fail",
		Mobiles:    []string{"0812345678"},
	})
	if err == nil {
		t.Fatal("expected error when step 1 fails")
	}
	if step2Called {
		t.Fatal("step 2 should not be called after step 1 failure")
	}
	if step3Called {
		t.Fatal("step 3 should not be called after step 1 failure")
	}
}

func TestSendSMS_NoMobiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN003",
		Campaign:   "No Numbers",
		Message:    "test",
		Mobiles:    []string{},
	})
	if err == nil {
		t.Fatal("expected error for empty mobiles")
	}
}

func TestSendSMS_SessionExpired_ReLogin(t *testing.T) {
	var loginCount int
	var step1Calls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			loginCount++
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			step1Calls++
			if step1Calls == 1 {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(`<html><body><form><input name="email" type="text"></form></body></html>`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":               true,
				"preview_id":            "prev-retry",
				"number_of_used_credit": 1,
				"correct":               1,
			})
		case "/dataset/confirmSend":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/send/confirmSendSMS":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN004",
		Campaign:   "ReLogin Test",
		Message:    "retry test",
		Mobiles:    []string{"0812345678"},
	})
	if err != nil {
		t.Fatalf("SendSMS: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected Success = true after re-login")
	}
	if loginCount < 2 {
		t.Fatalf("expected at least 2 login calls (initial + re-login), got %d", loginCount)
	}
}

func TestSendOTP_Step1HTTP419_ReLogin(t *testing.T) {
	const csrfToken = "otp-csrf"
	var loginCount int
	var previewCount int
	var confirmCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="` + csrfToken + `"></form>`))
		case "/login":
			loginCount++
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			previewCount++
			if previewCount == 1 {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(419)
				w.Write([]byte(`<!DOCTYPE html><html lang="en"><head><title>Page Expired</title></head><body>Page Expired</body></html>`))
				return
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse preview form: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if got := r.FormValue("_token"); got != csrfToken {
				t.Errorf("previewData _token = %q, want %q", got, csrfToken)
			}
			if got := r.FormValue("mobile"); got != "0812345678" {
				t.Errorf("previewData mobile = %q, want %q", got, "0812345678")
			}
			if got := r.FormValue("ref_no"); got != "otp" {
				t.Errorf("previewData ref_no = %q, want otp", got)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":               true,
				"preview_id":            "prev-otp",
				"number_of_used_credit": 1,
				"correct":               1,
			})
		case "/dataset/confirmSend":
			confirmCount++
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse confirm form: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if got := r.FormValue("_token"); got != csrfToken {
				t.Errorf("confirmSend _token = %q, want %q", got, csrfToken)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = c.SendOTP(context.Background(), "+66812345678", "123456", "otp message")
	if err != nil {
		t.Fatalf("SendOTP: %v", err)
	}
	if loginCount < 2 {
		t.Fatalf("expected at least 2 login calls (initial + re-login), got %d", loginCount)
	}
	if previewCount != 2 {
		t.Fatalf("expected 2 previewData calls (retry after 419), got %d", previewCount)
	}
	if confirmCount != 1 {
		t.Fatalf("expected 1 confirmSend call, got %d", confirmCount)
	}
}

func TestSendOTP_Step1HTTP401_ReLogin(t *testing.T) {
	const csrfToken = "otp-csrf-401"
	var loginCount int
	var previewCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="` + csrfToken + `"></form>`))
		case "/login":
			loginCount++
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			previewCount++
			if previewCount == 1 {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`<!DOCTYPE html><html><head><title>Unauthorized</title></head><body>Unauthorized</body></html>`))
				return
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse preview form: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if got := r.FormValue("_token"); got != csrfToken {
				t.Errorf("previewData _token = %q, want %q", got, csrfToken)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":               true,
				"preview_id":            "prev-otp-401",
				"number_of_used_credit": 1,
				"correct":               1,
			})
		case "/dataset/confirmSend":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse confirm form: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if got := r.FormValue("_token"); got != csrfToken {
				t.Errorf("confirmSend _token = %q, want %q", got, csrfToken)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = c.SendOTP(context.Background(), "+66812345678", "123456", "otp 401 test")
	if err != nil {
		t.Fatalf("SendOTP: %v", err)
	}
	if loginCount < 2 {
		t.Fatalf("expected at least 2 login calls (initial + re-login), got %d", loginCount)
	}
	if previewCount != 2 {
		t.Fatalf("expected 2 previewData calls (retry after 401), got %d", previewCount)
	}
}

func TestSendOTP_Step2HTTP419_ReLogin(t *testing.T) {
	const csrfToken = "otp-csrf-s2-419"
	var loginCount int
	var previewCount int
	var confirmCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="` + csrfToken + `"></form>`))
		case "/login":
			loginCount++
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			previewCount++
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse preview form: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":               true,
				"preview_id":            "prev-otp-s2-419",
				"number_of_used_credit": 1,
				"correct":               1,
			})
		case "/dataset/confirmSend":
			confirmCount++
			if confirmCount == 1 {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(419)
				w.Write([]byte(`<!DOCTYPE html><html lang="en"><head><title>Page Expired</title></head><body>Page Expired</body></html>`))
				return
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse confirm form: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if got := r.FormValue("_token"); got != csrfToken {
				t.Errorf("confirmSend _token = %q, want %q", got, csrfToken)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = c.SendOTP(context.Background(), "+66812345678", "123456", "otp step2 419 test")
	if err != nil {
		t.Fatalf("SendOTP: %v", err)
	}
	if loginCount < 2 {
		t.Fatalf("expected at least 2 login calls (initial + re-login), got %d", loginCount)
	}
	if previewCount != 1 {
		t.Fatalf("expected 1 previewData call, got %d", previewCount)
	}
	if confirmCount != 2 {
		t.Fatalf("expected 2 confirmSend calls (retry after 419), got %d", confirmCount)
	}
}

func TestSendOTP_ReLoginFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><body><form><input name="email" type="text"></form></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = c.SendOTP(context.Background(), "+66812345678", "123456", "otp relogin fail")
	if err == nil {
		t.Fatal("expected error when OTP re-login fails")
	}
	if !strings.Contains(err.Error(), "re-login") && !strings.Contains(err.Error(), "login page") {
		t.Fatalf("error should mention re-login/login page issue, got: %v", err)
	}
}

func TestSendSMS_ReLoginFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			// Always accept login (return redirect to sendsms)
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			// Keep returning the login page so the second attempt also fails.
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><body><form><input name="email" type="text"></form></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN-RLF",
		Campaign:   "ReLogin Fail",
		Message:    "test",
		Mobiles:    []string{"0812345678"},
	})
	if err == nil {
		t.Fatal("expected error when re-login fails")
	}
	// Error should mention the session issue
	if !strings.Contains(err.Error(), "session invalid") && !strings.Contains(err.Error(), "re-login") && !strings.Contains(err.Error(), "login page") {
		t.Fatalf("error should mention session/login issue, got: %v", err)
	}
}

func TestSendSMS_ScheduledTime(t *testing.T) {
	var receivedSendTime string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			r.ParseMultipartForm(1 << 20)
			receivedSendTime = r.FormValue("send_time")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":               true,
				"preview_id":            "prev-sched",
				"number_of_used_credit": 1,
				"correct":               1,
			})
		case "/dataset/confirmSend":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/send/confirmSendSMS":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN005",
		Campaign:   "Scheduled",
		Message:    "scheduled msg",
		Mobiles:    []string{"0812345678"},
	})
	if err != nil {
		t.Fatalf("SendSMS: %v", err)
	}
	if receivedSendTime != "" {
		t.Fatalf("expected empty send_time for immediate send, got %q", receivedSendTime)
	}
}

func TestSendSMS_WithSendTime(t *testing.T) {
	var receivedSendTime string
	sendAt := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			r.ParseMultipartForm(1 << 20)
			receivedSendTime = r.FormValue("send_time")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true, "preview_id": "p", "number_of_used_credit": 1, "correct": 1,
			})
		case "/dataset/confirmSend":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/send/confirmSendSMS":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN-ST",
		Campaign:   "Scheduled",
		Message:    "test",
		Mobiles:    []string{"0812345678"},
		SendTime:   &sendAt,
	})
	if err != nil {
		t.Fatalf("SendSMS: %v", err)
	}
	if receivedSendTime != "2026-06-15 10:30" {
		t.Fatalf("send_time = %q, want %q", receivedSendTime, "2026-06-15 10:30")
	}
}

// ---------------------------------------------------------------------------
// MockProvider tests
// ---------------------------------------------------------------------------

func TestMockProvider_SendSMS(t *testing.T) {
	p := &MockProvider{}
	resp, err := p.SendSMS(context.Background(), SendRequest{
		Mobiles: []string{"0812345678"},
		Message: "test",
	})
	if err != nil {
		t.Fatalf("MockProvider.SendSMS: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected Success = true")
	}
	if resp.CreditsUsed != 1 {
		t.Fatalf("CreditsUsed = %d, want 1", resp.CreditsUsed)
	}
}

func TestMockProvider_HealthCheck(t *testing.T) {
	p := &MockProvider{}
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatalf("MockProvider.HealthCheck: %v", err)
	}
}

func TestMockProvider_GetCredits(t *testing.T) {
	p := &MockProvider{}
	credits, err := p.GetCredits(context.Background())
	if err != nil {
		t.Fatalf("MockProvider.GetCredits: %v", err)
	}
	if credits != 9999 {
		t.Fatalf("credits = %d, want 9999", credits)
	}
}

// ---------------------------------------------------------------------------
// CSRF verification tests
// ---------------------------------------------------------------------------

func TestSendSMS_VerifiesCSRFInRequests(t *testing.T) {
	const csrfToken = "csrf-verify-123"
	var previewToken, confirmToken, sendToken string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="` + csrfToken + `"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			r.ParseMultipartForm(1 << 20)
			previewToken = r.FormValue("_token")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true, "preview_id": "prev-csrf", "number_of_used_credit": 1, "correct": 1,
			})
		case "/dataset/confirmSend":
			r.ParseMultipartForm(1 << 20)
			confirmToken = r.FormValue("_token")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/send/confirmSendSMS":
			r.ParseMultipartForm(1 << 20)
			sendToken = r.FormValue("_token")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN006",
		Campaign:   "CSRF Test",
		Message:    "csrf verify",
		Mobiles:    []string{"0812345678"},
	})
	if err != nil {
		t.Fatalf("SendSMS: %v", err)
	}

	if previewToken != csrfToken {
		t.Fatalf("previewData _token = %q, want %q", previewToken, csrfToken)
	}
	if confirmToken != csrfToken {
		t.Fatalf("confirmSend _token = %q, want %q", confirmToken, csrfToken)
	}
	if sendToken != csrfToken {
		t.Fatalf("confirmSendSMS _token = %q, want %q", sendToken, csrfToken)
	}
}

// ---------------------------------------------------------------------------
// Mobile normalization + sender tests
// ---------------------------------------------------------------------------

func TestSendSMS_MultipleMobilesJoinedWithNewlines(t *testing.T) {
	var receivedMobile string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			r.ParseMultipartForm(1 << 20)
			receivedMobile = r.FormValue("mobile")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true, "preview_id": "prev-multi", "number_of_used_credit": 3, "correct": 3,
			})
		case "/dataset/confirmSend":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/send/confirmSendSMS":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN007",
		Campaign:   "Multi Mobile",
		Message:    "multi test",
		Mobiles:    []string{"0811111111", "0822222222", "0833333333"},
	})
	if err != nil {
		t.Fatalf("SendSMS: %v", err)
	}

	expected := "0811111111\n0822222222\n0833333333"
	if receivedMobile != expected {
		t.Fatalf("mobile = %q, want %q", receivedMobile, expected)
	}
}

func TestSendSMS_SenderIsWarwick(t *testing.T) {
	var receivedSender string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			r.ParseMultipartForm(1 << 20)
			receivedSender = r.FormValue("sender")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true, "preview_id": "prev-sender", "number_of_used_credit": 1, "correct": 1,
			})
		case "/dataset/confirmSend":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/send/confirmSendSMS":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN008",
		Campaign:   "Sender Test",
		Message:    "sender verify",
		Mobiles:    []string{"0812345678"},
	})
	if err != nil {
		t.Fatalf("SendSMS: %v", err)
	}

	if receivedSender != "Warwick" {
		t.Fatalf("sender = %q, want %q", receivedSender, "Warwick")
	}
}

func TestSendSMS_VerifiesMobileNormalization(t *testing.T) {
	var receivedMobile string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			r.ParseMultipartForm(1 << 20)
			receivedMobile = r.FormValue("mobile")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true, "preview_id": "prev-norm", "number_of_used_credit": 1, "correct": 1,
			})
		case "/dataset/confirmSend":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/send/confirmSendSMS":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN009",
		Campaign:   "Norm Test",
		Message:    "normalize verify",
		Mobiles:    []string{"+6681-234-5678"},
	})
	if err != nil {
		t.Fatalf("SendSMS: %v", err)
	}

	if !strings.Contains(receivedMobile, "0812345678") {
		t.Fatalf("expected normalized mobile in request, got %q", receivedMobile)
	}
}

// ---------------------------------------------------------------------------
// Configurable sender/label tests
// ---------------------------------------------------------------------------

func TestConfigurableLabel(t *testing.T) {
	var receivedLabel string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			r.ParseMultipartForm(1 << 20)
			receivedLabel = r.FormValue("label")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true, "preview_id": "prev-lbl", "number_of_used_credit": 1, "correct": 1,
			})
		case "/dataset/confirmSend":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/send/confirmSendSMS":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := testCfg(srv.URL)
	cfg.Label = "MyCustom"
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN-LBL",
		Campaign:   "Label Test",
		Message:    "label verify",
		Mobiles:    []string{"0812345678"},
	})
	if err != nil {
		t.Fatalf("SendSMS: %v", err)
	}

	if receivedLabel != "MyCustom" {
		t.Fatalf("label = %q, want %q", receivedLabel, "MyCustom")
	}
}

func TestConfigurableSender(t *testing.T) {
	var receivedSender string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			r.ParseMultipartForm(1 << 20)
			receivedSender = r.FormValue("sender")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true, "preview_id": "prev-snd", "number_of_used_credit": 1, "correct": 1,
			})
		case "/dataset/confirmSend":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/send/confirmSendSMS":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := testCfg(srv.URL)
	cfg.Sender = "CustomSender"
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN-SND",
		Campaign:   "Sender Test",
		Message:    "sender verify",
		Mobiles:    []string{"0812345678"},
	})
	if err != nil {
		t.Fatalf("SendSMS: %v", err)
	}

	if receivedSender != "CustomSender" {
		t.Fatalf("sender = %q, want %q", receivedSender, "CustomSender")
	}
}

// ---------------------------------------------------------------------------
// Parser tests
// ---------------------------------------------------------------------------

func TestParsePreviewResponse_MalformedJSON(t *testing.T) {
	_, err := parsePreviewResponse([]byte(`not json at all`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParsePreviewResponse_MissingFields(t *testing.T) {
	resp, err := parsePreviewResponse([]byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success != false {
		t.Fatal("expected Success=false for missing field")
	}
	if resp.CreditsUsed != 0 {
		t.Fatalf("expected CreditsUsed=0, got %d", resp.CreditsUsed)
	}
}

func TestParsePreviewResponse_ErrorFields(t *testing.T) {
	body := `{"success":true,"preview_id":"p1","error_duplicate":5,"error_invalidmobile":2}`
	resp, err := parsePreviewResponse([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCounts["error_duplicate"] != 5 {
		t.Fatalf("error_duplicate = %d, want 5", resp.ErrorCounts["error_duplicate"])
	}
	if resp.ErrorCounts["error_invalidmobile"] != 2 {
		t.Fatalf("error_invalidmobile = %d, want 2", resp.ErrorCounts["error_invalidmobile"])
	}
}

func TestParsePreviewResponse_EmptyBody(t *testing.T) {
	_, err := parsePreviewResponse([]byte{})
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestParseSimpleSuccess_EmptyBody(t *testing.T) {
	_, err := parseSimpleSuccess([]byte{})
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

// ---------------------------------------------------------------------------
// Step failure isolation tests
// ---------------------------------------------------------------------------

func TestSendSMS_Step2Fails(t *testing.T) {
	var step3Called bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true, "preview_id": "prev-s2f", "number_of_used_credit": 1, "correct": 1,
			})
		case "/dataset/confirmSend":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "insufficient credit"})
		case "/send/confirmSendSMS":
			step3Called = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN-S2F",
		Campaign:   "Step2 Fail",
		Message:    "should fail at step 2",
		Mobiles:    []string{"0812345678"},
	})
	if err == nil {
		t.Fatal("expected error when step 2 fails")
	}
	if step3Called {
		t.Fatal("step 3 should not be called after step 2 failure")
	}
}

func TestSendSMS_Step3Fails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true, "preview_id": "prev-s3f", "number_of_used_credit": 1, "correct": 1,
			})
		case "/dataset/confirmSend":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/send/confirmSendSMS":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "dispatch failed"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN-S3F",
		Campaign:   "Step3 Fail",
		Message:    "should fail at step 3",
		Mobiles:    []string{"0812345678"},
	})
	if err == nil {
		t.Fatal("expected error when step 3 fails")
	}
}

func TestSendSMS_Server500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN-500",
		Campaign:   "Server Error",
		Message:    "500 test",
		Mobiles:    []string{"0812345678"},
	})
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error should mention status code 500, got: %v", err)
	}
}

func TestSendSMS_AllMobilesInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN-INV",
		Campaign:   "Invalid Numbers",
		Message:    "test",
		Mobiles:    []string{"abc", "123", "short"},
	})
	if err == nil {
		t.Fatal("expected error for all-invalid mobiles")
	}
}

// ---------------------------------------------------------------------------
// SSRF tests
// ---------------------------------------------------------------------------

func TestAbs_RejectsAbsoluteURL(t *testing.T) {
	c, _ := New(Config{BaseURL: "https://smcp.sc4msg.com", Username: "u", Password: "p", skipBaseURLValidation: true})

	tests := []struct {
		path    string
		want    string
		wantErr bool
	}{
		{"https://evil.com/steal", "", true},
		{"http://169.254.169.254/metadata", "", true},
		{"//evil.com/path", "", true},
		{"/sendsms", "https://smcp.sc4msg.com/sendsms", false},
	}
	for _, tt := range tests {
		got, err := c.abs(tt.path)
		if (err != nil) != tt.wantErr {
			t.Errorf("abs(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("abs(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Context cancellation tests
// ---------------------------------------------------------------------------

func TestSendSMS_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = c.SendSMS(ctx, SendRequest{
		CampaignNo: "CN-CTX",
		Campaign:   "Context Cancel",
		Message:    "test",
		Mobiles:    []string{"0812345678"},
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestLogin_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<form><input name="_token" value="tok"></form>`))
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = c.Login(ctx)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ---------------------------------------------------------------------------
// Network error tests
// ---------------------------------------------------------------------------

func TestSendSMS_NetworkError(t *testing.T) {
	c, err := New(Config{BaseURL: "http://127.0.0.1:1", Username: "u", Password: "p", skipBaseURLValidation: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.SendSMS(context.Background(), SendRequest{
		CampaignNo: "CN-NET",
		Campaign:   "Network Error",
		Message:    "test",
		Mobiles:    []string{"0812345678"},
	})
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestLogin_NetworkError(t *testing.T) {
	c, err := New(Config{BaseURL: "http://127.0.0.1:1", Username: "u", Password: "p", skipBaseURLValidation: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = c.Login(context.Background())
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

// ---------------------------------------------------------------------------
// Concurrent SendSMS test
// ---------------------------------------------------------------------------

func TestSendSMS_Concurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sendsms":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<form><input name="_token" value="concurrent-tok"></form>`))
		case "/login":
			http.Redirect(w, r, "/sendsms", http.StatusFound)
		case "/dataset/previewData":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true, "preview_id": "prev-conc", "number_of_used_credit": 1, "correct": 1,
			})
		case "/dataset/confirmSend":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/send/confirmSendSMS":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = c.SendSMS(context.Background(), SendRequest{
				CampaignNo: "CN-CONC",
				Campaign:   "Concurrent",
				Message:    "concurrent test",
				Mobiles:    []string{"0812345678"},
			})
		}(i)
	}

	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Errorf("goroutine %d: %v", i, e)
		}
	}
}

// ---------------------------------------------------------------------------
// HealthCheck tests
// ---------------------------------------------------------------------------

func TestHealthCheck_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}

func TestHealthCheck_NetworkError(t *testing.T) {
	c, err := New(Config{BaseURL: "http://127.0.0.1:1", Username: "u", Password: "p", skipBaseURLValidation: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.HealthCheck(context.Background()); err == nil {
		t.Fatal("expected error for network failure")
	}
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

func TestClient_SatisfiesSMSProvider(t *testing.T) {
	var _ SMSProvider = (*Client)(nil)
	var _ SMSProvider = (*MockProvider)(nil)
}

// ---------------------------------------------------------------------------
// expectJSON tests
// ---------------------------------------------------------------------------

func TestExpectJSON_Valid(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"object", `{"success":true}`},
		{"array", `[1,2,3]`},
		{"whitespace prefix", "  \n\t{\"ok\":true}"},
		{"BOM prefix", "\xEF\xBB\xBF{\"ok\":true}"},
		{"BOM + whitespace", "\xEF\xBB\xBF \n{\"ok\":true}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := expectJSON([]byte(tt.body), "test"); err != nil {
				t.Errorf("expectJSON(%q) = %v, want nil", tt.body, err)
			}
		})
	}
}

func TestExpectJSON_Invalid(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"html", `<html><body>error</body></html>`},
		{"empty", ``},
		{"whitespace only", `   `},
		{"plain text", `not json at all`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := expectJSON([]byte(tt.body), "test step"); err == nil {
				t.Errorf("expectJSON(%q) = nil, want error", tt.body)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HealthCheck login page test
// ---------------------------------------------------------------------------

func TestHealthCheck_LoginPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><form><input name="email" type="text"><input type="password"></form></body></html>`))
	}))
	defer srv.Close()

	c, err := New(testCfg(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = c.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error when health check returns login page")
	}
	if !strings.Contains(err.Error(), "login page") {
		t.Fatalf("error should mention login page, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// parseSimpleSuccess tests
// ---------------------------------------------------------------------------

func TestParseSimpleSuccess_MalformedJSON(t *testing.T) {
	_, err := parseSimpleSuccess([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseSimpleSuccess_SuccessFalse(t *testing.T) {
	resp, err := parseSimpleSuccess([]byte(`{"success":false}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Fatal("expected Success=false")
	}
}
