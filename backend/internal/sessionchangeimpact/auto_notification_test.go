package sessionchangeimpact

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sqldb "warwick-institute/internal/db"
)

// ---------------------------------------------------------------------------
// AUTO-xxx: automatic safe-move notification contract.
//
// AUTO-001: with sit_in_change_auto_notify_safe_moves=false (the default), a
// safe session change must not create any automatic notification. Automatic
// notification queueing for confirmed-safe moves is not implemented yet; this
// test locks the current contract so the feature cannot accidentally start
// notifying (and flips intentionally once AUTO-002 lands).
// ---------------------------------------------------------------------------

func autoTestDB(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

var autoMigrationsOnce sync.Once
var autoMigrationsErr error

func autoMigrateUpOnce(t *testing.T, databaseURL string) {
	t.Helper()
	autoMigrationsOnce.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		conn, err := sql.Open("pgx", databaseURL)
		if err != nil {
			autoMigrationsErr = err
			return
		}
		defer conn.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			autoMigrationsErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			autoMigrationsErr = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations"))
		autoMigrationsErr = goose.UpContext(ctx, conn, migrationsDir)
	})
	if autoMigrationsErr != nil {
		t.Fatal(autoMigrationsErr)
	}
}

func autoPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	// Unlike the db-package helpers, this pool keeps the default extended
	// protocol: the analysis pipeline passes []byte into jsonb columns, which
	// only works when pgx knows the parameter type from statement describe.
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

// Contract: the auto-notify flag defaults to false (migration 00073).
func TestAutoNotifySafeMoves_defaultsToFalse(t *testing.T) {
	databaseURL := autoTestDB(t)
	autoMigrateUpOnce(t, databaseURL)
	pool := autoPool(t, databaseURL)
	t.Cleanup(pool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var columnDefault string
	if err := pool.QueryRow(ctx, `
		SELECT column_default FROM information_schema.columns
		WHERE table_name = 'app_settings' AND column_name = 'sit_in_change_auto_notify_safe_moves'
	`).Scan(&columnDefault); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(columnDefault, "false") {
		t.Fatalf("sit_in_change_auto_notify_safe_moves default = %q, want false", columnDefault)
	}
}

// AUTO-001: even with both channels configured and a contactable student, a
// safe session move creates no automatic notification while the flag is off.
func TestAnalyze_safeMoveQueuesNoAutomaticNotification(t *testing.T) {
	databaseURL := autoTestDB(t)
	autoMigrateUpOnce(t, databaseURL)
	pool := autoPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	// Pin the flag off and fully configure notifications: with the flag
	// disabled, none of this may trigger an automatic message.
	var previousFlag bool
	var prev struct {
		smsEnabled, emailEnabled             bool
		smsTemplate, emailSubject, emailBody string
	}
	if err := pool.QueryRow(ctx, `
		SELECT sit_in_change_auto_notify_safe_moves, sit_in_change_sms_enabled, sit_in_change_email_enabled,
		       sit_in_change_sms_template, sit_in_change_email_subject, sit_in_change_email_body
		FROM app_settings WHERE id = true
	`).Scan(&previousFlag, &prev.smsEnabled, &prev.emailEnabled, &prev.smsTemplate, &prev.emailSubject, &prev.emailBody); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE app_settings
		SET sit_in_change_auto_notify_safe_moves = false,
		    sit_in_change_sms_enabled = true, sit_in_change_sms_template = 'Moved {{student_name}}',
		    sit_in_change_email_enabled = true, sit_in_change_email_subject = 'Sit-in moved',
		    sit_in_change_email_body = 'Hello {{student_name}}'
		WHERE id = true
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `
			UPDATE app_settings
			SET sit_in_change_auto_notify_safe_moves = $1,
			    sit_in_change_sms_enabled = $2, sit_in_change_sms_template = $3,
			    sit_in_change_email_enabled = $4, sit_in_change_email_subject = $5,
			    sit_in_change_email_body = $6
			WHERE id = true
		`, previousFlag, prev.smsEnabled, prev.smsTemplate, prev.emailEnabled, prev.emailSubject, prev.emailBody); err != nil {
			t.Logf("restore settings: %v", err)
		}
	})

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "auto-teacher-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "auto-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "AUTO-" + suffix, Name: "Auto " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	// Far-future session: the +1h move is far outside the default 24h warning window.
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherID,
		StartAt: pgtype.Timestamptz{Time: time.Date(2031, 3, 2, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2031, 3, 2, 10, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
		Wcode: "AUTO-" + suffix, CourseID: course.ID,
		DateFrom:      pgtype.Date{Time: session.StartAt.Time, Valid: true},
		DateTo:        pgtype.Date{Time: session.StartAt.Time, Valid: true},
		SitInCourseID: course.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE student_absences
		SET student_name = 'Auto Student', student_email = 'auto.student@example.edu', student_phone = '+66810000000'
		WHERE id = $1
	`, absence.ID); err != nil {
		t.Fatal(err)
	}
	// The production assignment path always captures a session snapshot; the
	// impact analysis copies it onto any issue it detects.
	if _, err := pool.Exec(ctx, `
		INSERT INTO absence_sit_ins (
			absence_id, session_id, session_version_at_assignment,
			session_snapshot_at_assignment, snapshot_schema_version,
			snapshot_captured_at, snapshot_quality, snapshot_source
		)
		VALUES ($1, $2, $3,
		        '{"schema_version":1}'::jsonb, 1, now(), 'exact', 'captured_at_assignment')
	`, absence.ID, session.ID, session.Version); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM notification_outbox WHERE absence_id = $1`, absence.ID); err != nil {
			t.Logf("cleanup outbox: %v", err)
		}
		// Cascade covers schedule issues; analysis writes no append-only events.
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, absence.ID); err != nil {
			t.Logf("cleanup absence: %v", err)
		}
	})

	// A safe move: same day/room/course/teacher, shifted by one hour.
	var changeID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO session_changes (
			session_id, session_version, changed_fields, before_snapshot, after_snapshot,
			old_start_at, old_end_at, new_start_at, new_end_at,
			old_course_id, new_course_id, old_teacher_id, new_teacher_id
		)
		SELECT id, version + 1,
		       '{"start_at":true,"end_at":true}'::jsonb, '{}'::jsonb, '{}'::jsonb,
		       start_at, end_at, start_at + interval '1 hour', end_at + interval '1 hour',
		       course_id, course_id, teacher_id, teacher_id
		FROM sessions WHERE id = $1
		RETURNING id
	`, session.ID).Scan(&changeID); err != nil {
		t.Fatal(err)
	}

	service := New(pool, q, "Asia/Bangkok", nil, slog.Default())
	if err := service.Analyze(ctx, changeID); err != nil {
		t.Fatal(err)
	}

	// Analysis completed...
	var runStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM session_change_impact_runs WHERE session_change_id = $1`, changeID).Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if runStatus != "completed" {
		t.Errorf("impact run status = %q, want completed", runStatus)
	}

	// ...but no automatic notification was queued for the affected student.
	var notifications int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE absence_id = $1`, absence.ID).Scan(&notifications); err != nil {
		t.Fatal(err)
	}
	if notifications != 0 {
		t.Errorf("automatic notifications = %d, want 0 (auto_notify_safe_moves=false)", notifications)
	}
}
