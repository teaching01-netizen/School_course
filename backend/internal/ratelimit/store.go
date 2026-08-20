package ratelimit

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Result struct {
	Allowed   bool
	Remaining int
	Limit     int
	ResetAt   time.Time
}

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error) {
	if s == nil || s.db == nil {
		return Result{}, fmt.Errorf("ratelimit store not configured")
	}
	if key == "" {
		return Result{}, fmt.Errorf("rate limit key required")
	}
	if limit <= 0 {
		return Result{}, fmt.Errorf("rate limit limit must be > 0")
	}
	if window <= 0 {
		return Result{}, fmt.Errorf("rate limit window must be > 0")
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, key); err != nil {
		return Result{}, err
	}

	cutoff := time.Now().UTC().Add(-window)
	if _, err := tx.Exec(ctx, `
		DELETE FROM http_rate_limit_events
		WHERE key = $1
		  AND created_at < $2
	`, key, cutoff); err != nil {
		return Result{}, err
	}

	var count int
	var oldest pgtype.Timestamptz
	if err := tx.QueryRow(ctx, `
		SELECT count(*), COALESCE(min(created_at), NULL)
		FROM http_rate_limit_events
		WHERE key = $1
	`, key).Scan(&count, &oldest); err != nil {
		return Result{}, err
	}

	now := time.Now().UTC()
	resetAt := now.Add(window)
	if oldest.Valid {
		resetAt = oldest.Time.Add(window)
	}

	if count >= limit {
		if err := tx.Commit(ctx); err != nil {
			return Result{}, err
		}
		return Result{
			Allowed:   false,
			Remaining: 0,
			Limit:     limit,
			ResetAt:   resetAt,
		}, nil
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO http_rate_limit_events (key, created_at)
		VALUES ($1, $2)
	`, key, now); err != nil {
		return Result{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}

	return Result{
		Allowed:   true,
		Remaining: int(math.Max(0, float64(limit-count-1))),
		Limit:     limit,
		ResetAt:   resetAt,
	}, nil
}

// SweepExpired deletes rate-limit events older than the cutoff in bounded
// batches. Allow() only ever consults rows inside a key's sliding window (it
// prunes them per key), so any row older than the largest configured window
// is dead weight. The table has no primary key, so each batch is selected by
// ctid; the subselect is re-evaluated per iteration and a concurrently
// deleted row can at worst cause a no-op removal of an equally eligible row.
func (s *Store) SweepExpired(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("ratelimit store not configured")
	}
	if batchSize <= 0 {
		batchSize = 5000
	}
	var total int64
	for {
		tag, err := s.db.Exec(ctx, `
			DELETE FROM http_rate_limit_events
			WHERE ctid IN (
				SELECT ctid FROM http_rate_limit_events
				WHERE created_at < $1
				LIMIT $2
			)
		`, before, batchSize)
		if err != nil {
			return total, fmt.Errorf("sweep rate limit events batch: %w", err)
		}
		deleted := tag.RowsAffected()
		total += deleted
		if deleted < int64(batchSize) {
			break
		}
	}
	return total, nil
}
