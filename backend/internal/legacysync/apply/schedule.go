package apply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/legacysync/normalize"
	"warwick-institute/internal/schedulelock"
	"warwick-institute/internal/schedulepolicy"
)

var (
	ErrMissingScheduleIdentity   = errors.New("legacy schedule: stable legacy schedule ID is required")
	ErrDuplicateScheduleIdentity = errors.New("legacy schedule: duplicate legacy schedule ID")
)

type ScheduleApplyRequest struct {
	CourseID        pgtype.UUID
	LegacyCourseID  string
	TeacherID       pgtype.UUID
	Aggregate       normalize.LegacyCourseAggregate
	ObservedAt      time.Time
	InstituteTZ     string
	ShadowMode      bool
	RealtimeEnabled bool
	allowConflicts  bool
}

type ScheduleApplyResult struct {
	SourceHash string
	Changed    bool
	Sessions   int
	// SkippedSessions counts schedule rows rejected by exclusion constraints
	// (legacy double-booked a room or teacher). The rows are recorded in
	// legacy_sync_conflicts and the rest of the course still syncs.
	SkippedSessions int
	AppliedAt       time.Time
}

// ScheduleApplier materializes legacy-site schedules into the local domain.
//
// It deliberately upserts sessions directly instead of routing through
// internal/scheduling: legacy sync is a mirror-replication pipeline over
// source-of-truth data (ON CONFLICT (legacy_schedule_id) upserts under a
// per-course advisory lock), not a user-driven scheduling write. Sessions are
// created under a source_kind='legacy' series with
// materialization_mode='external', keeping them separate from natively
// scheduled sessions. Changes that cannot be applied are recorded in the
// legacy_sync_conflicts table for admin review (/api/v1/admin/legacy-sync/
// conflicts); the inline explainable-conflict error shape documented in
// docs/code-quality.md is reserved for interactive scheduling endpoints.
type ScheduleApplier struct {
	pool   *pgxpool.Pool
	q      *sqldb.Queries
	source string
	policy schedulepolicy.Reader
	fault  FaultPoint
}

func NewScheduleApplier(pool *pgxpool.Pool, q *sqldb.Queries, source string, policy schedulepolicy.Reader) *ScheduleApplier {
	return &ScheduleApplier{pool: pool, q: q, source: source, policy: policy}
}

func ValidateScheduleAggregate(aggregate normalize.LegacyCourseAggregate) error {
	seen := make(map[string]struct{}, len(aggregate.Schedules))
	for _, schedule := range aggregate.Schedules {
		if schedule.LegacyScheduleID == "" {
			return ErrMissingScheduleIdentity
		}
		if _, exists := seen[schedule.LegacyScheduleID]; exists {
			return ErrDuplicateScheduleIdentity
		}
		seen[schedule.LegacyScheduleID] = struct{}{}
		date, err := parseSourceDate(schedule.Date)
		if err != nil {
			return fmt.Errorf("legacy schedule %s date: %w", schedule.LegacyScheduleID, err)
		}
		if _, _, err := normalize.SessionWindow(date, schedule.Begin, schedule.End, time.UTC); err != nil {
			return fmt.Errorf("legacy schedule %s time: %w", schedule.LegacyScheduleID, err)
		}
	}
	return nil
}

func ScheduleHash(schedule normalize.LegacySchedule) (string, error) {
	return normalize.HashCanonical(schedule)
}

func (a *ScheduleApplier) Apply(ctx context.Context, request ScheduleApplyRequest) (ScheduleApplyResult, error) {
	if err := ValidateScheduleAggregate(request.Aggregate); err != nil {
		return ScheduleApplyResult{}, err
	}
	if request.InstituteTZ == "" {
		request.InstituteTZ = "Asia/Bangkok"
	}
	loc, err := time.LoadLocation(request.InstituteTZ)
	if err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("load institute timezone: %w", err)
	}
	canonical, err := normalize.CanonicalJSON(request.Aggregate)
	if err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("canonicalize course aggregate: %w", err)
	}
	sourceHash, err := normalize.HashCanonical(request.Aggregate)
	if err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("hash course aggregate: %w", err)
	}
	if request.ObservedAt.IsZero() {
		return ScheduleApplyResult{}, errors.New("legacy schedule: observed time is required")
	}
	externalCourseID := request.LegacyCourseID
	if externalCourseID == "" {
		externalCourseID = uuidText(request.CourseID)
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("begin schedule apply: %w", err)
	}
	defer tx.Rollback(ctx)
	lockKey := request.LegacyCourseID
	if lockKey == "" {
		lockKey = uuidText(request.CourseID)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, legacyCourseLockKey(a.source, lockKey)); err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("lock legacy course: %w", err)
	}
	var currentTeacherID pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT teacher_id FROM courses WHERE id=$1`, request.CourseID).Scan(&currentTeacherID); err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("load current legacy course teacher: %w", err)
	}
	// The course's stored teacher is authoritative once set (mirrored by the
	// course apply); until then the request teacher from the aggregate is used.
	if currentTeacherID.Valid {
		request.TeacherID = currentTeacherID
	}
	qtx := a.q.WithTx(tx)
	if a.policy == nil {
		return ScheduleApplyResult{}, errors.New("legacy schedule: policy reader is required")
	}
	policy, err := a.policy.Load(ctx, tx)
	if err != nil {
		return ScheduleApplyResult{}, err
	}
	request.allowConflicts = !policy.Enforced(schedulepolicy.ScopeLegacySync)
	previous, err := qtx.SnapshotGet(ctx, sqldb.SnapshotGetParams{Source: a.source, EntityType: "course", ExternalID: externalCourseID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ScheduleApplyResult{}, fmt.Errorf("load course snapshot: %w", err)
	}
	// The unchanged-hash fast path may only fire when the previous run
	// applied the whole aggregate; a partial snapshot (rows skipped by
	// exclusion conflicts) must stay retryable after the local blocker is
	// resolved.
	if err == nil && previous.SourceHash == sourceHash && previous.Quality == "ok" {
		// Even an unchanged aggregate converges local drift: sessions that
		// were locally soft-deleted while still present in the source are
		// restored (the source is the contract).
		skipped := 0
		if !request.ShadowMode {
			skipped, err = a.restoreSourcePresentSessions(ctx, tx, qtx, request, loc)
			if err != nil {
				return ScheduleApplyResult{}, err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE courses SET legacy_last_seen_at=$1, legacy_last_synced_at=$1 WHERE id=$2`, request.ObservedAt, request.CourseID); err != nil {
			return ScheduleApplyResult{}, fmt.Errorf("update unchanged schedule metadata: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE external_refs SET last_seen_at=$1 WHERE source=$2 AND entity_type='course' AND external_id=$3`, request.ObservedAt, a.source, externalCourseID); err != nil {
			return ScheduleApplyResult{}, fmt.Errorf("update unchanged schedule mapping: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return ScheduleApplyResult{}, fmt.Errorf("commit unchanged schedule: %w", err)
		}
		return ScheduleApplyResult{SourceHash: sourceHash, SkippedSessions: skipped, AppliedAt: request.ObservedAt}, nil
	}
	if _, err := qtx.ExternalRefUpsert(ctx, sqldb.ExternalRefUpsertParams{Source: a.source, EntityType: "course", ExternalID: externalCourseID, InternalID: request.CourseID, SourceHash: pgtype.Text{String: sourceHash, Valid: true}}); err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("upsert course mapping: %w", err)
	}
	skipped := 0
	if !request.ShadowMode {
		skipped, err = a.applyDomain(ctx, tx, qtx, request, loc, sourceHash)
		if err != nil {
			return ScheduleApplyResult{}, err
		}
	}
	// The snapshot is stamped only after the domain application so it can
	// never mark a partially applied aggregate as fully synchronized.
	quality := "ok"
	if skipped > 0 {
		quality = "partial"
	}
	// A shadow run must never stamp a snapshot: it did not apply the
	// aggregate, so an 'ok' snapshot here would make a later non-shadow run
	// with the same hash take the unchanged-hash fast path and skip the
	// apply entirely.
	if !request.ShadowMode {
		if _, err := qtx.SnapshotUpsert(ctx, sqldb.SnapshotUpsertParams{Source: a.source, EntityType: "course", ExternalID: externalCourseID, CanonicalData: string(canonical), SourceHash: sourceHash, ParserVersion: 1, ObservedAt: timestamp(request.ObservedAt), Quality: quality}); err != nil {
			return ScheduleApplyResult{}, fmt.Errorf("store course snapshot: %w", err)
		}
	}
	if request.RealtimeEnabled && !request.ShadowMode {
		payload := fmt.Sprintf(`{"synced_at":%q}`, request.ObservedAt.UTC().Format(time.RFC3339Nano))
		if _, err := qtx.OutboxInsert(ctx, sqldb.OutboxInsertParams{SourceEventKey: "legacy:course:" + externalCourseID + ":" + sourceHash, EventType: "legacy.course.updated", Channel: "course:" + uuidText(request.CourseID), EntityType: pgtype.Text{String: "course", Valid: true}, ExternalID: pgtype.Text{String: externalCourseID, Valid: true}, Payload: payload}); err != nil {
			return ScheduleApplyResult{}, fmt.Errorf("write legacy outbox: %w", err)
		}
	}
	if err := a.hitFault("before_commit"); err != nil {
		return ScheduleApplyResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("commit schedule apply: %w", err)
	}
	return ScheduleApplyResult{SourceHash: sourceHash, Changed: true, Sessions: len(request.Aggregate.Schedules), SkippedSessions: skipped, AppliedAt: request.ObservedAt}, nil
}

func (a *ScheduleApplier) applyDomain(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, request ScheduleApplyRequest, loc *time.Location, sourceHash string) (int, error) {
	if err := a.lockScheduleResources(ctx, tx, qtx, request); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE courses SET legacy_source_hash=$1, legacy_last_seen_at=$2, legacy_last_synced_at=$2, source_kind='legacy', updated_at=now() WHERE id=$3`, sourceHash, request.ObservedAt, request.CourseID); err != nil {
		return 0, fmt.Errorf("update legacy course metadata: %w", err)
	}
	if err := a.hitFault("after_course_metadata_upsert"); err != nil {
		return 0, err
	}
	skipped := 0
	if len(request.Aggregate.Schedules) > 0 {
		firstRoomID, err := a.resolveRoomID(ctx, tx, request.Aggregate.Schedules[0].ClassroomLegacyID)
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `SAVEPOINT external_series_upsert`); err != nil {
			return 0, fmt.Errorf("savepoint external series: %w", err)
		}
		seriesID, err := a.ensureExternalSeries(ctx, tx, request, loc, firstRoomID)
		if err != nil {
			if isNotNullViolation(err) && !request.TeacherID.Valid {
				if _, rbErr := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT external_series_upsert`); rbErr != nil {
					return 0, fmt.Errorf("rollback savepoint external series: %w", rbErr)
				}
				if _, cErr := qtx.ConflictInsert(ctx, sqldb.ConflictInsertParams{
					EntityType:    "course",
					ExternalID:    request.LegacyCourseID,
					ConflictType:  "missing_reference:teacher",
					Category:      "missing_reference",
					SourcePayload: `{"reference_type":"teacher","series_teacher_required":true}`,
					LocalPayload:  "{}",
					Message:       pgtype.Text{String: fmt.Sprintf("course %s has no teacher so legacy sessions synced without an external series", request.LegacyCourseID), Valid: true},
				}); cErr != nil && !errors.Is(cErr, pgx.ErrNoRows) {
					return 0, fmt.Errorf("record teacherless series conflict: %w", cErr)
				}
				if _, ignoreErr := tx.Exec(ctx, `UPDATE legacy_sync_conflicts SET status='ignored', resolved_at=now() WHERE entity_type='course' AND external_id=$1 AND conflict_type='missing_reference:teacher' AND status='open'`, request.LegacyCourseID); ignoreErr != nil {
					return 0, fmt.Errorf("close teacherless series conflict: %w", ignoreErr)
				}
				seriesID = pgtype.UUID{}
			} else {
				return 0, err
			}
		} else {
			if _, err := tx.Exec(ctx, `RELEASE SAVEPOINT external_series_upsert`); err != nil {
				return 0, fmt.Errorf("release savepoint external series: %w", err)
			}
		}
		if err := a.hitFault("after_series_upsert"); err != nil {
			return 0, err
		}
		for index, schedule := range request.Aggregate.Schedules {
			date, err := parseSourceDate(schedule.Date)
			if err != nil {
				return skipped, fmt.Errorf("parse legacy schedule %s date: %w", schedule.LegacyScheduleID, err)
			}
			start, end, err := normalize.SessionWindow(date, schedule.Begin, schedule.End, loc)
			if err != nil {
				return skipped, fmt.Errorf("parse legacy schedule %s time: %w", schedule.LegacyScheduleID, err)
			}
			scheduleHash, err := ScheduleHash(schedule)
			if err != nil {
				return skipped, fmt.Errorf("hash legacy schedule %s: %w", schedule.LegacyScheduleID, err)
			}
			roomID := firstRoomID
			if index > 0 {
				roomID, err = a.resolveRoomID(ctx, tx, schedule.ClassroomLegacyID)
				if err != nil {
					return skipped, err
				}
			}
			if err := a.migrateDerivedScheduleIdentity(ctx, tx, request, schedule.LegacyScheduleID, roomID, start, end); err != nil {
				return skipped, err
			}
			nativeSessionID, err := a.findActiveNativeSession(ctx, qtx, request.CourseID, request.TeacherID, roomID, start, end)
			if err != nil {
				return skipped, err
			}
			if nativeSessionID.Valid {
				if err := a.linkNativeSchedule(ctx, tx, qtx, request, schedule, scheduleHash, nativeSessionID, roomID, start, end); err != nil {
					return skipped, err
				}
				if err := a.hitFault("after_schedule_mapping_upsert"); err != nil {
					return skipped, err
				}
				continue
			}
			forceOverride := false
			conflict, err := a.strictScheduleConflict(ctx, qtx, request, schedule.LegacyScheduleID, roomID, start, end)
			if err != nil {
				return skipped, err
			}
			if conflict != nil {
				if !request.allowConflicts {
					if err := a.recordScheduleConflict(ctx, tx, request, schedule, scheduleHash, conflict); err != nil {
						return skipped, err
					}
					skipped++
					continue
				}
				forceOverride = true
			}
			if _, err := tx.Exec(ctx, `SAVEPOINT legacy_schedule_upsert`); err != nil {
				return skipped, fmt.Errorf("savepoint legacy schedule %s: %w", schedule.LegacyScheduleID, err)
			}
			var sessionID pgtype.UUID
			upsertErr := tx.QueryRow(ctx, `INSERT INTO sessions (series_id, course_id, room_id, teacher_id, start_at, end_at, legacy_schedule_id, legacy_confirmed, legacy_confirmed_by, legacy_source_hash, legacy_last_synced_at, legacy_last_seen_at, source_kind, legacy_conflict_override) VALUES ($1,$2,NULLIF($3::text,'')::uuid,$4,$5,$6,$7,$8,NULLIF($9::text,''),$10,$11,$11,'legacy',$12) ON CONFLICT (legacy_schedule_id) WHERE legacy_schedule_id IS NOT NULL DO UPDATE SET series_id=EXCLUDED.series_id, course_id=EXCLUDED.course_id, room_id=EXCLUDED.room_id, teacher_id=EXCLUDED.teacher_id, start_at=EXCLUDED.start_at, end_at=EXCLUDED.end_at, deleted_at=NULL, legacy_confirmed=EXCLUDED.legacy_confirmed, legacy_confirmed_by=EXCLUDED.legacy_confirmed_by, legacy_source_hash=EXCLUDED.legacy_source_hash, legacy_last_synced_at=EXCLUDED.legacy_last_synced_at, legacy_last_seen_at=EXCLUDED.legacy_last_seen_at, source_kind='legacy', legacy_conflict_override=EXCLUDED.legacy_conflict_override, updated_at=now(), version=sessions.version+1 RETURNING id`, seriesID, request.CourseID, uuidText(roomID), request.TeacherID, start, end, schedule.LegacyScheduleID, schedule.Confirmed, schedule.ConfirmedBy, scheduleHash, request.ObservedAt, forceOverride).Scan(&sessionID)
			switch {
			case upsertErr == nil:
				if _, err := tx.Exec(ctx, `RELEASE SAVEPOINT legacy_schedule_upsert`); err != nil {
					return skipped, fmt.Errorf("release savepoint legacy schedule %s: %w", schedule.LegacyScheduleID, err)
				}
			case (isExclusionViolation(upsertErr) || isAvailabilityViolation(upsertErr)) && request.allowConflicts:
				if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT legacy_schedule_upsert`); err != nil {
					return skipped, fmt.Errorf("rollback savepoint legacy schedule %s: %w", schedule.LegacyScheduleID, err)
				}
				if err := tx.QueryRow(ctx, `
					INSERT INTO sessions (series_id, course_id, room_id, teacher_id, start_at, end_at, legacy_schedule_id,
						legacy_confirmed, legacy_confirmed_by, legacy_source_hash, legacy_last_synced_at, legacy_last_seen_at,
						source_kind, legacy_conflict_override)
					VALUES ($1,$2,NULLIF($3::text,'')::uuid,$4,$5,$6,$7,$8,NULLIF($9::text,''),$10,$11,$11,'legacy',true)
					ON CONFLICT (legacy_schedule_id) WHERE legacy_schedule_id IS NOT NULL DO UPDATE SET
						series_id=EXCLUDED.series_id, course_id=EXCLUDED.course_id, room_id=EXCLUDED.room_id,
						teacher_id=EXCLUDED.teacher_id, start_at=EXCLUDED.start_at, end_at=EXCLUDED.end_at,
						deleted_at=NULL, legacy_confirmed=EXCLUDED.legacy_confirmed, legacy_confirmed_by=EXCLUDED.legacy_confirmed_by,
						legacy_source_hash=EXCLUDED.legacy_source_hash, legacy_last_synced_at=EXCLUDED.legacy_last_synced_at,
						legacy_last_seen_at=EXCLUDED.legacy_last_seen_at, source_kind='legacy', legacy_conflict_override=true,
						updated_at=now(), version=sessions.version+1 RETURNING id`, seriesID, request.CourseID, uuidText(roomID), request.TeacherID,
					start, end, schedule.LegacyScheduleID, schedule.Confirmed, schedule.ConfirmedBy, scheduleHash, request.ObservedAt).Scan(&sessionID); err != nil {
					return skipped, fmt.Errorf("materialize conflicting legacy schedule %s: %w", schedule.LegacyScheduleID, err)
				}
				if err := a.resolveScheduleConflict(ctx, tx, request, schedule.LegacyScheduleID); err != nil {
					return skipped, err
				}
				upsertErr = nil
			case isExclusionViolation(upsertErr) || isAvailabilityViolation(upsertErr):
				if err := a.recordScheduleConflict(ctx, tx, request, schedule, scheduleHash, upsertErr); err != nil {
					return skipped, err
				}
				skipped++
				continue
			default:
				return skipped, fmt.Errorf("upsert legacy schedule %s: %w", schedule.LegacyScheduleID, upsertErr)
			}
			if err := a.hitFault("after_session_upsert"); err != nil {
				return skipped, err
			}
			if _, err := qtx.ExternalRefUpsert(ctx, sqldb.ExternalRefUpsertParams{Source: a.source, EntityType: "schedule", ExternalID: schedule.LegacyScheduleID, InternalID: sessionID, SourceHash: pgtype.Text{String: scheduleHash, Valid: true}}); err != nil {
				return skipped, fmt.Errorf("upsert schedule mapping %s: %w", schedule.LegacyScheduleID, err)
			}
			if err := a.resolveScheduleConflict(ctx, tx, request, schedule.LegacyScheduleID); err != nil {
				return skipped, err
			}
			if err := a.hitFault("after_schedule_mapping_upsert"); err != nil {
				return skipped, err
			}
		}
	}
	// The source set is authoritative: sessions for schedule rows that no
	// longer exist upstream are soft-deleted here (history preserved), never
	// left active locally.
	if err := a.deactivateMissingSchedules(ctx, tx, request); err != nil {
		return skipped, err
	}
	return skipped, nil
}

func (a *ScheduleApplier) migrateDerivedScheduleIdentity(ctx context.Context, tx pgx.Tx, request ScheduleApplyRequest, scheduleID string, roomID pgtype.UUID, start, end time.Time) error {
	if strings.HasPrefix(scheduleID, "derived:") {
		return nil
	}
	var previousID string
	var sessionID pgtype.UUID
	err := tx.QueryRow(ctx, `
		WITH candidates AS MATERIALIZED (
			SELECT id, legacy_schedule_id
			FROM sessions
			WHERE course_id=$1 AND teacher_id IS NOT DISTINCT FROM $2
			  AND start_at=$3 AND end_at=$4 AND source_kind='legacy' AND deleted_at IS NULL
			  AND legacy_schedule_id LIKE 'derived:%'
			  AND (room_id IS NULL OR room_id IS NOT DISTINCT FROM NULLIF($5::text, '')::uuid)
			FOR UPDATE
		), migrated AS (
			UPDATE sessions AS target
			SET legacy_schedule_id=$6, updated_at=now(), version=target.version+1
			FROM candidates AS candidate
			WHERE target.id=candidate.id AND (SELECT count(*) FROM candidates)=1
			  AND NOT EXISTS (SELECT 1 FROM sessions WHERE legacy_schedule_id=$6)
			RETURNING candidate.legacy_schedule_id, target.id
		)
		SELECT legacy_schedule_id, id FROM migrated`, request.CourseID, request.TeacherID, start, end, uuidText(roomID), scheduleID).Scan(&previousID, &sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("migrate derived legacy schedule identity %s: %w", scheduleID, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM external_refs WHERE source=$1 AND entity_type='schedule' AND external_id=$2 AND internal_id=$3`, a.source, previousID, sessionID); err != nil {
		return fmt.Errorf("remove derived schedule mapping %s: %w", previousID, err)
	}
	return nil
}

// restoreSourcePresentSessions un-soft-deletes local legacy sessions for
// schedule rows the source still carries, and reactivates their external
// mappings. Generic series cancellation can soft-delete sessions; without
// this, source-present rows would stay missing locally forever.
func (a *ScheduleApplier) restoreSourcePresentSessions(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, request ScheduleApplyRequest, loc *time.Location) (int, error) {
	incoming := make([]string, 0, len(request.Aggregate.Schedules))
	for _, schedule := range request.Aggregate.Schedules {
		incoming = append(incoming, schedule.LegacyScheduleID)
	}
	if len(incoming) == 0 {
		return 0, nil
	}

	if err := a.lockScheduleResources(ctx, tx, qtx, request); err != nil {
		return 0, err
	}
	skipped := make([]string, 0)
	allowed := make([]string, 0)
	for _, schedule := range request.Aggregate.Schedules {
		date, err := parseSourceDate(schedule.Date)
		if err != nil {
			return 0, fmt.Errorf("parse legacy schedule %s date: %w", schedule.LegacyScheduleID, err)
		}
		start, end, err := normalize.SessionWindow(date, schedule.Begin, schedule.End, loc)
		if err != nil {
			return 0, fmt.Errorf("parse legacy schedule %s time: %w", schedule.LegacyScheduleID, err)
		}
		roomID, err := a.resolveRoomID(ctx, tx, schedule.ClassroomLegacyID)
		if err != nil {
			return 0, err
		}
		scheduleHash, err := ScheduleHash(schedule)
		if err != nil {
			return 0, fmt.Errorf("hash legacy schedule %s: %w", schedule.LegacyScheduleID, err)
		}
		nativeSessionID, err := a.findActiveNativeSession(ctx, qtx, request.CourseID, request.TeacherID, roomID, start, end)
		if err != nil {
			return 0, err
		}
		if nativeSessionID.Valid {
			if err := a.linkNativeSchedule(ctx, tx, qtx, request, schedule, scheduleHash, nativeSessionID, roomID, start, end); err != nil {
				return 0, err
			}
			continue
		}
		conflict, err := a.strictScheduleConflict(ctx, qtx, request, schedule.LegacyScheduleID, roomID, start, end)
		if err != nil {
			return 0, err
		}
		if conflict == nil {
			continue
		}
		if !request.allowConflicts {
			if err := a.recordScheduleConflict(ctx, tx, request, schedule, scheduleHash, conflict); err != nil {
				return 0, err
			}
			skipped = append(skipped, schedule.LegacyScheduleID)
			continue
		}
		allowed = append(allowed, schedule.LegacyScheduleID)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE sessions
		SET deleted_at = NULL, legacy_conflict_override = false, updated_at = now(), version = sessions.version + 1
		WHERE course_id = $1 AND source_kind = 'legacy' AND legacy_schedule_id = ANY($2::text[])
		  AND NOT (legacy_schedule_id = ANY($3::text[]))
		  AND NOT (legacy_schedule_id = ANY($4::text[]))
		  AND (deleted_at IS NOT NULL OR legacy_conflict_override)`, request.CourseID, incoming, skipped, allowed); err != nil {
		return 0, fmt.Errorf("restore locally deleted legacy sessions: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE sessions
		SET deleted_at = NULL, legacy_conflict_override = true, updated_at = now(), version = sessions.version + 1
		WHERE course_id = $1 AND source_kind = 'legacy' AND legacy_schedule_id = ANY($2::text[])
		  AND (deleted_at IS NOT NULL OR NOT legacy_conflict_override)`, request.CourseID, allowed); err != nil {
		return 0, fmt.Errorf("restore allowed legacy conflicts: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE external_refs SET state='active'
		WHERE source=$1 AND entity_type='schedule' AND external_id = ANY($2::text[])
		  AND state IN ('tombstoned','suspected_missing','confirmed_missing')`, a.source, appendWithoutSkipped(incoming, skipped)); err != nil {
		return 0, fmt.Errorf("reactivate restored schedule mappings: %w", err)
	}
	return len(skipped), nil
}

func appendWithoutSkipped(incoming, skipped []string) []string {
	if len(skipped) == 0 {
		return incoming
	}
	skippedSet := make(map[string]struct{}, len(skipped))
	for _, id := range skipped {
		skippedSet[id] = struct{}{}
	}
	result := make([]string, 0, len(incoming)-len(skipped))
	for _, id := range incoming {
		if _, ok := skippedSet[id]; !ok {
			result = append(result, id)
		}
	}
	return result
}

func (a *ScheduleApplier) findActiveNativeSession(ctx context.Context, qtx *sqldb.Queries, courseID, teacherID, roomID pgtype.UUID, start, end time.Time) (pgtype.UUID, error) {
	sessionID, err := qtx.SessionFindActiveNativeExact(ctx, sqldb.SessionFindActiveNativeExactParams{
		CourseID:  courseID,
		TeacherID: teacherID,
		RoomID:    roomID,
		StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: end, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, nil
	}
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("find exact native schedule: %w", err)
	}
	return sessionID, nil
}

func (a *ScheduleApplier) linkNativeSchedule(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, request ScheduleApplyRequest, schedule normalize.LegacySchedule, scheduleHash string, nativeSessionID, roomID pgtype.UUID, start, end time.Time) error {
	if _, err := tx.Exec(ctx, `
		WITH retired AS (
			UPDATE sessions
			SET deleted_at = now(), updated_at = now(), version = sessions.version + 1
			WHERE deleted_at IS NULL
			  AND source_kind = 'legacy'
			  AND course_id = $2
			  AND id <> $1
			  AND (
				legacy_schedule_id = $3
				OR (
					teacher_id = $4
					AND room_id IS NOT DISTINCT FROM $5
					AND start_at = $6
					AND end_at = $7
				)
			  )
			RETURNING legacy_schedule_id
		)
		UPDATE external_refs AS refs
		SET state = 'tombstoned'
		FROM retired
		WHERE refs.source = $8
		  AND refs.entity_type = 'schedule'
		  AND refs.external_id = retired.legacy_schedule_id
		  AND retired.legacy_schedule_id IS NOT NULL
	`, nativeSessionID, request.CourseID, schedule.LegacyScheduleID, request.TeacherID, roomID, start, end, a.source); err != nil {
		return fmt.Errorf("retire duplicate legacy schedule %s: %w", schedule.LegacyScheduleID, err)
	}
	if _, err := qtx.ExternalRefUpsert(ctx, sqldb.ExternalRefUpsertParams{
		Source:     a.source,
		EntityType: "schedule",
		ExternalID: schedule.LegacyScheduleID,
		InternalID: nativeSessionID,
		SourceHash: pgtype.Text{String: scheduleHash, Valid: true},
	}); err != nil {
		return fmt.Errorf("map legacy schedule %s to native session: %w", schedule.LegacyScheduleID, err)
	}
	if err := a.resolveScheduleConflict(ctx, tx, request, schedule.LegacyScheduleID); err != nil {
		return err
	}
	return nil
}

func (a *ScheduleApplier) lockScheduleResources(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, request ScheduleApplyRequest) error {
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{CourseIDs: []pgtype.UUID{request.CourseID}}); err != nil {
		return fmt.Errorf("lock legacy course resources: %w", err)
	}
	students, err := qtx.CourseStudentsList(ctx, request.CourseID)
	if err != nil {
		return fmt.Errorf("list legacy course students: %w", err)
	}
	studentIDs := make([]pgtype.UUID, 0, len(students))
	for _, student := range students {
		studentIDs = append(studentIDs, student.StudentID)
	}
	existing, err := qtx.SessionListActiveByCourse(ctx, request.CourseID)
	if err != nil {
		return fmt.Errorf("list legacy course sessions: %w", err)
	}
	sessionIDs := make([]pgtype.UUID, 0, len(existing))
	roomIDs := make([]pgtype.UUID, 0, len(request.Aggregate.Schedules))
	for _, session := range existing {
		sessionIDs = append(sessionIDs, session.ID)
		roomIDs = append(roomIDs, session.RoomID)
	}
	for _, schedule := range request.Aggregate.Schedules {
		roomID, err := a.resolveRoomID(ctx, tx, schedule.ClassroomLegacyID)
		if err != nil {
			return err
		}
		roomIDs = append(roomIDs, roomID)
	}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{
		CourseIDs:  []pgtype.UUID{request.CourseID},
		StudentIDs: studentIDs,
		TeacherIDs: []pgtype.UUID{request.TeacherID},
		RoomIDs:    roomIDs,
		SessionIDs: sessionIDs,
	}); err != nil {
		return fmt.Errorf("lock legacy schedule resources: %w", err)
	}
	return nil
}

func (a *ScheduleApplier) strictScheduleConflict(ctx context.Context, qtx *sqldb.Queries, request ScheduleApplyRequest, scheduleID string, roomID pgtype.UUID, start, end time.Time) (error, error) {
	startAt := pgtype.Timestamptz{Time: start, Valid: true}
	endAt := pgtype.Timestamptz{Time: end, Valid: true}
	if request.TeacherID.Valid {
		availability, err := qtx.CheckTeacherAvailability(ctx, sqldb.CheckTeacherAvailabilityParams{TeacherID: request.TeacherID, Column2: startAt, Column3: endAt})
		if err != nil {
			return nil, fmt.Errorf("check legacy teacher availability: %w", err)
		}
		if availability.HasWindows && !availability.IsAvailable {
			return &pgconn.PgError{Code: "23514", Message: "teacher not available for requested time"}, nil
		}
	}
	if roomID.Valid {
		availability, err := qtx.CheckRoomAvailability(ctx, sqldb.CheckRoomAvailabilityParams{RoomID: roomID, Column2: startAt, Column3: endAt})
		if err != nil {
			return nil, fmt.Errorf("check legacy room availability: %w", err)
		}
		if availability.HasWindows && !availability.IsAvailable {
			return &pgconn.PgError{Code: "23514", Message: "room not available for requested time"}, nil
		}
	}
	var exists bool
	if request.TeacherID.Valid {
		if err := qtx.DBTX().QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM sessions
				WHERE deleted_at IS NULL AND teacher_id = $1
				  AND time_range && tstzrange($2, $3, '[)')
				  AND legacy_schedule_id IS DISTINCT FROM $4::text
			)`, request.TeacherID, start, end, scheduleID).Scan(&exists); err != nil {
			return nil, fmt.Errorf("check legacy teacher overlap: %w", err)
		}
		if exists {
			return &pgconn.PgError{Code: "23P01", ConstraintName: "sessions_no_teacher_overlap", Message: "teacher schedule overlap"}, nil
		}
	}
	if roomID.Valid {
		if err := qtx.DBTX().QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM sessions
				WHERE deleted_at IS NULL AND room_id = $1
				  AND time_range && tstzrange($2, $3, '[)')
				  AND legacy_schedule_id IS DISTINCT FROM $4::text
			)`, roomID, start, end, scheduleID).Scan(&exists); err != nil {
			return nil, fmt.Errorf("check legacy room overlap: %w", err)
		}
		if exists {
			return &pgconn.PgError{Code: "23P01", ConstraintName: "sessions_no_room_overlap", Message: "room schedule overlap"}, nil
		}
	}
	if err := qtx.DBTX().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM student_busy_ranges br
			JOIN sessions other ON other.id = br.session_id
			JOIN course_students cs ON cs.student_id = br.student_id AND cs.course_id = $1
			WHERE br.deleted_at IS NULL AND other.deleted_at IS NULL
			  AND br.time_range && tstzrange($2, $3, '[)')
			  AND other.legacy_schedule_id IS DISTINCT FROM $4::text
		)`, request.CourseID, start, end, scheduleID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check legacy student overlap: %w", err)
	}
	if exists {
		return &pgconn.PgError{Code: "23P01", ConstraintName: "student_busy_ranges_no_overlap", Message: "student schedule overlap"}, nil
	}
	if err := qtx.DBTX().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sessions
			WHERE deleted_at IS NULL AND course_id = $1
			  AND time_range && tstzrange($2, $3, '[)')
			  AND legacy_schedule_id IS DISTINCT FROM $4::text
		)`, request.CourseID, start, end, scheduleID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check legacy course overlap: %w", err)
	}
	if exists {
		return &pgconn.PgError{Code: "23P01", ConstraintName: "sessions_no_course_overlap", Message: "course session overlap"}, nil
	}
	return nil, nil
}

// deactivateMissingSchedules soft-deletes this course's local legacy sessions
// whose schedule rows disappeared from the source aggregate and tombstones
// their external mappings. With an empty incoming set every legacy session of
// the course is removed.
func (a *ScheduleApplier) deactivateMissingSchedules(ctx context.Context, tx pgx.Tx, request ScheduleApplyRequest) error {
	incoming := make([]string, 0, len(request.Aggregate.Schedules))
	for _, schedule := range request.Aggregate.Schedules {
		incoming = append(incoming, schedule.LegacyScheduleID)
	}
	rows, err := tx.Query(ctx, `
		UPDATE sessions SET deleted_at = now(), updated_at = now(), version = sessions.version + 1
		WHERE course_id = $1 AND source_kind = 'legacy' AND legacy_schedule_id IS NOT NULL
		  AND deleted_at IS NULL
		  AND legacy_schedule_id <> ALL($2::text[])
		RETURNING legacy_schedule_id`, request.CourseID, incoming)
	if err != nil {
		return fmt.Errorf("deactivate removed legacy schedules: %w", err)
	}
	defer rows.Close()
	var removed []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("collect removed legacy schedule: %w", err)
		}
		removed = append(removed, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("collect removed legacy schedules: %w", err)
	}
	if len(removed) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE external_refs SET state='tombstoned' WHERE source=$1 AND entity_type='schedule' AND external_id = ANY($2::text[])`, a.source, removed); err != nil {
		return fmt.Errorf("tombstone removed schedule mappings: %w", err)
	}
	return nil
}

// isExclusionViolation reports whether err is a Postgres exclusion-constraint
// rejection (SQLSTATE 23P01), e.g. sessions_no_teacher_overlap or
// sessions_no_room_overlap.
func isExclusionViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23P01"
}

func isAvailabilityViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514" && strings.Contains(strings.ToLower(pgErr.Message), "not available for requested time")
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// rejection (SQLSTATE 23505), e.g. courses_code_key.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isNotNullViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23502"
}

// recordScheduleConflict stores a skipped schedule row in
// legacy_sync_conflicts. Deduplicated per open conflict and legacy schedule
// ID so repeated refreshes do not accumulate duplicate rows.
func (a *ScheduleApplier) recordScheduleConflict(ctx context.Context, tx pgx.Tx, request ScheduleApplyRequest, schedule normalize.LegacySchedule, scheduleHash string, cause error) error {
	conflictType := "schedule_exclusion"
	constraintName := ""
	var pgErr *pgconn.PgError
	if errors.As(cause, &pgErr) {
		constraintName = pgErr.ConstraintName
	}
	switch {
	case isAvailabilityViolation(cause):
		conflictType = "availability"
	case constraintName == "sessions_no_teacher_overlap" || strings.Contains(cause.Error(), "sessions_no_teacher_overlap"):
		conflictType = "teacher_overlap"
	case constraintName == "sessions_no_room_overlap" || strings.Contains(cause.Error(), "sessions_no_room_overlap"):
		conflictType = "room_overlap"
	case constraintName == "student_busy_ranges_no_overlap" || strings.Contains(cause.Error(), "student_busy_ranges_no_overlap"):
		conflictType = "student_overlap"
	case constraintName == "sessions_no_course_overlap" || strings.Contains(cause.Error(), "sessions_no_course_overlap"):
		conflictType = "course_overlap"
	}
	payload, err := json.Marshal(map[string]any{
		"legacy_schedule_id":  schedule.LegacyScheduleID,
		"date":                schedule.Date,
		"begin":               schedule.Begin,
		"end":                 schedule.End,
		"classroom":           schedule.Classroom,
		"classroom_legacy_id": schedule.ClassroomLegacyID,
		"confirmed_by":        schedule.ConfirmedBy,
		"schedule_hash":       scheduleHash,
		"constraint_error":    cause.Error(),
	})
	if err != nil {
		return fmt.Errorf("encode schedule conflict payload %s: %w", schedule.LegacyScheduleID, err)
	}
	externalID := request.LegacyCourseID
	if externalID == "" {
		externalID = uuidText(request.CourseID)
	}
	message := fmt.Sprintf("legacy schedule %s (%s %s-%s) skipped: %s", schedule.LegacyScheduleID, schedule.Date, schedule.Begin, schedule.End, cause)
	if _, err := tx.Exec(ctx, `
		INSERT INTO legacy_sync_conflicts (entity_type, external_id, conflict_type, category, source_payload, message)
		SELECT $1, $2, $3, 'database_constraint', $4::jsonb, $5
		WHERE NOT EXISTS (
			SELECT 1 FROM legacy_sync_conflicts
			WHERE entity_type = $1 AND external_id = $2 AND conflict_type = $3 AND status = 'open'
			  AND source_payload->>'legacy_schedule_id' = $4::jsonb->>'legacy_schedule_id'
		)`,
		"course", externalID, conflictType, string(payload), message,
	); err != nil {
		return fmt.Errorf("record schedule conflict %s: %w", schedule.LegacyScheduleID, err)
	}
	return nil
}

func (a *ScheduleApplier) resolveScheduleConflict(ctx context.Context, tx pgx.Tx, request ScheduleApplyRequest, scheduleID string) error {
	externalID := request.LegacyCourseID
	if externalID == "" {
		externalID = uuidText(request.CourseID)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE legacy_sync_conflicts
		SET status='resolved', resolved_at=now()
		WHERE entity_type='course' AND external_id=$1 AND status='open'
		  AND source_payload->>'legacy_schedule_id'=$2`, externalID, scheduleID); err != nil {
		return fmt.Errorf("resolve legacy schedule conflict %s: %w", scheduleID, err)
	}
	return nil
}

func (a *ScheduleApplier) resolveRoomID(ctx context.Context, tx pgx.Tx, externalID string) (pgtype.UUID, error) {
	if externalID == "" {
		return pgtype.UUID{}, nil
	}
	var id pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT internal_id FROM external_refs WHERE source=$1 AND entity_type='room' AND external_id=$2 AND state IN ('active','restored')`, a.source, externalID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, nil
	}
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("resolve legacy room %s: %w", externalID, err)
	}
	return id, nil
}

func (a *ScheduleApplier) ensureExternalSeries(ctx context.Context, tx pgx.Tx, request ScheduleApplyRequest, loc *time.Location, roomID pgtype.UUID) (pgtype.UUID, error) {
	var seriesID pgtype.UUID
	groupKey := uuidText(request.CourseID)
	first, err := parseSourceDate(request.Aggregate.Schedules[0].Date)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("parse external series date: %w", err)
	}
	start, end, err := normalize.SessionWindow(first, request.Aggregate.Schedules[0].Begin, request.Aggregate.Schedules[0].End, loc)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("parse external series time: %w", err)
	}
	duration := int(end.Sub(start) / time.Minute)
	if duration < 1 {
		return pgtype.UUID{}, errors.New("legacy schedule: external series duration must be positive")
	}

	err = tx.QueryRow(ctx, `SELECT id FROM session_series WHERE course_id=$1 AND source_kind='legacy' AND materialization_mode='external' AND legacy_group_key=$2 AND deleted_at IS NULL ORDER BY created_at LIMIT 1 FOR UPDATE`, request.CourseID, groupKey).Scan(&seriesID)
	if err == nil {
		if _, err := tx.Exec(ctx, `UPDATE session_series SET room_id=$1, teacher_id=$2, institute_tz=$3, weekdays=ARRAY[$4]::smallint[], start_local_time=$5::time, duration_minutes=$6, start_date=$7::date, end_date=$7::date, count=$8, updated_at=now() WHERE id=$9`, roomID, request.TeacherID, loc.String(), int16(first.Weekday()), request.Aggregate.Schedules[0].Begin, duration, first.Format("2006-01-02"), len(request.Aggregate.Schedules), seriesID); err != nil {
			return pgtype.UUID{}, fmt.Errorf("update external series: %w", err)
		}
		return seriesID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, fmt.Errorf("load external series: %w", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO session_series (course_id, room_id, teacher_id, institute_tz, weekdays, start_local_time, duration_minutes, start_date, end_date, count, source_kind, materialization_mode, legacy_group_key) VALUES ($1,$2,$3,$4,ARRAY[$5]::smallint[],$6::time,$7,$8::date,$8::date,$9,'legacy','external',$10) RETURNING id`, request.CourseID, roomID, request.TeacherID, loc.String(), int16(first.Weekday()), request.Aggregate.Schedules[0].Begin, duration, first.Format("2006-01-02"), len(request.Aggregate.Schedules), groupKey).Scan(&seriesID); err != nil {
		return pgtype.UUID{}, fmt.Errorf("create external series: %w", err)
	}
	return seriesID, nil
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func uuidText(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return id.String()
}

func parseSourceDate(value string) (time.Time, error) {
	if date, err := time.Parse("2006-01-02", value); err == nil {
		return date, nil
	}
	return normalize.ParseLegacyDate(value)
}
func (a *ScheduleApplier) hitFault(name string) error {
	if a.fault == nil {
		return nil
	}
	if err := a.fault.Hit(name); err != nil {
		return fmt.Errorf("legacy schedule fault %s: %w", name, err)
	}
	return nil
}
