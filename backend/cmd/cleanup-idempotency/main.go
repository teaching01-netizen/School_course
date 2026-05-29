package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"warwick-institute/internal/config"
	"warwick-institute/internal/idempotency"
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

	result, err := idempotency.CleanupExpired(ctx, dbpool, 5000)
	if err != nil {
		log.Error("cleanup failed", "err", err)
		os.Exit(1)
	}

	log.Info("idempotency cleanup complete", "rows_deleted", result)
	fmt.Printf("Deleted %d expired idempotency keys\n", result)
}
