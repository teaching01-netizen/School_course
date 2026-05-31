package smartsms

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	smsCircuitFailureWindow = 5 * time.Minute
	smsCircuitOpenDuration  = 2 * time.Minute
	smsCircuitThreshold     = 3
)

type CircuitBreaker struct {
	db       *pgxpool.Pool
	provider string
	now      func() time.Time
}

func NewCircuitBreaker(db *pgxpool.Pool, provider string) *CircuitBreaker {
	return &CircuitBreaker{
		db:       db,
		provider: provider,
		now:      time.Now,
	}
}

func (c *CircuitBreaker) Allow(ctx context.Context) (bool, time.Duration, error) {
	if c == nil || c.db == nil {
		return true, 0, nil
	}
	now := c.now().UTC()
	tx, err := c.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, 0, err
	}
	defer tx.Rollback(ctx)

	state, err := loadCircuitState(ctx, tx, c.provider)
	if err != nil {
		return false, 0, err
	}

	if state.OpenUntil.Valid && now.Before(state.OpenUntil.Time) {
		retryAfter := time.Until(state.OpenUntil.Time)
		if err := tx.Commit(ctx); err != nil {
			return false, 0, err
		}
		return false, retryAfter, nil
	}

	if now.Sub(state.WindowStartedAt.Time) > smsCircuitFailureWindow {
		state.FailureCount = 0
		state.WindowStartedAt = pgtype.Timestamptz{Time: now, Valid: true}
		state.OpenUntil = pgtype.Timestamptz{}
		if err := updateCircuitState(ctx, tx, c.provider, state); err != nil {
			return false, 0, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, 0, err
	}
	return true, 0, nil
}

func (c *CircuitBreaker) ReportSuccess(ctx context.Context) error {
	if c == nil || c.db == nil {
		return nil
	}
	tx, err := c.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	state, err := loadCircuitState(ctx, tx, c.provider)
	if err != nil {
		return err
	}
	now := c.now().UTC()
	state.FailureCount = 0
	state.WindowStartedAt = pgtype.Timestamptz{Time: now, Valid: true}
	state.OpenUntil = pgtype.Timestamptz{}
	if err := updateCircuitState(ctx, tx, c.provider, state); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (c *CircuitBreaker) ReportFailure(ctx context.Context) (time.Duration, error) {
	if c == nil || c.db == nil {
		return 0, nil
	}
	tx, err := c.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	state, err := loadCircuitState(ctx, tx, c.provider)
	if err != nil {
		return 0, err
	}
	now := c.now().UTC()
	if now.Sub(state.WindowStartedAt.Time) > smsCircuitFailureWindow {
		state.FailureCount = 0
		state.WindowStartedAt = pgtype.Timestamptz{Time: now, Valid: true}
		state.OpenUntil = pgtype.Timestamptz{}
	}
	state.FailureCount++
	var retryAfter time.Duration
	if state.FailureCount >= smsCircuitThreshold {
		state.FailureCount = 0
		state.WindowStartedAt = pgtype.Timestamptz{Time: now, Valid: true}
		state.OpenUntil = pgtype.Timestamptz{Time: now.Add(smsCircuitOpenDuration), Valid: true}
		retryAfter = smsCircuitOpenDuration
	}
	if err := updateCircuitState(ctx, tx, c.provider, state); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return retryAfter, nil
}

type circuitState struct {
	FailureCount    int
	WindowStartedAt pgtype.Timestamptz
	OpenUntil       pgtype.Timestamptz
}

func loadCircuitState(ctx context.Context, tx pgx.Tx, provider string) (circuitState, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "sms:"+provider); err != nil {
		return circuitState{}, err
	}

	var state circuitState
	err := tx.QueryRow(ctx, `
		SELECT failure_count, window_started_at, COALESCE(open_until, NULL)
		FROM sms_circuit_breaker_state
		WHERE provider = $1
	`, provider).Scan(&state.FailureCount, &state.WindowStartedAt, &state.OpenUntil)
	if err == nil {
		return state, nil
	}
	if err != pgx.ErrNoRows {
		return circuitState{}, err
	}
	now := time.Now().UTC()
	state = circuitState{
		FailureCount:    0,
		WindowStartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		OpenUntil:       pgtype.Timestamptz{},
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO sms_circuit_breaker_state (provider, failure_count, window_started_at, open_until, updated_at)
		VALUES ($1, 0, $2, NULL, now())
	`, provider, state.WindowStartedAt); err != nil {
		return circuitState{}, err
	}
	return state, nil
}

func updateCircuitState(ctx context.Context, tx pgx.Tx, provider string, state circuitState) error {
	_, err := tx.Exec(ctx, `
		UPDATE sms_circuit_breaker_state
		SET failure_count = $2,
		    window_started_at = $3,
		    open_until = $4,
		    updated_at = now()
		WHERE provider = $1
	`, provider, state.FailureCount, state.WindowStartedAt, state.OpenUntil)
	return err
}
