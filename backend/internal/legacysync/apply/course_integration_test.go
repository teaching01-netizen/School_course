package apply

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	legacyclient "warwick-institute/internal/legacysync/client"
	"warwick-institute/internal/legacysync/normalize"
	"warwick-institute/internal/legacysync/outbox"
	"warwick-institute/internal/legacysync/parser"
	"warwick-institute/internal/realtime"
)

type faultPointFunc func(string) error

func (f faultPointFunc) Hit(name string) error {
	return f(name)
}

func insertLegacyTestCourse(t *testing.T, pool *pgxpool.Pool, suffix string) pgtype.UUID {
	t.Helper()
	var courseID pgtype.UUID
	if err := pool.QueryRow(t.Context(), `INSERT INTO courses (code, name) VALUES ($1, $2) RETURNING id`, "native-course-"+suffix, "Native course").Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	return courseID
}

func legacyCourseRequest(t *testing.T, pool *pgxpool.Pool, source, suffix string, realtime bool) (CourseApplyRequest, pgtype.UUID) {
	t.Helper()
	courseID := insertLegacyTestCourse(t, pool, suffix)
	master := NewMasterDataService(pool, sqldb.New(pool), source)
	_, err := master.ApplyTeacher(t.Context(), TeacherApplyRequest{
		Teacher: normalize.LegacyTeacher{LegacyID: "teacher-" + suffix, Name: "Teacher", IsActive: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = master.ApplySubject(t.Context(), SubjectApplyRequest{
		Subject: normalize.LegacySubject{LegacyID: "subject-" + suffix, Name: "Subject"},
		Code:    "subject-" + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	return CourseApplyRequest{
		CourseID:       courseID,
		LegacyCourseID: "course-" + suffix,
		Aggregate: normalize.LegacyCourseAggregate{
			Course: normalize.LegacyCourse{
				LegacyID:   "course-" + suffix,
				Code:       "legacy-code-" + suffix,
				Name:       "Legacy course",
				Status:     "active",
				Type:       "Group",
				Hours:      "2",
				ExpireDate: "2026-12-31",
				TeacherID:  "teacher-" + suffix,
				SubjectID:  "subject-" + suffix,
			},
			Attendees: []string{"student-1", "student-2"},
		},
		ObservedAt:      time.Date(2026, time.August, 4, 0, 0, 0, 0, time.UTC),
		RealtimeEnabled: realtime,
	}, courseID
}

func TestCourseApply_CommitsAggregateMetadataTogether(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID := legacyCourseRequest(t, pool, master.source, suffix, true)
	applier := NewCourseApplier(pool, sqldb.New(pool), master.source)

	result, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.SourceHash == "" {
		t.Fatalf("apply result = %+v, want changed result with hash", result)
	}

	var code, name, courseType, legacyStatus string
	var archived bool
	if err := pool.QueryRow(t.Context(), `SELECT code, name, course_type, legacy_status, legacy_archived FROM courses WHERE id=$1`, courseID).Scan(&code, &name, &courseType, &legacyStatus, &archived); err != nil {
		t.Fatal(err)
	}
	if code != request.Aggregate.Course.Code || name != request.Aggregate.Course.Name || courseType != "Group" || legacyStatus != "active" || archived {
		t.Fatalf("course fields = %q/%q/%q/%q/%v", code, name, courseType, legacyStatus, archived)
	}

	var mappingCount, snapshotCount, outboxCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM external_refs WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&mappingCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_entity_snapshots WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&snapshotCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_sync_outbox WHERE source_event_key=$1`, "legacy:course:"+request.LegacyCourseID+":"+result.SourceHash).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if mappingCount != 1 || snapshotCount != 1 || outboxCount != 1 {
		t.Fatalf("mapping/snapshot/outbox counts = %d/%d/%d, want 1/1/1", mappingCount, snapshotCount, outboxCount)
	}
}

func TestCourseApply_IdenticalHashIsCompleteNoOp(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID := legacyCourseRequest(t, pool, master.source, suffix, true)
	applier := NewCourseApplier(pool, sqldb.New(pool), master.source)
	first, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	var before time.Time
	if err := pool.QueryRow(t.Context(), `SELECT updated_at FROM courses WHERE id=$1`, courseID).Scan(&before); err != nil {
		t.Fatal(err)
	}

	request.ObservedAt = request.ObservedAt.Add(time.Hour)
	second, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed || second.SourceHash != first.SourceHash {
		t.Fatalf("replay result = %+v, want unchanged result with same hash", second)
	}
	var seen, synced, externalSeen time.Time
	if err := pool.QueryRow(t.Context(), `SELECT legacy_last_seen_at, legacy_last_synced_at FROM courses WHERE id=$1`, courseID).Scan(&seen, &synced); err != nil {
		t.Fatal(err)
	}
	if !seen.Equal(request.ObservedAt) || !synced.Equal(request.ObservedAt) {
		t.Fatalf("unchanged observation timestamps = %v/%v, want %v", seen, synced, request.ObservedAt)
	}
	if err := pool.QueryRow(t.Context(), `SELECT last_seen_at FROM external_refs WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&externalSeen); err != nil {
		t.Fatal(err)
	}
	if !externalSeen.Equal(request.ObservedAt) {
		t.Fatalf("unchanged external observation timestamp = %v, want %v", externalSeen, request.ObservedAt)
	}
	var after time.Time
	if err := pool.QueryRow(t.Context(), `SELECT updated_at FROM courses WHERE id=$1`, courseID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !before.Equal(after) {
		t.Fatalf("updated_at changed on no-op: before=%v after=%v", before, after)
	}
	var outboxCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_sync_outbox WHERE entity_type='course' AND external_id=$1`, request.LegacyCourseID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("outbox count = %d, want 1", outboxCount)
	}
}

func TestCourseApply_ReusedOutboxKeyDoesNotRollbackAggregate(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID := legacyCourseRequest(t, pool, master.source, suffix, true)
	applier := NewCourseApplier(pool, sqldb.New(pool), master.source)
	first, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	request.Aggregate.Course.Name = "Legacy course updated"
	request.ObservedAt = request.ObservedAt.Add(time.Hour)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}

	request.Aggregate.Course.Name = "Legacy course"
	request.ObservedAt = request.ObservedAt.Add(time.Hour)
	third, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatalf("reverted aggregate failed: %v", err)
	}
	if !third.Changed || third.SourceHash != first.SourceHash {
		t.Fatalf("reverted apply result = %+v, want changed result with original hash", third)
	}

	var code string
	if err := pool.QueryRow(t.Context(), `SELECT code FROM courses WHERE id=$1`, courseID).Scan(&code); err != nil {
		t.Fatal(err)
	}
	if code != request.Aggregate.Course.Code {
		t.Fatalf("course code after replay = %q, want %q", code, request.Aggregate.Course.Code)
	}
	var mappingCount, snapshotCount, outboxCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM external_refs WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&mappingCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_entity_snapshots WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&snapshotCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_sync_outbox WHERE entity_type='course' AND external_id=$1`, request.LegacyCourseID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if mappingCount != 1 || snapshotCount != 1 || outboxCount != 2 {
		t.Fatalf("mapping/snapshot/outbox counts after replay = %d/%d/%d, want 1/1/2", mappingCount, snapshotCount, outboxCount)
	}
}

func TestCourseApply_FaultInjectionRollsBackEveryAggregateWrite(t *testing.T) {
	points := []string{
		"after_teacher_mapping_resolution",
		"after_subject_mapping_resolution",
		"after_course_upsert",
		"after_external_ref_upsert",
		"after_snapshot_insert",
		"after_outbox_insert",
		"before_commit",
	}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			master, pool, suffix := masterDataTestService(t)
			request, courseID := legacyCourseRequest(t, pool, master.source, suffix, true)
			applier := NewCourseApplier(pool, sqldb.New(pool), master.source)
			applier.fault = faultPointFunc(func(name string) error {
				if name == point {
					return errors.New("injected failure")
				}
				return nil
			})

			if _, err := applier.Apply(t.Context(), request); err == nil {
				t.Fatal("expected injected apply failure")
			}
			var code string
			if err := pool.QueryRow(t.Context(), `SELECT code FROM courses WHERE id=$1`, courseID).Scan(&code); err != nil {
				t.Fatal(err)
			}
			if code == request.Aggregate.Course.Code {
				t.Fatalf("course update survived %s", point)
			}
			var mappingCount, snapshotCount, outboxCount int
			if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM external_refs WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&mappingCount); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_entity_snapshots WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&snapshotCount); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_sync_outbox WHERE entity_type='course' AND external_id=$1`, request.LegacyCourseID).Scan(&outboxCount); err != nil {
				t.Fatal(err)
			}
			if mappingCount != 0 || snapshotCount != 0 || outboxCount != 0 {
				t.Fatalf("rows after %s failure = mapping %d/snapshot %d/outbox %d, want 0/0/0", point, mappingCount, snapshotCount, outboxCount)
			}
		})
	}
}
func TestCourseApply_AtomicallyPersistsCourseScheduleAggregate(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID := legacyCourseRequest(t, pool, master.source, suffix, true)
	if _, err := master.ApplyRoom(t.Context(), RoomApplyRequest{
		Room: normalize.LegacyRoom{LegacyID: "room-" + suffix, Name: "Room"},
	}); err != nil {
		t.Fatal(err)
	}
	request.Aggregate.Schedules = []normalize.LegacySchedule{{
		LegacyScheduleID:  "schedule-" + suffix,
		Date:              "2026-08-04",
		Begin:             "09:00",
		End:               "10:00",
		ClassroomLegacyID: "room-" + suffix,
		Classroom:         "Room",
		Confirmed:         true,
		ConfirmedBy:       "teacher",
	}}
	applier := NewCourseApplier(pool, sqldb.New(pool), master.source)
	result, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sessions != 1 {
		t.Fatalf("apply sessions = %d, want 1", result.Sessions)
	}
	var sessionCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE course_id=$1 AND legacy_schedule_id=$2`, courseID, "schedule-"+suffix).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 1 {
		t.Fatalf("persisted schedule count = %d, want 1", sessionCount)
	}
}
func TestCourseApply_ScheduleFaultRollsBackCourseAndSchedule(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID := legacyCourseRequest(t, pool, master.source, suffix, true)
	if _, err := master.ApplyRoom(t.Context(), RoomApplyRequest{
		Room: normalize.LegacyRoom{LegacyID: "room-" + suffix, Name: "Room"},
	}); err != nil {
		t.Fatal(err)
	}
	request.Aggregate.Schedules = []normalize.LegacySchedule{{
		LegacyScheduleID:  "schedule-" + suffix,
		Date:              "2026-08-04",
		Begin:             "09:00",
		End:               "10:00",
		ClassroomLegacyID: "room-" + suffix,
		Classroom:         "Room",
	}}
	applier := NewCourseApplier(pool, sqldb.New(pool), master.source)
	applier.fault = faultPointFunc(func(name string) error {
		if name == "after_session_upsert" {
			return errors.New("injected schedule failure")
		}
		return nil
	})
	if _, err := applier.Apply(t.Context(), request); err == nil {
		t.Fatal("expected injected schedule failure")
	}
	var sessionCount, seriesCount, mappingCount, snapshotCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE course_id=$1`, courseID).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM session_series WHERE course_id=$1 AND source_kind='legacy'`, courseID).Scan(&seriesCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM external_refs WHERE source=$1 AND entity_type IN ('course','schedule') AND external_id IN ($2,$3)`, master.source, request.LegacyCourseID, "schedule-"+suffix).Scan(&mappingCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_entity_snapshots WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&snapshotCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 0 || seriesCount != 0 || mappingCount != 0 || snapshotCount != 0 {
		t.Fatalf("rows after schedule fault = sessions %d/series %d/mappings %d/snapshots %d, want 0/0/0/0", sessionCount, seriesCount, mappingCount, snapshotCount)
	}
}
func TestCourseApply_SameCourseConcurrentUpdatesAreSerialized(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID := legacyCourseRequest(t, pool, master.source, suffix, true)
	firstRequest := request
	firstRequest.Aggregate.Course.Code = "legacy-first-" + suffix
	firstRequest.ObservedAt = request.ObservedAt.Add(time.Minute)
	secondRequest := request
	secondRequest.Aggregate.Course.Code = "legacy-second-" + suffix
	secondRequest.ObservedAt = request.ObservedAt.Add(2 * time.Minute)
	firstHash, err := normalize.HashCanonical(firstRequest.Aggregate)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := normalize.HashCanonical(secondRequest.Aggregate)
	if err != nil {
		t.Fatal(err)
	}

	applier := NewCourseApplier(pool, sqldb.New(pool), master.source)
	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, candidate := range []CourseApplyRequest{firstRequest, secondRequest} {
		candidate := candidate
		go func() {
			<-start
			_, applyErr := applier.Apply(t.Context(), candidate)
			errs <- applyErr
		}()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}

	var code, sourceHash string
	if err := pool.QueryRow(t.Context(), `SELECT code, legacy_source_hash FROM courses WHERE id=$1`, courseID).Scan(&code, &sourceHash); err != nil {
		t.Fatal(err)
	}
	if (code != firstRequest.Aggregate.Course.Code || sourceHash != firstHash) &&
		(code != secondRequest.Aggregate.Course.Code || sourceHash != secondHash) {
		t.Fatalf("final course state = %q/%q, want one serialized update", code, sourceHash)
	}
	var mappingCount, snapshotCount, outboxCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM external_refs WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&mappingCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_entity_snapshots WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&snapshotCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_sync_outbox WHERE entity_type='course' AND external_id=$1`, request.LegacyCourseID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if mappingCount != 1 || snapshotCount != 1 || outboxCount != 2 {
		t.Fatalf("mapping/snapshot/outbox counts = %d/%d/%d, want 1/1/2", mappingCount, snapshotCount, outboxCount)
	}
}
func TestCourseApply_DatabaseFailureRollsBackScheduleAggregate(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID := legacyCourseRequest(t, pool, master.source, suffix, true)
	if _, err := master.ApplyRoom(t.Context(), RoomApplyRequest{
		Room: normalize.LegacyRoom{LegacyID: "room-" + suffix, Name: "Room"},
	}); err != nil {
		t.Fatal(err)
	}
	request.Aggregate.Schedules = []normalize.LegacySchedule{{
		LegacyScheduleID:  "schedule-" + suffix,
		Date:              "2026-08-04",
		Begin:             "09:00",
		End:               "10:00",
		ClassroomLegacyID: "room-" + suffix,
		Classroom:         "Room",
	}}
	const triggerName = "test_legacy_session_insert_failure"
	const functionName = "test_legacy_session_insert_failure_fn"
	if _, err := pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS test_legacy_session_insert_failure ON sessions; DROP FUNCTION IF EXISTS test_legacy_session_insert_failure_fn()`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `CREATE OR REPLACE FUNCTION test_legacy_session_insert_failure_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected database failure'; END $$; CREATE TRIGGER test_legacy_session_insert_failure BEFORE INSERT ON sessions FOR EACH ROW WHEN (NEW.source_kind='legacy') EXECUTE FUNCTION test_legacy_session_insert_failure_fn()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS `+triggerName+` ON sessions`)
		_, _ = pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS `+functionName+`()`)
	})

	applier := NewCourseApplier(pool, sqldb.New(pool), master.source)
	if _, err := applier.Apply(t.Context(), request); err == nil {
		t.Fatal("expected database failure")
	}
	var sessionCount, seriesCount, mappingCount, snapshotCount, outboxCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE course_id=$1`, courseID).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM session_series WHERE course_id=$1 AND source_kind='legacy'`, courseID).Scan(&seriesCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM external_refs WHERE source=$1 AND entity_type IN ('course','schedule') AND external_id IN ($2,$3)`, master.source, request.LegacyCourseID, "schedule-"+suffix).Scan(&mappingCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_entity_snapshots WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&snapshotCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_sync_outbox WHERE entity_type='course' AND external_id=$1`, request.LegacyCourseID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 0 || seriesCount != 0 || mappingCount != 0 || snapshotCount != 0 || outboxCount != 0 {
		t.Fatalf("rows after database failure = sessions %d/series %d/mappings %d/snapshots %d/outbox %d, want all zero", sessionCount, seriesCount, mappingCount, snapshotCount, outboxCount)
	}
}
func TestLegacySyncEndToEndCompletesWithinOneSecond(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, _ := legacyCourseRequest(t, pool, master.source, suffix, true)
	roomID := "120204"
	if _, err := master.ApplyRoom(t.Context(), RoomApplyRequest{Room: normalize.LegacyRoom{LegacyID: roomID, Name: "Auditorium"}}); err != nil {
		t.Fatal(err)
	}
	page := `<html><body><h2>Schedule</h2><table class="table"><thead><tr><th>Date</th><th>Begin</th><th>End</th><th>Duration</th><th>Classroom</th><th>Confirm</th><th>By</th></tr></thead><tbody><tr data-schedule-id="` + "schedule-" + suffix + `"><td>Sat 23 May 26</td><td>13:00</td><td>16:20</td><td>03:20</td><td>[120204] Auditorium</td><td>Yes</td><td>AJ. TY</td></tr></tbody></table></body></html>`
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/Account/Login":
			if r.Method == http.MethodGet {
				http.SetCookie(w, &http.Cookie{Name: "__RequestVerificationToken", Value: "cookie", Path: "/"})
				_, _ = w.Write([]byte(`<form action="/Account/Login"><input name="__RequestVerificationToken" value="token"></form>`))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "Identity", Value: "ok", Path: "/"})
			_, _ = w.Write([]byte(`<a href="/Account/Logout">logout</a>`))
		case "/Admin/Courses/Detail":
			_, _ = w.Write([]byte(page))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(source.Close)
	if _, err := pool.Exec(t.Context(), `DELETE FROM legacy_sync_outbox`); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	legacyClient, err := legacyclient.New(legacyclient.Config{BaseURL: source.URL, Username: "user", Password: "pass", MinRequestInterval: time.Nanosecond})
	if err != nil {
		t.Fatal(err)
	}
	response, err := legacyClient.Do(t.Context(), legacyclient.Request{Method: http.MethodGet, Path: "/Admin/Courses/Detail"})
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := parser.ParseCourseDetail(string(response.Body))
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Course = request.Aggregate.Course
	aggregate.Attendees = request.Aggregate.Attendees
	request.Aggregate = *aggregate
	applier := NewCourseApplier(pool, sqldb.New(pool), master.source)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}

	hub := realtime.NewHub()
	client := hub.NewClient()
	channel := "course:" + request.CourseID.String()
	client.Subscribe(channel)
	publisher, err := outbox.NewPublisher(pool, hub, time.Millisecond, nil)
	if err != nil {
		t.Fatal(err)
	}
	publishCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	publishErr := make(chan error, 1)
	go func() { publishErr <- publisher.Run(publishCtx) }()
	select {
	case <-client.Send():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for end-to-end realtime event")
	}
	cancel()
	<-publishErr
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("end-to-end synchronization took %v, want <= 1s", elapsed)
	}
	hub.Close()
}

// TestCourseApply_EmptySourceScheduleDeactivatesLocalSessions pins CB-02 at
// the course-applier entry: a valid source detail page with no schedule rows
// must converge the local course to zero active legacy sessions instead of
// leaving stale rows active.
func TestCourseApply_EmptySourceScheduleDeactivatesLocalSessions(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID := legacyCourseRequest(t, pool, master.source, suffix, false)
	if _, err := master.ApplyRoom(t.Context(), RoomApplyRequest{
		Room: normalize.LegacyRoom{LegacyID: "room-" + suffix, Name: "Room"},
	}); err != nil {
		t.Fatal(err)
	}
	request.Aggregate.Schedules = []normalize.LegacySchedule{{
		LegacyScheduleID:  "schedule-" + suffix,
		Date:              "2026-08-04",
		Begin:             "09:00",
		End:               "10:00",
		ClassroomLegacyID: "room-" + suffix,
		Classroom:         "Room",
		Confirmed:         true,
	}}
	applier := NewCourseApplier(pool, sqldb.New(pool), master.source)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}

	request.Aggregate.Schedules = nil
	request.ObservedAt = request.ObservedAt.Add(time.Hour)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	var activeCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE course_id=$1 AND deleted_at IS NULL`, courseID).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if activeCount != 0 {
		t.Fatalf("active sessions after empty source schedule = %d, want 0", activeCount)
	}
	var sessionID pgtype.UUID
	var deletedAt *time.Time
	if err := pool.QueryRow(t.Context(), `SELECT id, deleted_at FROM sessions WHERE legacy_schedule_id=$1`, "schedule-"+suffix).Scan(&sessionID, &deletedAt); err != nil {
		t.Fatal(err)
	}
	if deletedAt == nil {
		t.Fatal("schedule row from empty source still active: deleted_at is NULL")
	}
	var changeCount, impactRunCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM session_changes WHERE session_id=$1 AND change_source='legacy_sync'`, sessionID).Scan(&changeCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM session_change_impact_runs WHERE session_change_id IN (SELECT id FROM session_changes WHERE session_id=$1 AND change_source='legacy_sync')`, sessionID).Scan(&impactRunCount); err != nil {
		t.Fatal(err)
	}
	if changeCount != 1 || impactRunCount != 1 {
		t.Fatalf("legacy deactivation impact rows = changes %d/runs %d, want 1/1", changeCount, impactRunCount)
	}
}

// TestCourseApply_PartialScheduleSnapshotRetries pins CB-03 at the
// course-applier entry: a partially applied aggregate is recorded as partial
// and an unchanged-hash refresh retries the skipped rows after the local
// blocker is resolved.
func TestCourseApply_PartialScheduleSnapshotRetries(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	blocker, _, _ := legacyScheduleRequest(t, pool, master.source, suffix, false)
	if _, err := NewScheduleApplier(pool, sqldb.New(pool), master.source).Apply(t.Context(), blocker); err != nil {
		t.Fatal(err)
	}

	request, _ := legacyCourseRequest(t, pool, master.source, suffix+"-partial", false)
	request.Aggregate.Schedules = []normalize.LegacySchedule{
		{
			LegacyScheduleID:  "course-partial-conflict-" + suffix,
			Date:              blocker.Aggregate.Schedules[0].Date,
			Begin:             "09:30",
			End:               "10:30",
			Classroom:         blocker.Aggregate.Schedules[0].Classroom,
			ClassroomLegacyID: blocker.Aggregate.Schedules[0].ClassroomLegacyID,
		},
		{
			LegacyScheduleID:  "course-partial-clear-" + suffix,
			Date:              "2026-08-04",
			Begin:             "11:00",
			End:               "12:00",
			Classroom:         blocker.Aggregate.Schedules[0].Classroom,
			ClassroomLegacyID: blocker.Aggregate.Schedules[0].ClassroomLegacyID,
		},
	}
	applier := NewCourseApplier(pool, sqldb.New(pool), master.source)
	result, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkippedSessions != 1 {
		t.Fatalf("first apply skipped = %d, want 1", result.SkippedSessions)
	}
	var quality string
	if err := pool.QueryRow(t.Context(), `SELECT quality FROM legacy_entity_snapshots WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&quality); err != nil {
		t.Fatal(err)
	}
	if quality != "partial" {
		t.Fatalf("snapshot quality after partial apply = %q, want partial", quality)
	}

	if _, err := pool.Exec(t.Context(), `UPDATE sessions SET start_at = start_at + interval '4 hours', end_at = end_at + interval '4 hours' WHERE legacy_schedule_id=$1`, blocker.Aggregate.Schedules[0].LegacyScheduleID); err != nil {
		t.Fatal(err)
	}
	retry, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !retry.Changed || retry.SkippedSessions != 0 {
		t.Fatalf("retry result = %+v, want changed with zero skipped", retry)
	}
	if err := pool.QueryRow(t.Context(), `SELECT quality FROM legacy_entity_snapshots WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&quality); err != nil {
		t.Fatal(err)
	}
	if quality != "ok" {
		t.Fatalf("snapshot quality after retry = %q, want ok", quality)
	}
	var conflictStatus string
	if err := pool.QueryRow(t.Context(), `
		SELECT status FROM legacy_sync_conflicts
		WHERE external_id=$1 AND source_payload->>'legacy_schedule_id'=$2`,
		request.LegacyCourseID, request.Aggregate.Schedules[0].LegacyScheduleID).Scan(&conflictStatus); err != nil {
		t.Fatal(err)
	}
	if conflictStatus != "resolved" {
		t.Fatalf("resolved schedule conflict status = %q, want resolved", conflictStatus)
	}
}

// TestCourseApply_HonorsConfiguredTimezone pins CB-04: the course applier must
// interpret schedule times in the configured institute timezone, not a
// hardcoded Asia/Bangkok. A 09:00 UTC schedule stores 09:00 UTC.
func TestCourseApply_HonorsConfiguredTimezone(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID := legacyCourseRequest(t, pool, master.source, suffix, false)
	request.InstituteTZ = "UTC"
	request.Aggregate.Schedules = []normalize.LegacySchedule{{
		LegacyScheduleID: "tz-schedule-" + suffix,
		Date:             "2026-08-04",
		Begin:            "09:00",
		End:              "10:00",
		Confirmed:        true,
	}}
	applier := NewCourseApplier(pool, sqldb.New(pool), master.source)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	var startAt, endAt time.Time
	if err := pool.QueryRow(t.Context(), `SELECT start_at, end_at FROM sessions WHERE legacy_schedule_id=$1`, "tz-schedule-"+suffix).Scan(&startAt, &endAt); err != nil {
		t.Fatal(err)
	}
	utcStart := startAt.UTC()
	if utcStart.Hour() != 9 || utcStart.Minute() != 0 {
		t.Fatalf("start_at = %v, want 09:00 UTC (got %02d:%02d UTC)", startAt, utcStart.Hour(), utcStart.Minute())
	}
	if endAt.UTC().Hour() != 10 {
		t.Fatalf("end_at = %v, want 10:00 UTC", endAt)
	}
	var seriesTZ string
	if err := pool.QueryRow(t.Context(), `SELECT institute_tz FROM session_series WHERE course_id=$1 AND source_kind='legacy' AND materialization_mode='external'`, courseID).Scan(&seriesTZ); err != nil {
		t.Fatal(err)
	}
	if seriesTZ != "UTC" {
		t.Fatalf("series institute_tz = %q, want UTC", seriesTZ)
	}
}

// TestCourseApply_CodeCollisionRecordsSyncConflict pins CB-08: when a source
// course code change collides with an existing local course's unique code,
// the failure must become an actionable database_constraint conflict in the
// admin health view instead of only an opaque retrying job error.
func TestCourseApply_CodeCollisionRecordsSyncConflict(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, _ := legacyCourseRequest(t, pool, master.source, suffix, false)
	applier := NewCourseApplier(pool, sqldb.New(pool), master.source)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}

	// A different local course already owns the code the source now uses.
	takenCode := "taken-code-" + suffix
	if _, err := pool.Exec(t.Context(), `INSERT INTO courses (code, name) VALUES ($1, 'Native owner')`, takenCode); err != nil {
		t.Fatal(err)
	}
	request.Aggregate.Course.Code = takenCode
	request.ObservedAt = request.ObservedAt.Add(time.Hour)
	_, err := applier.Apply(t.Context(), request)
	if err == nil {
		t.Fatal("code collision apply unexpectedly succeeded")
	}

	var category, conflictType, status string
	if err := pool.QueryRow(t.Context(), `
		SELECT category, conflict_type, status FROM legacy_sync_conflicts
		WHERE entity_type='course' AND external_id=$1 AND category='database_constraint'`,
		request.LegacyCourseID,
	).Scan(&category, &conflictType, &status); err != nil {
		t.Fatalf("no database_constraint conflict recorded: %v", err)
	}
	if category != "database_constraint" || conflictType != "course_code_conflict" || status != "open" {
		t.Fatalf("conflict = %q/%q/%q, want database_constraint/course_code_conflict/open", category, conflictType, status)
	}
}
