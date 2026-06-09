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
	WCode        string
	FullName     string
	Nickname     string
	PrimaryEmail string
	StudentPhone string
	ParentPhone  string
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
			COALESCE(NULLIF(btrim(primary_email), ''), '') AS primary_email,
			COALESCE(NULLIF(btrim(mobile_phone), ''), '') AS student_phone,
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
		if err := rows.Scan(&sr.WCode, &sr.FullName, &sr.Nickname, &sr.PrimaryEmail, &sr.StudentPhone, &sr.ParentPhone); err != nil {
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

	// Batch upsert students: update core identity/contact fields and preserve notes.
	// We use a VALUES-based upsert for set-based operation.
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Older databases may not have the optional contact columns yet. Make the
	// student sync self-healing so the job can recover without a separate manual
	// migration step.
	if _, err := tx.Exec(ctx, `
		ALTER TABLE students
			ADD COLUMN IF NOT EXISTS email text NULL,
			ADD COLUMN IF NOT EXISTS nickname text NULL
	`); err != nil {
		return 0, fmt.Errorf("ensure students contact columns: %w", err)
	}

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
				nickname text NOT NULL DEFAULT '',
				email text NOT NULL DEFAULT '',
				student_phone text NOT NULL DEFAULT '',
				parent_phone text NOT NULL DEFAULT ''
			) ON COMMIT DROP
		`); err != nil {
			return 0, fmt.Errorf("create temp table: %w", err)
		}

		copyCount, err := tx.CopyFrom(
			ctx,
			pgx.Identifier{"_sync_students"},
			[]string{"wcode", "full_name", "nickname", "email", "student_phone", "parent_phone"},
			pgx.CopyFromRows(studentCopies(batch)),
		)
		if err != nil {
			return 0, fmt.Errorf("copy to temp: %w", err)
		}
		_ = copyCount

		res, err := tx.Exec(ctx, `
			INSERT INTO students (wcode, full_name, notes, nickname, email, student_phone, parent_phone)
			SELECT ss.wcode, ss.full_name, '', NULLIF(ss.nickname, ''), NULLIF(ss.email, ''), NULLIF(ss.student_phone, ''), NULLIF(ss.parent_phone, '') FROM _sync_students ss
			ON CONFLICT (wcode) DO UPDATE
			SET full_name = EXCLUDED.full_name,
			    nickname = CASE WHEN NULLIF(EXCLUDED.nickname, '') IS NOT NULL THEN EXCLUDED.nickname ELSE students.nickname END,
			    email = CASE WHEN NULLIF(EXCLUDED.email, '') IS NOT NULL THEN EXCLUDED.email ELSE students.email END,
			    student_phone = CASE WHEN NULLIF(EXCLUDED.student_phone, '') IS NOT NULL THEN EXCLUDED.student_phone ELSE students.student_phone END,
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
		sources[i] = []any{s.WCode, s.FullName, s.Nickname, s.PrimaryEmail, s.StudentPhone, s.ParentPhone}
	}
	return sources
}
