package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"warwick-institute/internal/config"
	"warwick-institute/internal/logging"
	"warwick-institute/internal/pg"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("config error", "err", err)
		os.Exit(2)
	}

	log := logging.New(cfg.LogLevel)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbpool, err := pg.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer dbpool.Close()

	var deletedCount int
	if err := dbpool.QueryRow(ctx, `SELECT cleanup_stale_parent_verification_sessions()`).Scan(&deletedCount); err != nil {
		log.Error("cleanup failed", "err", err)
		os.Exit(1)
	}

	log.Info("verification session cleanup complete", "rows_deleted", deletedCount)
	fmt.Printf("Cleaned up %d stale verification sessions\n", deletedCount)
}
