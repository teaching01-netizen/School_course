package legacysynchttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// TestAuditEndpointSummarizesImportsAndSkips seeds an imported course with
// sessions, a set of skip ledgers (schedule conflict, code-claimed conflict,
// dead letter, partial snapshot) and verifies the audit response carries them
// in the summary counts, the by-cause buckets, and the detail lists.
func TestAuditEndpointSummarizesImportsAndSkips(t *testing.T) {
	pool := requireSyncTestDB(t)
	q := sqldb.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := uuid.NewString()
	legacyCourseID := "AUD-L-" + suffix
	legacyCodeClaimID := "AUD-C-" + suffix
	deadLetterExternalID := "AUD-D-" + suffix

	s := conflictTestServer(pool, q)

	// Teacher required by sessions.teacher_id NOT NULL; a teacher without
	// availability windows passes the availability trigger.
	var teacherID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, role, password_hash, password_version, full_name)
		 VALUES ($1, 'Teacher', 'x', 1, 'Audit Teacher') RETURNING id`,
		"audit-teacher-"+suffix).Scan(&teacherID); err != nil {
		t.Fatalf("seed teacher: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, teacherID); err != nil {
			t.Errorf("cleanup teacher: %v", err)
		}
	})

	var courseID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO courses (code, name, legacy_course_id, source_kind, legacy_archived, legacy_last_synced_at)
		 VALUES ($1, $2, $3, 'legacy', false, now()) RETURNING id`,
		"AUD-"+suffix, "Audit Course "+suffix, legacyCourseID).Scan(&courseID); err != nil {
		t.Fatalf("seed linked course: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM courses WHERE id = $1`, courseID); err != nil {
			t.Errorf("cleanup course: %v", err)
		}
	})

	var seriesID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO session_series (course_id, teacher_id, institute_tz, start_local_time, duration_minutes, start_date, end_date, weekdays, count, source_kind, materialization_mode, legacy_group_key)
		 VALUES ($1, $2, 'Asia/Bangkok', '09:00:00', 60, '2026-09-01', '2026-09-30', ARRAY[2]::smallint[], 3, 'legacy', 'external', $3) RETURNING id`, courseID, teacherID, legacyCourseID).Scan(&seriesID); err != nil {
		t.Fatalf("seed external series: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM session_series WHERE id = $1`, seriesID); err != nil {
			t.Errorf("cleanup series: %v", err)
		}
	})
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM sessions WHERE course_id = $1 AND source_kind = 'legacy'`, courseID); err != nil {
			t.Errorf("cleanup sessions: %v", err)
		}
	})

	seedSession := func(scheduleID, start, end string, deleted bool) {
		t.Helper()
		deletedSQL := "NULL"
		if deleted {
			deletedSQL = "now()"
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO sessions (series_id, course_id, teacher_id, start_at, end_at, legacy_schedule_id, legacy_source_hash, legacy_last_synced_at, deleted_at, source_kind)
			 VALUES ($1, $2, $3, $4::timestamptz, $5::timestamptz, $6, 'audit-hash', now(), `+deletedSQL+`, 'legacy')`,
			seriesID, courseID, teacherID, start, end, scheduleID); err != nil {
			t.Fatalf("seed session %s: %v", scheduleID, err)
		}
	}
	seedSession("AUD-S-1-"+suffix, "2026-09-01 09:00:00Z", "2026-09-01 10:00:00Z", false)
	seedSession("AUD-S-2-"+suffix, "2026-09-02 09:00:00Z", "2026-09-02 10:00:00Z", false)
	seedSession("AUD-S-3-"+suffix, "2026-09-03 09:00:00Z", "2026-09-03 10:00:00Z", true)

	// Roster student + master data refs.
	var studentID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, $2) RETURNING id`,
		"AUD-W-"+suffix, "Audit Student").Scan(&studentID); err != nil {
		t.Fatalf("seed student: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM students WHERE id = $1`, studentID); err != nil {
			t.Errorf("cleanup student: %v", err)
		}
	})
	for _, entity := range []string{"student", "room", "teacher", "subject"} {
		externalID := "AUD-" + entity + "-" + suffix
		if _, err := pool.Exec(ctx,
			`INSERT INTO external_refs (source, entity_type, external_id, internal_id, state, last_seen_at)
			 VALUES ('audit-test', $1, $2, $3, 'active', now()) ON CONFLICT (source, entity_type, external_id) DO NOTHING`,
			entity, externalID, teacherID); err != nil {
			t.Fatalf("seed external_ref %s: %v", entity, err)
		}
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			if _, err := pool.Exec(cleanupCtx,
				`DELETE FROM external_refs WHERE source = 'audit-test' AND entity_type = $1 AND external_id = $2`,
				entity, externalID); err != nil {
				t.Errorf("cleanup external_ref %s: %v", entity, err)
			}
		})
	}

	// Skip ledgers: one open schedule conflict, one resolved schedule conflict,
	// one open code-claimed (course) conflict, one course dead letter, and a
	// partial course snapshot.
	seedConflict := func(conflictType, externalID, payload, status string) string {
		t.Helper()
		var id uuid.UUID
		sourcePayload := payload
		if sourcePayload == "" {
			sourcePayload = `{"code":"AUD"}`
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO legacy_sync_conflicts (entity_type, external_id, conflict_type, category, source_payload, message, status)
			 VALUES ('course', $1, $2, 'database_constraint', $3::jsonb, 'audit seeded', $4) RETURNING id`,
			externalID, conflictType, sourcePayload, status).Scan(&id); err != nil {
			t.Fatalf("seed conflict %s: %v", conflictType, err)
		}
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			if _, err := pool.Exec(cleanupCtx, `DELETE FROM legacy_sync_conflicts WHERE id = $1`, id); err != nil {
				t.Errorf("cleanup conflict %s: %v", conflictType, err)
			}
		})
		return id.String()
	}
	seedConflict("room_overlap", legacyCourseID, `{"legacy_schedule_id":"AUD-S-1-`+suffix+`","date":"2026-09-01","begin":"09:00","end":"10:00"}`, "open")
	seedConflict("room_overlap", legacyCourseID, `{"legacy_schedule_id":"AUD-S-2-`+suffix+`","date":"2026-09-02","begin":"09:00","end":"10:00"}`, "resolved")
	seedConflict("code_claimed", legacyCodeClaimID, `{"code":"AUD"}`, "open")

	var deadLetterID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO legacy_sync_dead_letters (job_type, unique_key, entity_type, external_id, payload, error_category, last_error, attempts)
		 VALUES ('legacy_refresh_course', $1, 'course', $2, '{}'::jsonb, 'database_constraint', 'audit seeded failure', 5) RETURNING id`,
		"audit-job-"+suffix, deadLetterExternalID).Scan(&deadLetterID); err != nil {
		t.Fatalf("seed dead letter: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM legacy_sync_dead_letters WHERE id = $1`, deadLetterID); err != nil {
			t.Errorf("cleanup dead letter: %v", err)
		}
	})

	if _, err := pool.Exec(ctx,
		`INSERT INTO legacy_entity_snapshots (source, entity_type, external_id, canonical_data, source_hash, parser_version, observed_at, quality)
		 VALUES ('audit-test', 'course', $1, '{}'::jsonb, 'audit-hash', 1, now(), 'partial')`,
		legacyCourseID); err != nil {
		t.Fatalf("seed partial snapshot: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx,
			`DELETE FROM legacy_entity_snapshots WHERE source = 'audit-test' AND entity_type = 'course' AND external_id = $1`,
			legacyCourseID); err != nil {
			t.Errorf("cleanup partial snapshot: %v", err)
		}
	})

	// Completed run counts feed the runs summary.
	run, err := q.SyncRunCreate(ctx, "full_sweep")
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := q.SyncRunComplete(ctx, sqldb.SyncRunCompleteParams{
		ID:                       run.ID,
		Status:                   "completed",
		PagesRequested:           2,
		EntitiesParsed:           10,
		EntitiesChanged:          8,
		EntitiesApplied:          8,
		ParseFailures:            1,
		ReconciliationMismatches: 0,
		SourceLatencyMs:          pgtype.Int4{},
		LastError:                pgtype.Text{},
	}); err != nil {
		t.Fatalf("complete run: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM legacy_sync_runs WHERE id = $1`, run.ID); err != nil {
			t.Errorf("cleanup run: %v", err)
		}
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/legacy-sync/audit", nil)
	response := httptest.NewRecorder()
	s.handleAudit(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("audit status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	var body legacyAuditDTO
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}

	if body.Totals.LinkedCourses < 1 || body.Totals.SyncedCourses < 1 {
		t.Fatalf("totals must count the seeded linked course, got %+v", body.Totals)
	}
	if body.Totals.ActiveSessions < 2 || body.Totals.SoftDeletedSessions < 1 || body.Totals.LegacySessions < 3 {
		t.Fatalf("totals must count seeded legacy sessions, got %+v", body.Totals)
	}
	if body.Totals.StudentsImported < 1 || body.Totals.MappedRooms < 1 || body.Totals.MappedTeachers < 1 || body.Totals.MappedSubjects < 1 {
		t.Fatalf("totals must count seeded external refs, got %+v", body.Totals)
	}
	if body.Runs.CompletedRuns < 1 || body.Runs.EntitiesParsed < 10 || body.Runs.EntitiesApplied < 8 || body.Runs.ParseFailures < 1 {
		t.Fatalf("runs summary must absorb the seeded run, got %+v", body.Runs)
	}

	if body.Skips.SessionsSkippedTotal < 2 || body.Skips.SessionsSkippedOpen < 1 {
		t.Fatalf("skip counts must count seeded schedule conflicts, got %+v", body.Skips)
	}
	if body.Skips.CoursesSkippedTotal < 2 || body.Skips.CoursesSkippedOpen < 2 {
		t.Fatalf("skip counts must count the code-claimed conflict and course dead letter, got %+v", body.Skips)
	}
	if body.Skips.PartialSnapshots < 1 {
		t.Fatalf("skip counts must count the partial snapshot, got %+v", body.Skips)
	}

	var sawOpenConflict, sawClosedConflict, sawDeadLetter, sawPartial bool
	for _, bucket := range body.Skips.ByCause {
		switch bucket.Cause {
		case "open_conflict":
			sawOpenConflict = true
			if bucket.Key == "room_overlap" && bucket.Count < 1 {
				t.Fatalf("room_overlap open bucket missing, got %+v", bucket)
			}
		case "closed_conflict":
			sawClosedConflict = true
		case "dead_letter":
			sawDeadLetter = true
		case "partial_snapshot":
			sawPartial = true
		}
	}
	if !sawOpenConflict || !sawClosedConflict || !sawDeadLetter || !sawPartial {
		t.Fatalf("by-cause buckets missing seeded causes, got %+v", body.Skips.ByCause)
	}

	var sawScheduledSkip bool
	for _, session := range body.SkippedSessions {
		if session.LegacyScheduleID == "AUD-S-1-"+suffix && session.Status == "open" {
			sawScheduledSkip = true
		}
	}
	if !sawScheduledSkip {
		t.Fatalf("skipped sessions must list the seeded open schedule conflict, got %+v", body.SkippedSessions)
	}

	var sawCodeClaim, sawDeadLetterCourse bool
	for _, course := range body.SkippedCourses {
		switch {
		case course.ReasonKind == "conflict" && course.ExternalID == legacyCodeClaimID && course.ConflictType == "code_claimed":
			sawCodeClaim = true
		case course.ReasonKind == "dead_letter" && course.ExternalID == deadLetterExternalID:
			sawDeadLetterCourse = true
		}
	}
	if !sawCodeClaim || !sawDeadLetterCourse {
		t.Fatalf("skipped courses must list the code-claimed conflict and the dead letter, got %+v", body.SkippedCourses)
	}

	var sawSeededDeadLetter bool
	for _, letter := range body.DeadLetters {
		if letter.ExternalID != nil && *letter.ExternalID == deadLetterExternalID {
			sawSeededDeadLetter = true
		}
	}
	if !sawSeededDeadLetter {
		t.Fatalf("dead letters must list the seeded letter, got %+v", body.DeadLetters)
	}
}
