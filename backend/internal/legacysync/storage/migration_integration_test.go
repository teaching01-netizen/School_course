package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	sqldb "warwick-institute/internal/db"
)

// setupTestDB mirrors the shared DB harness used across the repo's
// integration tests (see internal/courseadmin/service_integration_test.go):
// skip unless TEST_DATABASE_URL is set, migrate once, return a pooled handle.
func setupTestDB(t *testing.T) (*pgxpool.Pool, *sqldb.Queries) {
	t.Helper()
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	return pool, sqldb.New(pool)
}

func createTeacher(t *testing.T, pool *pgxpool.Pool, suffix string) pgtype.UUID {
	t.Helper()
	ctx := context.Background()
	var teacherID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (id, username, role, password_hash)
		VALUES (gen_random_uuid(), $1, 'Teacher', 'hash') RETURNING id`, "legacy-storage-teacher-"+suffix).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	return teacherID
}

func createCourse(t *testing.T, q *sqldb.Queries, suffix string) pgtype.UUID {
	t.Helper()
	course, err := q.CourseCreate(context.Background(), sqldb.CourseCreateParams{
		Code: "LEGACY-STORAGE-" + suffix,
		Name: "Legacy Storage " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	return course.ID
}

func requirePgCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected pg error code %s, got nil error", wantCode)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected *pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != wantCode {
		t.Errorf("expected pg error code %s, got %s: %v", wantCode, pgErr.Code, err)
	}
}

// TestExternalRefs_RejectDuplicateExternalIdentity
// The composite PK (source, entity_type, external_id) must reject a second
// row for the same external identity.
func TestExternalRefs_RejectDuplicateExternalIdentity(t *testing.T) {
	pool, _ := setupTestDB(t)
	ctx := context.Background()
	src := "scheduler-" + randString(6)

	insert := func() error {
		_, err := pool.Exec(ctx, `INSERT INTO external_refs (source, entity_type, external_id, internal_id)
			VALUES ($1, 'course', '7306', gen_random_uuid())`, src)
		return err
	}
	if err := insert(); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	requirePgCode(t, insert(), "23505")
}

// TestExternalRefs_AllowSameNumericIDAcrossEntityTypes
// The same external numeric id must coexist across entity types
// (e.g. course 7306 vs schedule 7306) because the PK includes entity_type.
func TestExternalRefs_AllowSameNumericIDAcrossEntityTypes(t *testing.T) {
	pool, _ := setupTestDB(t)
	ctx := context.Background()
	src := "scheduler-" + randString(6)

	for _, entityType := range []string{"course", "schedule"} {
		if _, err := pool.Exec(ctx, `INSERT INTO external_refs (source, entity_type, external_id, internal_id)
			VALUES ($1, $2, '7306', gen_random_uuid())`, src, entityType); err != nil {
			t.Fatalf("insert entity_type %q: %v", entityType, err)
		}
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM external_refs
		WHERE source = $1 AND external_id = '7306'`, src).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 refs (course + schedule), got %d", count)
	}
}

// TestEntitySnapshot_RequiresParserVersion
// parser_version is NOT NULL: omitting it must violate 23502.
func TestEntitySnapshot_RequiresParserVersion(t *testing.T) {
	pool, _ := setupTestDB(t)
	ctx := context.Background()
	src := "scheduler-" + randString(6)

	_, err := pool.Exec(ctx, `INSERT INTO legacy_entity_snapshots
		(source, entity_type, external_id, canonical_data, source_hash, observed_at)
		VALUES ($1, 'course', 's-1', '{}'::jsonb, 'h-1', now())`, src)
	requirePgCode(t, err, "23502")
}

// TestEntitySnapshot_SourceHashIsRequired
// source_hash is NOT NULL: omitting it must violate 23502.
func TestEntitySnapshot_SourceHashIsRequired(t *testing.T) {
	pool, _ := setupTestDB(t)
	ctx := context.Background()
	src := "scheduler-" + randString(6)

	_, err := pool.Exec(ctx, `INSERT INTO legacy_entity_snapshots
		(source, entity_type, external_id, canonical_data, parser_version, observed_at)
		VALUES ($1, 'course', 's-1', '{}'::jsonb, 1, now())`, src)
	requirePgCode(t, err, "23502")
}

// TestSnapshotUpsert_RoundTripsCanonicalData
// SnapshotUpsert must store a real canonical_data JSON payload and
// SnapshotGet must return it unchanged, together with the other snapshot
// columns (source_hash, parser_version, quality, applied_at).
func TestSnapshotUpsert_RoundTripsCanonicalData(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()
	src := "scheduler-" + randString(6)
	externalID := "snap-" + randString(6)
	sourceHash := "hash-" + randString(8)

	canonical := `{"code":"C1","name":"Canonical","rooms":[{"id":"r1","capacity":40}],"schedule":{"begin":"13:00","end":"16:20"}}`
	observed := time.Now().UTC().Truncate(time.Microsecond)
	applied := observed.Add(time.Minute)

	snap, err := q.SnapshotUpsert(ctx, sqldb.SnapshotUpsertParams{
		Source:        src,
		EntityType:    "course",
		ExternalID:    externalID,
		CanonicalData: canonical,
		SourceHash:    sourceHash,
		ParserVersion: 3,
		ObservedAt:    pgtype.Timestamptz{Time: observed, Valid: true},
		AppliedAt:     pgtype.Timestamptz{Time: applied, Valid: true},
		Quality:       "ok",
	})
	if err != nil {
		t.Fatalf("SnapshotUpsert: %v", err)
	}
	if snap.ParserVersion != 3 || snap.Quality != "ok" || !snap.AppliedAt.Valid {
		t.Errorf("SnapshotUpsert returned row not as inserted: %+v", snap)
	}

	got, err := q.SnapshotGet(ctx, sqldb.SnapshotGetParams{
		Source: src, EntityType: "course", ExternalID: externalID,
	})
	if err != nil {
		t.Fatalf("SnapshotGet: %v", err)
	}
	var gotData, wantData map[string]any
	if err := json.Unmarshal(got.CanonicalData, &gotData); err != nil {
		t.Fatalf("stored canonical_data is not valid JSON: %v (%q)", err, got.CanonicalData)
	}
	if err := json.Unmarshal([]byte(canonical), &wantData); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotData, wantData) {
		t.Errorf("canonical_data corrupted:\n got %v\nwant %v", gotData, wantData)
	}
	if got.SourceHash != sourceHash {
		t.Errorf("expected source_hash %q, got %q", sourceHash, got.SourceHash)
	}
	if got.ParserVersion != 3 {
		t.Errorf("expected parser_version 3, got %d", got.ParserVersion)
	}
	if got.Quality != "ok" {
		t.Errorf("expected quality ok, got %q", got.Quality)
	}
	if !got.AppliedAt.Valid || !got.AppliedAt.Time.Equal(applied) {
		t.Errorf("expected applied_at %v, got %+v", applied, got.AppliedAt)
	}
	if !got.ObservedAt.Valid || !got.ObservedAt.Time.Equal(observed) {
		t.Errorf("expected observed_at %v, got %+v", observed, got.ObservedAt)
	}
}

// TestSyncRun_TracksGenerationLifecycle
// SyncRunCreate starts a run as 'running'; SyncRunComplete flips it to
// 'completed' with counters and completed_at; SyncRunListRecent returns it
// first (newest run).
func TestSyncRun_TracksGenerationLifecycle(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()

	run, err := q.SyncRunCreate(ctx, "full_sweep")
	if err != nil {
		t.Fatalf("SyncRunCreate: %v", err)
	}
	if run.Mode != "full_sweep" {
		t.Errorf("expected mode full_sweep, got %q", run.Mode)
	}
	if run.Status != "running" {
		t.Errorf("expected status running, got %q", run.Status)
	}
	if !run.StartedAt.Valid {
		t.Error("expected started_at to be set")
	}
	if run.CompletedAt.Valid {
		t.Error("expected completed_at to be NULL while running")
	}

	err = q.SyncRunComplete(ctx, sqldb.SyncRunCompleteParams{
		ID:                       run.ID,
		Status:                   "completed",
		PagesRequested:           3,
		EntitiesParsed:           4,
		EntitiesChanged:          5,
		EntitiesApplied:          6,
		ParseFailures:            7,
		ReconciliationMismatches: 8,
		SourceLatencyMs:          pgtype.Int4{Int32: 250, Valid: true},
		LastError:                pgtype.Text{},
	})
	if err != nil {
		t.Fatalf("SyncRunComplete: %v", err)
	}

	recent, err := q.SyncRunListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("SyncRunListRecent: %v", err)
	}
	if len(recent) == 0 {
		t.Fatal("expected at least one run in SyncRunListRecent")
	}
	got := recent[0]
	if got.ID != run.ID {
		t.Errorf("expected first recent run to be the completed run, got %v", got.ID)
	}
	if got.Status != "completed" {
		t.Errorf("expected status completed, got %q", got.Status)
	}
	if !got.CompletedAt.Valid {
		t.Error("expected completed_at to be set")
	}
	if got.PagesRequested != 3 || got.EntitiesParsed != 4 || got.EntitiesChanged != 5 ||
		got.EntitiesApplied != 6 || got.ParseFailures != 7 || got.ReconciliationMismatches != 8 {
		t.Errorf("counters not persisted: %+v", got)
	}
	if !got.SourceLatencyMs.Valid || got.SourceLatencyMs.Int32 != 250 {
		t.Errorf("expected source_latency_ms 250, got %+v", got.SourceLatencyMs)
	}
}

// TestConflict_PreservesSourcePayload
// The source_payload jsonb must round-trip without corruption: every key and
// value the caller inserted must be present in the stored row.
//
// Note: jsonb parameters are text-typed with an explicit ::text::jsonb cast
// in the query (repo precedent: db/queries/session_change_impact.sql), so a
// real JSON string can be passed through this repo's simple-protocol pools.
func TestConflict_PreservesSourcePayload(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()

	payload := `{"external_id":"7306","delta":42,"nested":{"tags":["a","b"]}}`
	// ConflictInsert dedups open rows by (entity_type, external_id,
	// conflict_type), so a per-run identity keeps the insert valid even when
	// the shared test DB still holds leftover rows from earlier runs.
	externalID := "7306-" + uuid.NewString()
	conflict, err := q.ConflictInsert(ctx, sqldb.ConflictInsertParams{
		EntityType:    "course",
		ExternalID:    externalID,
		ConflictType:  "mapping_conflict",
		Category:      "mapping_conflict",
		SourcePayload: payload,
		LocalPayload:  `{"local":true}`,
		Message:       pgtype.Text{String: "mapping drift", Valid: true},
	})
	if err != nil {
		t.Fatalf("ConflictInsert: %v", err)
	}
	if conflict.Status != "open" {
		t.Errorf("expected status open, got %q", conflict.Status)
	}

	// Read back through ConflictListOpen; the shared DB may hold rows from
	// earlier runs, so take the newest match for this external identity.
	open, err := q.ConflictListOpen(ctx)
	if err != nil {
		t.Fatalf("ConflictListOpen: %v", err)
	}
	var stored *sqldb.LegacySyncConflict
	for i := range open {
		if open[i].EntityType == "course" && open[i].ExternalID == externalID {
			stored = &open[i]
			break
		}
	}
	if stored == nil {
		t.Fatal("conflict not found via ConflictListOpen")
	}

	var got, want map[string]any
	if err := json.Unmarshal(stored.SourcePayload, &got); err != nil {
		t.Fatalf("stored source_payload is not valid JSON: %v (%q)", err, stored.SourcePayload)
	}
	if err := json.Unmarshal([]byte(payload), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("source_payload corrupted:\n got %v\nwant %v", got, want)
	}
	var localGot map[string]any
	if err := json.Unmarshal(stored.LocalPayload, &localGot); err != nil {
		t.Fatalf("stored local_payload is not valid JSON: %v (%q)", err, stored.LocalPayload)
	}
	if !reflect.DeepEqual(localGot, map[string]any{"local": true}) {
		t.Errorf("local_payload corrupted: %v", localGot)
	}
}

// TestTombstoneState_RejectsInvalidTransition
// external_refs.state is CHECK-constrained to the tombstone lifecycle;
// an out-of-enum value must be rejected, the valid walk accepted.
func TestTombstoneState_RejectsInvalidTransition(t *testing.T) {
	pool, q := setupTestDB(t)
	ctx := context.Background()
	src := "scheduler-" + randString(6)

	ref, err := q.ExternalRefUpsert(ctx, sqldb.ExternalRefUpsertParams{
		Source:     src,
		EntityType: "course",
		ExternalID: "7306",
		InternalID: mustRandomUUID(t, pool),
		SourceHash: pgtype.Text{String: "h-1", Valid: true},
	})
	if err != nil {
		t.Fatalf("ExternalRefUpsert: %v", err)
	}
	if ref.State != "active" {
		t.Errorf("expected default state active, got %q", ref.State)
	}

	setState := func(state string) error {
		_, err := pool.Exec(ctx, `UPDATE external_refs SET state = $1
			WHERE source = $2 AND entity_type = 'course' AND external_id = '7306'`, state, src)
		return err
	}
	for _, state := range []string{"suspected_missing", "confirmed_missing", "tombstoned", "restored"} {
		if err := setState(state); err != nil {
			t.Fatalf("valid transition to %q rejected: %v", state, err)
		}
	}
	requirePgCode(t, setState("weird"), "23514")

	var finalState string
	if err := pool.QueryRow(ctx, `SELECT state FROM external_refs
		WHERE source = $1 AND entity_type = 'course' AND external_id = '7306'`, src).Scan(&finalState); err != nil {
		t.Fatal(err)
	}
	if finalState != "restored" {
		t.Errorf("expected failed update to leave state restored, got %q", finalState)
	}
}

// TestSessions_EnforceUniqueLegacyScheduleID
// ux_sessions_legacy_schedule_id must reject a second session mapped to the
// same legacy schedule row, while allowing many NULLs.
func TestSessions_EnforceUniqueLegacyScheduleID(t *testing.T) {
	pool, q := setupTestDB(t)
	ctx := context.Background()
	suffix := randString(6)
	teacherID := createTeacher(t, pool, suffix)
	courseID := createCourse(t, q, suffix)

	day := time.Now().UTC().AddDate(0, 0, 30).Truncate(24 * time.Hour)
	create := func(hour int) sqldb.SessionCreateRow {
		t.Helper()
		start := day.Add(time.Duration(hour) * time.Hour)
		row, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
			CourseID:  courseID,
			TeacherID: teacherID,
			StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:     pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
		})
		if err != nil {
			t.Fatalf("SessionCreate: %v", err)
		}
		return row
	}
	s1 := create(9)
	s2 := create(11)

	assign := func(id pgtype.UUID, scheduleID pgtype.Text) error {
		_, err := pool.Exec(ctx, `UPDATE sessions SET legacy_schedule_id = $1 WHERE id = $2`, scheduleID, id)
		return err
	}
	scheduleID := "SCH-" + randString(8)
	if err := assign(s1.ID, pgtype.Text{String: scheduleID, Valid: true}); err != nil {
		t.Fatalf("first legacy_schedule_id assignment: %v", err)
	}
	requirePgCode(t, assign(s2.ID, pgtype.Text{String: scheduleID, Valid: true}), "23505")

	// NULLs are exempt from the partial unique index.
	if err := assign(s2.ID, pgtype.Text{}); err != nil {
		t.Fatalf("second session back to NULL must be allowed: %v", err)
	}
}

// TestCourseLegacyFields_DefaultSafely
// A course inserted with only its required columns must get safe legacy
// defaults: source_kind 'native', legacy_archived false, everything else NULL.
func TestCourseLegacyFields_DefaultSafely(t *testing.T) {
	pool, q := setupTestDB(t)
	ctx := context.Background()

	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: "LEGACY-DEF-" + randString(6),
		Name: "Legacy Defaults",
	})
	if err != nil {
		t.Fatalf("CourseCreate: %v", err)
	}

	var (
		sourceKind   string
		archived     bool
		legacyStatus pgtype.Text
		legacyExpire pgtype.Date
		legacyHash   pgtype.Text
		legacySeen   pgtype.Timestamptz
	)
	err = pool.QueryRow(ctx, `SELECT source_kind, legacy_archived, legacy_status,
		legacy_expire_date, legacy_source_hash, legacy_last_seen_at
		FROM courses WHERE id = $1`, course.ID).Scan(
		&sourceKind, &archived, &legacyStatus, &legacyExpire, &legacyHash, &legacySeen)
	if err != nil {
		t.Fatal(err)
	}
	if sourceKind != "native" {
		t.Errorf("expected source_kind native, got %q", sourceKind)
	}
	if archived {
		t.Error("expected legacy_archived false")
	}
	if legacyStatus.Valid || legacyExpire.Valid || legacyHash.Valid || legacySeen.Valid {
		t.Errorf("expected legacy columns NULL, got status=%+v expire=%+v hash=%+v seen=%+v",
			legacyStatus, legacyExpire, legacyHash, legacySeen)
	}
}

// TestLegacyChangeEvents_RejectsDuplicateSourceEventKey
// source_event_key is UNIQUE: the same change event must not be recorded twice.
func TestLegacyChangeEvents_RejectsDuplicateSourceEventKey(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()
	key := "change-" + randString(10)

	params := sqldb.ChangeEventInsertParams{
		SourceEventKey: key,
		Detector:       "poll",
		EntityType:     pgtype.Text{String: "course", Valid: true},
		ExternalID:     pgtype.Text{String: "7306", Valid: true},
		Action:         pgtype.Text{String: "upsert", Valid: true},
		ObservedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		RawPayload:     `{"delta":1,"tags":["a","b"]}`, // jsonb param is text-typed with an explicit ::text::jsonb cast
	}
	if _, err := q.ChangeEventInsert(ctx, params); err != nil {
		t.Fatalf("first ChangeEventInsert: %v", err)
	}
	_, err := q.ChangeEventInsert(ctx, params)
	requirePgCode(t, err, "23505")
}

// TestExternalRefs_UpsertIsIdempotentAndTouchesMapping
// Re-upserting the same external identity must keep one row, refresh
// source_hash and internal_id, and advance last_seen_at.
func TestExternalRefs_UpsertIsIdempotentAndTouchesMapping(t *testing.T) {
	pool, q := setupTestDB(t)
	ctx := context.Background()
	src := "scheduler-" + randString(6)
	firstID := mustRandomUUID(t, pool)
	secondID := mustRandomUUID(t, pool)

	first, err := q.ExternalRefUpsert(ctx, sqldb.ExternalRefUpsertParams{
		Source:     src,
		EntityType: "course",
		ExternalID: "7306",
		InternalID: firstID,
		SourceHash: pgtype.Text{String: "hash-v1", Valid: true},
	})
	if err != nil {
		t.Fatalf("first ExternalRefUpsert: %v", err)
	}

	// Give now() room to advance between statements.
	time.Sleep(20 * time.Millisecond)

	second, err := q.ExternalRefUpsert(ctx, sqldb.ExternalRefUpsertParams{
		Source:     src,
		EntityType: "course",
		ExternalID: "7306",
		InternalID: secondID,
		SourceHash: pgtype.Text{String: "hash-v2", Valid: true},
	})
	if err != nil {
		t.Fatalf("second ExternalRefUpsert: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM external_refs
		WHERE source = $1 AND entity_type = 'course' AND external_id = '7306'`, src).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 row after upsert, got %d", count)
	}
	if second.InternalID != secondID {
		t.Errorf("expected internal_id %v, got %v", secondID, second.InternalID)
	}
	if !second.SourceHash.Valid || second.SourceHash.String != "hash-v2" {
		t.Errorf("expected source_hash hash-v2, got %+v", second.SourceHash)
	}
	if second.LastSeenAt.Time.Before(first.LastSeenAt.Time) {
		t.Errorf("expected last_seen_at to advance, first=%v second=%v", first.LastSeenAt.Time, second.LastSeenAt.Time)
	}
}

func TestOutbox_ReusesDuplicateSourceEventKey(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()
	key := "outbox-" + randString(10)

	params := sqldb.OutboxInsertParams{
		SourceEventKey: key,
		EventType:      "course.upsert",
		Channel:        "realtime",
		EntityType:     pgtype.Text{String: "course", Valid: true},
		ExternalID:     pgtype.Text{String: "7306", Valid: true},
		Payload:        `{"delta":1,"rooms":[{"id":"r1","capacity":40}]}`,
	}
	first, err := q.OutboxInsert(ctx, params)
	if err != nil {
		t.Fatalf("first OutboxInsert: %v", err)
	}
	second, err := q.OutboxInsert(ctx, params)
	if err != nil {
		t.Fatalf("replayed OutboxInsert: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("replayed outbox id = %v, want existing id %v", second.ID, first.ID)
	}
}

// TestMigration_DownStatementsAreSyntacticallyValid
// The -- +goose Down section of 00080 must execute cleanly (inside a rolled
// back transaction) so a future rollback cannot half-apply.
func TestMigration_DownStatementsAreSyntacticallyValid(t *testing.T) {
	pool, _ := setupTestDB(t)
	ctx := context.Background()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	migrationPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations", "00080_legacy_sync_infrastructure.sql"))
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration file: %v", err)
	}
	marker := "-- +goose Down"
	idx := strings.LastIndex(string(content), marker)
	if idx < 0 {
		t.Fatalf("migration file %s has no %q marker", migrationPath, marker)
	}
	down := strings.TrimLeft(string(content)[idx+len(marker):], " \t\r\n")

	// The Down script is DDL (DROP INDEX / ALTER TABLE ... DROP COLUMN) and
	// takes ACCESS EXCLUSIVE locks. When this package runs in parallel with
	// ./internal/legacysync — whose syncer holds courses→sessions row locks —
	// the DDL can deadlock (40P01, observed). Two mitigations:
	//   1. pre-lock the domain tables in the same order the syncer acquires
	//      them (courses → sessions → session_series), so lock acquisition
	//      order is consistent and no cycle can form with it;
	//   2. bounded retry on deadlock/serialization/lock-not-available to
	//      cover lock-order divergence from other test packages.
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = func() error {
			defer tx.Rollback(ctx) //nolint:errcheck // rollback is the point; schema must survive
			for _, stmt := range []string{
				`LOCK TABLE courses IN ACCESS EXCLUSIVE MODE`,
				`LOCK TABLE sessions IN ACCESS EXCLUSIVE MODE`,
				`LOCK TABLE session_series IN ACCESS EXCLUSIVE MODE`,
			} {
				if _, err := tx.Exec(ctx, stmt); err != nil {
					return err
				}
			}
			_, err := tx.Exec(ctx, down)
			return err
		}()
		if err == nil {
			return
		}
		lastErr = err
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) &&
			(pgErr.Code == "40P01" || pgErr.Code == "40001" || pgErr.Code == "55P03") {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}
	t.Fatalf("Down statements failed to execute: %v", lastErr)
}

// mustRandomUUID generates a fresh uuid via the database.
func mustRandomUUID(t *testing.T, pool *pgxpool.Pool) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := pool.QueryRow(context.Background(), `SELECT gen_random_uuid()`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
