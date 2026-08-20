package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type LegacySyncRunProgress struct {
	RunID             pgtype.UUID
	Phase             string
	CurrentEntity     pgtype.Text
	ProcessedEntities int32
	TotalEntities     int32
	ChangedEntities   int32
	AppliedEntities   int32
	Failures          int32
	UpdatedAt         pgtype.Timestamptz
}

type LegacySyncRunProgressUpsertParams struct {
	RunID             pgtype.UUID
	Phase             string
	CurrentEntity     pgtype.Text
	ProcessedEntities int32
	TotalEntities     int32
	ChangedEntities   int32
	AppliedEntities   int32
	Failures          int32
}

func (q *Queries) LegacySyncRunProgressUpsert(ctx context.Context, arg LegacySyncRunProgressUpsertParams) error {
	_, err := q.db.Exec(ctx, `
INSERT INTO legacy_sync_run_progress
    (run_id, phase, current_entity, processed_entities, total_entities, changed_entities, applied_entities, failures)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (run_id) DO UPDATE SET
    phase = EXCLUDED.phase,
    current_entity = EXCLUDED.current_entity,
    processed_entities = EXCLUDED.processed_entities,
    total_entities = EXCLUDED.total_entities,
    changed_entities = EXCLUDED.changed_entities,
    applied_entities = EXCLUDED.applied_entities,
    failures = EXCLUDED.failures,
    updated_at = now()
`, arg.RunID, arg.Phase, arg.CurrentEntity, arg.ProcessedEntities, arg.TotalEntities, arg.ChangedEntities, arg.AppliedEntities, arg.Failures)
	return err
}

func (q *Queries) LegacySyncRunProgressGet(ctx context.Context, runID pgtype.UUID) (LegacySyncRunProgress, error) {
	var progress LegacySyncRunProgress
	err := q.db.QueryRow(ctx, `
SELECT run_id, phase, current_entity, processed_entities, total_entities,
       changed_entities, applied_entities, failures, updated_at
FROM legacy_sync_run_progress
WHERE run_id = $1
`, runID).Scan(
		&progress.RunID,
		&progress.Phase,
		&progress.CurrentEntity,
		&progress.ProcessedEntities,
		&progress.TotalEntities,
		&progress.ChangedEntities,
		&progress.AppliedEntities,
		&progress.Failures,
		&progress.UpdatedAt,
	)
	return progress, err
}
