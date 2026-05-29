package crmimport

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/crmimport/xlsx"
)

// ImportService handles atomic replacement of crm_rows and upsert of crm_cycles.
type ImportService struct {
	db           *pgxpool.Pool
	instituteLoc *time.Location
}

// NewImportService creates a new ImportService with the given database pool and institute timezone.
func NewImportService(db *pgxpool.Pool, instituteTZ string) (*ImportService, error) {
	loc, err := time.LoadLocation(instituteTZ)
	if err != nil {
		return nil, err
	}
	return &ImportService{db: db, instituteLoc: loc}, nil
}

// ImportUpload atomically replaces all crm_rows with the given rows and upserts crm_cycles.
// Deprecated in favour of the v2 snapshot-based pipeline (SnapshotService + ReconcileV2Service).
func (s *ImportService) ImportUpload(ctx context.Context, rows []xlsx.Row) (UploadResult, error) {
	if len(rows) == 0 {
		return UploadResult{}, fmt.Errorf("0 rows to import")
	}

	// Deduplicate within this upload.
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].OrderQuoteUpdatedAt, rows[j].OrderQuoteUpdatedAt
		if a == nil && b == nil {
			return false
		}
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		return a.After(*b)
	})

	seen := map[string]struct{}{}
	deduped := make([]xlsx.Row, 0, len(rows))
	for _, r := range rows {
		h := r.Hash()
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		deduped = append(deduped, r)
	}

	cycleSet := map[string]struct{}{}
	for _, r := range deduped {
		cycleSet[r.CycleLabel] = struct{}{}
	}
	cycleLabels := make([]string, 0, len(cycleSet))
	for c := range cycleSet {
		cycleLabels = append(cycleLabels, c)
	}

	// Create a snapshot first — required because migration 00012 made crm_rows.snapshot_id NOT NULL.
	snapshotSvc := &SnapshotService{db: s.db, instituteLoc: s.instituteLoc}
	snapshotID, err := snapshotSvc.CreateSnapshot(ctx)
	if err != nil {
		return UploadResult{}, fmt.Errorf("create snapshot: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return UploadResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Atomically replace all rows.
	if _, err := tx.Exec(ctx, `DELETE FROM crm_rows`); err != nil {
		return UploadResult{}, err
	}

	for i, r := range deduped {
		var hours pgtype.Int4
		if r.Hours != nil {
			hours = pgtype.Int4{Int32: *r.Hours, Valid: true}
		}
		var updated pgtype.Timestamptz
		if r.OrderQuoteUpdatedAt != nil {
			updated = pgtype.Timestamptz{Time: *r.OrderQuoteUpdatedAt, Valid: true}
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO crm_rows (
				snapshot_id, xlsx_row_number, row_hash, cycle_label, course_name, wcode,
				first_name, last_name, nickname, secondary_school, academic_level,
				mobile_phone, hours, teachers_raw, primary_email,
				parent_name, parent_phone, parent_email, order_quote_updated_at
			) VALUES (
				$1,$2,$3,$4,$5,$6,
				NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),
				NULLIF($12,''),$13,NULLIF($14,''),NULLIF($15,''),
				NULLIF($16,''),NULLIF($17,''),NULLIF($18,''),$19
			)
		`,
			snapshotID, int32(i+1), r.Hash(), r.CycleLabel, r.CourseName, r.WCode,
			r.FirstName, r.LastName, r.Nickname, r.SecondarySchool, r.AcademicLevel,
			r.MobilePhone, hours, r.TeachersRaw, r.PrimaryEmail,
			r.ParentName, r.ParentPhone, r.ParentEmail, updated,
		); err != nil {
			return UploadResult{}, fmt.Errorf("insert row: %w", err)
		}
	}

	// Upsert crm_cycles from distinct labels.
	if _, err := tx.Exec(ctx, `
		INSERT INTO crm_cycles (id, label, last_imported_at)
		SELECT DISTINCT cycle_label, cycle_label, now() FROM crm_rows WHERE snapshot_id = $1
		ON CONFLICT (id) DO UPDATE
		SET label = EXCLUDED.label,
		    last_imported_at = now(),
		    updated_at = now()
	`, snapshotID); err != nil {
		return UploadResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		// Mark snapshot as failed if the transaction fails.
		_ = snapshotSvc.MarkSnapshotFailed(ctx, snapshotID, err.Error())
		return UploadResult{}, err
	}

	// Mark snapshot as ready after successful commit.
	if err := snapshotSvc.MarkSnapshotReady(ctx, snapshotID, len(deduped)); err != nil {
		return UploadResult{}, fmt.Errorf("mark snapshot ready: %w", err)
	}

	return UploadResult{
		RowsParsed:  len(rows),
		RowsStored:  len(deduped),
		CyclesFound: cycleLabels,
	}, nil
}
