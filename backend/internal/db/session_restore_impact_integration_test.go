package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// deletedIssueFingerprint mirrors the fingerprint the impact analysis builds
// for "session deleted" issues (sessionchangeimpact.issueFingerprint with an
// empty sit-in and missed session id).
func deletedIssueFingerprint(absenceID, sessionID pgtype.UUID, issueType string) string {
	data := []byte(fmt.Sprintf("%s|%s|%s|%s|%s", absenceID.String(), issueType, sessionID.String(), "", ""))
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func insertOpenDeletedIssue(t *testing.T, pool *pgxpool.Pool, absenceID, sessionID pgtype.UUID, issueType string, changeID pgtype.UUID) pgtype.UUID {
	t.Helper()
	var issueID pgtype.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO absence_schedule_issues (
			absence_id, issue_type, severity, status,
			first_session_change_id, latest_session_change_id, fingerprint, details_json
		)
		VALUES ($1, $2, 'critical', 'open', $4, $4, $3, jsonb_build_object('deleted_session_id', $5::text))
		RETURNING id
	`, absenceID, issueType, deletedIssueFingerprint(absenceID, sessionID, issueType), changeID, sessionID).Scan(&issueID); err != nil {
		t.Fatal(err)
	}
	return issueID
}

// TestSoftDeleteImpactRestoreSupersedesStaleImpact pins the 00093 supersede
// migration: a legacy restore (deleted_at cleared) is a newer session change
// that retires the pending delete-impact run and any open "session deleted"
// issues, and re-emits the occurrence event so realtime consumers refresh the
// restored session.
func TestSoftDeleteImpactRestoreSupersedesStaleImpact(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)
	fixture := newImpactFixture(t, q)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := dbpool.Exec(ctx, `INSERT INTO absence_sit_ins (absence_id, session_id) VALUES ($1, $2)`, fixture.absenceID, fixture.sessionID); err != nil {
		t.Fatal(err)
	}

	// Legacy sync tombstone: soft delete bumps version (apply/schedule.go).
	if _, err := dbpool.Exec(ctx, `UPDATE sessions SET deleted_at = now(), version = version + 1 WHERE id = $1`, fixture.sessionID); err != nil {
		t.Fatal(err)
	}
	var deleteChangeID pgtype.UUID
	var deleteVersion int32
	if err := dbpool.QueryRow(ctx, `
		SELECT id, session_version FROM session_changes
		WHERE session_id = $1 AND changed_fields @> '["deleted"]'
	`, fixture.sessionID).Scan(&deleteChangeID, &deleteVersion); err != nil {
		t.Fatal(err)
	}
	var pendingRuns int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM session_change_impact_runs WHERE session_change_id = $1 AND status = 'pending'`, deleteChangeID).Scan(&pendingRuns); err != nil {
		t.Fatal(err)
	}
	if pendingRuns != 1 {
		t.Fatalf("pending delete-impact runs = %d, want 1", pendingRuns)
	}
	// Impact analysis has already surfaced the deletion as a critical issue.
	issueID := insertOpenDeletedIssue(t, dbpool, fixture.absenceID, fixture.sessionID, "sit_in_session_deleted", deleteChangeID)

	// Legacy sync restore: deleted_at cleared, version bumped again.
	if _, err := dbpool.Exec(ctx, `UPDATE sessions SET deleted_at = NULL, version = version + 1 WHERE id = $1`, fixture.sessionID); err != nil {
		t.Fatal(err)
	}
	var restoreChangeID pgtype.UUID
	var restoreVersion int32
	if err := dbpool.QueryRow(ctx, `
		SELECT id, session_version FROM session_changes
		WHERE session_id = $1 AND changed_fields @> '["restored"]'
	`, fixture.sessionID).Scan(&restoreChangeID, &restoreVersion); err != nil {
		t.Fatalf("restore change not recorded: %v", err)
	}
	if restoreVersion <= deleteVersion {
		t.Fatalf("restore version = %d, want newer than delete version %d", restoreVersion, deleteVersion)
	}

	// The stale delete-impact run is retired and the restore queues no run.
	var deleteRunStatus string
	if err := dbpool.QueryRow(ctx, `SELECT status FROM session_change_impact_runs WHERE session_change_id = $1`, deleteChangeID).Scan(&deleteRunStatus); err != nil {
		t.Fatal(err)
	}
	if deleteRunStatus != "superseded" {
		t.Fatalf("delete-impact run status = %q, want superseded", deleteRunStatus)
	}
	var restoreRuns int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM session_change_impact_runs WHERE session_change_id = $1`, restoreChangeID).Scan(&restoreRuns); err != nil {
		t.Fatal(err)
	}
	if restoreRuns != 0 {
		t.Fatalf("restore change impact runs = %d, want 0", restoreRuns)
	}

	// The open delete issue is superseded by the restore.
	var issueStatus, resolutionAction string
	if err := dbpool.QueryRow(ctx, `SELECT status, COALESCE(resolution_action, '') FROM absence_schedule_issues WHERE id = $1`, issueID).Scan(&issueStatus, &resolutionAction); err != nil {
		t.Fatal(err)
	}
	if issueStatus != "superseded" || resolutionAction != "restored" {
		t.Fatalf("issue after restore = status %q action %q, want superseded/restored", issueStatus, resolutionAction)
	}

	// Delete and restore each emitted an occurrence event.
	var events int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id = $1 AND event_type = 'session.occurrence.changed.v1'`, fixture.sessionID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 2 {
		t.Fatalf("occurrence events = %d, want 2 (delete + restore)", events)
	}
}

// TestSoftDeleteImpactRestoreSupersedesMissedSessionIssue covers the second
// deletion issue type produced by impact analysis.
func TestSoftDeleteImpactRestoreSupersedesMissedSessionIssue(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)
	fixture := newImpactFixture(t, q)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := dbpool.Exec(ctx, `INSERT INTO absence_missed_sessions (absence_id, session_id) VALUES ($1, $2)`, fixture.absenceID, fixture.sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `UPDATE sessions SET deleted_at = now(), version = version + 1 WHERE id = $1`, fixture.sessionID); err != nil {
		t.Fatal(err)
	}
	var deleteChangeID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `SELECT id FROM session_changes WHERE session_id = $1 AND changed_fields @> '["deleted"]'`, fixture.sessionID).Scan(&deleteChangeID); err != nil {
		t.Fatal(err)
	}
	issueID := insertOpenDeletedIssue(t, dbpool, fixture.absenceID, fixture.sessionID, "missed_session_deleted", deleteChangeID)
	if _, err := dbpool.Exec(ctx, `UPDATE sessions SET deleted_at = NULL, version = version + 1 WHERE id = $1`, fixture.sessionID); err != nil {
		t.Fatal(err)
	}
	var issueStatus string
	if err := dbpool.QueryRow(ctx, `SELECT status FROM absence_schedule_issues WHERE id = $1`, issueID).Scan(&issueStatus); err != nil {
		t.Fatal(err)
	}
	if issueStatus != "superseded" {
		t.Fatalf("missed-session issue after restore = status %q, want superseded", issueStatus)
	}
}

// TestImpactRunStatusAllowsSuperseded pins the impact-run status contract:
// impact analysis retires stale runs with "superseded", and the runs status
// check must admit that value (00075's Up block allows it).
func TestImpactRunStatusAllowsSuperseded(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)
	fixture := newImpactFixture(t, q)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var changeID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO session_changes (
			session_id, session_version, changed_fields, before_snapshot, after_snapshot,
			old_start_at, old_end_at, new_start_at, new_end_at,
			old_course_id, new_course_id, old_teacher_id, new_teacher_id
		)
		SELECT id, version, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
		       start_at, end_at, start_at, end_at, course_id, course_id, teacher_id, teacher_id
		FROM sessions WHERE id = $1
		RETURNING id
	`, fixture.sessionID).Scan(&changeID); err != nil {
		t.Fatal(err)
	}
	if err := q.SessionChangeImpactRunCreate(ctx, changeID); err != nil {
		t.Fatal(err)
	}
	if err := q.SessionChangeImpactRunSetStatus(ctx, changeID, "superseded", "newer session change is already queued"); err != nil {
		t.Fatalf("marking an impact run superseded must be allowed: %v", err)
	}
	var status string
	if err := dbpool.QueryRow(ctx, `SELECT status FROM session_change_impact_runs WHERE session_change_id = $1`, changeID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "superseded" {
		t.Fatalf("impact run status = %q, want superseded", status)
	}
}
