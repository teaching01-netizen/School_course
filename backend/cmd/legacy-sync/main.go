package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/config"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/jobqueue"
	"warwick-institute/internal/legacysync"
	"warwick-institute/internal/legacysync/apply"
	"warwick-institute/internal/legacysync/normalize"
	"warwick-institute/internal/legacysync/outbox"
	"warwick-institute/internal/legacysync/reconcile"
	"warwick-institute/internal/logging"
	"warwick-institute/internal/realtime"
)

// listLinkedLegacyCourses returns the legacy course ids of local courses
// currently linked for sync. Archived courses that have already synced once
// are excluded: they are frozen ("sync once, then skip"), so the leader
// sweep must stop enqueuing refresh jobs for them.
func listLinkedLegacyCourses(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	return listLinkedLegacyCoursesWithCooldown(ctx, pool, legacyCourseRefreshInterval())
}

func legacyCourseRefreshInterval() time.Duration {
	if raw := os.Getenv("LEGACY_SYNC_COURSE_REFRESH_INTERVAL"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return 30 * time.Minute
}

func listLinkedLegacyCoursesWithCooldown(ctx context.Context, pool *pgxpool.Pool, refreshInterval time.Duration) ([]string, error) {
	rows, err := pool.Query(ctx, `SELECT legacy_course_id FROM courses
		WHERE legacy_course_id IS NOT NULL
		  AND NOT (legacy_archived AND legacy_last_synced_at IS NOT NULL)
		  AND (legacy_last_synced_at IS NULL OR legacy_last_synced_at < now() - $1::interval)
		ORDER BY legacy_course_id`, refreshInterval.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// linkedCourse describes the local course currently linked to a legacy
// course id, plus the bookkeeping the sync paths read: the teacher mapping
// for schedule attendance, and the archive stamp + last successful sync for
// the archived "sync once, then skip" rule.
type linkedCourse struct {
	courseID       pgtype.UUID
	teacherID      pgtype.UUID
	legacyArchived bool
	lastSyncedAt   pgtype.Timestamptz
}

// findLinkedLegacyCourse resolves the local course linked to a legacy course
// id. An absent link (no course carries the legacy id — the course was
// deleted or the link was cleared) is reported as found=false so the caller
// can skip it instead of escalating an error.
func findLinkedLegacyCourse(ctx context.Context, pool *pgxpool.Pool, legacyID string) (*linkedCourse, bool, error) {
	linked := &linkedCourse{}
	err := pool.QueryRow(ctx, `SELECT id, teacher_id, legacy_archived, legacy_last_synced_at FROM courses WHERE legacy_course_id = $1`, legacyID).
		Scan(&linked.courseID, &linked.teacherID, &linked.legacyArchived, &linked.lastSyncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return linked, true, nil
}

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("config", "error", err)
		os.Exit(1)
	}
	log := logging.New(cfg.LegacySyncLogLevel)
	if _, err := time.LoadLocation(cfg.InstituteTZ); err != nil {
		log.Error("timezone", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// The client is constructed before the pool: it performs no DB or network
	// I/O (transport, cookie jar, and source client only) and its in-flight
	// ceiling sizes the worker pool and the pgx connection budget below.
	client, err := legacysync.NewClient(cfg.LegacySyncURL, cfg.LegacySyncUsername, cfg.LegacySyncPassword, legacysync.WithMaxBodyBytes(16<<20))
	if err != nil {
		log.Error("legacy client", "error", err)
		os.Exit(1)
	}
	workers := workerConcurrency(client.MaxConcurrent())
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection", "error", err)
		os.Exit(1)
	}
	if envPool := intEnv("LEGACY_SYNC_POOL_MAX_CONNS", 0); envPool > 0 {
		poolConfig.MaxConns = int32(envPool) // explicit env knob wins
	} else if !strings.Contains(cfg.DatabaseURL, "pool_max_conns") {
		// ParseConfig always sets a default MaxConns, so a URL that never
		// tuned pool_max_conns gets the worker-derived budget instead.
		poolConfig.MaxConns = int32(maxPoolConns(0, workers))
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Error("database connection", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	fanout := realtime.NewPostgresFanout(pool, log)
	realtimeHub := realtime.NewHubWithFanout(ctx, workerID(), fanout, log)
	defer realtimeHub.Close()
	outboxPublisher, err := outbox.NewPublisher(pool, realtimeHub, 250*time.Millisecond, log)
	if err != nil {
		log.Error("legacy outbox", "error", err)
		os.Exit(1)
	}
	go func() {
		if err := outboxPublisher.Run(ctx); err != nil && ctx.Err() == nil {
			log.Error("legacy outbox publisher stopped", "error", err)
		}
	}()

	q := sqldb.New(pool)
	leaderConn, err := pool.Acquire(ctx)
	if err != nil {
		log.Error("legacy detector leadership connection", "error", err)
		os.Exit(1)
	}
	defer leaderConn.Release()
	store := jobqueue.NewPostgresStore(q)
	applier := apply.NewScheduleApplier(pool, q, "legacy_warwick")
	courseApplier := apply.NewCourseApplier(pool, q, "legacy_warwick")
	masterData := apply.NewMasterDataService(pool, q, "legacy_warwick")
	fullReconciler := reconcile.NewFullReconciler(pool, q, store, masterData, "legacy_warwick")
	syncer := newCourseSyncer(pool, q, client, masterData, courseApplier, applier, cfg.InstituteTZ, log, client.MaxConcurrent(), legacyCourseRefreshInterval())
	runner, err := legacysync.NewRunner(legacysync.RunnerConfig{
		Store:          store,
		WorkerID:       workerID(),
		SweepEvery:     durationEnv("LEGACY_SYNC_SWEEP_INTERVAL", 30*time.Second),
		Lease:          durationEnv("LEGACY_SYNC_LEASE", 30*time.Second),
		Concurrency:    workers,
		Logger:         log,
		Circuit:        client.CircuitState,
		BudgetExceeded: client.BudgetExceeded,
		ListCourses: func(ctx context.Context) ([]string, error) {
			return listLinkedLegacyCourses(ctx, pool)
		},
		SyncCourse: syncer.syncCourse,
		Controls: func(ctx context.Context) (legacysync.RunnerControls, error) {
			control, err := q.LegacySyncControlGet(ctx)
			if err != nil {
				return legacysync.RunnerControls{}, err
			}
			return legacysync.RunnerControls{DetectionEnabled: control.DetectionEnabled, FetchEnabled: control.FetchEnabled && control.ApplyEnabled}, nil
		},
		Leader: func(ctx context.Context) (bool, error) {
			var leader bool
			if err := leaderConn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('legacy-sync-detector', 0))`).Scan(&leader); err != nil {
				return false, err
			}
			return leader, nil
		},
		ProcessJob: func(ctx context.Context, job jobqueue.Job) (processErr error) {
			if job.JobType != "legacy_full_reconcile" {
				return fmt.Errorf("unsupported legacy job %q", job.JobType)
			}
			run, err := q.SyncRunCreate(ctx, "full_sweep")
			if err != nil {
				return fmt.Errorf("start legacy sync run: %w", err)
			}
			progress := &legacySyncProgressReporter{q: q, runID: run.ID}
			runStartedAt := time.Now()
			var stats reconcile.FullReconcileStats
			profilesApplied := 0
			defer func() {
				completeCtx := context.WithoutCancel(ctx)
				status := "completed"
				phase := "completed"
				lastError := pgtype.Text{}
				if processErr != nil {
					status = "failed"
					phase = "failed"
					lastError = pgtype.Text{String: processErr.Error(), Valid: true}
				}
				if err := progress.update(completeCtx, phase, "", stats.Courses, stats.Courses,
					stats.LinkedByCode+stats.Created,
					stats.Enqueued+stats.RosterStudents+stats.RosterEnrollments+profilesApplied,
					stats.Conflicts, true); err != nil && processErr == nil {
					processErr = fmt.Errorf("save legacy sync progress: %w", err)
				}
				if err := q.SyncRunComplete(completeCtx, sqldb.SyncRunCompleteParams{
					ID:                       run.ID,
					Status:                   status,
					PagesRequested:           2,
					EntitiesParsed:           int32(stats.Courses),
					EntitiesChanged:          int32(stats.LinkedByCode + stats.Created),
					EntitiesApplied:          int32(stats.Enqueued + stats.RosterStudents + stats.RosterEnrollments + profilesApplied),
					ParseFailures:            0,
					ReconciliationMismatches: int32(stats.Conflicts),
					SourceLatencyMs:          pgtype.Int4{Int32: int32(time.Since(runStartedAt).Milliseconds()), Valid: true},
					LastError:                lastError,
				}); err != nil && processErr == nil {
					processErr = fmt.Errorf("complete legacy sync run: %w", err)
				}
			}()
			if err := progress.update(ctx, "fetching_course_index", "Legacy course index", 0, 0, 0, 0, 0, true); err != nil {
				return fmt.Errorf("save legacy sync progress: %w", err)
			}
			// Bypass the course-index cache so a reconcile always works from
			// a fresh observation of the legacy course list.
			result, err := syncer.fetchCourseList(ctx)
			if err != nil {
				return fmt.Errorf("load legacy course index: %w", err)
			}
			if err := progress.update(ctx, "course_index_loaded", "Legacy course index", len(result.Courses), len(result.Courses), 0, 0, 0, true); err != nil {
				return fmt.Errorf("save legacy sync progress: %w", err)
			}
			control, err := q.LegacySyncControlGet(ctx)
			if err != nil {
				return err
			}
			stats, err = fullReconciler.Reconcile(ctx, result.Courses, result.Teachers, result.Subjects, reconcile.FullReconcileOptions{
				ObservedAt:     time.Now().UTC(),
				ShadowMode:     control.ShadowMode,
				StudentEnabled: control.StudentEnabled,
				Concurrency:    reconcileWorkers(client.MaxConcurrent()),
				Progress: func(update reconcile.FullReconcileProgress) error {
					return progress.update(ctx, update.Phase, update.CurrentLegacyID, update.ProcessedEntities, update.TotalEntities, update.ChangedEntities, update.AppliedEntities, update.Failures, false)
				},
			})
			if err != nil {
				return err
			}
			// After roster import, fill the student directory fields (nickname,
			// school, level, year, phone) from the old site's /Admin/Students
			// page. Fill-in-if-empty semantics in the reconciler preserve CRM
			// and human edits. Runs outside shadow mode only, and only when
			// student import is enabled.
			if control.StudentEnabled && !control.ShadowMode {
				profiles, err := syncer.syncStudentProfiles(ctx, func(update StudentProfileProgress) error {
					return progress.update(ctx, "importing_student_profiles", update.CurrentWCode, update.Processed, update.Total, update.ProfilesFound, 0, update.Failures, false)
				})
				if err != nil {
					return fmt.Errorf("sync legacy student profiles: %w", err)
				}
				profilesApplied, err = fullReconciler.ApplyStudentProfiles(ctx, profiles, reconcile.FullReconcileOptions{ObservedAt: time.Now().UTC()})
				if err != nil {
					return fmt.Errorf("apply legacy student profiles: %w", err)
				}
			}
			log.Info("legacy full reconcile complete",
				"courses", stats.Courses,
				"already_linked", stats.AlreadyLinked,
				"linked_by_code", stats.LinkedByCode,
				"created", stats.Created,
				"conflicts", stats.Conflicts,
				"master_data", stats.MasterData,
				"enqueued", stats.Enqueued,
				"roster_students", stats.RosterStudents,
				"roster_enrollments", stats.RosterEnrollments,
				"profiles_applied", profilesApplied,
				"shadow", control.ShadowMode,
				"student_import", control.StudentEnabled,
			)
			return nil
		},
	})
	if err != nil {
		log.Error("runner", "error", err)
		os.Exit(1)
	}

	log.Info("legacy sync service started", "worker_id", workerID(), "sweep_every", durationEnv("LEGACY_SYNC_SWEEP_INTERVAL", 30*time.Second))
	if err := runner.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("legacy sync service stopped", "error", err)
		os.Exit(1)
	}
}

// assignScheduleIDs gives every schedule row a non-empty stable identity for
// upserting. IDs parsed from the source page (data-schedule-id or
// courseScheduleId links) are kept as-is; only rows the page left unidentified
// fall back to the deterministic derived id, because ordinal identity is not
// stable across source insertions.
func assignScheduleIDs(aggregate *normalize.LegacyCourseAggregate, legacyCourseID string) {
	for i := range aggregate.Schedules {
		if aggregate.Schedules[i].LegacyScheduleID == "" {
			aggregate.Schedules[i].LegacyScheduleID = derivedScheduleID(legacyCourseID, i)
		}
	}
}

func derivedScheduleID(courseID string, ordinal int) string {
	value := courseID + "|schedule-row|" + strconv.Itoa(ordinal)
	sum := sha256.Sum256([]byte(value))
	return "derived:" + hex.EncodeToString(sum[:])
}

func workerID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "legacy-sync"
	}
	return host + "-" + strconv.Itoa(os.Getpid())
}

func durationEnv(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func intEnv(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
