package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"warwick-institute/internal/devseed"
	"warwick-institute/internal/pg"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if appEnv == "production" || appEnv == "prod" {
		log.Error("admin seeding is unavailable in production")
		os.Exit(2)
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		log.Error("DATABASE_URL is required")
		os.Exit(2)
	}
	username := strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))
	password := os.Getenv("ADMIN_PASSWORD")
	pepper := os.Getenv("AUTH_PEPPER")
	if username == "" || strings.TrimSpace(password) == "" || strings.TrimSpace(pepper) == "" {
		log.Error("ADMIN_USERNAME, ADMIN_PASSWORD, and AUTH_PEPPER are required")
		os.Exit(2)
	}

	pool, err := pg.NewPool(context.Background(), databaseURL)
	if err != nil {
		log.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := devseed.EnsureAdmin(context.Background(), log, pool, devseed.EnsureAdminParams{
		Username: username,
		Password: password,
		Pepper:   pepper,
	}); err != nil {
		log.Error("ensure admin user", "error", err)
		os.Exit(1)
	}
}
