package backfill

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/snapshot"
)

// Service orchestrates the historical backfill process.
type Service struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
	finder *EvidenceFinder
	config BackfillConfig
}

// NewService creates a new backfill service.
func NewService(pool *pgxpool.Pool, logger *slog.Logger, config BackfillConfig) *Service {
	return &Service{
		pool:   pool,
		logger: logger,
		finder: NewEvidenceFinder(pool, logger),
		config: config,
	}
}

// Run executes the backfill process, returning a report.
func (s *Service) Run(ctx context.Context) (*BackfillReport, error) {
	report := &BackfillReport{
		StartedAt: time.Now().UTC(),
	}

	// Count total eligible
	totalEligible, err := s.countEligible(ctx)
	if err != nil {
		return nil, fmt.Errorf("count eligible: %w", err)
	}
	report.TotalEligible = totalEligible
	report.Remaining = totalEligible

	s.logger.Info("backfill started",
		"total_eligible", totalEligible,
		"batch_size", s.config.BatchSize,
		"dry_run", s.config.DryRun,
	)

	var batchCount int
	var totalDuration time.Duration

batchLoop:
	for {
		// Check context
		if err := ctx.Err(); err != nil {
			s.logger.Info("backfill cancelled", "reason", err)
			break
		}

		// Check max batches
		if s.config.MaxBatches > 0 && batchCount >= s.config.MaxBatches {
			s.logger.Info("backfill reached max batches", "max", s.config.MaxBatches)
			break
		}

		// Process a batch
		batchResult, err := s.processBatch(ctx)
		if err != nil {
			return nil, fmt.Errorf("process batch %d: %w", batchCount+1, err)
		}

		if batchResult.BatchSize == 0 {
			s.logger.Info("backfill complete: no more eligible rows")
			break
		}

		// Update report
		batchCount++
		totalDuration += batchResult.Duration
		report.Exact += batchResult.Exact
		report.Reconstructed += batchResult.Reconstructed
		report.Unavailable += batchResult.Unavailable
		report.Failed += batchResult.Failed
		report.Remaining -= batchResult.Processed

		s.logger.Info("batch completed",
			"batch", batchCount,
			"size", batchResult.BatchSize,
			"processed", batchResult.Processed,
			"exact", batchResult.Exact,
			"reconstructed", batchResult.Reconstructed,
			"unavailable", batchResult.Unavailable,
			"failed", batchResult.Failed,
			"duration_ms", batchResult.Duration.Milliseconds(),
			"remaining", report.Remaining,
		)

		// Rate limiting
		if s.config.RateLimit > 0 {
			select {
			case <-ctx.Done():
				break batchLoop
			case <-time.After(s.config.RateLimit):
			}
		}
	}

	report.BatchCount = batchCount
	report.AvgBatchDur = time.Duration(int64(totalDuration) / int64(batchCount))
	report.CompletedAt = time.Now().UTC()

	return report, nil
}

// countEligible returns the total number of rows eligible for backfill.
func (s *Service) countEligible(ctx context.Context) (int, error) {
	var total int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*)::int4
		FROM absence_sit_ins
		WHERE snapshot_quality = 'unavailable'
	`).Scan(&total)
	return total, err
}

// processBatch processes a single batch of eligible assignments.
func (s *Service) processBatch(ctx context.Context) (*BatchResult, error) {
	start := time.Now()
	result := &BatchResult{}

	// Begin transaction for batch processing
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Fetch eligible assignments with row locking
	rows, err := tx.Query(ctx, `
		SELECT
		  asi.id,
		  asi.absence_id,
		  asi.session_id,
		  asi.session_version_at_assignment,
		  asi.assigned_at,
		  COALESCE(s.version, 0) AS current_session_version,
		  (s.deleted_at IS NOT NULL) AS current_session_deleted
		FROM absence_sit_ins asi
		LEFT JOIN sessions s ON s.id = asi.session_id
		WHERE asi.snapshot_quality = 'unavailable'
		ORDER BY asi.assigned_at, asi.id
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, s.config.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("query eligible: %w", err)
	}
	defer rows.Close()

	// Collect assignments
	var assignments []*EligibleAssignment
	for rows.Next() {
		var a EligibleAssignment
		var sessionVersionAtAssignment *int32
		var currentSessionVersion int32
		var currentSessionDeleted bool

		if err := rows.Scan(
			&a.ID,
			&a.AbsenceID,
			&a.SessionID,
			&sessionVersionAtAssignment,
			&a.AssignedAt,
			&currentSessionVersion,
			&currentSessionDeleted,
		); err != nil {
			return nil, fmt.Errorf("scan assignment: %w", err)
		}

		a.SessionVersionAtAssignment = sessionVersionAtAssignment
		a.CurrentSessionVersion = currentSessionVersion
		a.CurrentSessionDeleted = currentSessionDeleted

		assignments = append(assignments, &a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	result.BatchSize = len(assignments)

	// Process each assignment
	for _, assignment := range assignments {
		if err := ctx.Err(); err != nil {
			break
		}

		evidence, err := s.finder.FindEvidence(ctx, assignment)
		if err != nil {
			s.logger.Error("evidence lookup failed",
				"assignment_id", assignment.ID,
				"error", err,
			)
			result.Failed++
			continue
		}

		// Update the assignment
		if err := s.updateAssignment(ctx, tx, assignment, evidence); err != nil {
			s.logger.Error("update assignment failed",
				"assignment_id", assignment.ID,
				"error", err,
			)
			result.Failed++
			continue
		}

		// Track metrics
		switch evidence.Quality {
		case QualityExact:
			result.Exact++
		case QualityReconstructed:
			result.Reconstructed++
		case QualityUnavailable:
			result.Unavailable++
		}
		result.Processed++
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	result.Duration = time.Since(start)
	return result, nil
}

// updateAssignment updates a single assignment with evidence data.
func (s *Service) updateAssignment(
	ctx context.Context,
	tx pgx.Tx,
	assignment *EligibleAssignment,
	evidence *EvidenceResult,
) error {
	var snapshotJSON []byte
	var schemaVersion *int16
	var capturedAt *time.Time
	var quality string
	var source string

	if evidence.Quality == QualityUnavailable {
		quality = string(QualityUnavailable)
		source = ""
	} else {
		quality = string(evidence.Quality)
		source = string(evidence.Source)
		snapshotJSON = evidence.Snapshot

		if evidence.Version != nil {
			v := int16(1) // Schema version 1
			schemaVersion = &v
		}

		if evidence.CapturedAt != nil {
			capturedAt = evidence.CapturedAt
		}
	}

	// Use COALESCE to handle NULL values properly
	_, err := tx.Exec(ctx, `
		UPDATE absence_sit_ins
		SET
		  session_snapshot_at_assignment = COALESCE($2, session_snapshot_at_assignment),
		  snapshot_schema_version = COALESCE($3, snapshot_schema_version),
		  snapshot_captured_at = COALESCE($4, snapshot_captured_at),
		  snapshot_quality = $5,
		  snapshot_source = COALESCE($6, snapshot_source)
		WHERE id = $1
	`, assignment.ID, snapshotJSON, schemaVersion, capturedAt, quality, source)

	return err
}

// GetSampleRecords returns sample records from each quality category for validation.
func (s *Service) GetSampleRecords(ctx context.Context, sampleSize int) (map[string][]json.RawMessage, error) {
	samples := make(map[string][]json.RawMessage)

	for _, quality := range []string{"exact", "reconstructed", "unavailable"} {
		rows, err := s.pool.Query(ctx, `
			SELECT jsonb_build_object(
				'id', asi.id,
				'absence_id', asi.absence_id,
				'session_id', asi.session_id,
				'snapshot_quality', asi.snapshot_quality,
				'snapshot_source', asi.snapshot_source,
				'snapshot_captured_at', asi.snapshot_captured_at,
				'has_snapshot', (asi.session_snapshot_at_assignment IS NOT NULL)
			)
			FROM absence_sit_ins asi
			WHERE asi.snapshot_quality = $1
			ORDER BY RANDOM()
			LIMIT $2
		`, quality, sampleSize)
		if err != nil {
			return nil, fmt.Errorf("query samples for %s: %w", quality, err)
		}
		defer rows.Close()

		var records []json.RawMessage
		for rows.Next() {
			var record json.RawMessage
			if err := rows.Scan(&record); err != nil {
				return nil, fmt.Errorf("scan sample: %w", err)
			}
			records = append(records, record)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate samples: %w", err)
		}

		samples[quality] = records
	}

	return samples, nil
}

// validateSnapshotForUpdate prepares snapshot data for update, ensuring
// consistency with the database constraints.
func validateSnapshotForUpdate(data []byte, quality string) ([]byte, *int16, *time.Time, error) {
	if quality == string(QualityUnavailable) {
		return nil, nil, nil, nil
	}

	var snap snapshot.SessionSnapshotV1
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, nil, nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}

	v := int16(snap.SchemaVersion)
	capturedAt := snap.CapturedAt

	return data, &v, &capturedAt, nil
}
