package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type legacySyncProgressReporter struct {
	q             *sqldb.Queries
	runID         pgtype.UUID
	lastPhase     string
	lastProcessed int
	lastUpdatedAt time.Time
}

func (r *legacySyncProgressReporter) update(ctx context.Context, phase, currentEntity string, processed, total, changed, applied, failures int, force bool) error {
	now := time.Now()
	if !force && phase == r.lastPhase && processed-r.lastProcessed < 25 && now.Sub(r.lastUpdatedAt) < 250*time.Millisecond {
		return nil
	}
	if err := r.q.LegacySyncRunProgressUpsert(ctx, sqldb.LegacySyncRunProgressUpsertParams{
		RunID:             r.runID,
		Phase:             phase,
		CurrentEntity:     pgtype.Text{String: currentEntity, Valid: currentEntity != ""},
		ProcessedEntities: int32(processed),
		TotalEntities:     int32(total),
		ChangedEntities:   int32(changed),
		AppliedEntities:   int32(applied),
		Failures:          int32(failures),
	}); err != nil {
		return err
	}
	r.lastPhase = phase
	r.lastProcessed = processed
	r.lastUpdatedAt = now
	return nil
}
