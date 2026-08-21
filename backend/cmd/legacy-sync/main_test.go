package main

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/legacysync/normalize"
	"warwick-institute/internal/legacysync/parser"
)

// TestAssignScheduleIDs_PreservesSourceIdentity pins the CB-01 fix: schedule
// IDs parsed from the source page must never be replaced by ordinal-derived
// identity, otherwise inserting a row on the source re-keys every following
// session and attaches attendance/history to the wrong event.
func TestAssignScheduleIDs_PreservesSourceIdentity(t *testing.T) {
	aggregate := normalize.LegacyCourseAggregate{Schedules: []normalize.LegacySchedule{
		{LegacyScheduleID: "109541", Date: "2026-05-23", Begin: "13:00", End: "16:20"},
		{LegacyScheduleID: "109542", Date: "2026-05-30", Begin: "13:00", End: "16:20"},
	}}
	assignScheduleIDs(&aggregate, "7090")
	if aggregate.Schedules[0].LegacyScheduleID != "109541" || aggregate.Schedules[1].LegacyScheduleID != "109542" {
		t.Fatalf("source schedule ids were replaced: got %q, %q, want 109541, 109542",
			aggregate.Schedules[0].LegacyScheduleID, aggregate.Schedules[1].LegacyScheduleID)
	}
}

// TestAssignScheduleIDs_DerivesFallbackOnlyForMissingIdentity verifies rows
// without a source-exposed ID still receive the deterministic derived id so
// older page formats keep syncing.
func TestAssignScheduleIDs_DerivesFallbackOnlyForMissingIdentity(t *testing.T) {
	aggregate := normalize.LegacyCourseAggregate{Schedules: []normalize.LegacySchedule{
		{LegacyScheduleID: "", Date: "2026-05-23", Begin: "13:00", End: "16:20"},
		{LegacyScheduleID: "109542", Date: "2026-05-30", Begin: "13:00", End: "16:20"},
	}}
	assignScheduleIDs(&aggregate, "7090")
	derived := aggregate.Schedules[0].LegacyScheduleID
	if derived == "" || derived == derivedScheduleID("7090", 1) {
		t.Fatalf("row 0 derived id = %q, want the ordinal-0 derived id", derived)
	}
	if aggregate.Schedules[1].LegacyScheduleID != "109542" {
		t.Fatalf("row 1 id = %q, want preserved 109542", aggregate.Schedules[1].LegacyScheduleID)
	}
}

// TestShouldSkipArchivedSync pins the "sync once, then skip" rule: an
// archived course that already has a successful sync (legacy_last_synced_at
// set by a real apply) is skipped on every later sync — including the
// leader sweep, per-course refresh jobs, and the admin refresh button —
// while an archived course that has never synced, or any active course,
// keeps syncing.
func TestShouldSkipArchivedSync(t *testing.T) {
	synced := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	tests := []struct {
		name       string
		archived   bool
		lastSynced pgtype.Timestamptz
		want       bool
	}{
		{"archived course already synced once skips", true, synced, true},
		{"archived course never synced must sync", true, pgtype.Timestamptz{}, false},
		{"active course always refreshes", false, synced, false},
		{"active unsynced course syncs", false, pgtype.Timestamptz{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSkipArchivedSync(tt.archived, tt.lastSynced); got != tt.want {
				t.Fatalf("shouldSkipArchivedSync(archived=%v, synced=%v) = %v, want %v", tt.archived, tt.lastSynced.Valid, got, tt.want)
			}
		})
	}
}

// TestBuildCourseIndex verifies the complete course directory is indexed by
// legacy id for courses, teachers, and subjects — the refresh path consumes
// all three (the course row for identity, teachers/subjects to self-apply
// the master data the course references).
func TestBuildCourseIndex(t *testing.T) {
	empty := buildCourseIndex(nil)
	if len(empty.courses) != 0 || len(empty.teachers) != 0 || len(empty.subjects) != 0 {
		t.Fatalf("nil result must build an empty index, got courses=%d teachers=%d subjects=%d", len(empty.courses), len(empty.teachers), len(empty.subjects))
	}

	result := &parser.CourseListResult{
		Courses: []normalize.LegacyCourse{
			{LegacyID: "4471", Code: "PHY101", Status: "active"},
			{LegacyID: "4472", Code: "CHE101", Status: "archived"},
		},
		Teachers: []normalize.LegacyTeacher{{LegacyID: "88", Name: "Somchai", IsActive: true}},
		Subjects: []normalize.LegacySubject{{LegacyID: "5", Name: "Maths"}},
	}
	index := buildCourseIndex(result)
	if len(index.courses) != 2 {
		t.Fatalf("courses index has %d entries, want 2", len(index.courses))
	}
	if index.courses["4471"].Code != "PHY101" || index.courses["4472"].Status != "archived" {
		t.Fatalf("courses index = %#v, want both legacy courses keyed by id", index.courses)
	}
	if got := index.teachers["88"]; got.Name != "Somchai" {
		t.Fatalf("teachers index[88] = %#v, want Somchai", got)
	}
	if got := index.subjects["5"]; got.Name != "Maths" {
		t.Fatalf("subjects index[5] = %#v, want Maths", got)
	}
}

func TestTuningHelpers(t *testing.T) {
	t.Run("workerConcurrency", func(t *testing.T) {
		t.Setenv("LEGACY_SYNC_WORKERS", "")
		if got := workerConcurrency(32); got != 32 {
			t.Fatalf("unset env: workerConcurrency = %d, want clientMax 32", got)
		}
		t.Setenv("LEGACY_SYNC_WORKERS", "junk")
		if got := workerConcurrency(32); got != 32 {
			t.Fatalf("junk env: workerConcurrency = %d, want clientMax 32", got)
		}
		t.Setenv("LEGACY_SYNC_WORKERS", "3")
		if got := workerConcurrency(32); got != 3 {
			t.Fatalf("workerConcurrency = %d, want 3", got)
		}
	})

	t.Run("reconcileWorkers", func(t *testing.T) {
		// Unset falls back to min(clientMax, 16).
		t.Setenv("LEGACY_SYNC_RECONCILE_WORKERS", "")
		if got := reconcileWorkers(32); got != 16 {
			t.Fatalf("unset env: reconcileWorkers = %d, want 16 (min(32,16))", got)
		}
		if got := reconcileWorkers(8); got != 8 {
			t.Fatalf("unset env: reconcileWorkers = %d, want 8 (min(8,16))", got)
		}
		// Zero preserves the serial path.
		t.Setenv("LEGACY_SYNC_RECONCILE_WORKERS", "0")
		if got := reconcileWorkers(32); got != 0 {
			t.Fatalf("reconcileWorkers = %d, want 0 (serial)", got)
		}
		t.Setenv("LEGACY_SYNC_RECONCILE_WORKERS", "1")
		if got := reconcileWorkers(32); got != 1 {
			t.Fatalf("reconcileWorkers = %d, want 1", got)
		}
		t.Setenv("LEGACY_SYNC_RECONCILE_WORKERS", "8")
		if got := reconcileWorkers(32); got != 8 {
			t.Fatalf("reconcileWorkers = %d, want 8", got)
		}
		t.Setenv("LEGACY_SYNC_RECONCILE_WORKERS", "junk")
		if got := reconcileWorkers(32); got != 16 {
			t.Fatalf("junk env: reconcileWorkers = %d, want 16 (default)", got)
		}
	})

	t.Run("maxPoolConns", func(t *testing.T) {
		cases := []struct {
			env, workers, want int
		}{
			{0, 8, 64},
			{0, 40, 80},
			{100, 8, 100},
			{-5, 8, 64},
		}
		for _, tc := range cases {
			if got := maxPoolConns(tc.env, tc.workers); got != tc.want {
				t.Fatalf("maxPoolConns(%d, %d) = %d, want %d", tc.env, tc.workers, got, tc.want)
			}
		}
	})
}
