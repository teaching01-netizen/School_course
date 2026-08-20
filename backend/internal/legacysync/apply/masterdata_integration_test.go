package apply

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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/legacysync/normalize"
)

var (
	masterDataMigrationsOnce sync.Once
	masterDataMigrationsErr  error
)

func masterDataTestService(t *testing.T) (*MasterDataService, *pgxpool.Pool, string) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	masterDataMigrationsOnce.Do(func() {
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			masterDataMigrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			masterDataMigrationsErr = err
			return
		}
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			masterDataMigrationsErr = fmt.Errorf("locate migration test")
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations"))
		masterDataMigrationsErr = goose.Up(db, migrationsDir)
	})
	if masterDataMigrationsErr != nil {
		t.Fatal(masterDataMigrationsErr)
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
	return NewMasterDataService(pool, sqldb.New(pool), "test_"+uuid.NewString()), pool, uuid.NewString()
}

func TestMasterDataApply_StableExternalMappings(t *testing.T) {
	service, pool, suffix := masterDataTestService(t)
	observedAt := time.Date(2026, time.August, 4, 0, 0, 0, 0, time.UTC)
	legacyID := "teacher-" + suffix

	first, err := service.ApplyTeacher(t.Context(), TeacherApplyRequest{
		Teacher:    normalize.LegacyTeacher{LegacyID: legacyID, Name: "Teacher One", IsActive: true},
		ObservedAt: observedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ApplyTeacher(t.Context(), TeacherApplyRequest{
		Teacher:    normalize.LegacyTeacher{LegacyID: legacyID, Name: "Teacher Renamed", IsActive: true},
		ObservedAt: observedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.InternalID != second.InternalID || !first.InternalID.Valid {
		t.Fatalf("teacher mapping changed across refresh: first=%v second=%v", first.InternalID, second.InternalID)
	}

	var mappingCount, userCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM external_refs WHERE source=$1 AND entity_type='teacher' AND external_id=$2`, service.source, legacyID).Scan(&mappingCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE username=$1`, "legacy:"+legacyID).Scan(&userCount); err != nil {
		t.Fatal(err)
	}
	if mappingCount != 1 || userCount != 1 {
		t.Fatalf("mapping/user counts = %d/%d, want 1/1", mappingCount, userCount)
	}
	var fullName string
	if err := pool.QueryRow(t.Context(), `SELECT full_name FROM users WHERE username=$1`, "legacy:"+legacyID).Scan(&fullName); err != nil {
		t.Fatal(err)
	}
	if fullName != "Teacher Renamed" {
		t.Fatalf("teacher full_name = %q, want latest name %q", fullName, "Teacher Renamed")
	}
}

func TestMasterDataApply_SameNumericIDAcrossEntityTypesDoesNotCollide(t *testing.T) {
	service, _, suffix := masterDataTestService(t)
	legacyID := "shared-" + suffix
	teacher, err := service.ApplyTeacher(t.Context(), TeacherApplyRequest{
		Teacher: normalize.LegacyTeacher{LegacyID: legacyID, Name: "Teacher", IsActive: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	subject, err := service.ApplySubject(t.Context(), SubjectApplyRequest{
		Subject: normalize.LegacySubject{LegacyID: legacyID, Name: "Subject"},
		Code:    "subject-" + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !teacher.InternalID.Valid || !subject.InternalID.Valid || teacher.InternalID == subject.InternalID {
		t.Fatalf("cross-type mappings collided: teacher=%v subject=%v", teacher.InternalID, subject.InternalID)
	}
}

func TestMasterDataApply_SubjectUpdateMatchesFinalSchema(t *testing.T) {
	service, pool, suffix := masterDataTestService(t)
	legacyID := "subject-update-" + suffix

	first, err := service.ApplySubject(t.Context(), SubjectApplyRequest{
		Subject: normalize.LegacySubject{LegacyID: legacyID, Name: "Subject Original"},
		Code:    "SU-ORIG-" + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The second apply reaches the UPDATE branch. Subjects lost their
	// deleted_at column to the hard-delete migration, so the update must not
	// reference a soft-delete column that no longer exists.
	second, err := service.ApplySubject(t.Context(), SubjectApplyRequest{
		Subject: normalize.LegacySubject{LegacyID: legacyID, Name: "Subject Renamed"},
		Code:    "SU-NEW-" + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.InternalID.Valid || first.InternalID != second.InternalID {
		t.Fatalf("subject mapping changed across refresh: first=%v second=%v", first.InternalID, second.InternalID)
	}
	var name, code string
	if err := pool.QueryRow(t.Context(), `SELECT name, code FROM subjects WHERE id=$1`, first.InternalID).Scan(&name, &code); err != nil {
		t.Fatal(err)
	}
	if name != "Subject Renamed" || code != "SU-NEW-"+suffix {
		t.Fatalf("subject after update = %q/%q, want renamed subject with new code", name, code)
	}
}

func TestMasterDataApply_DuplicateRoomNamesKeepDistinctSourceIdentity(t *testing.T) {
	service, pool, suffix := masterDataTestService(t)
	name := "Shared Room " + suffix
	first, err := service.ApplyRoom(t.Context(), RoomApplyRequest{
		Room: normalize.LegacyRoom{LegacyID: "room-a-" + suffix, Name: name},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ApplyRoom(t.Context(), RoomApplyRequest{
		Room: normalize.LegacyRoom{LegacyID: "room-b-" + suffix, Name: name},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.InternalID.Valid || !second.InternalID.Valid || first.InternalID == second.InternalID {
		t.Fatalf("same-name rooms were merged: first=%v second=%v", first.InternalID, second.InternalID)
	}
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM rooms WHERE name=$1`, name).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("room count = %d, want 2", count)
	}
}

func TestMasterDataApply_ConcurrentTeacherCreateUsesOneMapping(t *testing.T) {
	service, pool, suffix := masterDataTestService(t)
	legacyID := "concurrent-" + suffix
	const workers = 10
	results := make(chan MasterDataApplyResult, workers)
	errs := make(chan error, workers)
	start := make(chan struct{})
	for range workers {
		go func() {
			<-start
			result, err := service.ApplyTeacher(t.Context(), TeacherApplyRequest{
				Teacher: normalize.LegacyTeacher{LegacyID: legacyID, Name: "Concurrent Teacher", IsActive: true},
			})
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	var wantID pgtype.UUID
	for range workers {
		select {
		case err := <-errs:
			t.Fatal(err)
		case result := <-results:
			if !result.InternalID.Valid {
				t.Fatal("concurrent apply returned invalid internal ID")
			}
			if !wantID.Valid {
				wantID = result.InternalID
			} else if result.InternalID != wantID {
				t.Fatalf("concurrent IDs differ: %v and %v", wantID, result.InternalID)
			}
		}
	}
	var mappingCount, userCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM external_refs WHERE source=$1 AND entity_type='teacher' AND external_id=$2`, service.source, legacyID).Scan(&mappingCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE username=$1`, "legacy:"+legacyID).Scan(&userCount); err != nil {
		t.Fatal(err)
	}
	if mappingCount != 1 || userCount != 1 {
		t.Fatalf("concurrent mapping/user counts = %d/%d, want 1/1", mappingCount, userCount)
	}
}
