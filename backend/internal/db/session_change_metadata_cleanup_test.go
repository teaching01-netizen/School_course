package db

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestScheduleImpactRetiresMetadataOnlyBacklog(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	defer pool.Close()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	f := newImpactFixture(t, New(tx))
	version := f.version
	insertChange := func(fields, shift string) pgtype.UUID {
		t.Helper()
		version++
		var id pgtype.UUID
		err := tx.QueryRow(ctx, `INSERT INTO session_changes(session_id,session_version,changed_fields,before_snapshot,after_snapshot,
 old_start_at,old_end_at,new_start_at,new_end_at,old_course_id,new_course_id,old_teacher_id,new_teacher_id)
 SELECT id,$4,$2::jsonb,'{}','{}',start_at,end_at,CASE WHEN $2::jsonb = '["end_at"]'::jsonb THEN start_at ELSE start_at+$3::interval END,end_at+$3::interval,
 course_id,course_id,teacher_id,teacher_id FROM sessions WHERE id=$1 RETURNING id`, f.sessionID, fields, shift, version).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	insertIssue := func(change pgtype.UUID, kind, status string) pgtype.UUID {
		t.Helper()
		var id pgtype.UUID
		err := tx.QueryRow(ctx, `INSERT INTO absence_schedule_issues(absence_id,issue_type,severity,status,
 first_session_change_id,latest_session_change_id,fingerprint,details_json)
 VALUES($1,$2,'critical',$3,$4,$4,$4::text||$2,'{"reasons":["regular_session_overlap"]}') RETURNING id`, f.absenceID, kind, status, change).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	cases := map[pgtype.UUID]string{}
	for _, fields := range []string{`["room_id"]`, `["teacher_id"]`, `["room_id","teacher_id"]`} {
		change := insertChange(fields, "0 hours")
		cases[insertIssue(change, "sit_in_ineligible", "open")] = "superseded"
		cases[insertIssue(change, "short_notice_change", "needs_review")] = "superseded"
		cases[insertIssue(change, "past_time_change", "resolved")] = "resolved"
		if _, err := tx.Exec(ctx, `INSERT INTO session_change_impact_runs(session_change_id,status) VALUES($1,'pending')`, change); err != nil {
			t.Fatal(err)
		}
	}
	for _, shift := range []string{"1 hour", "1 day"} {
		change := insertChange(`["start_at","end_at","room_id"]`, shift)
		cases[insertIssue(change, "short_notice_change", "open")] = "open"
	}
	endOnly := insertChange(`["end_at"]`, "1 hour")
	cases[insertIssue(endOnly, "short_notice_change", "open")] = "open"
	deletion := insertChange(`["deleted"]`, "0 hours")
	cases[insertIssue(deletion, "sit_in_session_deleted", "open")] = "open"
	cases[insertIssue(deletion, "missed_session_deleted", "open")] = "open"
	if _, err := tx.Exec(ctx, `INSERT INTO session_change_impact_targets(session_change_id,absence_id,relation_type) VALUES($1,$2,'sit_in')`, deletion, f.absenceID); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../db/migrations/00127_schedule_impact_retire_metadata_only.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Split(string(migration), "-- +goose Down")[0]
	if _, err := tx.Exec(ctx, body); err != nil {
		t.Fatal(err)
	}
	for id, want := range cases {
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM absence_schedule_issues WHERE id=$1`, id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != want {
			t.Errorf("issue %v status=%s, want %s", id, status, want)
		}
	}
	q := New(tx)
	absences, _, err := q.ManagedAbsenceList(ctx, AbsenceFilter{IDs: []pgtype.UUID{f.absenceID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(absences) != 1 {
		t.Fatalf("inbox rows=%d, want 1", len(absences))
	}
	absence := absences[0]
	if absence.OpenScheduleIssues != 5 {
		t.Fatalf("inbox issue count=%d, want 5 time/deletion issues", absence.OpenScheduleIssues)
	}
	queue, err := q.ScheduleImpactQueue(ctx, ScheduleImpactQueueFilter{Query: absence.Wcode, Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 5 {
		t.Fatalf("queue count=%d, want 5 time/deletion issues", len(queue))
	}
	var pending int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM session_change_impact_runs run JOIN session_changes sc ON sc.id=run.session_change_id
 WHERE sc.session_id=$1 AND run.status='pending'`, f.sessionID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("metadata-only pending runs=%d", pending)
	}
	// Replaying cleanup must leave issue versions stable.
	var before, after int
	if err := tx.QueryRow(ctx, `SELECT sum(issue_version) FROM absence_schedule_issues WHERE absence_id=$1`, f.absenceID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, body); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT sum(issue_version) FROM absence_schedule_issues WHERE absence_id=$1`, f.absenceID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("cleanup is not idempotent")
	}
}
