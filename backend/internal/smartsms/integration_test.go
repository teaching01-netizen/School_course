package smartsms

import (
	"context"
	"fmt"
	"os"
	"testing"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func liveCfg(t *testing.T) Config {
	t.Helper()
	baseURL := envOrDefault("SMS_SERVICE_BASE_URL", "https://smcp.sc4msg.com")
	username := os.Getenv("SMS_SERVICE_USERNAME")
	password := os.Getenv("SMS_SERVICE_PASSWORD")
	if username == "" || password == "" {
		t.Skip("SMS_SERVICE_USERNAME / SMS_SERVICE_PASSWORD not set; skipping integration test")
	}
	return Config{
		BaseURL:  baseURL,
		Username: username,
		Password: password,
	}
}

// TestIntegration_Login verifies login against the live SmartSMS site.
func TestIntegration_Login(t *testing.T) {
	c, err := New(liveCfg(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if !c.LoggedIn() {
		t.Fatal("expected LoggedIn() = true")
	}
	fmt.Println("LOGIN OK")
}

// TestIntegration_HealthCheck verifies health check after login.
func TestIntegration_HealthCheck(t *testing.T) {
	c, err := New(liveCfg(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := c.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	fmt.Println("HEALTHCHECK OK")
}

// TestIntegration_OTP_SendHappyPath sends a real OTP to verify the full flow.
func TestIntegration_OTP_SendHappyPath(t *testing.T) {
	c, err := New(liveCfg(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}

	err = c.SendOTP(context.Background(), "+66812345678", "999999", "Warwick Institute integration test OTP: 999999")
	if err != nil {
		t.Fatalf("SendOTP failed: %v", err)
	}
	fmt.Println("OTP SEND OK")
}

// TestIntegration_OTP_StaleCSRFReLogin logs in, corrupts the stored CSRF token
// so SmartSMS returns 419, then verifies the client re-logs in and retries.
func TestIntegration_OTP_StaleCSRFReLogin(t *testing.T) {
	c, err := New(liveCfg(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Corrupt the CSRF token — SmartSMS will reject with 419 or return login page.
	c.mu.Lock()
	c.csrfToken.Store("stale-integration-test-token")
	c.mu.Unlock()

	err = c.SendOTP(context.Background(), "+66812345678", "888888", "Warwick stale-CSRF re-login test: 888888")
	if err != nil {
		t.Fatalf("SendOTP with stale CSRF should re-login and succeed, got: %v", err)
	}
	fmt.Println("STALE CSRF RE-LOGIN OK")
}

// TestIntegration_OTP_ForcedReLogin forces loggedIn=false so ensureSession triggers
// a fresh login, then verifies OTP send succeeds.
func TestIntegration_OTP_ForcedReLogin(t *testing.T) {
	c, err := New(liveCfg(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Force session invalid — ensureSession will re-login.
	c.mu.Lock()
	c.loggedIn = false
	c.mu.Unlock()

	err = c.SendOTP(context.Background(), "+66812345678", "777777", "Warwick forced re-login test: 777777")
	if err != nil {
		t.Fatalf("SendOTP with forced re-login should succeed, got: %v", err)
	}
	fmt.Println("FORCED RE-LOGIN OK")
}
