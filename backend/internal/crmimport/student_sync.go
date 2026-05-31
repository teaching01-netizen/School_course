package crmimport

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// studentRow represents a student identity extracted from a snapshot.
type studentRow struct {
	WCode       string
	FullName    string
	Nickname    string
	ParentPhone string
}

// StudentSyncService handles syncing student identities from a CRM snapshot.
type StudentSyncService struct {
	db *pgxpool.Pool
}

// NewStudentSyncService creates a new StudentSyncService.
func NewStudentSyncService(db *pgxpool.Pool) *StudentSyncService {
	return &StudentSyncService{db: db}
}

// SyncFromSnapshot selects distinct student identities from the snapshot with
// deterministic tie-breakers and upserts into the students table.
// It does NOT touch the notes field.
func (s *StudentSyncService) SyncFromSnapshot(ctx context.Context, snapshotID pgtype.UUID) (int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ON (wcode)
			wcode,
			COALESCE(NULLIF(btrim(first_name), ''), '') ||
			CASE
				WHEN COALESCE(NULLIF(btrim(last_name), ''), '') != ''
				THEN ' ' || btrim(last_name)
				ELSE ''
			END AS full_name,
			COALESCE(NULLIF(btrim(nickname), ''), '') AS nickname,
			COALESCE(NULLIF(btrim(parent_phone), ''), '') AS parent_phone
		FROM crm_rows
		WHERE snapshot_id = $1
		  AND btrim(wcode) != ''
		ORDER BY wcode, order_quote_updated_at DESC NULLS LAST, xlsx_row_number ASC, row_hash ASC
	`, snapshotID)
	if err != nil {
		return 0, fmt.Errorf("query snapshot students: %w", err)
	}
	defer rows.Close()

	var students []studentRow
	for rows.Next() {
		var sr studentRow
		if err := rows.Scan(&sr.WCode, &sr.FullName, &sr.Nickname, &sr.ParentPhone); err != nil {
			return 0, fmt.Errorf("scan student: %w", err)
		}
		sr.WCode = strings.TrimSpace(sr.WCode)
		sr.FullName = strings.TrimSpace(sr.FullName)
		if sr.FullName == "" {
			sr.FullName = sr.WCode
		}
		students = append(students, sr)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if len(students) == 0 {
		return 0, nil
	}

	// Batch upsert students: update full_name only, preserve notes.
	// We use a VALUES-based upsert for set-based operation.
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Use a values CTE for bulk upsert.
	batchSize := 500
	upserted := 0
	for i := 0; i < len(students); i += batchSize {
		end := i + batchSize
		if end > len(students) {
			end = len(students)
		}
		batch := students[i:end]

		// Build VALUES clause manually for the batch.
		// Use the temp table approach for cleaner SQL.
		// Create a temporary table, insert values, then upsert from it.
		if _, err := tx.Exec(ctx, `
			CREATE TEMP TABLE _sync_students (
				wcode text NOT NULL,
				full_name text NOT NULL,
				parent_phone text NOT NULL DEFAULT ''
			) ON COMMIT DROP
		`); err != nil {
			return 0, fmt.Errorf("create temp table: %w", err)
		}

		copyCount, err := tx.CopyFrom(
			ctx,
			pgx.Identifier{"_sync_students"},
			[]string{"wcode", "full_name", "parent_phone"},
			pgx.CopyFromRows(studentCopies(batch)),
		)
		if err != nil {
			return 0, fmt.Errorf("copy to temp: %w", err)
		}
		_ = copyCount

		res, err := tx.Exec(ctx, `
			INSERT INTO students (wcode, full_name, notes, parent_phone)
			SELECT ss.wcode, ss.full_name, '', NULLIF(ss.parent_phone, '') FROM _sync_students ss
			ON CONFLICT (wcode) DO UPDATE
			SET full_name = EXCLUDED.full_name,
			    parent_phone = CASE WHEN NULLIF(EXCLUDED.parent_phone, '') IS NOT NULL THEN EXCLUDED.parent_phone ELSE students.parent_phone END,
			    updated_at = now()
		`)
		if err != nil {
			return 0, fmt.Errorf("upsert students: %w", err)
		}
		upserted += int(res.RowsAffected())

		// Drop temp table.
		if _, err := tx.Exec(ctx, `DROP TABLE IF EXISTS _sync_students`); err != nil {
			return 0, fmt.Errorf("drop temp: %w", err)
		}
	}

	return upserted, tx.Commit(ctx)
}

func studentCopies(students []studentRow) [][]any {
	sources := make([][]any, len(students))
	for i, s := range students {
		sources[i] = []any{s.WCode, s.FullName, s.ParentPhone}
	}
	return sources
}
