package config

import (
	"errors"
	"os"
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

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.AuthPepper == "" {
		return Config{}, errors.New("AUTH_PEPPER is required")
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
