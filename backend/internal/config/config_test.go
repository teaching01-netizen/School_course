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

func TestFromEnvReadsStaticDirectoryDefaultsAndOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("AUTH_PEPPER", "pepper")
	t.Setenv("OTP_HMAC_KEY", "otp-key")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.StaticDir != "../dist" {
		t.Fatalf("StaticDir = %q, want ../dist by default", cfg.StaticDir)
	}

	t.Setenv("STATIC_DIR", "/tmp/warwick-dist")
	cfg, err = FromEnv()
	if err != nil {
		t.Fatalf("FromEnv with STATIC_DIR: %v", err)
	}
	if cfg.StaticDir != "/tmp/warwick-dist" {
		t.Fatalf("StaticDir = %q, want override", cfg.StaticDir)
	}
}

func TestFromEnvLegacySyncLogLevelDefaultsToWarnAndReadsOverride(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("AUTH_PEPPER", "pepper")
	t.Setenv("OTP_HMAC_KEY", "otp-key")
	t.Setenv("LEGACY_SYNC_LOG_LEVEL", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.LegacySyncLogLevel != "warn" {
		t.Fatalf("LegacySyncLogLevel = %q, want warn by default", cfg.LegacySyncLogLevel)
	}

	t.Setenv("LEGACY_SYNC_LOG_LEVEL", "error")
	cfg, err = FromEnv()
	if err != nil {
		t.Fatalf("FromEnv with LEGACY_SYNC_LOG_LEVEL: %v", err)
	}
	if cfg.LegacySyncLogLevel != "error" {
		t.Fatalf("LegacySyncLogLevel = %q, want error override", cfg.LegacySyncLogLevel)
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
func TestFromEnvRejectsAdminSeedConfigurationInProduction(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("AUTH_PEPPER", "pepper")
	t.Setenv("OTP_HMAC_KEY", "otp-key")
	t.Setenv("APP_ENV", "production")
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "secret")

	if _, err := FromEnv(); err == nil {
		t.Fatal("FromEnv succeeded with production admin seed configuration")
	}
}
func TestFromEnvRejectsInsecureCookiesInProduction(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("AUTH_PEPPER", "pepper")
	t.Setenv("OTP_HMAC_KEY", "otp-key")
	t.Setenv("APP_ENV", "prod")
	t.Setenv("COOKIE_SECURE", "false")

	if _, err := FromEnv(); err == nil {
		t.Fatal("FromEnv succeeded with COOKIE_SECURE=false in production")
	}
}
func TestFromEnvRejectsAnyAdminSeedFieldInProductionAliases(t *testing.T) {
	tests := []struct {
		name     string
		appEnv   string
		username string
		password string
	}{
		{name: "prod username only", appEnv: "prod", username: "admin"},
		{name: "production password only", appEnv: "production", password: "secret"},
		{name: "production whitespace username with password", appEnv: "production", username: " \t", password: "secret"},
		{name: "production whitespace password with username", appEnv: "production", username: "admin", password: " \t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://example")
			t.Setenv("AUTH_PEPPER", "pepper")
			t.Setenv("OTP_HMAC_KEY", "otp-key")
			t.Setenv("APP_ENV", tt.appEnv)
			t.Setenv("ADMIN_USERNAME", tt.username)
			t.Setenv("ADMIN_PASSWORD", tt.password)

			if _, err := FromEnv(); err == nil {
				t.Fatal("FromEnv succeeded with production admin seed configuration")
			}
		})
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

func TestFromEnvDerivesOTPSMSProvider(t *testing.T) {
	baseEnv := func(t *testing.T) {
		t.Helper()
		t.Setenv("DATABASE_URL", "postgres://example")
		t.Setenv("AUTH_PEPPER", "pepper")
		t.Setenv("OTP_HMAC_KEY", "otp-key")
		t.Setenv("OTP_SMS_PROVIDER", "")
		t.Setenv("SMS_SERVICE_USERNAME", "")
		t.Setenv("SMS_SERVICE_PASSWORD", "")
	}

	t.Run("creds set with empty provider derives smartsms", func(t *testing.T) {
		baseEnv(t)
		t.Setenv("SMS_SERVICE_USERNAME", "user01@warwick")
		t.Setenv("SMS_SERVICE_PASSWORD", "secret")

		cfg, err := FromEnv()
		if err != nil {
			t.Fatalf("FromEnv: %v", err)
		}
		if cfg.OTPSMSProvider != "smartsms" {
			t.Fatalf("OTPSMSProvider = %q, want derived smartsms", cfg.OTPSMSProvider)
		}
	})

	t.Run("explicit provider wins over creds", func(t *testing.T) {
		baseEnv(t)
		t.Setenv("SMS_SERVICE_USERNAME", "user01@warwick")
		t.Setenv("SMS_SERVICE_PASSWORD", "secret")
		t.Setenv("OTP_SMS_PROVIDER", "twilio")

		cfg, err := FromEnv()
		if err != nil {
			t.Fatalf("FromEnv: %v", err)
		}
		if cfg.OTPSMSProvider != "twilio" {
			t.Fatalf("OTPSMSProvider = %q, want explicit twilio", cfg.OTPSMSProvider)
		}
	})

	t.Run("no creds falls back to mock", func(t *testing.T) {
		baseEnv(t)

		cfg, err := FromEnv()
		if err != nil {
			t.Fatalf("FromEnv: %v", err)
		}
		if cfg.OTPSMSProvider != "mock" {
			t.Fatalf("OTPSMSProvider = %q, want mock fallback", cfg.OTPSMSProvider)
		}
	})
}
