package config

import (
	"errors"
	"os"
	"strings"

	"warwick-institute/internal/clientip"
)

type Config struct {
	Addr                string
	DatabaseURL         string
	RealtimeDatabaseURL string
	AuthPepper          string
	CookieSecure        bool
	TrustedProxyCIDRs   string
	StaticDir           string
	LogLevel            string
	LegacySyncLogLevel  string
	InstituteTZ         string
	InstituteName       string

	CRMBaseURL  string
	CRMUsername string
	CRMPassword string

	LegacySyncURL      string
	LegacySyncUsername string
	LegacySyncPassword string

	SMSServiceBaseURL  string
	SMSServiceUsername string
	SMSServicePassword string

	OTPHMACKey                string
	OTPSMSProvider            string
	OTPAsyncDeliveryEnabled   bool
	OTPDeliveryEncryptionKeys string
	AppOrigin                 string
	EmailWebhookURL           string
	EmailWebhookSecret        string
	EmailReminderEnabled      bool
	EmailReminderTime         string
}

func FromEnv() (Config, error) {
	var cfg Config
	cfg.Addr = envOr("ADDR", ":8080")
	cfg.StaticDir = envOr("STATIC_DIR", "../dist")
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	cfg.RealtimeDatabaseURL = strings.TrimSpace(os.Getenv("REALTIME_DATABASE_URL"))
	cfg.AuthPepper = os.Getenv("AUTH_PEPPER")
	cfg.CookieSecure = envBoolOr("COOKIE_SECURE", true)
	cfg.TrustedProxyCIDRs = strings.TrimSpace(os.Getenv("TRUSTED_PROXY_CIDRS"))
	cfg.LogLevel = envOr("LOG_LEVEL", "info")
	cfg.LegacySyncLogLevel = envOr("LEGACY_SYNC_LOG_LEVEL", "warn")
	cfg.InstituteTZ = envOr("INSTITUTE_TZ", "Asia/Bangkok")
	cfg.InstituteName = envOr("INSTITUTE_NAME", "Warwick Institute")
	cfg.CRMBaseURL = envOr("CRM_BASE_URL", "")
	cfg.CRMUsername = os.Getenv("CRM_USERNAME")
	cfg.CRMPassword = os.Getenv("CRM_PASSWORD")

	cfg.LegacySyncURL = envOr("LEGACY_SYNC_URL", "https://warwick.azurewebsites.net")
	cfg.LegacySyncUsername = os.Getenv("LEGACY_SYNC_USERNAME")
	cfg.LegacySyncPassword = os.Getenv("LEGACY_SYNC_PASSWORD")
	cfg.SMSServiceBaseURL = envOr("SMS_SERVICE_BASE_URL", "")
	cfg.SMSServiceUsername = os.Getenv("SMS_SERVICE_USERNAME")
	cfg.SMSServicePassword = os.Getenv("SMS_SERVICE_PASSWORD")
	cfg.OTPHMACKey = os.Getenv("OTP_HMAC_KEY")
	cfg.OTPSMSProvider = strings.ToLower(strings.TrimSpace(os.Getenv("OTP_SMS_PROVIDER")))
	cfg.OTPAsyncDeliveryEnabled = envBoolOr("OTP_ASYNC_DELIVERY_ENABLED", false)
	cfg.OTPDeliveryEncryptionKeys = strings.TrimSpace(os.Getenv("OTP_DELIVERY_ENCRYPTION_KEYS"))
	cfg.AppOrigin = strings.TrimSpace(os.Getenv("APP_ORIGIN"))
	cfg.EmailWebhookURL = strings.TrimSpace(os.Getenv("INSTITUTE_EMAIL_WEBHOOK_URL"))
	cfg.EmailWebhookSecret = strings.TrimSpace(os.Getenv("INSTITUTE_EMAIL_WEBHOOK_SECRET"))
	cfg.EmailReminderEnabled = os.Getenv("EMAIL_REMINDER_ENABLED") == "true"
	cfg.EmailReminderTime = envOr("EMAIL_REMINDER_TIME", "08:00")

	if cfg.OTPSMSProvider == "" {
		if cfg.SMSServiceUsername != "" && cfg.SMSServicePassword != "" {
			cfg.OTPSMSProvider = "smartsms"
		} else {
			cfg.OTPSMSProvider = "mock"
		}
	}

	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if (appEnv == "production" || appEnv == "prod") && !cfg.CookieSecure {
		return Config{}, errors.New("COOKIE_SECURE=false is forbidden in production")
	}
	if (appEnv == "production" || appEnv == "prod") &&
		(strings.TrimSpace(os.Getenv("ADMIN_USERNAME")) != "" || strings.TrimSpace(os.Getenv("ADMIN_PASSWORD")) != "") {
		return Config{}, errors.New("ADMIN_USERNAME/ADMIN_PASSWORD seed configuration is forbidden in production")
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.AuthPepper == "" {
		return Config{}, errors.New("AUTH_PEPPER is required")
	}
	if cfg.OTPHMACKey == "" {
		return Config{}, errors.New("OTP_HMAC_KEY is required")
	}
	if cfg.OTPAsyncDeliveryEnabled && cfg.OTPDeliveryEncryptionKeys == "" {
		return Config{}, errors.New("OTP_DELIVERY_ENCRYPTION_KEYS is required when OTP_ASYNC_DELIVERY_ENABLED=true")
	}

	if _, err := clientip.NewResolver(cfg.TrustedProxyCIDRs); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBoolOr(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "":
		return fallback
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
