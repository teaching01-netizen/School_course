package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// LegacyAuditTotals summarises how much data the legacy sync has imported
// from the old site: linked courses, materialized legacy sessions (active and
// later soft-deleted), external series, roster-imported students, and mapped
// master data rows. Counts are subselects so one round trip serves the whole
// summary.
type LegacyAuditTotals struct {
	LinkedCourses       int32
	ArchivedCourses     int32
	SyncedCourses       int32
	LegacySessions      int32
	ActiveSessions      int32
	SoftDeletedSessions int32
	ExternalSeries      int32
	StudentsImported    int32
	MappedRooms         int32
	MappedTeachers      int32
	MappedSubjects      int32
}

func (q *Queries) LegacyAuditTotals(ctx context.Context) (LegacyAuditTotals, error) {
	var totals LegacyAuditTotals
	err := q.db.QueryRow(ctx, `
	SELECT
	    (SELECT count(*) FROM courses WHERE legacy_course_id IS NOT NULL)::int,
	    (SELECT count(*) FROM courses WHERE legacy_course_id IS NOT NULL AND legacy_archived)::int,
	    (SELECT count(*) FROM courses WHERE legacy_course_id IS NOT NULL AND legacy_last_synced_at IS NOT NULL)::int,
	    (SELECT count(*) FROM sessions WHERE source_kind = 'legacy' AND legacy_schedule_id IS NOT NULL)::int,
	    (SELECT count(*) FROM sessions WHERE source_kind = 'legacy' AND legacy_schedule_id IS NOT NULL AND deleted_at IS NULL)::int,
	    (SELECT count(*) FROM sessions WHERE source_kind = 'legacy' AND legacy_schedule_id IS NOT NULL AND deleted_at IS NOT NULL)::int,
	    (SELECT count(*) FROM session_series WHERE source_kind = 'legacy' AND materialization_mode = 'external')::int,
	    (SELECT count(*) FROM external_refs WHERE entity_type = 'student')::int,
	    (SELECT count(*) FROM external_refs WHERE entity_type = 'room')::int,
	    (SELECT count(*) FROM external_refs WHERE entity_type = 'teacher')::int,
	    (SELECT count(*) FROM external_refs WHERE entity_type = 'subject')::int
	`).Scan(
		&totals.LinkedCourses,
		&totals.ArchivedCourses,
		&totals.SyncedCourses,
		&totals.LegacySessions,
		&totals.ActiveSessions,
		&totals.SoftDeletedSessions,
		&totals.ExternalSeries,
		&totals.StudentsImported,
		&totals.MappedRooms,
		&totals.MappedTeachers,
		&totals.MappedSubjects,
	)
	return totals, err
}

// LegacyAuditRuns aggregates the sync run history: how many runs completed
// and the cumulative parse/apply/failure counters plus the most recent
// successful completion.
type LegacyAuditRuns struct {
	CompletedRuns            int32
	EntitiesParsed           int64
	EntitiesApplied          int64
	ParseFailures            int64
	ReconciliationMismatches int64
	LastSuccessfulAt         pgtype.Timestamptz
}

func (q *Queries) LegacyAuditRuns(ctx context.Context) (LegacyAuditRuns, error) {
	var runs LegacyAuditRuns
	err := q.db.QueryRow(ctx, `
	SELECT
	    count(*) FILTER (WHERE status = 'completed')::int,
	    COALESCE(sum(entities_parsed), 0),
	    COALESCE(sum(entities_applied), 0),
	    COALESCE(sum(parse_failures), 0),
	    COALESCE(sum(reconciliation_mismatches), 0),
	    max(completed_at) FILTER (WHERE status = 'completed')
	FROM legacy_sync_runs
	`).Scan(
		&runs.CompletedRuns,
		&runs.EntitiesParsed,
		&runs.EntitiesApplied,
		&runs.ParseFailures,
		&runs.ReconciliationMismatches,
		&runs.LastSuccessfulAt,
	)
	return runs, err
}

// LegacyAuditSkipCounts totals every recorded skip: schedule rows rejected by
// exclusion constraints are mirrored into legacy_sync_conflicts carrying a
// legacy_schedule_id payload; courses that could not be linked (mapping
// conflicts) or whose refresh job died are counted as skipped courses; courses
// whose last apply left rows out carry a 'partial' snapshot.
type LegacyAuditSkipCounts struct {
	SessionsSkippedTotal int32
	SessionsSkippedOpen  int32
	CoursesSkippedTotal  int32
	CoursesSkippedOpen   int32
	PartialSnapshots     int32
}

func (q *Queries) LegacyAuditSkipCounts(ctx context.Context) (LegacyAuditSkipCounts, error) {
	var counts LegacyAuditSkipCounts
	err := q.db.QueryRow(ctx, `
	SELECT
	    (SELECT count(*) FROM legacy_sync_conflicts WHERE source_payload->>'legacy_schedule_id' IS NOT NULL)::int,
	    (SELECT count(*) FROM legacy_sync_conflicts WHERE source_payload->>'legacy_schedule_id' IS NOT NULL AND status = 'open')::int,
	    (SELECT count(*) FROM legacy_sync_conflicts WHERE entity_type = 'course' AND source_payload->>'legacy_schedule_id' IS NULL)::int
	        + (SELECT count(*) FROM legacy_sync_dead_letters WHERE entity_type = 'course')::int,
	    (SELECT count(*) FROM legacy_sync_conflicts WHERE entity_type = 'course' AND source_payload->>'legacy_schedule_id' IS NULL AND status = 'open')::int
	        + (SELECT count(*) FROM legacy_sync_dead_letters WHERE entity_type = 'course')::int,
	    (SELECT count(*) FROM legacy_entity_snapshots WHERE quality = 'partial')::int
	`).Scan(
		&counts.SessionsSkippedTotal,
		&counts.SessionsSkippedOpen,
		&counts.CoursesSkippedTotal,
		&counts.CoursesSkippedOpen,
		&counts.PartialSnapshots,
	)
	return counts, err
}

// LegacyAuditSkipBucket is one row of the "skips by cause" breakdown. Cause is
// open_conflict, closed_conflict, dead_letter, or partial_snapshot; Key is the
// conflict type, error category, or snapshot quality depending on the cause.
type LegacyAuditSkipBucket struct {
	Cause      string
	EntityType string
	Key        string
	Count      int32
}

func (q *Queries) LegacyAuditSkipsByCause(ctx context.Context) ([]LegacyAuditSkipBucket, error) {
	rows, err := q.db.Query(ctx, `
	SELECT cause, entity_type, key, count
	FROM (
	    SELECT 'open_conflict' AS cause, entity_type, conflict_type AS key, count(*)::int AS count
	    FROM legacy_sync_conflicts
	    WHERE status = 'open'
	    GROUP BY entity_type, conflict_type
	    UNION ALL
	    SELECT 'closed_conflict', entity_type, conflict_type, count(*)::int
	    FROM legacy_sync_conflicts
	    WHERE status IN ('resolved', 'ignored')
	    GROUP BY entity_type, conflict_type
	    UNION ALL
	    SELECT 'dead_letter', COALESCE(entity_type, ''), COALESCE(error_category, ''), count(*)::int
	    FROM legacy_sync_dead_letters
	    GROUP BY entity_type, error_category
	    UNION ALL
	    SELECT 'partial_snapshot', entity_type, quality, count(*)::int
	    FROM legacy_entity_snapshots
	    WHERE quality = 'partial'
	    GROUP BY entity_type, quality
	) buckets
	ORDER BY cause, entity_type, key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LegacyAuditSkipBucket
	for rows.Next() {
		var bucket LegacyAuditSkipBucket
		if err := rows.Scan(&bucket.Cause, &bucket.EntityType, &bucket.Key, &bucket.Count); err != nil {
			return nil, err
		}
		out = append(out, bucket)
	}
	return out, rows.Err()
}

// LegacyAuditSkippedSession is one skipped schedule row, joined to the linked
// local course when one exists (the left join keeps rows whose course was
// never linked visible in the audit).
type LegacyAuditSkippedSession struct {
	LegacyScheduleID string
	Date             pgtype.Text
	Begin            pgtype.Text
	End              pgtype.Text
	Classroom        pgtype.Text
	ConflictType     string
	Category         string
	Message          pgtype.Text
	Status           string
	CreatedAt        pgtype.Timestamptz
	CourseID         pgtype.UUID
	CourseCode       pgtype.Text
	CourseName       pgtype.Text
	LegacyCourseID   string
}

func (q *Queries) LegacyAuditSkippedSessions(ctx context.Context, limit int32) ([]LegacyAuditSkippedSession, error) {
	rows, err := q.db.Query(ctx, `
	SELECT
	    c.source_payload->>'legacy_schedule_id',
	    c.source_payload->>'date',
	    c.source_payload->>'begin',
	    c.source_payload->>'end',
	    c.source_payload->>'classroom',
	    c.conflict_type,
	    c.category,
	    c.message,
	    c.status,
	    c.created_at,
	    co.id,
	    co.code,
	    co.name,
	    c.external_id
	FROM legacy_sync_conflicts c
	LEFT JOIN courses co ON co.legacy_course_id = c.external_id
	WHERE c.source_payload->>'legacy_schedule_id' IS NOT NULL
	ORDER BY (c.status = 'open') DESC, c.created_at DESC
	LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LegacyAuditSkippedSession
	for rows.Next() {
		var row LegacyAuditSkippedSession
		if err := rows.Scan(
			&row.LegacyScheduleID,
			&row.Date,
			&row.Begin,
			&row.End,
			&row.Classroom,
			&row.ConflictType,
			&row.Category,
			&row.Message,
			&row.Status,
			&row.CreatedAt,
			&row.CourseID,
			&row.CourseCode,
			&row.CourseName,
			&row.LegacyCourseID,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// LegacyAuditSkippedCourse is a course the sync could not bring over: a
// mapping conflict (conflict_type) or a dead letter on a course refresh job.
// ReasonKind distinguishes the two ledgers.
type LegacyAuditSkippedCourse struct {
	ReasonKind    string
	ExternalID    string
	ConflictType  string
	ErrorCategory pgtype.Text
	Message       pgtype.Text
	Status        string
	CreatedAt     pgtype.Timestamptz
	CourseID      pgtype.UUID
	CourseCode    pgtype.Text
	CourseName    pgtype.Text
}

func (q *Queries) LegacyAuditSkippedCourses(ctx context.Context, limit int32) ([]LegacyAuditSkippedCourse, error) {
	rows, err := q.db.Query(ctx, `
	SELECT reason_kind, external_id, conflict_type, error_category, message, status, created_at,
	       course_id, course_code, course_name
	FROM (
	    SELECT 'conflict' AS reason_kind, c.external_id, c.conflict_type, NULL::text AS error_category,
	           c.message, c.status, c.created_at,
	           co.id AS course_id, co.code AS course_code, co.name AS course_name
	    FROM legacy_sync_conflicts c
	    LEFT JOIN courses co ON co.legacy_course_id = c.external_id
	    WHERE c.entity_type = 'course' AND c.source_payload->>'legacy_schedule_id' IS NULL
	    UNION ALL
	    SELECT 'dead_letter', d.external_id, d.job_type, d.error_category,
	           d.last_error, 'dead', d.created_at,
	           co.id, co.code, co.name
	    FROM legacy_sync_dead_letters d
	    LEFT JOIN courses co ON co.legacy_course_id = d.external_id
	    WHERE d.entity_type = 'course'
	) skipped
	ORDER BY (status = 'open' OR status = 'dead') DESC, created_at DESC
	LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LegacyAuditSkippedCourse
	for rows.Next() {
		var row LegacyAuditSkippedCourse
		if err := rows.Scan(
			&row.ReasonKind,
			&row.ExternalID,
			&row.ConflictType,
			&row.ErrorCategory,
			&row.Message,
			&row.Status,
			&row.CreatedAt,
			&row.CourseID,
			&row.CourseCode,
			&row.CourseName,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// LegacyAuditDeadLetters lists dead-lettered sync jobs with the entity they
// failed for and the recorded error category.
func (q *Queries) LegacyAuditDeadLetters(ctx context.Context, limit int32) ([]LegacySyncDeadLetter, error) {
	rows, err := q.db.Query(ctx, `
	SELECT id, job_type, unique_key, entity_type, external_id, payload, error_category,
	       last_error, attempts, created_at
	FROM legacy_sync_dead_letters
	ORDER BY created_at DESC
	LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LegacySyncDeadLetter
	for rows.Next() {
		var row LegacySyncDeadLetter
		if err := rows.Scan(
			&row.ID,
			&row.JobType,
			&row.UniqueKey,
			&row.EntityType,
			&row.ExternalID,
			&row.Payload,
			&row.ErrorCategory,
			&row.LastError,
			&row.Attempts,
			&row.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
