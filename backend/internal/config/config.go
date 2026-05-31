package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	Addr         string
	DatabaseURL  string
	AuthPepper  string
	StaticDir   string
	LogLevel     string
	InstituteTZ  string

	CRMBaseURL  string
	CRMUsername string
	CRMPassword string

	SMSServiceBaseURL string
	SMSServiceUsername string
	SMSServicePassword string

	OTPHMACKey    string
	OTPSMSProvider string
	AppOrigin     string
}

func FromEnv() (Config, error) {
	var cfg Config
	cfg.Addr = envOr("ADDR", ":8080")
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	cfg.AuthPepper = os.Getenv("AUTH_PEPPER")
	cfg.StaticDir = envOr("STATIC_DIR", "../dist")
	cfg.LogLevel = envOr("LOG_LEVEL", "info")
	cfg.InstituteTZ = envOr("INSTITUTE_TZ", "Asia/Bangkok")
	cfg.CRMBaseURL = envOr("CRM_BASE_URL", "")
	cfg.CRMUsername = os.Getenv("CRM_USERNAME")
	cfg.CRMPassword = os.Getenv("CRM_PASSWORD")
	cfg.SMSServiceBaseURL = envOr("SMS_SERVICE_BASE_URL", "")
	cfg.SMSServiceUsername = os.Getenv("SMS_SERVICE_USERNAME")
	cfg.SMSServicePassword = os.Getenv("SMS_SERVICE_PASSWORD")
	cfg.OTPHMACKey = os.Getenv("OTP_HMAC_KEY")
	cfg.OTPSMSProvider = strings.ToLower(strings.TrimSpace(os.Getenv("OTP_SMS_PROVIDER")))
	cfg.AppOrigin = strings.TrimSpace(os.Getenv("APP_ORIGIN"))

	if cfg.OTPSMSProvider == "" {
		if cfg.SMSServiceUsername != "" && cfg.SMSServicePassword != "" {
			cfg.OTPSMSProvider = "smartsms"
		} else {
			cfg.OTPSMSProvider = "mock"
		}
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

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
