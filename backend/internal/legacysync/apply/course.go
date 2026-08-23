package apply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/legacysync/normalize"
	"warwick-institute/internal/schedulepolicy"
)

var (
	ErrMissingCourseIdentity = errors.New("legacy course: identity is required")
	ErrMissingReference      = errors.New("legacy course: referenced master data is missing")
)

type CourseApplyRequest struct {
	CourseID        pgtype.UUID
	LegacyCourseID  string
	Aggregate       normalize.LegacyCourseAggregate
	ObservedAt      time.Time
	InstituteTZ     string
	ShadowMode      bool
	RealtimeEnabled bool
	allowConflicts  bool
}

// FaultPoint is an injectable failure boundary used by deterministic integration tests.
type FaultPoint interface {
	Hit(name string) error
}

type CourseApplier struct {
	pool   *pgxpool.Pool
	q      *sqldb.Queries
	source string
	policy schedulepolicy.Reader
	fault  FaultPoint
}

func NewCourseApplier(pool *pgxpool.Pool, q *sqldb.Queries, source string, policy schedulepolicy.Reader) *CourseApplier {
	return &CourseApplier{pool: pool, q: q, source: source, policy: policy}
}

func ValidateCourseAggregate(request CourseApplyRequest) error {
	if !request.CourseID.Valid || request.LegacyCourseID == "" {
		return ErrMissingCourseIdentity
	}
	if request.Aggregate.Course.LegacyID != "" && request.Aggregate.Course.LegacyID != request.LegacyCourseID {
		return fmt.Errorf("legacy course: aggregate ID %q does not match request ID %q", request.Aggregate.Course.LegacyID, request.LegacyCourseID)
	}
	status := strings.ToLower(strings.TrimSpace(request.Aggregate.Course.Status))
	if status != "" && status != "active" && status != "draft" && status != "archived" {
		return fmt.Errorf("legacy course: unsupported status %q", request.Aggregate.Course.Status)
	}
	typ := strings.TrimSpace(request.Aggregate.Course.Type)
	if typ != "" && typ != "Private" && typ != "Group" && typ != "General" {
		return fmt.Errorf("legacy course: unsupported type %q", request.Aggregate.Course.Type)
	}
	if hours := strings.TrimSpace(request.Aggregate.Course.Hours); hours != "" {
		value, err := strconv.Atoi(hours)
		if err != nil || value < 0 {
			return fmt.Errorf("legacy course: invalid hours %q", hours)
		}
	}
	return ValidateScheduleAggregate(request.Aggregate)
}

func (a *CourseApplier) Apply(ctx context.Context, request CourseApplyRequest) (ScheduleApplyResult, error) {
	if err := ValidateCourseAggregate(request); err != nil {
		return ScheduleApplyResult{}, err
	}
	if a.pool == nil || a.q == nil {
		return ScheduleApplyResult{}, errors.New("legacy course: pool and queries are required")
	}
	if request.ObservedAt.IsZero() {
		request.ObservedAt = time.Now().UTC()
	}
	canonical, err := normalize.CanonicalJSON(request.Aggregate)
	if err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("canonicalize legacy course: %w", err)
	}
	sourceHash, err := normalize.HashCanonical(request.Aggregate)
	if err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("hash legacy course: %w", err)
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("begin legacy course apply: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, legacyCourseLockKey(a.source, request.LegacyCourseID)); err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("lock legacy course %s: %w", request.LegacyCourseID, err)
	}
	// Lock on code as well (order: id then code) to prevent code collisions during update.
	if request.Aggregate.Course.Code != "" {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, a.source+":code:"+request.Aggregate.Course.Code); err != nil {
			return ScheduleApplyResult{}, fmt.Errorf("lock legacy course code %s: %w", request.Aggregate.Course.Code, err)
		}
	}
	qtx := a.q.WithTx(tx)
	if a.policy == nil {
		return ScheduleApplyResult{}, errors.New("legacy course: policy reader is required")
	}
	policy, err := a.policy.Load(ctx, tx)
	if err != nil {
		return ScheduleApplyResult{}, err
	}
	request.allowConflicts = !policy.Enforced(schedulepolicy.ScopeLegacySync)
	previous, err := qtx.SnapshotGet(ctx, sqldb.SnapshotGetParams{Source: a.source, EntityType: "course", ExternalID: request.LegacyCourseID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ScheduleApplyResult{}, fmt.Errorf("load legacy course snapshot: %w", err)
	}
	// The unchanged-hash fast path may only fire when the previous run
	// applied the whole aggregate; a partial snapshot must stay retryable.
	// Additionally, verify local code and references still match (integrity check).
	if err == nil && previous.SourceHash == sourceHash && previous.Quality == "ok" {
		if a.fastPathIntegrityValid(ctx, tx, request) {
			if _, err := tx.Exec(ctx, `UPDATE courses SET legacy_last_seen_at=$1, legacy_last_synced_at=$1 WHERE id=$2`, request.ObservedAt, request.CourseID); err != nil {
				return ScheduleApplyResult{}, fmt.Errorf("update unchanged legacy course metadata: %w", err)
			}
			if _, err := tx.Exec(ctx, `UPDATE external_refs SET last_seen_at=$1 WHERE source=$2 AND entity_type='course' AND external_id=$3`, request.ObservedAt, a.source, request.LegacyCourseID); err != nil {
				return ScheduleApplyResult{}, fmt.Errorf("update unchanged legacy course mapping: %w", err)
			}
			if err := tx.Commit(ctx); err != nil {
				return ScheduleApplyResult{}, fmt.Errorf("commit unchanged legacy course: %w", err)
			}
			return ScheduleApplyResult{SourceHash: sourceHash, AppliedAt: request.ObservedAt}, nil
		}
		// Integrity check failed, fall through to full apply (will record conflict via R2/R3).
	}
	if request.ShadowMode {
		if err := tx.Commit(ctx); err != nil {
			return ScheduleApplyResult{}, fmt.Errorf("commit shadow legacy course: %w", err)
		}
		return ScheduleApplyResult{SourceHash: sourceHash, AppliedAt: request.ObservedAt}, nil
	}
	teacherID, err := a.resolveReference(ctx, tx, "teacher", request.Aggregate.Course.TeacherID)
	if err != nil {
		if errors.Is(err, ErrMissingReference) {
			if _, recordErr := qtx.ConflictInsert(ctx, sqldb.ConflictInsertParams{
				EntityType:    "course",
				ExternalID:    request.LegacyCourseID,
				ConflictType:  "missing_reference:teacher",
				Category:      "missing_reference",
				SourcePayload: fmt.Sprintf(`{"reference_type":"teacher","reference_id":%q}`, request.Aggregate.Course.TeacherID),
				LocalPayload:  "{}",
				Message:       pgtype.Text{String: fmt.Sprintf("teacher reference %s not found for legacy course %s", request.Aggregate.Course.TeacherID, request.LegacyCourseID), Valid: true},
			}); recordErr != nil && !errors.Is(recordErr, pgx.ErrNoRows) {
				return ScheduleApplyResult{}, fmt.Errorf("record missing teacher reference: %w", recordErr)
			}
			if _, ignoreErr := tx.Exec(ctx, `UPDATE legacy_sync_conflicts SET status='ignored', resolved_at=now() WHERE entity_type='course' AND external_id=$1 AND conflict_type='missing_reference:teacher' AND status='open'`, request.LegacyCourseID); ignoreErr != nil {
				return ScheduleApplyResult{}, fmt.Errorf("close missing teacher reference: %w", ignoreErr)
			}
			teacherID = pgtype.UUID{} // zero UUID
		} else {
			return ScheduleApplyResult{}, err
		}
	}
	if err := a.hitFault("after_teacher_mapping_resolution"); err != nil {
		return ScheduleApplyResult{}, err
	}
	subjectID, err := a.resolveReference(ctx, tx, "subject", request.Aggregate.Course.SubjectID)
	if err != nil {
		if errors.Is(err, ErrMissingReference) {
			if _, recordErr := qtx.ConflictInsert(ctx, sqldb.ConflictInsertParams{
				EntityType:    "course",
				ExternalID:    request.LegacyCourseID,
				ConflictType:  "missing_reference:subject",
				Category:      "missing_reference",
				SourcePayload: fmt.Sprintf(`{"reference_type":"subject","reference_id":%q}`, request.Aggregate.Course.SubjectID),
				LocalPayload:  "{}",
				Message:       pgtype.Text{String: fmt.Sprintf("subject reference %s not found for legacy course %s", request.Aggregate.Course.SubjectID, request.LegacyCourseID), Valid: true},
			}); recordErr != nil && !errors.Is(recordErr, pgx.ErrNoRows) {
				return ScheduleApplyResult{}, fmt.Errorf("record missing subject reference: %w", recordErr)
			}
			if _, ignoreErr := tx.Exec(ctx, `UPDATE legacy_sync_conflicts SET status='ignored', resolved_at=now() WHERE entity_type='course' AND external_id=$1 AND conflict_type='missing_reference:subject' AND status='open'`, request.LegacyCourseID); ignoreErr != nil {
				return ScheduleApplyResult{}, fmt.Errorf("close missing subject reference: %w", ignoreErr)
			}
			subjectID = pgtype.UUID{} // zero UUID
		} else {
			return ScheduleApplyResult{}, err
		}
	}
	if err := a.hitFault("after_subject_mapping_resolution"); err != nil {
		return ScheduleApplyResult{}, err
	}
	// Wrap updateCourse in a SAVEPOINT to handle code collisions gracefully.
	if _, err := tx.Exec(ctx, `SAVEPOINT course_update`); err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("savepoint course update: %w", err)
	}
	// Read current code before update to retain it on collision.
	var currentCode string
	if err := tx.QueryRow(ctx, `SELECT code FROM courses WHERE id=$1`, request.CourseID).Scan(&currentCode); err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("read course code for savepoint: %w", err)
	}
	codeCollision := false
	if err := a.updateCourse(ctx, tx, request, teacherID, subjectID, sourceHash); err != nil {
		if isUniqueViolation(err) {
			if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT course_update`); err != nil {
				return ScheduleApplyResult{}, fmt.Errorf("rollback course update savepoint: %w", err)
			}
			// Record conflict in-tx, then re-run updateCourse with the retained current code.
			if recordErr := a.recordCourseCodeConflict(ctx, request, request.Aggregate.Course.Code, err); recordErr != nil {
				return ScheduleApplyResult{}, fmt.Errorf("%w (recording conflict failed: %v)", err, recordErr)
			}
			if _, ignoreErr := tx.Exec(ctx, `UPDATE legacy_sync_conflicts SET status='ignored', resolved_at=now() WHERE entity_type='course' AND external_id=$1 AND conflict_type='course_code_conflict' AND status='open'`, request.LegacyCourseID); ignoreErr != nil {
				return ScheduleApplyResult{}, fmt.Errorf("close course code conflict: %w", ignoreErr)
			}
			// Re-run updateCourse with retained code to apply the rest of the aggregate.
			retainedRequest := request
			retainedRequest.Aggregate.Course.Code = currentCode
			if err := a.updateCourse(ctx, tx, retainedRequest, teacherID, subjectID, sourceHash); err != nil {
				return ScheduleApplyResult{}, err
			}
			if _, err := tx.Exec(ctx, `UPDATE courses SET legacy_source_code=$1, legacy_code_conflict=true WHERE id=$2`, request.Aggregate.Course.Code, request.CourseID); err != nil {
				return ScheduleApplyResult{}, fmt.Errorf("store legacy course code conflict: %w", err)
			}
			codeCollision = true
		} else {
			return ScheduleApplyResult{}, err
		}
	} else {
		if _, err := tx.Exec(ctx, `RELEASE SAVEPOINT course_update`); err != nil {
			return ScheduleApplyResult{}, fmt.Errorf("release course update savepoint: %w", err)
		}
	}
	// Mirror courses.teacher_id into course_teachers as the primary row so
	// legacy-synced courses keep the INV-001 invariant established by
	// migration 00079 (every non-null teacher_id has a matching row).
	if err := a.mirrorPrimaryTeacher(ctx, tx, request.CourseID, teacherID); err != nil {
		return ScheduleApplyResult{}, err
	}
	if err := a.hitFault("after_course_upsert"); err != nil {
		return ScheduleApplyResult{}, err
	}
	skipped := 0
	// applyDomain runs even with an empty schedule set: the source set is
	// authoritative, so a source page with no rows must converge the course
	// to zero active legacy sessions. Schedule times are interpreted in the
	// configured institute timezone, never a hardcoded one.
	if request.InstituteTZ == "" {
		request.InstituteTZ = "Asia/Bangkok"
	}
	loc, err := time.LoadLocation(request.InstituteTZ)
	if err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("load legacy course timezone: %w", err)
	}
	scheduleApplier := &ScheduleApplier{source: a.source, policy: a.policy, fault: a.fault}
	scheduleRequest := ScheduleApplyRequest{
		CourseID:       request.CourseID,
		LegacyCourseID: request.LegacyCourseID,
		TeacherID:      teacherID,
		Aggregate:      request.Aggregate,
		ObservedAt:     request.ObservedAt,
		InstituteTZ:    loc.String(),
		allowConflicts: request.allowConflicts,
	}
	skipped, err = scheduleApplier.applyDomain(ctx, tx, qtx, scheduleRequest, loc, sourceHash)
	if err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("apply legacy course schedules: %w", err)
	}
	if _, err := qtx.ExternalRefUpsert(ctx, sqldb.ExternalRefUpsertParams{Source: a.source, EntityType: "course", ExternalID: request.LegacyCourseID, InternalID: request.CourseID, SourceHash: pgtype.Text{String: sourceHash, Valid: true}}); err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("upsert legacy course mapping: %w", err)
	}
	if err := a.hitFault("after_external_ref_upsert"); err != nil {
		return ScheduleApplyResult{}, err
	}
	quality := "ok"
	if skipped > 0 && !request.allowConflicts {
		quality = "partial"
	}
	if _, err := qtx.SnapshotUpsert(ctx, sqldb.SnapshotUpsertParams{Source: a.source, EntityType: "course", ExternalID: request.LegacyCourseID, CanonicalData: string(canonical), SourceHash: sourceHash, ParserVersion: 1, ObservedAt: timestamp(request.ObservedAt), Quality: quality}); err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("store legacy course snapshot: %w", err)
	}
	if err := a.hitFault("after_snapshot_insert"); err != nil {
		return ScheduleApplyResult{}, err
	}
	if request.RealtimeEnabled {
		payload := fmt.Sprintf(`{"synced_at":%q}`, request.ObservedAt.UTC().Format(time.RFC3339Nano))
		if _, err := qtx.OutboxInsert(ctx, sqldb.OutboxInsertParams{SourceEventKey: "legacy:course:" + request.LegacyCourseID + ":" + sourceHash, EventType: "legacy.course.updated", Channel: "course:" + uuidText(request.CourseID), EntityType: pgtype.Text{String: "course", Valid: true}, ExternalID: pgtype.Text{String: request.LegacyCourseID, Valid: true}, Payload: payload}); err != nil {
			return ScheduleApplyResult{}, fmt.Errorf("write legacy course outbox: %w", err)
		}
	}
	if err := a.hitFault("after_outbox_insert"); err != nil {
		return ScheduleApplyResult{}, err
	}
	// R7: auto-resolve conflicts that this apply healed.
	// Best-effort: non-fatal if resolution fails.
	codeAppliedCleanly := !codeCollision
	_ = a.resolveHealedConflicts(ctx, tx, request, teacherID, subjectID, codeAppliedCleanly)
	if _, err := tx.Exec(ctx, `DELETE FROM legacy_sync_dead_letters WHERE job_type='legacy_refresh_course' AND external_id=$1`, request.LegacyCourseID); err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("clear completed legacy dead letter %s: %w", request.LegacyCourseID, err)
	}
	if err := a.hitFault("before_commit"); err != nil {
		return ScheduleApplyResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ScheduleApplyResult{}, fmt.Errorf("commit legacy course: %w", err)
	}
	return ScheduleApplyResult{SourceHash: sourceHash, Changed: true, Sessions: len(request.Aggregate.Schedules), SkippedSessions: skipped, AppliedAt: request.ObservedAt}, nil
}

// recordCourseCodeConflict stores a native uniqueness collision (typically
// courses_code_key) in legacy_sync_conflicts so it is actionable in the admin
// health view. It runs on its own connection: the apply transaction is already
// aborted. Deduplicated per open conflict and legacy course.
func (a *CourseApplier) recordCourseCodeConflict(ctx context.Context, request CourseApplyRequest, code string, cause error) error {
	constraint := "unknown"
	var pgErr *pgconn.PgError
	if errors.As(cause, &pgErr) {
		constraint = pgErr.ConstraintName
	}
	payload, err := json.Marshal(map[string]any{
		"legacy_course_id": request.LegacyCourseID,
		"code":             code,
		"constraint":       constraint,
		"constraint_error": cause.Error(),
	})
	if err != nil {
		return fmt.Errorf("encode course conflict payload %s: %w", request.LegacyCourseID, err)
	}
	message := fmt.Sprintf("legacy course %s code %q conflicts with an existing local course (%s)", request.LegacyCourseID, code, constraint)
	if _, err := a.pool.Exec(ctx, `
		INSERT INTO legacy_sync_conflicts (entity_type, external_id, conflict_type, category, source_payload, message, status, resolved_at)
		SELECT 'course', $1, 'course_code_conflict', 'database_constraint', $2::jsonb, $3, 'ignored', now()
		WHERE NOT EXISTS (
			SELECT 1 FROM legacy_sync_conflicts
			WHERE entity_type = 'course' AND external_id = $1
			  AND conflict_type = 'course_code_conflict' AND status = 'open'
		)`,
		request.LegacyCourseID, string(payload), message,
	); err != nil {
		return fmt.Errorf("record course code conflict %s: %w", request.LegacyCourseID, err)
	}
	return nil
}

func (a *CourseApplier) resolveReference(ctx context.Context, tx pgx.Tx, entityType, externalID string) (pgtype.UUID, error) {
	if externalID == "" {
		return pgtype.UUID{}, nil
	}
	var internalID pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT internal_id FROM external_refs WHERE source=$1 AND entity_type=$2 AND external_id=$3 AND state IN ('active','restored') FOR SHARE`, a.source, entityType, externalID).Scan(&internalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, fmt.Errorf("%w: %s %s", ErrMissingReference, entityType, externalID)
	}
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("resolve legacy %s %s: %w", entityType, externalID, err)
	}
	return internalID, nil
}

func (a *CourseApplier) updateCourse(ctx context.Context, tx pgx.Tx, request CourseApplyRequest, teacherID, subjectID pgtype.UUID, sourceHash string) error {
	course := request.Aggregate.Course
	hour := 0
	if course.Hours != "" {
		hour, _ = strconv.Atoi(course.Hours)
	}
	studentCount := len(request.Aggregate.Attendees)
	status := strings.ToLower(strings.TrimSpace(course.Status))
	result, err := tx.Exec(ctx, `UPDATE courses SET code=COALESCE(NULLIF($1,''), code), name=COALESCE(NULLIF($2,''), name), teacher_id=NULLIF($3::text,'')::uuid, subject_id=NULLIF($4::text,'')::uuid, hour=NULLIF($5,0), student_count=CASE WHEN $6 THEN $7 ELSE student_count END, course_type=NULLIF($8,''), legacy_status=NULLIF($9,''), legacy_expire_date=NULLIF($10,'')::date, legacy_archived=($9='archived'), legacy_source_hash=$11, legacy_last_seen_at=$12, legacy_last_synced_at=$12, source_kind='legacy', updated_at=now() WHERE id=$13`, course.Code, course.Name, uuidText(teacherID), uuidText(subjectID), hour, request.Aggregate.Attendees != nil, studentCount, course.Type, status, course.ExpireDate, sourceHash, request.ObservedAt, request.CourseID)
	if err != nil {
		return fmt.Errorf("update legacy course %s: %w", request.LegacyCourseID, err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("legacy course %s: local course not found", request.LegacyCourseID)
	}
	return nil
}
func (a *CourseApplier) mirrorPrimaryTeacher(ctx context.Context, tx pgx.Tx, courseID, teacherID pgtype.UUID) error {
	if !teacherID.Valid {
		if _, err := tx.Exec(ctx, `DELETE FROM course_teachers WHERE course_id=$1 AND is_primary=true`, courseID); err != nil {
			return fmt.Errorf("remove stale primary teacher %s: %w", courseID, err)
		}
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE course_teachers SET is_primary = false
		WHERE course_id = $1 AND is_primary = true AND teacher_id IS DISTINCT FROM $2
	`, courseID, teacherID); err != nil {
		return fmt.Errorf("demote legacy course primaries %s: %w", courseID, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO course_teachers (course_id, teacher_id, is_primary)
		VALUES ($1, $2, true)
		ON CONFLICT (course_id, teacher_id) DO UPDATE SET is_primary = true
	`, courseID, teacherID); err != nil {
		return fmt.Errorf("mirror legacy course primary %s: %w", courseID, err)
	}
	return nil
}

func (a *CourseApplier) hitFault(name string) error {
	if a.fault == nil {
		return nil
	}
	if err := a.fault.Hit(name); err != nil {
		return fmt.Errorf("legacy course fault %s: %w", name, err)
	}
	return nil
}

// fastPathIntegrityValid checks whether the local course state still matches the
// aggregate code and references for the fast-path to be safe.
func (a *CourseApplier) fastPathIntegrityValid(ctx context.Context, tx pgx.Tx, request CourseApplyRequest) bool {
	// Verify local code still matches the aggregate code.
	var currentCode string
	if err := tx.QueryRow(ctx, `SELECT code FROM courses WHERE id=$1`, request.CourseID).Scan(&currentCode); err != nil {
		return false
	}
	if currentCode != request.Aggregate.Course.Code {
		return false
	}
	// Verify teacher reference still resolves.
	if request.Aggregate.Course.TeacherID != "" {
		if _, err := a.resolveReference(ctx, tx, "teacher", request.Aggregate.Course.TeacherID); err != nil {
			return false
		}
	}
	// Verify subject reference still resolves.
	if request.Aggregate.Course.SubjectID != "" {
		if _, err := a.resolveReference(ctx, tx, "subject", request.Aggregate.Course.SubjectID); err != nil {
			return false
		}
	}
	return true
}

// resolveHealedConflicts resolves open conflicts that this apply healed.
// It is best-effort: errors are returned but should not fail the apply.
func (a *CourseApplier) resolveHealedConflicts(ctx context.Context, tx pgx.Tx, request CourseApplyRequest, teacherID, subjectID pgtype.UUID, codeAppliedCleanly bool) error {
	if codeAppliedCleanly {
		if _, err := tx.Exec(ctx, `UPDATE courses SET legacy_code_conflict=false, legacy_source_code=NULL WHERE id=$1`, request.CourseID); err != nil {
			return fmt.Errorf("clear healed course code conflict for %s: %w", request.LegacyCourseID, err)
		}
	}
	// If code applied cleanly, resolve code-related conflicts.
	if codeAppliedCleanly {
		for _, conflictType := range []string{"course_code_conflict", "code_collision"} {
			if _, err := tx.Exec(ctx, `
				UPDATE legacy_sync_conflicts
				SET status='resolved', resolved_at=now()
				WHERE entity_type='course' AND external_id=$1 AND conflict_type=$2 AND status='open'
			`, request.LegacyCourseID, conflictType); err != nil {
				return fmt.Errorf("resolve %s for %s: %w", conflictType, request.LegacyCourseID, err)
			}
		}
	}
	// If teacher reference is valid, resolve missing teacher reference conflicts.
	if teacherID.Valid {
		if _, err := tx.Exec(ctx, `
			UPDATE legacy_sync_conflicts
			SET status='resolved', resolved_at=now()
			WHERE entity_type='course' AND external_id=$1 AND conflict_type='missing_reference:teacher' AND status='open'
		`, request.LegacyCourseID); err != nil {
			return fmt.Errorf("resolve missing_reference:teacher for %s: %w", request.LegacyCourseID, err)
		}
	}
	// If subject reference is valid, resolve missing subject reference conflicts.
	if subjectID.Valid {
		if _, err := tx.Exec(ctx, `
			UPDATE legacy_sync_conflicts
			SET status='resolved', resolved_at=now()
			WHERE entity_type='course' AND external_id=$1 AND conflict_type='missing_reference:subject' AND status='open'
		`, request.LegacyCourseID); err != nil {
			return fmt.Errorf("resolve missing_reference:subject for %s: %w", request.LegacyCourseID, err)
		}
	}
	return nil
}

func legacyCourseLockKey(source, externalCourseID string) string {
	return source + ":course:" + externalCourseID
}
