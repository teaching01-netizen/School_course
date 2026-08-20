package crmimport

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NormalizeWCode trims whitespace and lowercases a wcode for consistent
// storage and comparison. Call this before any insert or lookup that uses
// wcode as an identity key.
func NormalizeWCode(wcode string) string {
	return strings.ToLower(strings.TrimSpace(wcode))
}

// studentRow represents a student identity extracted from a snapshot.
type studentRow struct {
	WCode        string
	FullName     string
	Nickname     string
	School       string
	Level        string
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
		WITH merged AS (
			SELECT
				LOWER(BTRIM(wcode)) AS wcode,
				(ARRAY_AGG(NULLIF(BTRIM(first_name), '') ORDER BY order_quote_updated_at DESC NULLS LAST, xlsx_row_number ASC, row_hash ASC) FILTER (WHERE NULLIF(BTRIM(first_name), '') IS NOT NULL))[1] AS first_name,
				(ARRAY_AGG(NULLIF(BTRIM(last_name), '') ORDER BY order_quote_updated_at DESC NULLS LAST, xlsx_row_number ASC, row_hash ASC) FILTER (WHERE NULLIF(BTRIM(last_name), '') IS NOT NULL))[1] AS last_name,
				(ARRAY_AGG(NULLIF(BTRIM(nickname), '') ORDER BY order_quote_updated_at DESC NULLS LAST, xlsx_row_number ASC, row_hash ASC) FILTER (WHERE NULLIF(BTRIM(nickname), '') IS NOT NULL))[1] AS nickname,
				(ARRAY_AGG(NULLIF(BTRIM(secondary_school), '') ORDER BY order_quote_updated_at DESC NULLS LAST, xlsx_row_number ASC, row_hash ASC) FILTER (WHERE NULLIF(BTRIM(secondary_school), '') IS NOT NULL))[1] AS school,
				(ARRAY_AGG(NULLIF(BTRIM(academic_level), '') ORDER BY order_quote_updated_at DESC NULLS LAST, xlsx_row_number ASC, row_hash ASC) FILTER (WHERE NULLIF(BTRIM(academic_level), '') IS NOT NULL))[1] AS level,
				(ARRAY_AGG(NULLIF(BTRIM(primary_email), '') ORDER BY order_quote_updated_at DESC NULLS LAST, xlsx_row_number ASC, row_hash ASC) FILTER (WHERE NULLIF(BTRIM(primary_email), '') IS NOT NULL))[1] AS primary_email,
				(ARRAY_AGG(NULLIF(BTRIM(mobile_phone), '') ORDER BY order_quote_updated_at DESC NULLS LAST, xlsx_row_number ASC, row_hash ASC) FILTER (WHERE NULLIF(BTRIM(mobile_phone), '') IS NOT NULL))[1] AS student_phone,
				(ARRAY_AGG(NULLIF(BTRIM(parent_phone), '') ORDER BY order_quote_updated_at DESC NULLS LAST, xlsx_row_number ASC, row_hash ASC) FILTER (WHERE NULLIF(BTRIM(parent_phone), '') IS NOT NULL))[1] AS parent_phone
			FROM crm_rows
			WHERE snapshot_id = $1
			  AND NULLIF(BTRIM(wcode), '') IS NOT NULL
			GROUP BY LOWER(BTRIM(wcode))
		)
		SELECT wcode,
			COALESCE(first_name, '') || CASE WHEN COALESCE(last_name, '') <> '' THEN ' ' || last_name ELSE '' END AS full_name,
			COALESCE(nickname, ''), COALESCE(school, ''), COALESCE(level, ''),
			COALESCE(primary_email, ''), COALESCE(student_phone, ''), COALESCE(parent_phone, '')
		FROM merged
		ORDER BY wcode
	`, snapshotID)
	if err != nil {
		return 0, fmt.Errorf("query snapshot students: %w", err)
	}
	defer rows.Close()

	var students []studentRow
	for rows.Next() {
		var sr studentRow
		if err := rows.Scan(&sr.WCode, &sr.FullName, &sr.Nickname, &sr.School, &sr.Level, &sr.PrimaryEmail, &sr.StudentPhone, &sr.ParentPhone); err != nil {
			return 0, fmt.Errorf("scan student: %w", err)
		}
		sr.WCode = NormalizeWCode(sr.WCode)
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
			ADD COLUMN IF NOT EXISTS email_crm text NULL,
			ADD COLUMN IF NOT EXISTS email_system text NULL,
			ADD COLUMN IF NOT EXISTS nickname text NULL,
			ADD COLUMN IF NOT EXISTS school text NULL,
			ADD COLUMN IF NOT EXISTS level text NULL,
			ADD COLUMN IF NOT EXISTS year text NULL
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
				school text NOT NULL DEFAULT '',
				level text NOT NULL DEFAULT '',
				email_crm text NOT NULL DEFAULT '',
				student_phone text NOT NULL DEFAULT '',
				parent_phone text NOT NULL DEFAULT ''
			) ON COMMIT DROP
		`); err != nil {
			return 0, fmt.Errorf("create temp table: %w", err)
		}

		copyCount, err := tx.CopyFrom(
			ctx,
			pgx.Identifier{"_sync_students"},
			[]string{"wcode", "full_name", "nickname", "school", "level", "email_crm", "student_phone", "parent_phone"},
			pgx.CopyFromRows(studentCopies(batch)),
		)
		if err != nil {
			return 0, fmt.Errorf("copy to temp: %w", err)
		}
		_ = copyCount

		res, err := tx.Exec(ctx, `
			INSERT INTO students (wcode, full_name, notes, nickname, school, level, email_crm, student_phone, parent_phone)
			SELECT ss.wcode, ss.full_name, '', NULLIF(ss.nickname, ''), NULLIF(ss.school, ''), NULLIF(ss.level, ''), NULLIF(ss.email_crm, ''), NULLIF(ss.student_phone, ''), NULLIF(ss.parent_phone, '') FROM _sync_students ss
			ON CONFLICT (LOWER(wcode)) DO UPDATE
			SET full_name = CASE
			                  WHEN LOWER(BTRIM(EXCLUDED.full_name)) = LOWER(BTRIM(EXCLUDED.wcode))
			                    THEN students.full_name
			                  ELSE EXCLUDED.full_name
			                END,
			    nickname = CASE WHEN NULLIF(EXCLUDED.nickname, '') IS NOT NULL THEN EXCLUDED.nickname ELSE students.nickname END,
			    school = CASE WHEN NULLIF(EXCLUDED.school, '') IS NOT NULL THEN EXCLUDED.school ELSE students.school END,
			    level = CASE WHEN NULLIF(EXCLUDED.level, '') IS NOT NULL THEN EXCLUDED.level ELSE students.level END,
			    email_crm = CASE WHEN NULLIF(EXCLUDED.email_crm, '') IS NOT NULL THEN EXCLUDED.email_crm ELSE students.email_crm END,
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
		sources[i] = []any{s.WCode, s.FullName, s.Nickname, s.School, s.Level, s.PrimaryEmail, s.StudentPhone, s.ParentPhone}
	}
	return sources
}
