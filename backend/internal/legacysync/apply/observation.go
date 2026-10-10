package apply

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrIncompleteScheduleSnapshot   = errors.New("legacy sync: schedule snapshot is incomplete")
	ErrInvalidObservationGeneration = errors.New("legacy sync: observation generation must be positive")
	ErrStaleObservation             = errors.New("legacy sync: stale observation generation")
)

const legacyScheduleMissingGrace = 24 * time.Hour

func validateScheduleObservation(complete bool, generation int64) error {
	if !complete {
		return ErrIncompleteScheduleSnapshot
	}
	if generation <= 0 {
		return ErrInvalidObservationGeneration
	}
	return nil
}

func checkCourseObservationGeneration(ctx context.Context, tx pgx.Tx, source, externalCourseID string, generation int64) error {
	var current int64
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(last_generation, 0)
		FROM external_refs
		WHERE source=$1 AND entity_type='course' AND external_id=$2
	`, source, externalCourseID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load applied legacy observation generation: %w", err)
	}
	if generation < current {
		return fmt.Errorf("incoming generation %d is older than applied generation %d: %w", generation, current, ErrStaleObservation)
	}
	return nil
}

func markCourseObservationApplied(ctx context.Context, tx pgx.Tx, source, externalCourseID string, generation int64, observedAt time.Time) error {
	tag, err := tx.Exec(ctx, `
		UPDATE external_refs
		SET last_generation=GREATEST(COALESCE(last_generation, 0), $1),
		    last_seen_at=$2,
		    last_applied_at=$2,
		    state='active'
		WHERE source=$3 AND entity_type='course' AND external_id=$4
	`, generation, observedAt, source, externalCourseID)
	if err != nil {
		return fmt.Errorf("mark legacy course observation generation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("legacy sync: course mapping missing while recording observation generation")
	}
	return nil
}
