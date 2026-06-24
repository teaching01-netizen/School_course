package config

import "testing"

func TestFromEnvCookieSecureDefaultTrue(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("AUTH_PEPPER", "pepper")
	t.Setenv("OTP_HMAC_KEY", "otp-key")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if !cfg.CookieSecure {
		t.Fatal("CookieSecure = false, want true by default")
	}
}

func TestFromEnvCookieSecureCanBeDisabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("AUTH_PEPPER", "pepper")
	t.Setenv("OTP_HMAC_KEY", "otp-key")
	t.Setenv("COOKIE_SECURE", "false")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.CookieSecure {
		t.Fatal("CookieSecure = true, want false when COOKIE_SECURE=false")
	}
}

func TestFromEnvReadsDedicatedRealtimeDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://primary")
	t.Setenv("REALTIME_DATABASE_URL", "postgres://session-capable")
	t.Setenv("AUTH_PEPPER", "pepper")
	t.Setenv("OTP_HMAC_KEY", "otp-key")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.RealtimeDatabaseURL != "postgres://session-capable" {
		t.Fatalf("RealtimeDatabaseURL = %q", cfg.RealtimeDatabaseURL)
	}
}

func TestFromEnvAsyncOTPDeliveryRequiresEncryptionKeys(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("AUTH_PEPPER", "pepper")
	t.Setenv("OTP_HMAC_KEY", "otp-key")
	t.Setenv("OTP_ASYNC_DELIVERY_ENABLED", "true")
	t.Setenv("OTP_DELIVERY_ENCRYPTION_KEYS", "")

	if _, err := FromEnv(); err == nil {
		t.Fatal("FromEnv succeeded, want missing OTP delivery encryption key error")
	}
}

func TestFromEnvAsyncOTPDeliveryDefaultsDisabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("AUTH_PEPPER", "pepper")
	t.Setenv("OTP_HMAC_KEY", "otp-key")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.OTPAsyncDeliveryEnabled {
		t.Fatal("OTPAsyncDeliveryEnabled = true, want false by default")
	}
}
