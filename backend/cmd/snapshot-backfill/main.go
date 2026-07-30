package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/backfill"
	"warwick-institute/internal/config"
	"warwick-institute/internal/logging"
)

func main() {
	var (
		batchSize  = flag.Int("batch-size", 500, "Number of rows to process per batch")
		rateLimit  = flag.Duration("rate-limit", 100*time.Millisecond, "Delay between batches")
		maxBatches = flag.Int("max-batches", 0, "Maximum batches to process (0 = unlimited)")
		dryRun     = flag.Bool("dry-run", false, "Count eligible rows without processing")
		sampleSize = flag.Int("sample-size", 10, "Number of samples per quality category for validation")
		showHelp   = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *showHelp {
		fmt.Println(`Usage: snapshot-backfill [options]

Historical backfill command to reconstruct snapshots from evidence.

Options:
  -batch-size    Number of rows to process per batch (default: 500)
  -rate-limit    Delay between batches (default: 100ms)
  -max-batches   Maximum batches to process, 0 for unlimited (default: 0)
  -dry-run       Count eligible rows without processing
  -sample-size   Number of samples per quality category (default: 10)
  -help          Show this help message

Evidence Hierarchy:
  1. Exact assignment event snapshot
  2. Immutable session revision matching stored assignment version
  3. Matching session_changes.before_snapshot or after_snapshot
  4. Current session only when current version equals stored assignment version
  5. Otherwise leave unavailable

The backfill job is:
  - Restartable: Can be stopped and resumed safely
  - Idempotent: Running multiple times produces same result
  - Rate limited: Configurable delay between batches
  - Observable: Detailed logging and final report
  - Safe: Uses FOR UPDATE SKIP LOCKED for concurrent safety

Examples:
  # Dry run to see how many rows need backfill
  snapshot-backfill -dry-run

  # Process 1000 rows in batches of 200
  snapshot-backfill -batch-size 200 -max-batches 5

  # Run with rate limiting
  snapshot-backfill -rate-limit 500ms`)
		os.Exit(0)
	}

	log := logging.New("info")

	cfg, err := config.FromEnv()
	if err != nil {
		log.Error("config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("received interrupt, shutting down gracefully...")
		cancel()
	}()

	// Connect to database
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	config := backfill.BackfillConfig{
		BatchSize:  *batchSize,
		RateLimit:  *rateLimit,
		MaxBatches: *maxBatches,
		DryRun:     *dryRun,
		SampleSize: *sampleSize,
	}

	service := backfill.NewService(pool, log, config)

	if *dryRun {
		// Just count eligible rows
		total, err := countEligible(ctx, pool)
		if err != nil {
			log.Error("count eligible", "error", err)
			os.Exit(1)
		}
		fmt.Printf("Eligible rows for backfill: %d\n", total)
		os.Exit(0)
	}

	// Run backfill
	log.Info("starting snapshot backfill",
		"batch_size", config.BatchSize,
		"rate_limit", config.RateLimit,
		"max_batches", config.MaxBatches,
	)

	report, err := service.Run(ctx)
	if err != nil {
		log.Error("backfill failed", "error", err)
		os.Exit(1)
	}

	// Print report
	fmt.Println("\n=== Backfill Report ===")
	fmt.Printf("Total eligible:  %d\n", report.TotalEligible)
	fmt.Printf("Exact:           %d\n", report.Exact)
	fmt.Printf("Reconstructed:   %d\n", report.Reconstructed)
	fmt.Printf("Unavailable:     %d\n", report.Unavailable)
	fmt.Printf("Failed:          %d\n", report.Failed)
	fmt.Printf("Remaining:       %d\n", report.Remaining)
	fmt.Printf("Batches:         %d\n", report.BatchCount)
	fmt.Printf("Avg batch dur:   %v\n", report.AvgBatchDur)
	fmt.Printf("Duration:        %v\n", report.CompletedAt.Sub(report.StartedAt))

	// Get sample records for validation
	fmt.Println("\n=== Sample Records for Validation ===")
	samples, err := service.GetSampleRecords(ctx, config.SampleSize)
	if err != nil {
		log.Warn("failed to get samples", "error", err)
	} else {
		for quality, records := range samples {
			fmt.Printf("\n--- %s (%d records) ---\n", quality, len(records))
			for i, record := range records {
				var pretty map[string]interface{}
				if err := json.Unmarshal(record, &pretty); err == nil {
					data, _ := json.MarshalIndent(pretty, "", "  ")
					fmt.Printf("  %d. %s\n", i+1, string(data))
				}
			}
		}
	}

	if report.Failed > 0 {
		os.Exit(1)
	}
}

func countEligible(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var total int
	err := pool.QueryRow(ctx, `
		SELECT count(*)::int4
		FROM absence_sit_ins
		WHERE snapshot_quality = 'unavailable'
	`).Scan(&total)
	return total, err
}
