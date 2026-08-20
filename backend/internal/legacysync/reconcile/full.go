package reconcile

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	Progress       func(FullReconcileProgress) error
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
	for index, course := range ordered {
		courseID, lastSyncedAt, linked, err := r.linkCourse(ctx, course, opts.ObservedAt, &stats)
		if err != nil {
			return stats, fmt.Errorf("full reconcile: link legacy course %s: %w", course.LegacyID, err)
		}
		if !linked {
			if err := report(FullReconcileProgress{
				Phase:             "reconciling_courses",
				CurrentLegacyID:   course.LegacyID,
				ProcessedEntities: index + 1,
				TotalEntities:     len(ordered),
				ChangedEntities:   stats.LinkedByCode + stats.Created,
				AppliedEntities:   stats.Enqueued,
				Failures:          stats.Conflicts,
			}); err != nil {
				return stats, err
			}
			continue
		}
		if opts.StudentEnabled {
			if err := r.applyRoster(ctx, course, courseID, &stats); err != nil {
				return stats, fmt.Errorf("full reconcile: import roster for legacy course %s: %w", course.LegacyID, err)
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
			return stats, fmt.Errorf("full reconcile: mirror legacy status of course %s: %w", course.LegacyID, err)
		}
		// Archived courses sync once, then stop: an archived course that
		// already had a successful sync needs no further refresh job.
		if course.Status == "archived" && lastSyncedAt.Valid {
			continue
		}
		if _, err := r.store.Enqueue(ctx, jobqueue.EnqueueRequest{
			JobType:     "legacy_refresh_course",
			EntityType:  "course",
			ExternalID:  course.LegacyID,
			UniqueKey:   "legacy:course:" + course.LegacyID,
			Priority:    2,
			RunAfter:    opts.ObservedAt,
			MaxAttempts: 5,
		}); err != nil {
			return stats, fmt.Errorf("full reconcile: enqueue legacy course %s: %w", course.LegacyID, err)
		}
		stats.Enqueued++
		if err := report(FullReconcileProgress{
			Phase:             "reconciling_courses",
			CurrentLegacyID:   course.LegacyID,
			ProcessedEntities: index + 1,
			TotalEntities:     len(ordered),
			ChangedEntities:   stats.LinkedByCode + stats.Created,
			AppliedEntities:   stats.Enqueued,
			Failures:          stats.Conflicts,
		}); err != nil {
			return stats, err
		}
	}
	if err := r.resolveCodeClaimConflicts(ctx); err != nil {
		return stats, err
	}
	return stats, nil
}

// resolveCodeClaimConflicts closes code_claimed conflicts whose legacy
// course has since become linked (by any means): the mapping collision that
// justified the open row no longer holds. Runs once per reconcile pass so
// repeated runs converge the conflict list instead of accumulating it.
func (r *FullReconciler) resolveCodeClaimConflicts(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, `
		UPDATE legacy_sync_conflicts c
		SET status = 'resolved', resolved_at = now()
		WHERE c.status = 'open' AND c.conflict_type = 'code_claimed'
		  AND EXISTS (SELECT 1 FROM courses x WHERE x.legacy_course_id = c.external_id)
	`); err != nil {
		return fmt.Errorf("resolve code claim conflicts: %w", err)
	}
	return nil
}

// recordCodeClaimConflict records that course.Code's local course is already
// linked to another legacy course (claimedBy may be nil when the winner's
// link was cleared concurrently). Deduplicated per open conflict: repeated
// reconcile passes must not accumulate duplicate rows for the same legacy
// course (an already-open row is left as-is).
func (r *FullReconciler) recordCodeClaimConflict(ctx context.Context, tx pgx.Tx, course normalize.LegacyCourse, claimedBy *string) error {
	winner := "unknown"
	if claimedBy != nil {
		winner = *claimedBy
	}
	_, err := r.q.WithTx(tx).ConflictInsert(ctx, sqldb.ConflictInsertParams{
		EntityType:    "course",
		ExternalID:    course.LegacyID,
		ConflictType:  "code_claimed",
		Category:      "mapping_conflict",
		SourcePayload: fmt.Sprintf(`{"code":%q,"claimed_by_legacy_id":%q}`, course.Code, winner),
		LocalPayload:  fmt.Sprintf(`{"legacy_course_id":%q}`, winner),
		Message:       pgtype.Text{String: fmt.Sprintf("local course code %s is already linked to legacy course %s", course.Code, winner), Valid: true},
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("record link conflict: %w", err)
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
			// course: report a conflict instead of success so the loser's
			// refresh job is not enqueued against a course it does not own.
			if err := tx.QueryRow(ctx, `SELECT legacy_course_id FROM courses WHERE id = $1`, localID).Scan(&claimedBy); err != nil {
				return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("read concurrent claim winner: %w", err)
			}
			if err := r.recordCodeClaimConflict(ctx, tx, course, claimedBy); err != nil {
				return pgtype.UUID{}, pgtype.Timestamptz{}, false, err
			}
			if err := tx.Commit(ctx); err != nil {
				return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("commit lost claim: %w", err)
			}
			stats.Conflicts++
			return pgtype.UUID{}, pgtype.Timestamptz{}, false, nil
		}
		if err := tx.Commit(ctx); err != nil {
			return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("commit claim: %w", err)
		}
		stats.LinkedByCode++
		return localID, pgtype.Timestamptz{}, true, nil
	case err == nil:
		if err := r.recordCodeClaimConflict(ctx, tx, course, claimedBy); err != nil {
			return pgtype.UUID{}, pgtype.Timestamptz{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return pgtype.UUID{}, pgtype.Timestamptz{}, false, fmt.Errorf("commit conflict: %w", err)
		}
		stats.Conflicts++
		return pgtype.UUID{}, pgtype.Timestamptz{}, false, nil
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
