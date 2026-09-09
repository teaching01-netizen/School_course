package db

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// RED (Hole B backlog): migration 00125 must retire version-only false-criticals
// and empty-reason valid warnings, while keeping overlap and deletion rows open.
func TestScheduleImpactRetiresVersionOnlyNoise(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	defer pool.Close()
	ctx := context.Background()
	f := newImpactFixture(t, New(pool))
	var changeID pgtype.UUID
	err := pool.QueryRow(ctx, `INSERT INTO session_changes(session_id,session_version,changed_fields,before_snapshot,after_snapshot,old_start_at,old_end_at,new_start_at,new_end_at,old_course_id,new_course_id,old_teacher_id,new_teacher_id) SELECT id,version+1,'["room_id"]','{}','{}',start_at,end_at,start_at,end_at,course_id,course_id,teacher_id,teacher_id FROM sessions WHERE id=$1 RETURNING id`, f.sessionID).Scan(&changeID)
	if err != nil {
		t.Fatal(err)
	}
	insertIssue := func(issueType, details string) pgtype.UUID {
		t.Helper()
		var id pgtype.UUID
		// Fingerprint includes the change id: the shared migrated test DB
		// persists rows across runs, so constant fingerprints would collide
		// with still-open rows from a previous run.
		err := pool.QueryRow(ctx, `INSERT INTO absence_schedule_issues(absence_id,issue_type,severity,status,first_session_change_id,latest_session_change_id,fingerprint,details_json) VALUES($1,$2,'critical','open',$3,$3,$2||$3::text||$4::text,$4::jsonb) RETURNING id`, f.absenceID, issueType, changeID, details).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	versionOnly := insertIssue("sit_in_ineligible", `{"reasons":["session_version_changed"]}`)
	emptyWarning := insertIssue("sit_in_session_changed", `{}`)
	overlap := insertIssue("regular_session_overlap", `{"reasons":["regular_session_overlap"]}`)
	deleted := insertIssue("sit_in_session_deleted", `{"reasons":["session_deleted"]}`)

	migration, err := os.ReadFile("../../db/migrations/00125_schedule_impact_retire_version_only_noise.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Split(string(migration), "-- +goose Down")[0]
	body = body[strings.Index(body, "UPDATE absence_schedule_issues"):]
	// Reopen rows the shared migrated test DB may already have retired, then replay.
	for _, id := range []pgtype.UUID{versionOnly, emptyWarning, overlap, deleted} {
		if _, err := pool.Exec(ctx, `UPDATE absence_schedule_issues SET status='open' WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, body); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[pgtype.UUID]string{
		versionOnly: "superseded", emptyWarning: "superseded",
		overlap: "open", deleted: "open",
	} {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM absence_schedule_issues WHERE id=$1`, id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != want {
			t.Errorf("issue status=%s, want %s", status, want)
		}
	}
}
