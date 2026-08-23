package apply

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/legacysync/normalize"
)

func legacyScheduleRequest(t *testing.T, pool *pgxpool.Pool, source, suffix string, realtime bool) (ScheduleApplyRequest, pgtype.UUID, string) {
	t.Helper()
	courseID := insertLegacyTestCourse(t, pool, suffix)
	master := NewMasterDataService(pool, sqldb.New(pool), source)
	teacher, err := master.ApplyTeacher(t.Context(), TeacherApplyRequest{
		Teacher: normalize.LegacyTeacher{LegacyID: "schedule-teacher-" + suffix, Name: "Schedule Teacher", IsActive: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	room, err := master.ApplyRoom(t.Context(), RoomApplyRequest{
		Room: normalize.LegacyRoom{LegacyID: "schedule-room-" + suffix, Name: "Schedule Room " + suffix},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ScheduleApplyRequest{
		CourseID:       courseID,
		LegacyCourseID: "schedule-course-" + suffix,
		TeacherID:      teacher.InternalID,
		Aggregate: normalize.LegacyCourseAggregate{
			Course: normalize.LegacyCourse{LegacyID: "schedule-course-" + suffix, Status: "active"},
			Schedules: []normalize.LegacySchedule{{
				LegacyScheduleID:  "schedule-" + suffix,
				Date:              "2026-08-04",
				Begin:             "09:00",
				End:               "10:00",
				Classroom:         room.InternalID.String(),
				ClassroomLegacyID: "schedule-room-" + suffix,
				Confirmed:         true,
				ConfirmedBy:       "teacher",
			}},
		},
		ObservedAt:      time.Date(2026, time.August, 4, 0, 0, 0, 0, time.UTC),
		InstituteTZ:     "Asia/Bangkok",
		RealtimeEnabled: realtime,
	}, courseID, "schedule-" + suffix
}

func TestScheduleApply_ReusesStableLegacyScheduleIdentity(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID, scheduleID := legacyScheduleRequest(t, pool, master.source, suffix, true)
	applier := newTestScheduleApplier(pool, sqldb.New(pool), master.source)

	first, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	var firstSessionID pgtype.UUID
	var firstVersion int32
	if err := pool.QueryRow(t.Context(), `SELECT id, version FROM sessions WHERE legacy_schedule_id=$1`, scheduleID).Scan(&firstSessionID, &firstVersion); err != nil {
		t.Fatal(err)
	}

	request.Aggregate.Schedules[0].Begin = "10:00"
	request.Aggregate.Schedules[0].End = "11:00"
	second, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Changed || second.SourceHash == first.SourceHash {
		t.Fatalf("changed schedule result = %+v, want changed hash", second)
	}
	var secondSessionID pgtype.UUID
	var secondVersion int32
	var startAt time.Time
	if err := pool.QueryRow(t.Context(), `SELECT id, version, start_at FROM sessions WHERE legacy_schedule_id=$1`, scheduleID).Scan(&secondSessionID, &secondVersion, &startAt); err != nil {
		t.Fatal(err)
	}
	if secondSessionID != firstSessionID || secondVersion <= firstVersion {
		t.Fatalf("session identity/version changed incorrectly: first=%v/%d second=%v/%d", firstSessionID, firstVersion, secondSessionID, secondVersion)
	}
	if startAt.In(time.FixedZone("ICT", 7*60*60)).Hour() != 10 {
		t.Fatalf("updated start time = %v, want 10:00 Bangkok", startAt)
	}
	var sessionCount, mappingCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE legacy_schedule_id=$1`, scheduleID).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM external_refs WHERE source=$1 AND entity_type='schedule' AND external_id=$2`, master.source, scheduleID).Scan(&mappingCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 1 || mappingCount != 1 {
		t.Fatalf("session/mapping counts = %d/%d, want 1/1", sessionCount, mappingCount)
	}
	var seriesDuration int32
	var materializationMode string
	if err := pool.QueryRow(t.Context(), `SELECT duration_minutes, materialization_mode FROM session_series WHERE course_id=$1 AND source_kind='legacy' AND legacy_group_key=$2`, courseID, courseID.String()).Scan(&seriesDuration, &materializationMode); err != nil {
		t.Fatal(err)
	}
	if seriesDuration != 60 || materializationMode != "external" {
		t.Fatalf("external series = duration %d/mode %q, want 60/external", seriesDuration, materializationMode)
	}
	_ = courseID
}

func TestScheduleApply_UsesCurrentCourseTeacher(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID, scheduleID := legacyScheduleRequest(t, pool, master.source, suffix, false)
	current, err := master.ApplyTeacher(t.Context(), TeacherApplyRequest{
		Teacher: normalize.LegacyTeacher{LegacyID: "current-teacher-" + suffix, Name: "Current Teacher", IsActive: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE courses SET teacher_id=$1 WHERE id=$2`, current.InternalID, courseID); err != nil {
		t.Fatal(err)
	}

	if _, err := newTestScheduleApplier(pool, sqldb.New(pool), master.source).Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}

	var sessionTeacher, seriesTeacher pgtype.UUID
	if err := pool.QueryRow(t.Context(), `SELECT teacher_id FROM sessions WHERE legacy_schedule_id=$1`, scheduleID).Scan(&sessionTeacher); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT teacher_id FROM session_series WHERE course_id=$1 AND source_kind='legacy' AND materialization_mode='external'`, courseID).Scan(&seriesTeacher); err != nil {
		t.Fatal(err)
	}
	if sessionTeacher != current.InternalID || seriesTeacher != current.InternalID {
		t.Fatalf("legacy teacher ids = session %v/series %v, want current %v", sessionTeacher, seriesTeacher, current.InternalID)
	}
}

func TestScheduleApply_IdenticalAggregateIsNoOp(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, _, scheduleID := legacyScheduleRequest(t, pool, master.source, suffix, true)
	applier := newTestScheduleApplier(pool, sqldb.New(pool), master.source)
	first, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	var beforeVersion int32
	if err := pool.QueryRow(t.Context(), `SELECT version FROM sessions WHERE legacy_schedule_id=$1`, scheduleID).Scan(&beforeVersion); err != nil {
		t.Fatal(err)
	}
	request.ObservedAt = request.ObservedAt.Add(time.Hour)
	second, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed || second.SourceHash != first.SourceHash {
		t.Fatalf("replay result = %+v, want no-op", second)
	}
	var afterVersion, outboxCount int32
	if err := pool.QueryRow(t.Context(), `SELECT version FROM sessions WHERE legacy_schedule_id=$1`, scheduleID).Scan(&afterVersion); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_sync_outbox WHERE entity_type='course' AND external_id=$1`, request.LegacyCourseID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if afterVersion != beforeVersion || outboxCount != 1 {
		t.Fatalf("version/outbox = %d/%d, want %d/1", afterVersion, outboxCount, beforeVersion)
	}
}
func TestScheduleApply_SameCourseConcurrentUpdatesReuseSession(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID, scheduleID := legacyScheduleRequest(t, pool, master.source, suffix, true)
	firstRequest := request
	firstRequest.Aggregate.Schedules = append([]normalize.LegacySchedule(nil), request.Aggregate.Schedules...)
	firstRequest.Aggregate.Schedules[0].Begin = "09:00"
	secondRequest := request
	secondRequest.Aggregate.Schedules = append([]normalize.LegacySchedule(nil), request.Aggregate.Schedules...)
	secondRequest.Aggregate.Schedules[0].Begin = "11:00"
	firstHash, err := ScheduleHash(firstRequest.Aggregate.Schedules[0])
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := ScheduleHash(secondRequest.Aggregate.Schedules[0])
	if err != nil {
		t.Fatal(err)
	}
	applier := newTestScheduleApplier(pool, sqldb.New(pool), master.source)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var group sync.WaitGroup
	for _, candidate := range []ScheduleApplyRequest{firstRequest, secondRequest} {
		candidate := candidate
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, applyErr := applier.Apply(t.Context(), candidate)
			errs <- applyErr
		}()
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	var sessionCount, version int
	var begin, sourceHash string
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE course_id=$1 AND legacy_schedule_id=$2`, courseID, scheduleID).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT version, to_char(start_at AT TIME ZONE 'Asia/Bangkok', 'HH24:MI'), legacy_source_hash FROM sessions WHERE course_id=$1 AND legacy_schedule_id=$2`, courseID, scheduleID).Scan(&version, &begin, &sourceHash); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 1 || version != 2 {
		t.Fatalf("session count/version = %d/%d, want 1/2", sessionCount, version)
	}
	if (begin != "09:00" || sourceHash != firstHash) && (begin != "11:00" || sourceHash != secondHash) {
		t.Fatalf("final schedule state = %q/%q, want one serialized update", begin, sourceHash)
	}
	var mappingCount, snapshotCount, outboxCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM external_refs WHERE source=$1 AND entity_type='schedule' AND external_id=$2`, master.source, scheduleID).Scan(&mappingCount); err != nil {
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
func TestScheduleApply_HistoricalCorrectionPreservesAttendanceDependency(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, _, scheduleID := legacyScheduleRequest(t, pool, master.source, suffix, true)
	request.Aggregate.Schedules[0].Date = "2020-05-23"
	applier := newTestScheduleApplier(pool, sqldb.New(pool), master.source)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	var sessionID, studentID pgtype.UUID
	if err := pool.QueryRow(t.Context(), `SELECT id FROM sessions WHERE legacy_schedule_id=$1`, scheduleID).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `INSERT INTO students (wcode, full_name) VALUES ($1,$2) RETURNING id`, "HIST-"+suffix, "Historical Student").Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO session_attendance (session_id, student_id, status) VALUES ($1,$2,'included')`, sessionID, studentID); err != nil {
		t.Fatal(err)
	}
	request.Aggregate.Schedules[0].Begin = "10:00"
	request.Aggregate.Schedules[0].End = "11:00"
	request.ObservedAt = request.ObservedAt.Add(time.Hour)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	var correctedSessionID pgtype.UUID
	var attendanceCount int
	if err := pool.QueryRow(t.Context(), `SELECT id FROM sessions WHERE legacy_schedule_id=$1`, scheduleID).Scan(&correctedSessionID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM session_attendance WHERE session_id=$1 AND student_id=$2`, correctedSessionID, studentID).Scan(&attendanceCount); err != nil {
		t.Fatal(err)
	}
	if correctedSessionID != sessionID || attendanceCount != 1 {
		t.Fatalf("historical correction changed dependency = %v/%d, want %v/1", correctedSessionID, attendanceCount, sessionID)
	}
}

func TestScheduleApply_RepeatedHistoricalRefreshesRemainBounded(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID, scheduleID := legacyScheduleRequest(t, pool, master.source, suffix, true)
	applier := newTestScheduleApplier(pool, sqldb.New(pool), master.source)
	for range 100 {
		if _, err := applier.Apply(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	var sessionCount, mappingCount, snapshotCount, outboxCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE course_id=$1 AND legacy_schedule_id=$2`, courseID, scheduleID).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM external_refs WHERE source=$1 AND entity_type='schedule' AND external_id=$2`, master.source, scheduleID).Scan(&mappingCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_entity_snapshots WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&snapshotCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_sync_outbox WHERE entity_type='course' AND external_id=$1`, request.LegacyCourseID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 1 || mappingCount != 1 || snapshotCount != 1 || outboxCount != 1 {
		t.Fatalf("bounded refresh rows = sessions %d/mapping %d/snapshot %d/outbox %d, want 1/1/1/1", sessionCount, mappingCount, snapshotCount, outboxCount)
	}
}
func TestScheduleApply_UnassignedRoomPreservesNullRoom(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, _, scheduleID := legacyScheduleRequest(t, pool, master.source, suffix, false)
	request.Aggregate.Schedules[0].Classroom = "[NOT SET]"
	request.Aggregate.Schedules[0].ClassroomLegacyID = ""
	applier := newTestScheduleApplier(pool, sqldb.New(pool), master.source)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	var roomID pgtype.UUID
	if err := pool.QueryRow(t.Context(), `SELECT room_id FROM sessions WHERE legacy_schedule_id=$1`, scheduleID).Scan(&roomID); err != nil {
		t.Fatal(err)
	}
	if roomID.Valid {
		t.Fatalf("unassigned room = %v, want NULL", roomID)
	}
}

func TestScheduleApply_OverlappingScheduleSkippedAndRecorded(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	first, _, _ := legacyScheduleRequest(t, pool, master.source, suffix, false)
	applier := newTestScheduleApplier(pool, sqldb.New(pool), master.source)
	if _, err := applier.Apply(t.Context(), first); err != nil {
		t.Fatal(err)
	}

	// Second course reuses the same room at an overlapping time; its other
	// two sessions must still sync and the conflict must be recorded.
	secondTeacher, err := master.ApplyTeacher(t.Context(), TeacherApplyRequest{
		Teacher: normalize.LegacyTeacher{LegacyID: "schedule-teacher-" + suffix + "-overlap", Name: "Overlap Teacher", IsActive: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondCourseID := insertLegacyTestCourse(t, pool, suffix+"-overlap")
	second := ScheduleApplyRequest{
		CourseID:       secondCourseID,
		LegacyCourseID: "schedule-course-" + suffix + "-overlap",
		TeacherID:      secondTeacher.InternalID,
		Aggregate: normalize.LegacyCourseAggregate{
			Course: normalize.LegacyCourse{LegacyID: "schedule-course-" + suffix + "-overlap", Status: "active"},
			Schedules: []normalize.LegacySchedule{
				{
					LegacyScheduleID: "overlap-before-" + suffix, Date: "2026-08-04", Begin: "07:00", End: "08:00",
					Classroom: first.Aggregate.Schedules[0].Classroom, ClassroomLegacyID: first.Aggregate.Schedules[0].ClassroomLegacyID,
				},
				{
					LegacyScheduleID: "overlap-conflict-" + suffix, Date: "2026-08-04", Begin: "09:30", End: "10:30",
					Classroom: first.Aggregate.Schedules[0].Classroom, ClassroomLegacyID: first.Aggregate.Schedules[0].ClassroomLegacyID,
				},
				{
					LegacyScheduleID: "overlap-after-" + suffix, Date: "2026-08-04", Begin: "11:00", End: "12:00",
					Classroom: first.Aggregate.Schedules[0].Classroom, ClassroomLegacyID: first.Aggregate.Schedules[0].ClassroomLegacyID,
				},
			},
		},
		ObservedAt:  first.ObservedAt,
		InstituteTZ: "Asia/Bangkok",
	}
	result, err := applier.Apply(t.Context(), second)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkippedSessions != 1 {
		t.Fatalf("skipped sessions = %d, want 1 (result %+v)", result.SkippedSessions, result)
	}
	var sessionCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE course_id=$1`, secondCourseID).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 2 {
		t.Fatalf("sessions for overlapping course = %d, want 2", sessionCount)
	}
	var conflictType, message string
	var status string
	if err := pool.QueryRow(t.Context(), `
		SELECT conflict_type, status, message FROM legacy_sync_conflicts
		WHERE external_id=$1 AND source_payload->>'legacy_schedule_id'=$2`,
		second.LegacyCourseID, "overlap-conflict-"+suffix,
	).Scan(&conflictType, &status, &message); err != nil {
		t.Fatal(err)
	}
	if conflictType != "room_overlap" || status != "open" {
		t.Fatalf("conflict = %q/%q, want room_overlap/open", conflictType, status)
	}
	if !strings.Contains(message, "skipped") {
		t.Fatalf("conflict message = %q, want skip explanation", message)
	}

	// Re-applying the same aggregate must not duplicate the open conflict.
	if _, err := applier.Apply(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	var conflictCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_sync_conflicts WHERE external_id=$1`, second.LegacyCourseID).Scan(&conflictCount); err != nil {
		t.Fatal(err)
	}
	if conflictCount != 1 {
		t.Fatalf("conflict rows after re-apply = %d, want 1", conflictCount)
	}
}

func TestScheduleApply_AvailabilityConflictSkipsOnlyOneRow(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID, _ := legacyScheduleRequest(t, pool, master.source, suffix+"-availability", false)
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	windowStart := time.Date(2026, time.August, 4, 8, 0, 0, 0, loc)
	if _, err := pool.Exec(t.Context(), `INSERT INTO teacher_availability (teacher_id, start_at, end_at) VALUES ($1,$2,$3)`, request.TeacherID, windowStart, windowStart.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	request.Aggregate.Schedules = []normalize.LegacySchedule{
		{
			LegacyScheduleID:  "availability-conflict-" + suffix,
			Date:              "2026-08-04",
			Begin:             "09:00",
			End:               "10:00",
			ClassroomLegacyID: request.Aggregate.Schedules[0].ClassroomLegacyID,
			Classroom:         request.Aggregate.Schedules[0].Classroom,
		},
		{
			LegacyScheduleID:  "availability-clear-" + suffix,
			Date:              "2026-08-04",
			Begin:             "08:00",
			End:               "09:00",
			ClassroomLegacyID: request.Aggregate.Schedules[0].ClassroomLegacyID,
			Classroom:         request.Aggregate.Schedules[0].Classroom,
		},
	}

	result, err := newTestScheduleApplier(pool, sqldb.New(pool), master.source).Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkippedSessions != 1 {
		t.Fatalf("skipped sessions = %d, want 1", result.SkippedSessions)
	}
	var activeCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE course_id=$1 AND deleted_at IS NULL`, courseID).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatalf("active sessions = %d, want 1", activeCount)
	}
	var conflictType, status string
	if err := pool.QueryRow(t.Context(), `
		SELECT conflict_type, status FROM legacy_sync_conflicts
		WHERE external_id=$1 AND source_payload->>'legacy_schedule_id'=$2`,
		request.LegacyCourseID, "availability-conflict-"+suffix).Scan(&conflictType, &status); err != nil {
		t.Fatal(err)
	}
	if conflictType != "availability" || status != "open" {
		t.Fatalf("availability conflict = %q/%q, want availability/open", conflictType, status)
	}
}

// TestScheduleApply_RemovedSchedulesAreDeactivated pins CB-02: schedule rows
// that disappear from the source must be soft-deleted locally (never hard
// deleted — attendance history stays attached), their external mappings
// tombstoned, and rows still present must remain active.
func TestScheduleApply_RemovedSchedulesAreDeactivated(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, courseID, _ := legacyScheduleRequest(t, pool, master.source, suffix, false)
	keepID := request.Aggregate.Schedules[0].LegacyScheduleID
	dropID := "schedule-" + suffix + "-drop"
	request.Aggregate.Schedules = append(request.Aggregate.Schedules, normalize.LegacySchedule{
		LegacyScheduleID:  dropID,
		Date:              "2026-08-05",
		Begin:             "11:00",
		End:               "12:00",
		Classroom:         request.Aggregate.Schedules[0].Classroom,
		ClassroomLegacyID: request.Aggregate.Schedules[0].ClassroomLegacyID,
		Confirmed:         true,
	})
	applier := newTestScheduleApplier(pool, sqldb.New(pool), master.source)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	var activeCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE course_id=$1 AND deleted_at IS NULL`, courseID).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if activeCount != 2 {
		t.Fatalf("active sessions after first apply = %d, want 2", activeCount)
	}

	// Source removes the second schedule row.
	request.Aggregate.Schedules = request.Aggregate.Schedules[:1]
	request.ObservedAt = request.ObservedAt.Add(time.Hour)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	var dropDeletedAt, keepDeletedAt *time.Time
	if err := pool.QueryRow(t.Context(), `SELECT deleted_at FROM sessions WHERE legacy_schedule_id=$1`, dropID).Scan(&dropDeletedAt); err != nil {
		t.Fatal(err)
	}
	if dropDeletedAt == nil {
		t.Fatal("removed schedule still active locally: deleted_at is NULL")
	}
	if err := pool.QueryRow(t.Context(), `SELECT deleted_at FROM sessions WHERE legacy_schedule_id=$1`, keepID).Scan(&keepDeletedAt); err != nil {
		t.Fatal(err)
	}
	if keepDeletedAt != nil {
		t.Fatal("still-present schedule was deactivated")
	}
	var dropState string
	if err := pool.QueryRow(t.Context(), `SELECT state FROM external_refs WHERE source=$1 AND entity_type='schedule' AND external_id=$2`, master.source, dropID).Scan(&dropState); err != nil {
		t.Fatal(err)
	}
	if dropState != "tombstoned" {
		t.Fatalf("removed schedule mapping state = %q, want tombstoned", dropState)
	}

	// A fully empty source schedule removes everything but keeps the rows.
	request.Aggregate.Schedules = nil
	request.ObservedAt = request.ObservedAt.Add(time.Hour)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE course_id=$1 AND deleted_at IS NULL`, courseID).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if activeCount != 0 {
		t.Fatalf("active sessions after empty source = %d, want 0", activeCount)
	}
	var remainingRows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE course_id=$1 AND deleted_at IS NOT NULL`, courseID).Scan(&remainingRows); err != nil {
		t.Fatal(err)
	}
	if remainingRows != 2 {
		t.Fatalf("soft-deleted session rows = %d, want 2 (history preserved)", remainingRows)
	}
}

// TestScheduleApply_PartialApplyRetriesAfterConflictResolution pins CB-03: a
// run that skipped rows must be recorded as a partial snapshot, and an
// unchanged-hash refresh must retry the skipped rows once the local blocker is
// resolved instead of taking the no-op fast path.
func TestScheduleApply_PartialApplyRetriesAfterConflictResolution(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	blocker, _, _ := legacyScheduleRequest(t, pool, master.source, suffix, false)
	blockerApplier := newTestScheduleApplier(pool, sqldb.New(pool), master.source)
	if _, err := blockerApplier.Apply(t.Context(), blocker); err != nil {
		t.Fatal(err)
	}

	// This course reuses the blocker's room at an overlapping time; its other
	// row applies, the overlapping row is skipped.
	teacher, err := master.ApplyTeacher(t.Context(), TeacherApplyRequest{
		Teacher: normalize.LegacyTeacher{LegacyID: "partial-teacher-" + suffix, Name: "Partial Teacher", IsActive: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	courseID := insertLegacyTestCourse(t, pool, suffix+"-partial")
	conflictID := "partial-conflict-" + suffix
	request := ScheduleApplyRequest{
		CourseID:       courseID,
		LegacyCourseID: "partial-course-" + suffix,
		TeacherID:      teacher.InternalID,
		Aggregate: normalize.LegacyCourseAggregate{
			Course: normalize.LegacyCourse{LegacyID: "partial-course-" + suffix, Status: "active"},
			Schedules: []normalize.LegacySchedule{
				{
					LegacyScheduleID: conflictID, Date: blocker.Aggregate.Schedules[0].Date,
					Begin: "09:30", End: "10:30",
					Classroom:         blocker.Aggregate.Schedules[0].Classroom,
					ClassroomLegacyID: blocker.Aggregate.Schedules[0].ClassroomLegacyID,
				},
				{
					LegacyScheduleID: "partial-clear-" + suffix, Date: "2026-08-04",
					Begin: "11:00", End: "12:00",
					Classroom:         blocker.Aggregate.Schedules[0].Classroom,
					ClassroomLegacyID: blocker.Aggregate.Schedules[0].ClassroomLegacyID,
				},
			},
		},
		ObservedAt:  blocker.ObservedAt.Add(time.Minute),
		InstituteTZ: "Asia/Bangkok",
	}
	applier := newTestScheduleApplier(pool, sqldb.New(pool), master.source)
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

	// Resolve the local blocker without changing the source aggregate, then
	// refresh: the previously skipped row must be retried and created.
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
		t.Fatalf("snapshot quality after successful retry = %q, want ok", quality)
	}
	var active int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE legacy_schedule_id=$1 AND deleted_at IS NULL`, conflictID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("previously skipped session active = %d, want 1", active)
	}
}

// TestScheduleApply_ShadowModeDoesNotStampOkSnapshot pins the shadow-mode
// poisoning fix: a shadow run must never write a quality='ok' snapshot,
// otherwise a later non-shadow run with the same source hash takes the
// unchanged-hash fast path and the aggregate is never applied.
func TestScheduleApply_ShadowModeDoesNotStampOkSnapshot(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, _, scheduleID := legacyScheduleRequest(t, pool, master.source, suffix, false)
	applier := newTestScheduleApplier(pool, sqldb.New(pool), master.source)

	request.ShadowMode = true
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	// A shadow run must leave no snapshot behind: only a real apply may claim
	// quality='ok'.
	var snapshotCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM legacy_entity_snapshots WHERE source=$1 AND entity_type='course' AND external_id=$2`, master.source, request.LegacyCourseID).Scan(&snapshotCount); err != nil {
		t.Fatal(err)
	}
	if snapshotCount != 0 {
		t.Fatalf("shadow run wrote %d snapshots, want 0", snapshotCount)
	}

	// The non-shadow run with the same hash must apply the aggregate instead of
	// taking the unchanged-hash fast path.
	request.ShadowMode = false
	result, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed {
		t.Fatalf("post-shadow real apply result = %+v, want changed run that applies the aggregate", result)
	}
	var sessionCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE legacy_schedule_id=$1`, scheduleID).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 1 {
		t.Fatalf("sessions after real apply = %d, want 1 (aggregate was never applied)", sessionCount)
	}
}

// TestScheduleApply_ReapplicationRestoresDeletedSessionsAndMappings pins
// CB-07: a source-present schedule row must come back after it was locally
// soft-deleted (and its mapping tombstoned) — the source aggregate is the
// contract, so apply both restores the session and reactivates the mapping.
func TestScheduleApply_ReapplicationRestoresDeletedSessionsAndMappings(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, _, scheduleID := legacyScheduleRequest(t, pool, master.source, suffix, false)
	applier := newTestScheduleApplier(pool, sqldb.New(pool), master.source)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	// Source removes the row: session soft-deleted, mapping tombstoned.
	request.Aggregate.Schedules = nil
	request.ObservedAt = request.ObservedAt.Add(time.Minute)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}

	// Source still has the row again (hash changes back).
	request.Aggregate.Schedules = []normalize.LegacySchedule{{
		LegacyScheduleID:  scheduleID,
		Date:              "2026-08-04",
		Begin:             "09:00",
		End:               "10:00",
		Classroom:         request.Aggregate.Course.Status + "",
		ClassroomLegacyID: "schedule-room-" + suffix,
		Confirmed:         true,
	}}
	request.ObservedAt = request.ObservedAt.Add(time.Minute)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	var deletedAt *time.Time
	if err := pool.QueryRow(t.Context(), `SELECT deleted_at FROM sessions WHERE legacy_schedule_id=$1`, scheduleID).Scan(&deletedAt); err != nil {
		t.Fatal(err)
	}
	if deletedAt != nil {
		t.Fatal("source-present schedule not restored: deleted_at still set")
	}
	var state string
	if err := pool.QueryRow(t.Context(), `SELECT state FROM external_refs WHERE source=$1 AND entity_type='schedule' AND external_id=$2`, master.source, scheduleID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "active" {
		t.Fatalf("restored schedule mapping state = %q, want active", state)
	}
}

// TestScheduleApply_UnchangedRefreshRestoresLocallyDeletedSessions pins
// CB-07 on the fast path: refreshing an unchanged source aggregate must
// restore sessions that were locally soft-deleted, instead of taking the
// no-op path and leaving the source row missing locally.
func TestScheduleApply_UnchangedRefreshRestoresLocallyDeletedSessions(t *testing.T) {
	master, pool, suffix := masterDataTestService(t)
	request, _, scheduleID := legacyScheduleRequest(t, pool, master.source, suffix, false)
	applier := newTestScheduleApplier(pool, sqldb.New(pool), master.source)
	if _, err := applier.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE sessions SET deleted_at = now() WHERE legacy_schedule_id=$1`, scheduleID); err != nil {
		t.Fatal(err)
	}

	result, err := applier.Apply(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed {
		t.Fatalf("unchanged aggregate reported changed: %+v", result)
	}
	var deletedAt *time.Time
	if err := pool.QueryRow(t.Context(), `SELECT deleted_at FROM sessions WHERE legacy_schedule_id=$1`, scheduleID).Scan(&deletedAt); err != nil {
		t.Fatal(err)
	}
	if deletedAt != nil {
		t.Fatal("unchanged-source refresh did not restore locally deleted session")
	}
}
