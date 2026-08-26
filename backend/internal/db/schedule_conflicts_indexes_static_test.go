package db

import (
	"strings"
	"testing"
)

func TestScheduleConflictsOverviewIndexesMigration(t *testing.T) {
	sql := readMigration(t, "00119_schedule_conflicts_overview_indexes.sql")
	for _, required := range []string{
		"-- +goose NO TRANSACTION",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_active_native_occurrence_idx",
		"WHERE deleted_at IS NULL AND source_kind = 'native'",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_active_conflict_override_idx",
		"WHERE deleted_at IS NULL AND (conflict_override OR legacy_conflict_override)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS student_busy_ranges_active_student_range_idx",
		"ON student_busy_ranges USING GIST (student_id, time_range)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS student_busy_ranges_active_conflict_override_idx",
		"WHERE deleted_at IS NULL AND conflict_override",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_active_start_idx",
		"DROP INDEX CONCURRENTLY IF EXISTS sessions_active_start_idx",
		"DROP INDEX CONCURRENTLY IF EXISTS student_busy_ranges_active_student_range_idx",
		"DROP INDEX CONCURRENTLY IF EXISTS sessions_active_native_occurrence_idx",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("00119 missing %q", required)
		}
	}
}
