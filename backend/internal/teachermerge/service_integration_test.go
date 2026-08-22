package teachermerge

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/legacysync/apply"
	"warwick-institute/internal/legacysync/normalize"
)

var (
	mergeMigrationsOnce sync.Once
	mergeMigrationsErr  error
)

func mergeTestService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	mergeMigrationsOnce.Do(func() {
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			mergeMigrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			mergeMigrationsErr = err
			return
		}
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			mergeMigrationsErr = fmt.Errorf("locate migration test")
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations"))
		mergeMigrationsErr = goose.Up(db, migrationsDir)
	})
	if mergeMigrationsErr != nil {
		t.Fatal(mergeMigrationsErr)
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return New(pool, sqldb.New(pool)), pool
}

func nextMonday(from time.Time) time.Time {
	days := (int(time.Monday) - int(from.Weekday()) + 7) % 7
	if days == 0 {
		days = 7
	}
	return from.AddDate(0, 0, days).Truncate(time.Hour)
}

func TestMergeRepointsEverythingAndSurvivesNextSync(t *testing.T) {
	service, pool := mergeTestService(t)
	ctx := t.Context()
	source := "test_" + uuid.NewString()
	legacyID := "merge-" + uuid.NewString()
	var adminID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1,'Admin','x') RETURNING id`, "admin-"+legacyID).Scan(&adminID); err != nil {
		t.Fatal(err)
	}

	var duplicateID, canonicalID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash, full_name) VALUES ($1,'Teacher','x','Legacy Shell') RETURNING id`, "legacy:"+legacyID).Scan(&duplicateID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash, full_name, email) VALUES ($1,'Teacher','x','Real Teacher','real@example.test') RETURNING id`, "real-"+legacyID).Scan(&canonicalID); err != nil {
		t.Fatal(err)
	}

	// Real subject/course/room so FKs hold; availability constrains the
	// canonical teacher to Mon–Tue 08:00–18:00 of next week.
	var subjectID, courseID, roomID, altRoomID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO subjects (code, name) VALUES ($1,'Merge Subject') RETURNING id`, "sub-"+legacyID).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO courses (code, name, subject_id, teacher_id, source_kind) VALUES ($1,'Merge Course',$2,$3,'legacy') RETURNING id`, "crs-"+legacyID, subjectID, duplicateID).Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO rooms (name) VALUES ($1) RETURNING id`, "room-"+legacyID).Scan(&roomID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO rooms (name) VALUES ($1) RETURNING id`, "altroom-"+legacyID).Scan(&altRoomID); err != nil {
		t.Fatal(err)
	}

	base := nextMonday(time.Now().UTC())
	if _, err := pool.Exec(ctx, `INSERT INTO teacher_availability (teacher_id, start_at, end_at) VALUES ($1,$2,$3)`,
		canonicalID, base.Add(8*time.Hour), base.Add(18*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO teacher_availability (teacher_id, start_at, end_at) VALUES ($1,$2,$3)`,
		canonicalID, base.AddDate(0, 0, 1).Add(8*time.Hour), base.AddDate(0, 0, 1).Add(18*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO teacher_availability (teacher_id, start_at, end_at) VALUES ($1,$2,$3)`,
		canonicalID, base.AddDate(0, 0, 2).Add(8*time.Hour), base.AddDate(0, 0, 2).Add(18*time.Hour)); err != nil {
		t.Fatal(err)
	}

	// Canonical's own session Mon 09:00–10:00.
	if _, err := pool.Exec(ctx, `INSERT INTO sessions (course_id, room_id, teacher_id, start_at, end_at) VALUES ($1,$2,$3,$4,$5)`,
		courseID, roomID, canonicalID, base.Add(9*time.Hour), base.Add(10*time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Duplicate's sessions: Mon 09:30–10:30 (overlaps canonical), Tue
	// 19:00–20:00 (outside availability), Wed 09:00–10:00 (clean), plus one
	// already-deleted session.
	for _, s := range []struct {
		day    int
		hour   int
		deleted bool
	}{{0, 9, false}, {1, 19, false}, {2, 9, false}, {2, 11, true}} {
		start := base.AddDate(0, 0, s.day).Add(time.Duration(s.hour) * time.Hour).Add(30 * time.Minute)
		if s.deleted {
			if _, err := pool.Exec(ctx, `INSERT INTO sessions (course_id, room_id, teacher_id, start_at, end_at, deleted_at) VALUES ($1,$2,$3,$4,$5,now())`,
				courseID, altRoomID, duplicateID, start, start.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if _, err := pool.Exec(ctx, `INSERT INTO sessions (course_id, room_id, teacher_id, start_at, end_at, legacy_schedule_id, source_kind) VALUES ($1,$2,$3,$4,$5,$6,'legacy')`,
			courseID, altRoomID, duplicateID, start, start.Add(time.Hour), "sched-"+legacyID+"-"+fmt.Sprint(s.day)+fmt.Sprint(s.hour)); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := pool.Exec(ctx, `INSERT INTO session_series (course_id, room_id, teacher_id, weekdays, start_local_time, duration_minutes, start_date, count, source_kind, materialization_mode) VALUES ($1,$2,$3,'{1}','09:30',60,CURRENT_DATE,8,'legacy','external')`,
		courseID, altRoomID, duplicateID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO course_teachers (course_id, teacher_id, is_primary) VALUES ($1,$2,true)`, courseID, duplicateID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO external_refs (source, entity_type, external_id, internal_id) VALUES ($1,'teacher',$2,$3)`,
		source, legacyID, duplicateID); err != nil {
		t.Fatal(err)
	}

	preview, err := service.Preview(ctx, duplicateID, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Impact.SessionsLive != 3 || preview.Impact.SessionsDeleted != 1 {
		t.Fatalf("preview sessions = %d/%d, want 3/1", preview.Impact.SessionsLive, preview.Impact.SessionsDeleted)
	}
	if preview.Impact.ConflictSessions != 2 {
		t.Fatalf("preview conflicts = %d, want 2 (overlap + availability)", preview.Impact.ConflictSessions)
	}
	if preview.Impact.Courses != 1 || preview.Impact.Series != 1 || preview.Impact.CourseTeacherRows != 1 || preview.Impact.ExternalRefMappings != 1 {
		t.Fatalf("preview impact = %+v", preview.Impact)
	}

	result, err := service.Merge(ctx, adminID, duplicateID, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Impact.SessionsLive != 3 || result.Impact.SessionsDeleted != 1 || result.Impact.ConflictSessions != 2 {
		t.Fatalf("merge impact = %+v", result.Impact)
	}

	var teacherID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT teacher_id FROM courses WHERE id=$1`, courseID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if teacherID != canonicalID {
		t.Fatalf("course teacher = %v, want canonical %v", teacherID, canonicalID)
	}
	var primaryID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT teacher_id FROM course_teachers WHERE course_id=$1 AND is_primary`, courseID).Scan(&primaryID); err != nil {
		t.Fatal(err)
	}
	if primaryID != canonicalID {
		t.Fatalf("primary course teacher = %v, want canonical %v", primaryID, canonicalID)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE teacher_id=$1`, duplicateID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d sessions still on duplicate", n)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE teacher_id=$1 AND legacy_conflict_override`, canonicalID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("conflict-flagged sessions = %d, want 2", n)
	}
	var dupDeleted bool
	if err := pool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM users WHERE id=$1`, duplicateID).Scan(&dupDeleted); err != nil {
		t.Fatal(err)
	}
	if !dupDeleted {
		t.Fatal("duplicate user should be deactivated after merge")
	}
	var mappedID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT internal_id FROM external_refs WHERE source=$1 AND entity_type='teacher' AND external_id=$2`, source, legacyID).Scan(&mappedID); err != nil {
		t.Fatal(err)
	}
	if mappedID != canonicalID {
		t.Fatalf("external_ref points at %v, want canonical %v", mappedID, canonicalID)
	}

	// The merge must survive the next sync run: ApplyTeacher follows the
	// re-pointed mapping and must not clobber the canonical account's
	// username, email, or activation.
	master := apply.NewMasterDataService(pool, sqldb.New(pool), source)
	applied, err := master.ApplyTeacher(ctx, apply.TeacherApplyRequest{
		Teacher:    normalize.LegacyTeacher{LegacyID: legacyID, Name: "Legacy Shell", Email: "shell@example.test", IsActive: false},
		ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied.InternalID.Bytes != canonicalID {
		t.Fatalf("post-merge sync resolved to %v, want canonical", applied.InternalID.Bytes)
	}
	var username, email string
	var deleted bool
	if err := pool.QueryRow(ctx, `SELECT username, COALESCE(email,''), deleted_at IS NOT NULL FROM users WHERE id=$1`, canonicalID).Scan(&username, &email, &deleted); err != nil {
		t.Fatal(err)
	}
	if username != "real-"+legacyID || email != "real@example.test" || deleted {
		t.Fatalf("post-merge sync clobbered canonical account: username=%q email=%q deleted=%v", username, email, deleted)
	}
}

func TestMergeRejectsBadDirection(t *testing.T) {
	service, pool := mergeTestService(t)
	ctx := t.Context()

	var shellID, realID, otherShellID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1,'Teacher','x') RETURNING id`, "legacy:bad-"+uuid.NewString()).Scan(&shellID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1,'Teacher','x') RETURNING id`, "real-"+uuid.NewString()).Scan(&realID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1,'Teacher','x') RETURNING id`, "legacy:other-"+uuid.NewString()).Scan(&otherShellID); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Merge(ctx, uuid.New(), realID, shellID); err == nil {
		t.Fatal("merging into a legacy shell should be rejected")
	}
	if _, err := service.Merge(ctx, uuid.New(), shellID, otherShellID); err == nil {
		t.Fatal("merging shell into another shell should be rejected")
	}
	if _, err := service.Merge(ctx, uuid.New(), shellID, shellID); err == nil {
		t.Fatal("merging an account into itself should be rejected")
	}

	// A rejected merge must leave no trace.
	var shellDeactivated bool
	if err := pool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM users WHERE id=$1`, shellID).Scan(&shellDeactivated); err != nil {
		t.Fatal(err)
	}
	if shellDeactivated {
		t.Fatal("rejected merge must not deactivate the duplicate")
	}
}
