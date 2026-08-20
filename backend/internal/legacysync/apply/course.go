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
}

// FaultPoint is an injectable failure boundary used by deterministic integration tests.
type FaultPoint interface {
	Hit(name string) error
}

type CourseApplier struct {
	pool   *pgxpool.Pool
	q      *sqldb.Queries
	source string
	fault  FaultPoint
}

func NewCourseApplier(pool *pgxpool.Pool, q *sqldb.Queries, source string) *CourseApplier {
	return &CourseApplier{pool: pool, q: q, source: source}
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
	qtx := a.q.WithTx(tx)
	previous, err := qtx.SnapshotGet(ctx, sqldb.SnapshotGetParams{Source: a.source, EntityType: "course", ExternalID: request.LegacyCourseID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ScheduleApplyResult{}, fmt.Errorf("load legacy course snapshot: %w", err)
	}
	// The unchanged-hash fast path may only fire when the previous run
	// applied the whole aggregate; a partial snapshot must stay retryable.
	if err == nil && previous.SourceHash == sourceHash && previous.Quality == "ok" {
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
	if request.ShadowMode {
		if err := tx.Commit(ctx); err != nil {
			return ScheduleApplyResult{}, fmt.Errorf("commit shadow legacy course: %w", err)
		}
		return ScheduleApplyResult{SourceHash: sourceHash, AppliedAt: request.ObservedAt}, nil
	}
	teacherID, err := a.resolveReference(ctx, tx, "teacher", request.Aggregate.Course.TeacherID)
	if err != nil {
		return ScheduleApplyResult{}, err
	}
	if err := a.hitFault("after_teacher_mapping_resolution"); err != nil {
		return ScheduleApplyResult{}, err
	}
	subjectID, err := a.resolveReference(ctx, tx, "subject", request.Aggregate.Course.SubjectID)
	if err != nil {
		return ScheduleApplyResult{}, err
	}
	if err := a.hitFault("after_subject_mapping_resolution"); err != nil {
		return ScheduleApplyResult{}, err
	}
	if err := a.updateCourse(ctx, tx, request, teacherID, subjectID, sourceHash); err != nil {
		// A source change that collides with native uniqueness (e.g. the
		// legacy course now reuses another local course's code) can never be
		// resolved by retrying: record it as an admin-visible conflict outside
		// the aborted transaction before surfacing the error.
		if isUniqueViolation(err) {
			_ = tx.Rollback(ctx)
			if recordErr := a.recordCourseCodeConflict(ctx, request, request.Aggregate.Course.Code, err); recordErr != nil {
				return ScheduleApplyResult{}, fmt.Errorf("%w (recording conflict failed: %v)", err, recordErr)
			}
		}
		return ScheduleApplyResult{}, err
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
	scheduleApplier := &ScheduleApplier{source: a.source, fault: a.fault}
	scheduleRequest := ScheduleApplyRequest{
		CourseID:       request.CourseID,
		LegacyCourseID: request.LegacyCourseID,
		TeacherID:      teacherID,
		Aggregate:      request.Aggregate,
		ObservedAt:     request.ObservedAt,
		InstituteTZ:    loc.String(),
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
	if skipped > 0 {
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
		INSERT INTO legacy_sync_conflicts (entity_type, external_id, conflict_type, category, source_payload, message)
		SELECT 'course', $1, 'course_code_conflict', 'database_constraint', $2::jsonb, $3
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

func legacyCourseLockKey(source, externalCourseID string) string {
	return source + ":course:" + externalCourseID
}
