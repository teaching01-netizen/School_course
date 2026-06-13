package crmimport

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/crmimport/xlsx"
)

// SnapshotService manages CRM snapshots: creation, row population, and lifecycle.
type SnapshotService struct {
	db           *pgxpool.Pool
	instituteLoc *time.Location
}

// NewSnapshotService creates a new SnapshotService.
func NewSnapshotService(db *pgxpool.Pool, instituteTZ string) (*SnapshotService, error) {
	loc, err := time.LoadLocation(instituteTZ)
	if err != nil {
		return nil, err
	}
	return &SnapshotService{db: db, instituteLoc: loc}, nil
}

// CreateSnapshot creates a new snapshot row and returns the ID.
func (s *SnapshotService) CreateSnapshot(ctx context.Context) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.db.QueryRow(ctx,
		`INSERT INTO crm_snapshots (status) VALUES ('importing') RETURNING id`,
	).Scan(&id)
	return id, err
}

// PopulateRows inserts parsed rows into crm_rows associated with the snapshot.
// Each row gets (snapshot_id, xlsx_row_number) as the primary key.
// CopyFrom is used for efficient bulk insert.
func (s *SnapshotService) PopulateRows(ctx context.Context, snapshotID pgtype.UUID, rows []xlsx.Row, parsedCount int) (int, error) {
	if len(rows) == 0 {
		return 0, fmt.Errorf("0 rows to populate")
	}

	// Use CopyFrom for efficient bulk insert.
	copyCount, err := s.db.CopyFrom(
		ctx,
		pgx.Identifier{"crm_rows"},
		[]string{
			"snapshot_id", "xlsx_row_number",
			"row_hash", "cycle_label", "course_name", "wcode",
			"first_name", "last_name", "nickname", "secondary_school", "academic_level",
			"mobile_phone", "hours", "teachers_raw", "primary_email",
			"parent_name", "parent_phone", "parent_email", "order_quote_updated_at",
			"extra_note",
		},
		pgx.CopyFromRows(rowCopies(snapshotID, rows)),
	)
	if err != nil {
		return 0, fmt.Errorf("copy from: %w", err)
	}

	return int(copyCount), nil
}

// rowCopies converts rows to [][]any for pgx.CopyFromRows with snapshot_id and xlsx_row_number.
func rowCopies(snapshotID pgtype.UUID, rows []xlsx.Row) [][]any {
	sources := make([][]any, len(rows))
	for i, r := range rows {
		var hours pgtype.Int4
		if r.Hours != nil {
			hours = pgtype.Int4{Int32: *r.Hours, Valid: true}
		}
		var updated pgtype.Timestamptz
		if r.OrderQuoteUpdatedAt != nil {
			updated = pgtype.Timestamptz{Time: *r.OrderQuoteUpdatedAt, Valid: true}
		}

		sources[i] = []any{
			snapshotID,
			int32(i + 1),
			r.Hash(),
			r.CycleLabel,
			r.CourseName,
			r.WCode,
			nullIfEmpty(r.FirstName),
			nullIfEmpty(r.LastName),
			nullIfEmpty(r.Nickname),
			nullIfEmpty(r.SecondarySchool),
			nullIfEmpty(r.AcademicLevel),
			nullIfEmpty(r.MobilePhone),
			hours,
			nullIfEmpty(r.TeachersRaw),
			nullIfEmpty(r.PrimaryEmail),
			nullIfEmpty(r.ParentName),
			nullIfEmpty(r.ParentPhone),
			nullIfEmpty(r.ParentEmail),
			updated,
			r.ExtraNote,
		}
	}
	return sources
}

func nullIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// MarkSnapshotReady sets snapshot status to 'ready' and sets it as active.
func (s *SnapshotService) MarkSnapshotReady(ctx context.Context, snapshotID pgtype.UUID, rowCount int) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Update snapshot status and row count.
	if _, err := tx.Exec(ctx,
		`UPDATE crm_snapshots SET status = 'ready', row_count = $2 WHERE id = $1`,
		snapshotID, rowCount,
	); err != nil {
		return fmt.Errorf("update snapshot: %w", err)
	}

	// Set as active snapshot in crm_state.
	if _, err := tx.Exec(ctx,
		`UPDATE crm_state SET active_snapshot_id = $1, updated_at = now() WHERE singleton = true`,
		snapshotID,
	); err != nil {
		return fmt.Errorf("update crm_state: %w", err)
	}

	// Upsert cycles from distinct labels in this snapshot.
	if _, err := tx.Exec(ctx, `
		INSERT INTO crm_cycles (id, label, last_imported_at)
		SELECT DISTINCT cycle_label, cycle_label, now() FROM crm_rows WHERE snapshot_id = $1
		ON CONFLICT (id) DO UPDATE
		SET label = EXCLUDED.label,
		    last_imported_at = now(),
		    updated_at = now()
	`, snapshotID); err != nil {
		return fmt.Errorf("upsert cycles: %w", err)
	}

	return tx.Commit(ctx)
}

// MarkSnapshotFailed sets snapshot status to 'failed'.
func (s *SnapshotService) MarkSnapshotFailed(ctx context.Context, snapshotID pgtype.UUID, errMsg string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE crm_snapshots SET status = 'failed', error_msg = $2 WHERE id = $1`,
		snapshotID, errMsg,
	)
	return err
}

// GetActiveSnapshotID returns the current active snapshot ID from crm_state.
func (s *SnapshotService) GetActiveSnapshotID(ctx context.Context) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.db.QueryRow(ctx,
		`SELECT COALESCE(active_snapshot_id, '00000000-0000-0000-0000-000000000000'::uuid) FROM crm_state WHERE singleton = true`,
	).Scan(&id)
	return id, err
}
