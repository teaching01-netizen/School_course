package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/legacysync"
	"warwick-institute/internal/legacysync/apply"
	"warwick-institute/internal/legacysync/normalize"
	"warwick-institute/internal/legacysync/parser"
)

// courseIndex is the complete legacy course directory observed from the old
// site, indexed by legacy id: the courses plus the teacher and subject
// master data their rows reference (the refresh path applies both).
type courseIndex struct {
	courses  map[string]normalize.LegacyCourse
	teachers map[string]normalize.LegacyTeacher
	subjects map[string]normalize.LegacySubject
}

// buildCourseIndex indexes a parsed course list result by legacy id. A nil
// result builds an empty, usable index.
func buildCourseIndex(result *parser.CourseListResult) *courseIndex {
	index := &courseIndex{
		courses:  make(map[string]normalize.LegacyCourse),
		teachers: make(map[string]normalize.LegacyTeacher),
		subjects: make(map[string]normalize.LegacySubject),
	}
	if result == nil {
		return index
	}
	for _, course := range result.Courses {
		index.courses[course.LegacyID] = course
	}
	for _, teacher := range result.Teachers {
		index.teachers[teacher.LegacyID] = teacher
	}
	for _, subject := range result.Subjects {
		index.subjects[subject.LegacyID] = subject
	}
	return index
}

// shouldSkipArchivedSync is the "sync once, then skip" rule for archived
// courses: an archived course that has already synced at least once
// (legacy_last_synced_at is set by a real, non-shadow successful apply —
// never in shadow mode or after a failed apply) is left alone on every
// later sync. Courses that never synced — or are active — keep syncing.
func shouldSkipArchivedSync(archived bool, lastSynced pgtype.Timestamptz) bool {
	return archived && lastSynced.Valid
}

// courseSyncer runs one linked legacy course through the whole refresh
// pipeline: resolve the local link, fetch the old-site schedule page, apply
// the observed master data (rooms, teacher, subject) and then the course and
// schedule aggregates. It is the SyncCourse callback of the runner and also
// owns the course-index cache the callback and the full reconcile share.
type courseSyncer struct {
	pool        *pgxpool.Pool
	q           *sqldb.Queries
	client      *legacysync.Client
	master      *apply.MasterDataService
	courseApp   *apply.CourseApplier
	scheduleApp *apply.ScheduleApplier
	instituteTZ string
	log         *slog.Logger

	// studentProfileWorkers bounds the parallel /Admin/Students lookups
	// during profile sync; 1 runs the historical sequential loop.
	studentProfileWorkers int

	indexMu      sync.Mutex
	courseList   *parser.CourseListResult
	courseListAt time.Time
}

func newCourseSyncer(pool *pgxpool.Pool, q *sqldb.Queries, client *legacysync.Client, master *apply.MasterDataService, courseApp *apply.CourseApplier, scheduleApp *apply.ScheduleApplier, instituteTZ string, log *slog.Logger, studentProfileWorkers int) *courseSyncer {
	return &courseSyncer{
		pool:                  pool,
		q:                     q,
		client:                client,
		master:                master,
		courseApp:             courseApp,
		scheduleApp:           scheduleApp,
		instituteTZ:           instituteTZ,
		log:                   log,
		studentProfileWorkers: studentProfileWorkers,
	}
}

// fetchCourseList observes the complete legacy course directory: the plain
// listing (active and draft) plus the archive-search listing (archived
// only), merged with plain entries winning. Both pages come from the old
// site and either parse failure aborts the caller. The two fetches run
// concurrently — they are independent reads — cutting the directory
// observation from two round trips to one.
func (s *courseSyncer) fetchCourseList(ctx context.Context) (*parser.CourseListResult, error) {
	var (
		plainPage, archivedPage string
		errMu                   sync.Mutex
		fetchErr                error
		wg                      sync.WaitGroup
	)
	record := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if fetchErr == nil {
			fetchErr = err
		}
		errMu.Unlock()
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		page, err := s.client.FetchCourseListPageContext(ctx)
		record(err)
		if err == nil {
			plainPage = page
		}
	}()
	go func() {
		defer wg.Done()
		page, err := s.client.FetchArchivedCourseListPageContext(ctx)
		record(err)
		if err == nil {
			archivedPage = page
		}
	}()
	wg.Wait()
	if fetchErr != nil {
		return nil, fetchErr
	}
	plain, err := parser.ParseCourseList(plainPage)
	if err != nil {
		return nil, err
	}
	archived, err := parser.ParseCourseList(archivedPage)
	if err != nil {
		return nil, err
	}
	merged := parser.MergeCourseLists(plain, archived)
	s.log.Info("legacy course list fetched",
		"plain", len(plain.Courses),
		"archived", len(archived.Courses),
		"merged", len(merged.Courses))
	return merged, nil
}

// loadCourseIndex returns the indexed course directory, cached for five
// minutes so per-course refreshes do not fetch the (multi-MB) list page for
// every course.
func (s *courseSyncer) loadCourseIndex(ctx context.Context) (*courseIndex, error) {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	if s.courseList != nil && time.Since(s.courseListAt) < 5*time.Minute {
		return buildCourseIndex(s.courseList), nil
	}
	parsed, err := s.fetchCourseList(ctx)
	if err != nil {
		return nil, err
	}
	s.courseList = parsed
	s.courseListAt = time.Now()
	return buildCourseIndex(parsed), nil
}

// applyCourseMasterData makes the master data referenced by a course
// available before the course apply resolves it: the course's teacher and
// subject are applied from the course directory, exactly like the rooms
// loop applies each schedule's classroom. Both applies are idempotent
// snapshot-hash fast paths and honor shadow mode, so repeated refreshes are
// no-ops. A reference missing from the directory is left for a future full
// reconcile: the apply will then fail retryably instead of leaving silent
// mismatches.
func (s *courseSyncer) applyCourseMasterData(ctx context.Context, index *courseIndex, course normalize.LegacyCourse, observedAt time.Time, shadowMode, realtimeEnabled bool) error {
	if course.TeacherID != "" {
		teacher, ok := index.teachers[course.TeacherID]
		if !ok {
			s.log.Warn("legacy teacher not in course index", "legacy_course_id", course.LegacyID, "legacy_teacher_id", course.TeacherID)
		} else if _, err := s.master.ApplyTeacher(ctx, apply.TeacherApplyRequest{Teacher: teacher, ObservedAt: observedAt, ShadowMode: shadowMode, RealtimeEnabled: realtimeEnabled}); err != nil {
			return fmt.Errorf("apply legacy teacher %s: %w", course.TeacherID, err)
		}
	}
	if course.SubjectID != "" {
		subject, ok := index.subjects[course.SubjectID]
		if !ok {
			s.log.Warn("legacy subject not in course index", "legacy_course_id", course.LegacyID, "legacy_subject_id", course.SubjectID)
		} else if _, err := s.master.ApplySubject(ctx, apply.SubjectApplyRequest{Subject: subject, ObservedAt: observedAt, ShadowMode: shadowMode, RealtimeEnabled: realtimeEnabled}); err != nil {
			return fmt.Errorf("apply legacy subject %s: %w", course.SubjectID, err)
		}
	}
	return nil
}

// syncCourse refreshes one linked legacy course. The local link is
// authoritative: an absent link skips the job (the course was deleted or
// the link cleared since enqueue), and an archived course that already
// synced once is skipped before ANY source request ("sync once, then
// skip") — so the leader sweep, reconcile jobs, and the admin refresh
// button all become no-ops for it.
func (s *courseSyncer) syncCourse(ctx context.Context, legacyID string) error {
	linked, found, err := findLinkedLegacyCourse(ctx, s.pool, legacyID)
	if err != nil {
		return fmt.Errorf("find linked course %s: %w", legacyID, err)
	}
	if !found {
		// The local link is gone (e.g. the course was deleted since the
		// refresh job was enqueued): nothing to refresh for this legacy id.
		// Skip without error escalation so the job is not retried forever.
		return nil
	}
	if shouldSkipArchivedSync(linked.legacyArchived, linked.lastSyncedAt) {
		s.log.Info("skipping archived legacy course (already synced once)", "legacy_course_id", legacyID)
		return nil
	}
	page, err := s.client.FetchSchedulePageContext(ctx, legacyID)
	if err != nil {
		return err
	}
	aggregate, err := parser.ParseCourseDetail(page)
	if err != nil {
		return err
	}
	index, err := s.loadCourseIndex(ctx)
	if err != nil {
		return fmt.Errorf("load legacy course index: %w", err)
	}
	course, ok := index.courses[legacyID]
	if !ok {
		return fmt.Errorf("legacy course %s not found in complete course index", legacyID)
	}
	aggregate.Course = course
	assignScheduleIDs(aggregate, legacyID)
	control, err := s.q.LegacySyncControlGet(ctx)
	if err != nil {
		return err
	}
	observedAt := time.Now().UTC()
	// Rooms referenced by schedule rows become master data so the schedule
	// applier can resolve them to internal room ids.
	seenRooms := make(map[string]bool)
	for _, schedule := range aggregate.Schedules {
		if schedule.ClassroomLegacyID == "" || schedule.Classroom == "" || seenRooms[schedule.ClassroomLegacyID] {
			continue
		}
		seenRooms[schedule.ClassroomLegacyID] = true
		if _, err := s.master.ApplyRoom(ctx, apply.RoomApplyRequest{
			Room:            normalize.LegacyRoom{LegacyID: schedule.ClassroomLegacyID, Name: schedule.Classroom},
			ObservedAt:      observedAt,
			ShadowMode:      control.ShadowMode,
			RealtimeEnabled: control.RealtimeEnabled,
		}); err != nil {
			return fmt.Errorf("apply legacy room %s: %w", schedule.ClassroomLegacyID, err)
		}
	}
	if err := s.applyCourseMasterData(ctx, index, course, observedAt, control.ShadowMode, control.RealtimeEnabled); err != nil {
		return err
	}
	if _, err := s.courseApp.Apply(ctx, apply.CourseApplyRequest{
		CourseID:        linked.courseID,
		LegacyCourseID:  legacyID,
		Aggregate:       *aggregate,
		ObservedAt:      observedAt,
		InstituteTZ:     s.instituteTZ,
		ShadowMode:      control.ShadowMode,
		RealtimeEnabled: control.RealtimeEnabled,
	}); err != nil {
		return err
	}
	_, err = s.scheduleApp.Apply(ctx, apply.ScheduleApplyRequest{
		CourseID:        linked.courseID,
		LegacyCourseID:  legacyID,
		TeacherID:       linked.teacherID,
		Aggregate:       *aggregate,
		ObservedAt:      observedAt,
		InstituteTZ:     s.instituteTZ,
		ShadowMode:      control.ShadowMode,
		RealtimeEnabled: control.RealtimeEnabled,
	})
	return err
}
