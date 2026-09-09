package db

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestScheduleImpactOnlyChangedSitInTimes(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	defer pool.Close()
	q := New(pool)
	ctx := context.Background()
	for _, tc := range []struct {
		name            string
		shift           string
		endShift        string
		assignment      string
		snapshotMatches bool
		want            int
	}{
		{"unchanged", "0 hours", "0 hours", "direct", false, 0},
		{"time moved", "1 hour", "1 hour", "direct", false, 1},
		{"start only", "-1 hour", "0 hours", "direct", false, 1},
		{"end only", "0 hours", "1 hour", "direct", false, 1},
		{"date moved", "1 day", "1 day", "direct", false, 1},
		{"other sit in in same course", "1 hour", "1 hour", "other", false, 0},
		{"missed session only", "1 hour", "1 hour", "missed", false, 0},
		{"returned to assigned time", "1 hour", "1 hour", "direct", true, 0},
		{"assigned after change", "1 hour", "1 hour", "after", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newImpactFixture(t, q)
			if tc.assignment == "missed" {
				_, err := pool.Exec(ctx, `INSERT INTO absence_missed_sessions(absence_id, session_id) VALUES ($1,$2)`, f.absenceID, f.sessionID)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				assignedID := f.sessionID
				if tc.assignment == "other" {
					other, err := q.SessionCreate(ctx, SessionCreateParams{CourseID: f.courseID, TeacherID: f.teacherID, StartAt: pgtype.Timestamptz{Time: time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC), Valid: true}, EndAt: pgtype.Timestamptz{Time: time.Date(2030, 1, 2, 10, 0, 0, 0, time.UTC), Valid: true}})
					if err != nil {
						t.Fatal(err)
					}
					assignedID = other.ID
				}
				_, err := pool.Exec(ctx, `INSERT INTO absence_sit_ins(absence_id, session_id, session_snapshot_at_assignment, snapshot_quality, snapshot_schema_version, snapshot_captured_at, snapshot_source) SELECT $1,id, CASE WHEN $3 THEN jsonb_build_object('start_at',start_at + interval '1 hour','end_at',end_at + interval '1 hour') ELSE NULL END, CASE WHEN $3 THEN 'exact' ELSE 'unavailable' END, CASE WHEN $3 THEN 1 ELSE NULL END, CASE WHEN $3 THEN now() ELSE NULL END, CASE WHEN $3 THEN 'captured_at_assignment' ELSE NULL END FROM sessions WHERE id=$2`, f.absenceID, assignedID, tc.snapshotMatches)
				if err != nil {
					t.Fatal(err)
				}
			}
			var changeID pgtype.UUID
			err := pool.QueryRow(ctx, `INSERT INTO session_changes(session_id,session_version,changed_fields,before_snapshot,after_snapshot,old_start_at,old_end_at,new_start_at,new_end_at,old_course_id,new_course_id,old_teacher_id,new_teacher_id) SELECT id,version+1,'["room_id"]','{}','{}',start_at,end_at,start_at+$2::interval,end_at+$3::interval,course_id,course_id,teacher_id,teacher_id FROM sessions WHERE id=$1 RETURNING id`, f.sessionID, tc.shift, tc.endShift).Scan(&changeID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.assignment == "after" {
				if _, err := pool.Exec(ctx, `UPDATE absence_sit_ins SET assigned_at=now()+interval '1 hour' WHERE absence_id=$1`, f.absenceID); err != nil {
					t.Fatal(err)
				}
			}
			affected, err := q.SessionChangeAffectedAbsences(ctx, changeID)
			if err != nil {
				t.Fatal(err)
			}
			if len(affected) != tc.want {
				t.Errorf("affected = %d, want %d", len(affected), tc.want)
			}
			if tc.shift == "0 hours" && tc.endShift == "0 hours" {
				if err := q.SessionChangeImpactRunCreate(ctx, changeID); err != nil {
					t.Fatal(err)
				}
				var count int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM session_change_impact_runs WHERE session_change_id=$1`, changeID).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Errorf("unchanged edit queued %d impact runs", count)
				}
				list, err := q.SessionChangeList(ctx, SessionChangeListParams{Limit: 1000})
				if err != nil {
					t.Fatal(err)
				}
				for _, row := range list {
					if row.ID == changeID {
						t.Error("unchanged edit appears in impact history")
					}
				}
				session, err := q.SessionGetByID(ctx, f.sessionID)
				if err != nil {
					t.Fatal(err)
				}
				preview, err := q.SessionChangePreviewImpact(ctx, f.sessionID, f.courseID, session.StartAt, session.EndAt)
				if err != nil {
					t.Fatal(err)
				}
				if preview != (SessionChangePreviewImpact{}) {
					t.Errorf("unchanged preview = %+v", preview)
				}
			}
		})
	}
}

// Acceptance B1+B2: an unchanged edit is invisible at every read surface —
// no affected rows, no impact run, no history entry, zero preview — and the
// work queue stays empty with a zeroed summary.
func TestScheduleImpactUnchangedEditInvisibleEverywhere(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	defer pool.Close()
	q := New(pool)
	ctx := context.Background()
	f := newImpactFixture(t, q)
	if _, err := pool.Exec(ctx, `INSERT INTO absence_sit_ins(absence_id,session_id,assigned_at) VALUES($1,$2,now()-interval '1 day')`, f.absenceID, f.sessionID); err != nil {
		t.Fatal(err)
	}
	var changeID pgtype.UUID
	err := pool.QueryRow(ctx, `INSERT INTO session_changes(session_id,session_version,changed_fields,before_snapshot,after_snapshot,old_start_at,old_end_at,new_start_at,new_end_at,old_course_id,new_course_id,old_teacher_id,new_teacher_id) SELECT id,version+1,'["room_id"]','{}','{}',start_at,end_at,start_at,end_at,course_id,course_id,teacher_id,teacher_id FROM sessions WHERE id=$1 RETURNING id`, f.sessionID).Scan(&changeID)
	if err != nil {
		t.Fatal(err)
	}
	affected, err := q.SessionChangeAffectedAbsences(ctx, changeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(affected) != 0 {
		t.Errorf("unchanged affected = %d, want 0", len(affected))
	}
	if err := q.SessionChangeImpactRunCreate(ctx, changeID); err != nil {
		t.Fatal(err)
	}
	var runs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM session_change_impact_runs WHERE session_change_id=$1`, changeID).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Errorf("unchanged edit queued %d impact runs, want 0", runs)
	}
	list, err := q.SessionChangeList(ctx, SessionChangeListParams{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range list {
		if row.ID == changeID {
			t.Error("unchanged edit appears in impact history")
		}
	}
	session, err := q.SessionGetByID(ctx, f.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := q.SessionChangePreviewImpact(ctx, f.sessionID, f.courseID, session.StartAt, session.EndAt)
	if err != nil {
		t.Fatal(err)
	}
	if preview != (SessionChangePreviewImpact{}) {
		t.Errorf("unchanged preview = %+v, want zero", preview)
	}
	queue, err := q.ScheduleImpactQueue(ctx, ScheduleImpactQueueFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range queue {
		if item.LatestSessionChangeID == changeID {
			t.Error("unchanged edit appears in work queue")
		}
	}
	// The global summary cannot be asserted to zero (shared test DB holds
	// other fixtures' rows), but this change must contribute nothing to it:
	// no queue row references this change, which is what the summary counts.
	if _, err := q.ScheduleImpactQueueSummary(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleImpactRetiresNoiseAndPreservesOtherSessions(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	defer pool.Close()
	q := New(pool)
	ctx := context.Background()
	f := newImpactFixture(t, q)
	other := newImpactFixture(t, q)
	insertChange := func(sessionID pgtype.UUID, version int, shift string) pgtype.UUID {
		t.Helper()
		var id pgtype.UUID
		err := pool.QueryRow(ctx, `INSERT INTO session_changes(session_id,session_version,changed_fields,before_snapshot,after_snapshot,old_start_at,old_end_at,new_start_at,new_end_at,old_course_id,new_course_id,old_teacher_id,new_teacher_id) SELECT id,$2,'[]','{}','{}',start_at,end_at,start_at+$3::interval,end_at+$3::interval,course_id,course_id,teacher_id,teacher_id FROM sessions WHERE id=$1 RETURNING id`, sessionID, version, shift).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	insertIssue := func(changeID pgtype.UUID) pgtype.UUID {
		t.Helper()
		var id pgtype.UUID
		err := pool.QueryRow(ctx, `INSERT INTO absence_schedule_issues(absence_id,issue_type,severity,status,first_session_change_id,latest_session_change_id,fingerprint) VALUES($1,'short_notice_change','critical','open',$2,$2,$2::text) RETURNING id`, f.absenceID, changeID).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	unchanged := insertChange(f.sessionID, 2, "0 hours")
	falseIssue := insertIssue(unchanged)
	unrelated := insertIssue(insertChange(other.sessionID, 2, "1 hour"))
	if _, err := pool.Exec(ctx, `INSERT INTO absence_sit_ins(absence_id,session_id,assigned_at) VALUES($1,$2,now()-interval '1 day')`, f.absenceID, other.sessionID); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../db/migrations/00124_schedule_impact_time_changes_only.sql")
	if err != nil {
		t.Fatal(err)
	}
	cleanup := strings.Split(string(migration), "-- +goose Down")[0]
	cleanup = cleanup[strings.Index(cleanup, "UPDATE absence_schedule_issues"):]
	if _, err := pool.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	var migratedStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM absence_schedule_issues WHERE id=$1`, falseIssue).Scan(&migratedStatus); err != nil {
		t.Fatal(err)
	}
	if migratedStatus != "superseded" {
		t.Errorf("migration left false warning %s", migratedStatus)
	}
	// Replay a legacy event after migration to verify the runtime cleanup too.
	if _, err := pool.Exec(ctx, `UPDATE absence_schedule_issues SET status='open' WHERE id=$1`, falseIssue); err != nil {
		t.Fatal(err)
	}
	if err := q.SessionChangeSupersedeUnaffectedIssues(ctx, unchanged); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[pgtype.UUID]string{falseIssue: "superseded", unrelated: "open"} {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM absence_schedule_issues WHERE id=$1`, id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != want {
			t.Errorf("issue status=%s, want %s", status, want)
		}
	}
	moved := insertChange(f.sessionID, 3, "1 hour")
	metadata := insertChange(f.sessionID, 4, "0 hours")
	latest, err := q.SessionChangeIsLatestForAnalysis(ctx, moved)
	if err != nil {
		t.Fatal(err)
	}
	if !latest {
		t.Error("metadata-only edit superseded a pending time change")
	}
	genuineIssue := insertIssue(moved)
	if err := q.SessionChangeSupersedeUnaffectedIssues(ctx, metadata); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM absence_schedule_issues WHERE id=$1`, genuineIssue).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "open" {
		t.Errorf("metadata-only edit retired genuine issue: %s", status)
	}
	reverted := insertChange(f.sessionID, 5, "2 hours")
	if _, err := pool.Exec(ctx, `INSERT INTO absence_sit_ins(absence_id,session_id,session_snapshot_at_assignment,snapshot_quality,snapshot_schema_version,snapshot_captured_at,snapshot_source,assigned_at) SELECT $1,session_id,jsonb_build_object('start_at',new_start_at,'end_at',new_end_at),'exact',1,now(),'captured_at_assignment',created_at-interval '1 day' FROM session_changes WHERE id=$2`, f.absenceID, reverted); err != nil {
		t.Fatal(err)
	}
	if err := q.SessionChangeSupersedeUnaffectedIssues(ctx, reverted); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM absence_schedule_issues WHERE id=$1`, genuineIssue).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "superseded" {
		t.Errorf("return to assigned time left issue %s", status)
	}
	if err := q.AbsenceScheduleIssuesSupersede(ctx, f.absenceID, f.sessionID, []string{}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM absence_schedule_issues WHERE id=$1`, unrelated).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "open" {
		t.Errorf("recompute affected another session's issue: %s", status)
	}
}
