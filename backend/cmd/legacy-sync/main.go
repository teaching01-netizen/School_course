package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/config"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/legacysync"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cfg, err := config.FromEnv()
	if err != nil {
		log.Error("config", "error", err)
		os.Exit(1)
	}

	loc, err := time.LoadLocation(cfg.InstituteTZ)
	if err != nil {
		log.Error("timezone", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	q := sqldb.New(pool)

	client, err := legacysync.NewClient(cfg.LegacySyncURL, cfg.LegacySyncUsername, cfg.LegacySyncPassword)
	if err != nil {
		log.Error("legacy client", "error", err)
		os.Exit(1)
	}

	scraper := legacysync.NewScraper(client, pool, q, log, loc)

	rows, err := pool.Query(ctx, `SELECT id, legacy_course_id FROM courses WHERE legacy_course_id IS NOT NULL AND deleted_at IS NULL`)
	if err != nil {
		log.Error("query linked courses", "error", err)
		os.Exit(1)
	}
	defer rows.Close()

	type linkedCourse struct {
		ID             pgtype.UUID
		LegacyCourseID string
	}
	var courses []linkedCourse
	for rows.Next() {
		var c linkedCourse
		if err := rows.Scan(&c.ID, &c.LegacyCourseID); err != nil {
			log.Error("scan course", "error", err)
			continue
		}
		courses = append(courses, c)
	}

	if len(courses) == 0 {
		log.Info("no legacy-linked courses found")
		return
	}

	log.Info("starting legacy sync", "course_count", len(courses))

	for _, c := range courses {
		courseIDStr := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", c.ID.Bytes[0:4], c.ID.Bytes[4:6], c.ID.Bytes[6:8], c.ID.Bytes[8:10], c.ID.Bytes[10:16])
		log.Info("syncing course", "course_id", courseIDStr, "legacy_course_id", c.LegacyCourseID)

		result, err := scraper.SyncCourse(ctx, c.ID, c.LegacyCourseID)
		if err != nil {
			log.Error("sync failed", "course_id", courseIDStr, "legacy_course_id", c.LegacyCourseID, "error", err)
			continue
		}

		log.Info("synced course", "course_id", courseIDStr, "sessions_created", result.SessionsCreated, "synced_at", result.SyncedAt)
	}

	log.Info("legacy sync complete", "course_count", len(courses))
}
