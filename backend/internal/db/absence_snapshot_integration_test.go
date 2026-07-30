package db

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/snapshot"
)

func TestAbsenceMissedSessionsCreate_StoresSnapshot(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-snap-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "SnapRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "SNAP-" + suffix, Name: "Snapshot Course " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	sessionID := createTestSession(t, ctx, q, course.ID, teacherID, room.ID,
		time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 10, 11, 0, 0, 0, time.UTC),
	)

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:    "WSNAP-" + suffix,
		CourseID: course.ID,
		DateFrom: pgtype.Date{Time: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:   pgtype.Date{Time: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:   pgtype.Text{String: "test", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := q.AbsenceMissedSessionsCreate(ctx, absence.ID, []pgtype.UUID{sessionID}); err != nil {
		t.Fatal(err)
	}

	// Query the raw row to verify snapshot metadata columns
	var snapshotJSON []byte
	var schemaVersion pgtype.Int2
	var capturedAt pgtype.Timestamptz
	var quality string
	var source pgtype.Text
	err = q.db.QueryRow(ctx, `
		SELECT session_snapshot_at_submission, snapshot_schema_version,
		       snapshot_captured_at, snapshot_quality, snapshot_source
		FROM absence_missed_sessions
		WHERE absence_id = $1 AND session_id = $2
	`, absence.ID, sessionID).Scan(&snapshotJSON, &schemaVersion, &capturedAt, &quality, &source)
	if err != nil {
		t.Fatalf("query snapshot metadata: %v", err)
	}

	if snapshotJSON == nil {
		t.Fatal("expected session_snapshot_at_submission to be non-nil")
	}
	if !schemaVersion.Valid || schemaVersion.Int16 != 1 {
		t.Fatalf("expected snapshot_schema_version = 1, got %v", schemaVersion)
	}
	if !capturedAt.Valid {
		t.Fatal("expected snapshot_captured_at to be set")
	}
	if quality != "exact" {
		t.Fatalf("expected snapshot_quality = 'exact', got %q", quality)
	}
	if !source.Valid || source.String != "captured_at_submission" {
		t.Fatalf("expected snapshot_source = 'captured_at_submission', got %v", source)
	}

	// Verify the snapshot JSON is a valid SessionSnapshotV1
	var snap snapshot.SessionSnapshotV1
	if err := json.Unmarshal(snapshotJSON, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.SchemaVersion != 1 {
		t.Errorf("expected snapshot schema_version 1, got %d", snap.SchemaVersion)
	}
	if snap.Course.Code != course.Code {
		t.Errorf("expected snapshot course code %q, got %q", course.Code, snap.Course.Code)
	}
}

func TestAbsenceMissedSessionsCreateWithSnapshot_VersionConflict(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-vconf-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "VConfRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "VCONF-" + suffix, Name: "Version Conflict Course " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	sessionID := createTestSession(t, ctx, q, course.ID, teacherID, room.ID,
		time.Date(2026, 6, 11, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 11, 11, 0, 0, 0, time.UTC),
	)

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:    "WVCONF-" + suffix,
		CourseID: course.ID,
		DateFrom: pgtype.Date{Time: time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:   pgtype.Date{Time: time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:   pgtype.Text{String: "test", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Get current session version
	sessionRow, err := q.SessionGetByIDForSnapshot(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	currentVersion := sessionRow.Version

	// Mutate the session to increment its version
	if _, err := q.db.Exec(ctx, `UPDATE sessions SET version = version + 1 WHERE id = $1`, sessionID); err != nil {
		t.Fatal(err)
	}

	// Try to create missed session with the old version - should fail
	wrongVersion := currentVersion
	inputs := []MissedSessionSnapshotInput{
		{SessionID: sessionID, ExpectedVersion: &wrongVersion},
	}
	_, err = q.AbsenceMissedSessionsCreateWithSnapshot(ctx, absence.ID, inputs, "Europe/London", DefaultSnapshotBuilder)
	if err == nil {
		t.Fatal("expected version conflict error, got nil")
	}
	var versionErr *SessionVersionConflictError
	if !errors.As(err, &versionErr) {
		t.Fatalf("expected *SessionVersionConflictError, got %T: %v", err, err)
	}
	if versionErr.ExpectedVersion != int(currentVersion) {
		t.Errorf("expected ExpectedVersion %d, got %d", currentVersion, versionErr.ExpectedVersion)
	}
	if versionErr.ActualVersion != int(currentVersion+1) {
		t.Errorf("expected ActualVersion %d, got %d", currentVersion+1, versionErr.ActualVersion)
	}
}

func TestAbsenceMissedSessionsCreateWithSnapshot_NoVersionCheck(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-nvc-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "NVCRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "NVC-" + suffix, Name: "No Version Check Course " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	sessionID := createTestSession(t, ctx, q, course.ID, teacherID, room.ID,
		time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 12, 11, 0, 0, 0, time.UTC),
	)

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:    "WNVC-" + suffix,
		CourseID: course.ID,
		DateFrom: pgtype.Date{Time: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:   pgtype.Date{Time: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:   pgtype.Text{String: "test", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mutate the session to increment its version
	if _, err := q.db.Exec(ctx, `UPDATE sessions SET version = version + 1 WHERE id = $1`, sessionID); err != nil {
		t.Fatal(err)
	}

	// Create missed session with nil ExpectedVersion - should succeed despite version mismatch
	inputs := []MissedSessionSnapshotInput{
		{SessionID: sessionID, ExpectedVersion: nil},
	}
	results, err := q.AbsenceMissedSessionsCreateWithSnapshot(ctx, absence.ID, inputs, "Europe/London", DefaultSnapshotBuilder)
	if err != nil {
		t.Fatalf("expected success with nil version, got: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Quality != "exact" {
		t.Errorf("expected quality 'exact', got %q", results[0].Quality)
	}
	if results[0].Source != "captured_at_submission" {
		t.Errorf("expected source 'captured_at_submission', got %q", results[0].Source)
	}
	if results[0].SchemaVersion != 1 {
		t.Errorf("expected schema_version 1, got %d", results[0].SchemaVersion)
	}
}

func TestAbsenceMissedSessionsCreate_DuplicateSessionIDempotent(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-idem-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "IdemRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "IDEM-" + suffix, Name: "Idempotent Course " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	sessionID := createTestSession(t, ctx, q, course.ID, teacherID, room.ID,
		time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 13, 11, 0, 0, 0, time.UTC),
	)

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:    "WIDEM-" + suffix,
		CourseID: course.ID,
		DateFrom: pgtype.Date{Time: time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:   pgtype.Date{Time: time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:   pgtype.Text{String: "test", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Insert the same missed session twice - should be idempotent
	if err := q.AbsenceMissedSessionsCreate(ctx, absence.ID, []pgtype.UUID{sessionID}); err != nil {
		t.Fatal(err)
	}
	if err := q.AbsenceMissedSessionsCreate(ctx, absence.ID, []pgtype.UUID{sessionID}); err != nil {
		t.Fatal(err)
	}

	// Verify only one row exists
	var count int
	err = q.db.QueryRow(ctx, `
		SELECT count(*) FROM absence_missed_sessions
		WHERE absence_id = $1 AND session_id = $2
	`, absence.ID, sessionID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row after duplicate insert, got %d", count)
	}
}
