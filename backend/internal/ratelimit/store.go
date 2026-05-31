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
