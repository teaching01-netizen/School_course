package db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/snapshot"
)

func TestEnforceAbsenceSitInsSnapshot_InsertRequiresSnapshot(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Creating a sit-in with quality='exact' but no snapshot should fail.
	_, err := dbpool.Exec(ctx, `
		INSERT INTO absence_sit_ins (absence_id, session_id, snapshot_quality)
		VALUES ('00000000-0000-0000-0000-000000000001'::uuid,
		        '00000000-0000-0000-0000-000000000002'::uuid,
		        'exact')
	`)
	if err == nil {
		t.Fatal("expected error inserting sit-in with quality=exact but no snapshot")
	}
	t.Logf("got expected error: %v", err)
}

func TestEnforceAbsenceSitInsSnapshot_InsertUnavailableAllowsNull(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	q := New(dbpool)

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teach-env-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "ENV-" + suffix, Name: "Enforcement Course " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "EnvRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}

	sessionID := createTestSession(t, ctx, q, course.ID, teacherID, room.ID,
		time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC),
	)

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:    "WENV-" + suffix,
		CourseID: course.ID,
		DateFrom: pgtype.Date{Time: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:   pgtype.Date{Time: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:   pgtype.Text{String: "test enforcement", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// quality='unavailable' with no snapshot should succeed.
	_, err = dbpool.Exec(ctx, `
		INSERT INTO absence_sit_ins (absence_id, session_id, snapshot_quality)
		VALUES ($1, $2, 'unavailable')
	`, absence.ID, sessionID)
	if err != nil {
		t.Fatalf("expected unavailable to succeed without snapshot, got: %v", err)
	}
}

func TestEnforceAbsenceSitInsSnapshot_InsertExactRequiresAllColumns(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	q := New(dbpool)

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teach-env2-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "ENV2-" + suffix, Name: "Enforcement Course 2 " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "EnvRoom2-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}

	sessionID := createTestSession(t, ctx, q, course.ID, teacherID, room.ID,
		time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 2, 11, 0, 0, 0, time.UTC),
	)

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:    "WENV2-" + suffix,
		CourseID: course.ID,
		DateFrom: pgtype.Date{Time: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:   pgtype.Date{Time: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:   pgtype.Text{String: "test enforcement 2", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// quality='exact' with snapshot JSON but missing schema_version should fail.
	snapJSON, _ := json.Marshal(snapshot.SessionSnapshotV1{
		SchemaVersion: 1,
		SessionID:     uuidFromPgtypeDB(sessionID),
	})
	_, err = dbpool.Exec(ctx, `
		INSERT INTO absence_sit_ins (absence_id, session_id, session_snapshot_at_assignment, snapshot_quality)
		VALUES ($1, $2, $3, 'exact')
	`, absence.ID, sessionID, snapJSON)
	if err == nil {
		t.Fatal("expected error inserting with quality=exact but missing schema_version")
	}
	t.Logf("got expected error: %v", err)
}

func TestEnforceAbsenceSitInsSnapshot_InsertExactSucceedsWithAllColumns(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	q := New(dbpool)

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teach-env3-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "ENV3-" + suffix, Name: "Enforcement Course 3 " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "EnvRoom3-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}

	sessionID := createTestSession(t, ctx, q, course.ID, teacherID, room.ID,
		time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 3, 11, 0, 0, 0, time.UTC),
	)

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:    "WENV3-" + suffix,
		CourseID: course.ID,
		DateFrom: pgtype.Date{Time: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:   pgtype.Date{Time: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:   pgtype.Text{String: "test enforcement 3", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// quality='exact' with all required columns should succeed.
	snapJSON, _ := json.Marshal(snapshot.SessionSnapshotV1{
		SchemaVersion: 1,
		SessionID:     uuidFromPgtypeDB(sessionID),
	})
	capturedAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	_, err = dbpool.Exec(ctx, `
		INSERT INTO absence_sit_ins (
			absence_id, session_id,
			session_snapshot_at_assignment, snapshot_schema_version,
			snapshot_captured_at, snapshot_quality, snapshot_source
		)
		VALUES ($1, $2, $3, 1, $4, 'exact', 'captured_at_assignment')
	`, absence.ID, sessionID, snapJSON, capturedAt)
	if err != nil {
		t.Fatalf("expected insert with full snapshot to succeed, got: %v", err)
	}
}

func TestEnforceAbsenceSitInsSnapshot_UpdateRejectsSnapshotChange(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	q := New(dbpool)

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teach-env4-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "ENV4-" + suffix, Name: "Enforcement Course 4 " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "EnvRoom4-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}

	sessionID := createTestSession(t, ctx, q, course.ID, teacherID, room.ID,
		time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 4, 11, 0, 0, 0, time.UTC),
	)

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:    "WENV4-" + suffix,
		CourseID: course.ID,
		DateFrom: pgtype.Date{Time: time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:   pgtype.Date{Time: time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:   pgtype.Text{String: "test enforcement 4", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Insert a row with quality='unavailable'.
	_, err = dbpool.Exec(ctx, `
		INSERT INTO absence_sit_ins (absence_id, session_id, snapshot_quality)
		VALUES ($1, $2, 'unavailable')
	`, absence.ID, sessionID)
	if err != nil {
		t.Fatal(err)
	}

	// Try to update it to quality='exact' with a snapshot — the immutability
	// trigger should reject the quality change first.
	var rowID pgtype.UUID
	err = dbpool.QueryRow(ctx, `
		SELECT id FROM absence_sit_ins WHERE absence_id = $1 AND session_id = $2
	`, absence.ID, sessionID).Scan(&rowID)
	if err != nil {
		t.Fatal(err)
	}

	snapJSON, _ := json.Marshal(snapshot.SessionSnapshotV1{
		SchemaVersion: 1,
		SessionID:     uuidFromPgtypeDB(sessionID),
	})
	_, err = dbpool.Exec(ctx, `
		UPDATE absence_sit_ins
		SET snapshot_quality = 'exact',
		    session_snapshot_at_assignment = $3,
		    snapshot_schema_version = 1,
		    snapshot_captured_at = now()
		WHERE id = $1 AND snapshot_quality = $2
	`, rowID, "unavailable", snapJSON)
	if err == nil {
		t.Fatal("expected error updating snapshot_quality (immutability trigger should reject)")
	}
	t.Logf("got expected error: %v", err)
}

func TestAbsenceSitInsCreateWithSnapshot_StoresSnapshotData(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teach-create-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "CREATE-" + suffix, Name: "Create Course " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "CreateRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}

	sessionID := createTestSession(t, ctx, q, course.ID, teacherID, room.ID,
		time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 5, 11, 0, 0, 0, time.UTC),
	)

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:    "WCREATE-" + suffix,
		CourseID: course.ID,
		DateFrom: pgtype.Date{Time: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:   pgtype.Date{Time: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:   pgtype.Text{String: "test create with snapshot", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	inputs := []SitInSnapshotInput{
		{SessionID: sessionID},
	}

	err = q.AbsenceSitInsCreateWithSnapshot(ctx, absence.ID, inputs, "Europe/London", DefaultSnapshotBuilder)
	if err != nil {
		t.Fatalf("AbsenceSitInsCreateWithSnapshot failed: %v", err)
	}

	// Verify snapshot metadata was stored.
	var snapshotJSON []byte
	var schemaVersion pgtype.Int2
	var quality string
	var source pgtype.Text
	err = dbpool.QueryRow(ctx, `
		SELECT session_snapshot_at_assignment, snapshot_schema_version,
		       snapshot_quality, snapshot_source
		FROM absence_sit_ins
		WHERE absence_id = $1 AND session_id = $2
	`, absence.ID, sessionID).Scan(&snapshotJSON, &schemaVersion, &quality, &source)
	if err != nil {
		t.Fatalf("query snapshot metadata: %v", err)
	}

	if snapshotJSON == nil {
		t.Fatal("expected session_snapshot_at_assignment to be non-nil")
	}
	if !schemaVersion.Valid || schemaVersion.Int16 != 1 {
		t.Fatalf("expected snapshot_schema_version = 1, got %v", schemaVersion)
	}
	if quality != "exact" {
		t.Fatalf("expected snapshot_quality = exact, got %q", quality)
	}
	if !source.Valid || source.String != "captured_at_assignment" {
		t.Fatalf("expected snapshot_source = captured_at_assignment, got %v", source)
	}

	// Verify snapshot JSON is decodable.
	snap, err := snapshot.DecodeSessionSnapshotV1(snapshotJSON)
	if err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snap.Course.Code != course.Code {
		t.Errorf("expected course code %q, got %q", course.Code, snap.Course.Code)
	}
}
