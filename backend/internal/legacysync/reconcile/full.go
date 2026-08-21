package reconcile

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/jobqueue"
	"warwick-institute/internal/legacysync/apply"
	"warwick-institute/internal/legacysync/normalize"
)

// FullReconciler converges the local course catalogue with the complete
// legacy course index. For every legacy course it links a local course by
// legacy_course_id: reusing an already-linked course, claiming an unlinked
// local course with the same code, or creating a minimal linked course. It
// also applies the observed teacher/subject master data and enqueues a
// refresh job per legacy course so schedules sync promptly.
type FullReconciler struct {
	pool   *pgxpool.Pool
	q      *sqldb.Queries
	store  jobqueue.Store
	master *apply.MasterDataService
	source string
}

func NewFullReconciler(pool *pgxpool.Pool, q *sqldb.Queries, store jobqueue.Store, master *apply.MasterDataService, source string) *FullReconciler {
	return &FullReconciler{pool: pool, q: q, store: store, master: master, source: source}
}

// FullReconcileOptions carries the sync control flags observed when the
// reconcile run starts. In shadow mode nothing is linked, created, or
// enqueued; master data is recorded as snapshots only (the master data
// service applies the same shadow rule internally). StudentEnabled imports
// course rosters (students + enrollments, add-only) for linked courses.
type FullReconcileOptions struct {
	ObservedAt     time.Time
	ShadowMode     bool
	StudentEnabled bool
	// Concurrency > 1 runs the master-data and course phases on bounded
	// worker pools; 0 and 1 select the exact serial path. The per-course
	// advisory locks, add-only roster applies, and enqueue deduplication
	// make both modes converge to the same catalogue.
	Concurrency int
	Progress    func(FullReconcileProgress) error
}

type FullReconcileProgress struct {
	Phase             string
	CurrentLegacyID   string
	ProcessedEntities int
	TotalEntities     int
	ChangedEntities   int
	AppliedEntities   int
	Failures          int
}

// FullReconcileStats summarizes one reconcile pass.
type FullReconcileStats struct {
	Courses           int // legacy courses in the index
	AlreadyLinked     int // local course already linked to this legacy id
	LinkedByCode      int // unlinked local course claimed by matching code
	Created           int // minimal local course created and linked
	Suffixed          int // local course created with suffixed code due to collision
	Conflicts         int // legacy courses that could not be linked
	MasterData        int // teacher and subject applies
	Enqueued          int // legacy_refresh_course jobs enqueued
	RosterStudents    int // roster students created (add-only)
	RosterEnrollments int // roster enrollments added (add-only)
}

func (r *FullReconciler) Reconcile(
	ctx context.Context,
	courses []normalize.LegacyCourse,
	teachers []normalize.LegacyTeacher,
	subjects []normalize.LegacySubject,
	opts FullReconcileOptions,
) (FullReconcileStats, error) {
	stats := FullReconcileStats{Courses: len(courses)}
	if r.pool == nil || r.q == nil || r.master == nil {
		return stats, errors.New("legacy full reconcile: pool, queries, and master data service are required")
	}
	if opts.ObservedAt.IsZero() {
		opts.ObservedAt = time.Now().UTC()
	}
	report := func(progress FullReconcileProgress) error {
		if opts.Progress == nil {
			return nil
		}
		if err := opts.Progress(progress); err != nil {
			return fmt.Errorf("full reconcile: report progress: %w", err)
		}
		return nil
	}

	if err := report(FullReconcileProgress{
		Phase:         "applying_master_data",
		TotalEntities: len(teachers) + len(subjects),
	}); err != nil {
		return stats, err
	}

	if opts.Concurrency > 1 {
		applied, err := r.applyMasterDataParallel(ctx, teachers, subjects, opts, report)
		if err != nil {
			return stats, err
		}
		stats.MasterData = applied
	} else {
		processedMasterData := 0
		for _, teacher := range teachers {
			if _, err := r.master.ApplyTeacher(ctx, apply.TeacherApplyRequest{Teacher: teacher, ObservedAt: opts.ObservedAt, ShadowMode: opts.ShadowMode}); err != nil {
				return stats, fmt.Errorf("full reconcile: apply legacy teacher %s: %w", teacher.LegacyID, err)
			}
			stats.MasterData++
			processedMasterData++
			if err := report(FullReconcileProgress{
				Phase:             "applying_master_data",
				CurrentLegacyID:   teacher.LegacyID,
				ProcessedEntities: processedMasterData,
				TotalEntities:     len(teachers) + len(subjects),
				AppliedEntities:   stats.MasterData,
			}); err != nil {
				return stats, err
			}
		}
		for _, subject := range subjects {
			if _, err := r.master.ApplySubject(ctx, apply.SubjectApplyRequest{Subject: subject, ObservedAt: opts.ObservedAt, ShadowMode: opts.ShadowMode}); err != nil {
				return stats, fmt.Errorf("full reconcile: apply legacy subject %s: %w", subject.LegacyID, err)
			}
			stats.MasterData++
			processedMasterData++
			if err := report(FullReconcileProgress{
				Phase:             "applying_master_data",
				CurrentLegacyID:   subject.LegacyID,
				ProcessedEntities: processedMasterData,
				TotalEntities:     len(teachers) + len(subjects),
				AppliedEntities:   stats.MasterData,
			}); err != nil {
				return stats, err
			}
		}
	}

	if opts.ShadowMode {
		if err := report(FullReconcileProgress{
			Phase:             "observing_legacy_courses",
			ProcessedEntities: len(courses),
			TotalEntities:     len(courses),
			AppliedEntities:   stats.MasterData,
		}); err != nil {
			return stats, err
		}
		return stats, nil
	}

	ordered := append([]normalize.LegacyCourse(nil), courses...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].LegacyID < ordered[j].LegacyID })
	if err := report(FullReconcileProgress{
		Phase:         "reconciling_courses",
		TotalEntities: len(ordered),
	}); err != nil {
		return stats, err
	}
	if opts.Concurrency > 1 {
		if err := r.reconcileCoursesParallel(ctx, ordered, opts, report, &stats); err != nil {
			return stats, err
		}
	} else {
		for index, course := range ordered {
			if err := r.reconcileOne(ctx, index, len(ordered), course, opts.ObservedAt, opts, &stats, report); err != nil {
				return stats, err
			}
		}
	}
	if err := r.resolveCodeClaimConflicts(ctx); err != nil {
		return stats, err
	}
	if opts.Concurrency > 1 && len(ordered) > 0 {
		if err := report(FullReconcileProgress{
			Phase:             "reconciling_courses",
			CurrentLegacyID:   ordered[len(ordered)-1].LegacyID,
			ProcessedEntities: len(ordered),
			TotalEntities:     len(ordered),
			ChangedEntities:   stats.LinkedByCode + stats.Created,
			AppliedEntities:   stats.Enqueued,
			Failures:          stats.Conflicts,
		}); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

// reconcileOne performs the per-course reconcile work for exactly one legacy
// course: link it (via the existing advisory-locked claim), mirror its status,
// import its roster when enabled, and enqueue its refresh job — with the
// per-course progress report emitted only when report is non-nil. Serial mode
// passes the shared stats and a live reporter (byte-identical reporting to the
// historical loop); parallel workers pass local stats and nil so the single
// coordinator owns all reporting.
func (r *FullReconciler) reconcileOne(
	ctx context.Context,
	index, total int,
	course normalize.LegacyCourse,
	observedAt time.Time,
	opts FullReconcileOptions,
	stats *FullReconcileStats,
	report func(FullReconcileProgress) error, // nil = no per-item reporting
) error {
	courseID, lastSyncedAt, linked, err := r.linkCourse(ctx, course, observedAt, stats)
	if err != nil {
		return fmt.Errorf("full reconcile: link legacy course %s: %w", course.LegacyID, err)
	}
	emit := func() error {
		if report == nil {
			return nil
		}
		return report(FullReconcileProgress{
			Phase:             "reconciling_courses",
			CurrentLegacyID:   course.LegacyID,
			ProcessedEntities: index + 1,
			TotalEntities:     total,
			ChangedEntities:   stats.LinkedByCode + stats.Created,
			AppliedEntities:   stats.Enqueued,
			Failures:          stats.Conflicts,
		})
	}
	if !linked {
		return emit()
	}
	if opts.StudentEnabled {
		if err := r.applyRoster(ctx, course, courseID, stats); err != nil {
			return fmt.Errorf("full reconcile: import roster for legacy course %s: %w", course.LegacyID, err)
		}
	}
	// The upstream status is mirrored BOTH ways: archived courses get
	// their archive stamp locally, and a course un-archived on the old
	// site is flipped back to active. The archive flag gates the sweep
	// ("sync once, then skip"), so a stale flag would hide a reactivated
	// course forever; the update is idempotent and skips unchanged rows.
	if _, err := r.pool.Exec(ctx, `
		UPDATE courses SET legacy_status = $2, legacy_archived = ($2 = 'archived'), updated_at = now()
		WHERE id = $1 AND (legacy_status IS DISTINCT FROM $2 OR legacy_archived IS DISTINCT FROM ($2 = 'archived'))
	`, courseID, course.Status); err != nil {
		return fmt.Errorf("full reconcile: mirror legacy status of course %s: %w", course.LegacyID, err)
	}
	// Archived courses sync once, then stop: an archived course that
	// already had a successful sync needs no further refresh job. The
	// original serial loop reported nothing for this branch, so neither do
	// we (parallel workers pass a nil report and still event the course).
	if course.Status == "archived" && lastSyncedAt.Valid {
		return nil
	}
	if _, err := r.store.Enqueue(ctx, jobqueue.EnqueueRequest{
		JobType:     "legacy_refresh_course",
		EntityType:  "course",
		ExternalID:  course.LegacyID,
		UniqueKey:   "legacy:course:" + course.LegacyID,
		Priority:    2,
		RunAfter:    observedAt,
		MaxAttempts: 5,
	}); err != nil {
		return fmt.Errorf("full reconcile: enqueue legacy course %s: %w", course.LegacyID, err)
	}
	stats.Enqueued++
	return emit()
}

// courseWorkers sizes the reconcile worker pools: never more than the
// requested concurrency, never more than the number of items; 0/1 report the
// serial flag back so callers can select the exact serial path.
func courseWorkers(concurrency, items int) int {
	if concurrency < items {
		return concurrency
	}
	return items
}

// batchCollectWindow is how long the coordinator keeps collecting completion
// events for the current batch after the first one arrives. A purely
// non-blocking drain races the workers: it runs microseconds after the first
// completion, before the other workers of the same wave finish, so batches
// would rarely exceed one event. The window is far shorter than the per-course
// DB work and only exists on the parallel path — the serial path never
// batches by construction.
const batchCollectWindow = 2 * time.Millisecond

// mergeStats folds one worker's local pass totals into the shared stats.
// Courses is excluded: workers never touch it and the caller sets it from
// the ordered course list.
func mergeStats(dst *FullReconcileStats, delta FullReconcileStats) {
	dst.AlreadyLinked += delta.AlreadyLinked
	dst.LinkedByCode += delta.LinkedByCode
	dst.Created += delta.Created
	dst.Suffixed += delta.Suffixed
	dst.Conflicts += delta.Conflicts
	dst.MasterData += delta.MasterData
	dst.Enqueued += delta.Enqueued
	dst.RosterStudents += delta.RosterStudents
	dst.RosterEnrollments += delta.RosterEnrollments
}

// applyMasterDataParallel applies the observed teachers and subjects through
// a bounded pool. Each worker keeps a local applied count and emits one
// completion event per item; the single coordinator (the only goroutine
// calling opts.Progress) drains pending events into batches — one progress
// callback per batch — and folds the applied totals into the returned count.
// The first worker error cancels the pool and wins.
func (r *FullReconciler) applyMasterDataParallel(
	ctx context.Context,
	teachers []normalize.LegacyTeacher,
	subjects []normalize.LegacySubject,
	opts FullReconcileOptions,
	report func(FullReconcileProgress) error,
) (int, error) {
	total := len(teachers) + len(subjects)
	workers := courseWorkers(opts.Concurrency, total)
	if workers == 0 {
		return 0, nil
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type masterItem struct {
		kind     string
		legacyID string
		teacher  normalize.LegacyTeacher
		subject  normalize.LegacySubject
	}
	type mdEvent struct {
		legacyID string
		applied  int
		err      error
	}
	events := make(chan mdEvent, workers)
	jobs := make(chan masterItem)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if err := workCtx.Err(); err != nil {
					return
				}
				var err error
				if item.kind == "teacher" {
					_, err = r.master.ApplyTeacher(workCtx, apply.TeacherApplyRequest{Teacher: item.teacher, ObservedAt: opts.ObservedAt, ShadowMode: opts.ShadowMode})
				} else {
					_, err = r.master.ApplySubject(workCtx, apply.SubjectApplyRequest{Subject: item.subject, ObservedAt: opts.ObservedAt, ShadowMode: opts.ShadowMode})
				}
				if err != nil {
					events <- mdEvent{legacyID: item.legacyID, err: fmt.Errorf("full reconcile: apply legacy %s %s: %w", item.kind, item.legacyID, err)}
					cancel()
					return
				}
				events <- mdEvent{legacyID: item.legacyID, applied: 1}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, teacher := range teachers {
			select {
			case jobs <- masterItem{kind: "teacher", legacyID: teacher.LegacyID, teacher: teacher}:
			case <-workCtx.Done():
				return
			}
		}
		for _, subject := range subjects {
			select {
			case jobs <- masterItem{kind: "subject", legacyID: subject.LegacyID, subject: subject}:
			case <-workCtx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(events)
	}()

	var firstErr error
	processed, applied := 0, 0
	for ev := range events {
		if ev.err != nil && firstErr == nil {
			firstErr = ev.err
		}
		if firstErr != nil {
			continue
		}
		processed++
		applied += ev.applied
		last := ev
	inner:
		for {
			select {
			case next, ok := <-events:
				if !ok {
					break inner // channel closed: no more events coming
				}
				if next.err != nil && firstErr == nil {
					firstErr = next.err
				}
				if firstErr != nil {
					break inner
				}
				processed++
				applied += next.applied
				last = next
			case <-time.After(batchCollectWindow):
				break inner
			}
		}
		if firstErr != nil {
			continue
		}
		if err := report(FullReconcileProgress{
			Phase:             "applying_master_data",
			CurrentLegacyID:   last.legacyID,
			ProcessedEntities: processed,
			TotalEntities:     total,
			AppliedEntities:   applied,
		}); err != nil {
			cancel()
			for range events { // keep workers unblocked while they wind down
			}
			return applied, err
		}
	}
	return applied, firstErr
}

// reconcileCoursesParallel links every ordered course through a bounded pool
// of reconcileOne calls, each with its own local stats (linkCourse already
// writes only through its stats argument, so workers are race-free without
// atomics). Workers emit completion events; the single coordinator merges
// worker deltas and emits batched progress callbacks — the only goroutine
// that calls opts.Progress. The first worker error cancels the pool and is
// returned; per-course advisory locks and add-only roster applies keep the
// concurrent passes convergent and idempotent.
func (r *FullReconciler) reconcileCoursesParallel(
	ctx context.Context,
	ordered []normalize.LegacyCourse,
	opts FullReconcileOptions,
	report func(FullReconcileProgress) error,
	stats *FullReconcileStats,
) error {
	workers := courseWorkers(opts.Concurrency, len(ordered))
	if workers == 0 {
		return nil
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type courseEvent struct {
		legacyID string
		delta    FullReconcileStats
		err      error
	}
	events := make(chan courseEvent, workers)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				if err := workCtx.Err(); err != nil {
					return
				}
				local := &FullReconcileStats{}
				if err := r.reconcileOne(workCtx, index, len(ordered), ordered[index], opts.ObservedAt, opts, local, nil); err != nil {
					events <- courseEvent{legacyID: ordered[index].LegacyID, err: err}
					cancel()
					return
				}
				events <- courseEvent{legacyID: ordered[index].LegacyID, delta: *local}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range ordered {
			select {
			case jobs <- index:
			case <-workCtx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(events)
	}()

	processed := 0
	var firstErr error
	for ev := range events {
		if ev.err != nil && firstErr == nil {
			firstErr = ev.err
		}
		if firstErr != nil {
			continue
		}
		processed++
		mergeStats(stats, ev.delta)
		last := ev
	inner:
		for {
			select {
			case next, ok := <-events:
				if !ok {
					break inner // channel closed: no more events coming
				}
				if next.err != nil && firstErr == nil {
					firstErr = next.err
				}
				if firstErr != nil {
					break inner
				}
				processed++
				mergeStats(stats, next.delta)
				last = next
			case <-time.After(batchCollectWindow):
				break inner
			}
		}
		if firstErr != nil {
			continue
		}
		if err := report(FullReconcileProgress{
			Phase:             "reconciling_courses",
			CurrentLegacyID:   last.legacyID,
			ProcessedEntities: processed,
			TotalEntities:     len(ordered),
			ChangedEntities:   stats.LinkedByCode + stats.Created,
			AppliedEntities:   stats.Enqueued,
			Failures:          stats.Conflicts,
		}); err != nil {
			cancel()
			for range events { // keep workers unblocked while they wind down
			}
			return err
		}
	}
	return firstErr
}

// resolveCodeClaimConflicts closes conflicts whose underlying collision no
// longer holds, once per reconcile pass so repeated runs converge the
// conflict list instead of accumulating it:
//   - code_claimed: the legacy course has since become linked (by any means);
//   - code_collision: the legacy course now holds the original code it was
//     suffixed away from (e.g. the previous winner released it), so the
//     collision that justified the open row is gone. A course still holding
//     a suffixed code keeps its open collision.
func (r *FullReconciler) resolveCodeClaimConflicts(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, `
		UPDATE legacy_sync_conflicts c
		SET status = 'resolved', resolved_at = now()
		WHERE c.status = 'open' AND c.conflict_type = 'code_claimed'
		  AND EXISTS (SELECT 1 FROM courses x WHERE x.legacy_course_id = c.external_id)
	`); err != nil {
		return fmt.Errorf("resolve code claim conflicts: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `
		UPDATE legacy_sync_conflicts c
		SET status = 'resolved', resolved_at = now()
		WHERE c.status = 'open' AND c.conflict_type = 'code_collision'
		  AND EXISTS (SELECT 1 FROM courses x
		              WHERE x.legacy_course_id = c.external_id
		                AND x.code = c.source_payload->>'code')
	`); err != nil {
		return fmt.Errorf("resolve code collision conflicts: %w", err)
	}
	return nil
}

// linkCourse resolves the local course for one legacy course. It returns
// the local course id, the last successful sync stamp of an already-linked
// course (invalid when the course was just linked/created this pass — the
// "sync once, then skip" rule needs the pre-pass value), and whether the
// course ended up linked (and therefore worth refreshing).
func (r *FullReconciler) linkCourse(ctx context.Context, course normalize.LegacyCourse, observedAt time.Time, stats *FullReconcileStats) (pgtype.UUID, pgtype.Timestamptz, bool, error) {
	if course.LegacyID == "" {
		return pgtype.UUID{}, pgtype.Timestamptz{}, false, errors.New("legacy course id is required")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(context.Background()) // no-op on committed tx
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, r.source+":course:"+course.LegacyID); err != nil {
		return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("lock: %w", err)
	}
	// Claims are also serialized by course code: two legacy courses sharing
	// one code must not both believe they claimed the same local course.
	// Lock ordering is always legacy-id then code, so reconciles cannot
	// deadlock against each other.
	if course.Code != "" {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, r.source+":code:"+course.Code); err != nil {
			return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("lock code: %w", err)
		}
	}

	var courseID pgtype.UUID
	var lastSyncedAt pgtype.Timestamptz
	err = tx.QueryRow(ctx, `SELECT id, legacy_last_synced_at FROM courses WHERE legacy_course_id = $1`, course.LegacyID).Scan(&courseID, &lastSyncedAt)
	if err == nil {
		stats.AlreadyLinked++
		return courseID, lastSyncedAt, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("find linked course: %w", err)
	}

	// If code is empty, use LEGACY-legacyID as the code.
	if course.Code == "" {
		course.Code = fmt.Sprintf("LEGACY-%s", course.LegacyID)
	}

	var localID pgtype.UUID
	var claimedBy *string
	err = tx.QueryRow(ctx, `SELECT id, legacy_course_id FROM courses WHERE code = $1`, course.Code).Scan(&localID, &claimedBy)
	switch {
	case err == nil && claimedBy == nil:
		claim, err := tx.Exec(ctx, `UPDATE courses SET legacy_course_id = $1, source_kind = 'legacy', updated_at = now() WHERE id = $2 AND legacy_course_id IS NULL`, course.LegacyID, localID)
		if err != nil {
			return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("claim unlinked course by code: %w", err)
		}
		if claim.RowsAffected() != 1 {
			// Lost the race against a concurrent claim by another legacy
			// course: suffix the code and create a new course.
			return r.createSuffixedCourse(ctx, tx, course, stats)
		}
		if err := tx.Commit(ctx); err != nil {
			return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("commit claim: %w", err)
		}
		stats.LinkedByCode++
		return localID, pgtype.Timestamptz{}, true, nil
	case err == nil:
		// Code is already claimed by another legacy course, suffix and create.
		return r.createSuffixedCourse(ctx, tx, course, stats)
	case errors.Is(err, pgx.ErrNoRows):
		name := course.Name
		if name == "" {
			name = course.Code
		}
		if err := tx.QueryRow(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind) VALUES ($1, $2, $3, 'legacy') RETURNING id`, course.Code, name, course.LegacyID).Scan(&courseID); err != nil {
			return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("create linked course: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("commit create: %w", err)
		}
		stats.Created++
		return courseID, pgtype.Timestamptz{}, true, nil
	default:
		return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("find course by code: %w", err)
	}
}

// createSuffixedCourse creates a new local course with a deterministic suffixed code
// when the original code is unavailable. The suffix scheme:
// 1. {code}-{legacyID}
// 2. {code}-{legacyID}-{n} for n=2..10
// 3. LEGACY-{legacyID}-{hash8} where hash8 = sha256(legacyID)[:8]
func (r *FullReconciler) createSuffixedCourse(ctx context.Context, tx pgx.Tx, course normalize.LegacyCourse, stats *FullReconcileStats) (pgtype.UUID, pgtype.Timestamptz, bool, error) {
	originalCode := course.Code
	suffixedCode := ""
	// Try {code}-{legacyID}
	suffixedCode = fmt.Sprintf("%s-%s", originalCode, course.LegacyID)
	if r.codeAvailable(ctx, tx, suffixedCode) {
		return r.insertSuffixedCourse(ctx, tx, course, suffixedCode, originalCode, stats)
	}
	// Try {code}-{legacyID}-{n} for n=2..10
	for n := 2; n <= 10; n++ {
		suffixedCode = fmt.Sprintf("%s-%s-%d", originalCode, course.LegacyID, n)
		if r.codeAvailable(ctx, tx, suffixedCode) {
			return r.insertSuffixedCourse(ctx, tx, course, suffixedCode, originalCode, stats)
		}
	}
	// Fallback: LEGACY-{legacyID}-{hash8}
	hash := sha256.Sum256([]byte(course.LegacyID))
	hash8 := fmt.Sprintf("%x", hash[:4])
	suffixedCode = fmt.Sprintf("LEGACY-%s-%s", course.LegacyID, hash8)
	return r.insertSuffixedCourse(ctx, tx, course, suffixedCode, originalCode, stats)
}

// codeAvailable checks if a course code is available (not in use).
func (r *FullReconciler) codeAvailable(ctx context.Context, tx pgx.Tx, code string) bool {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM courses WHERE code = $1)`, code).Scan(&exists); err != nil {
		return false
	}
	return !exists
}

// insertSuffixedCourse inserts a course with a suffixed code and records the conflict.
func (r *FullReconciler) insertSuffixedCourse(ctx context.Context, tx pgx.Tx, course normalize.LegacyCourse, suffixedCode, originalCode string, stats *FullReconcileStats) (pgtype.UUID, pgtype.Timestamptz, bool, error) {
	name := course.Name
	if name == "" {
		name = suffixedCode
	}
	var courseID pgtype.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind) VALUES ($1, $2, $3, 'legacy') RETURNING id`, suffixedCode, name, course.LegacyID).Scan(&courseID); err != nil {
		return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("create suffixed course: %w", err)
	}
	// Record the code collision conflict.
	if err := r.recordCodeCollisionConflict(ctx, tx, course, originalCode, suffixedCode); err != nil {
		return pgtype.UUID{}, pgtype.Timestamptz{}, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE courses SET legacy_source_code=$1, legacy_code_conflict=true WHERE id=$2`, originalCode, courseID); err != nil {
		return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("mark suffixed course conflict: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("commit suffixed course: %w", err)
	}
	stats.Suffixed++
	stats.Conflicts++
	return courseID, pgtype.Timestamptz{}, true, nil
}

// recordCodeCollisionConflict records that a course code was already in use
// and the legacy course was ingested with a suffixed code.
func (r *FullReconciler) recordCodeCollisionConflict(ctx context.Context, tx pgx.Tx, course normalize.LegacyCourse, originalCode, suffixedCode string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO legacy_sync_conflicts (entity_type, external_id, conflict_type, category, source_payload, local_payload, message, status, resolved_at)
		VALUES ('course', $1, 'code_collision', 'mapping_conflict', $2::jsonb, $3::jsonb, $4, 'ignored', now())
		ON CONFLICT DO NOTHING`, course.LegacyID,
		fmt.Sprintf(`{"code":%q,"legacy_id":%q}`, originalCode, course.LegacyID),
		fmt.Sprintf(`{"suffixed_code":%q}`, suffixedCode),
		fmt.Sprintf("code already owned by legacy course; ingested as %s", suffixedCode))
	if err != nil {
		return fmt.Errorf("record code collision conflict: %w", err)
	}
	return nil
}
