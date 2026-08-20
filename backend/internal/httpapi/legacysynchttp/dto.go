package legacysynchttp

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type controlDTO struct {
	DetectionEnabled bool    `json:"detection_enabled"`
	FetchEnabled     bool    `json:"fetch_enabled"`
	ApplyEnabled     bool    `json:"apply_enabled"`
	StudentEnabled   bool    `json:"student_enabled"`
	TombstoneEnabled bool    `json:"tombstone_enabled"`
	RealtimeEnabled  bool    `json:"realtime_enabled"`
	ShadowMode       bool    `json:"shadow_mode"`
	UpdatedAt        *string `json:"updated_at"`
}

type queueDTO struct {
	Queued    int32 `json:"queued"`
	Running   int32 `json:"running"`
	Completed int32 `json:"completed"`
	Dead      int32 `json:"dead"`
}

type runDTO struct {
	ID                       string       `json:"id"`
	Mode                     string       `json:"mode"`
	Status                   string       `json:"status"`
	StartedAt                *string      `json:"started_at"`
	CompletedAt              *string      `json:"completed_at"`
	PagesRequested           int32        `json:"pages_requested"`
	EntitiesParsed           int32        `json:"entities_parsed"`
	EntitiesChanged          int32        `json:"entities_changed"`
	EntitiesApplied          int32        `json:"entities_applied"`
	ParseFailures            int32        `json:"parse_failures"`
	ReconciliationMismatches int32        `json:"reconciliation_mismatches"`
	SourceLatencyMs          *int32       `json:"source_latency_ms"`
	LastError                *string      `json:"last_error"`
	Progress                 *progressDTO `json:"progress"`
}

type progressDTO struct {
	Phase             string  `json:"phase"`
	CurrentEntity     *string `json:"current_entity"`
	ProcessedEntities int32   `json:"processed_entities"`
	TotalEntities     int32   `json:"total_entities"`
	ChangedEntities   int32   `json:"changed_entities"`
	AppliedEntities   int32   `json:"applied_entities"`
	Failures          int32   `json:"failures"`
	UpdatedAt         *string `json:"updated_at"`
}

type conflictDTO struct {
	ID            string  `json:"id"`
	EntityType    string  `json:"entity_type"`
	ExternalID    string  `json:"external_id"`
	ConflictType  string  `json:"conflict_type"`
	Category      string  `json:"category"`
	Message       *string `json:"message"`
	SourcePayload *string `json:"source_payload"`
	LocalPayload  *string `json:"local_payload"`
	Status        string  `json:"status"`
	CreatedAt     *string `json:"created_at"`
	ResolvedAt    *string `json:"resolved_at"`
}

type jobDTO struct {
	ID          string  `json:"id"`
	JobType     string  `json:"job_type"`
	EntityType  *string `json:"entity_type"`
	ExternalID  *string `json:"external_id"`
	UniqueKey   *string `json:"unique_key"`
	Priority    int32   `json:"priority"`
	Status      string  `json:"status"`
	DeadlineAt  *string `json:"deadline_at"`
	Attempt     int32   `json:"attempt"`
	MaxAttempts int32   `json:"max_attempts"`
	LockedUntil *string `json:"locked_until"`
	HeartbeatAt *string `json:"heartbeat_at"`
	RunAfter    *string `json:"run_after"`
	LastError   *string `json:"last_error"`
	CreatedAt   *string `json:"created_at"`
	UpdatedAt   *string `json:"updated_at"`
}

type healthDTO struct {
	Status           string     `json:"status"`
	Paused           bool       `json:"paused"`
	ShadowMode       bool       `json:"shadow_mode"`
	Control          controlDTO `json:"control"`
	Queue            queueDTO   `json:"queue"`
	OpenConflicts    int        `json:"open_conflicts"`
	LatestRun        *runDTO    `json:"latest_run"`
	LastSuccessfulAt *string    `json:"last_successful_at"`
	FreshnessSeconds *int64     `json:"freshness_seconds"`
}

type legacyAuditTotalsDTO struct {
	LinkedCourses       int32 `json:"linked_courses"`
	ArchivedCourses     int32 `json:"archived_courses"`
	SyncedCourses       int32 `json:"synced_courses"`
	LegacySessions      int32 `json:"legacy_sessions"`
	ActiveSessions      int32 `json:"active_sessions"`
	SoftDeletedSessions int32 `json:"soft_deleted_sessions"`
	ExternalSeries      int32 `json:"external_series"`
	StudentsImported    int32 `json:"students_imported"`
	MappedRooms         int32 `json:"mapped_rooms"`
	MappedTeachers      int32 `json:"mapped_teachers"`
	MappedSubjects      int32 `json:"mapped_subjects"`
}

type legacyAuditRunsDTO struct {
	CompletedRuns            int32   `json:"completed_runs"`
	EntitiesParsed           int64   `json:"entities_parsed"`
	EntitiesApplied          int64   `json:"entities_applied"`
	ParseFailures            int64   `json:"parse_failures"`
	ReconciliationMismatches int64   `json:"reconciliation_mismatches"`
	LastSuccessfulAt         *string `json:"last_successful_at"`
}

type legacyAuditBucketDTO struct {
	Cause      string `json:"cause"`
	EntityType string `json:"entity_type"`
	Key        string `json:"key"`
	Count      int32  `json:"count"`
}

type legacyAuditSkipsDTO struct {
	SessionsSkippedTotal int32                  `json:"sessions_skipped_total"`
	SessionsSkippedOpen  int32                  `json:"sessions_skipped_open"`
	CoursesSkippedTotal  int32                  `json:"courses_skipped_total"`
	CoursesSkippedOpen   int32                  `json:"courses_skipped_open"`
	PartialSnapshots     int32                  `json:"partial_snapshots"`
	ByCause              []legacyAuditBucketDTO `json:"by_cause"`
}

type skippedSessionDTO struct {
	LegacyScheduleID string  `json:"legacy_schedule_id"`
	Date             *string `json:"date"`
	Begin            *string `json:"begin"`
	End              *string `json:"end"`
	Classroom        *string `json:"classroom"`
	ConflictType     string  `json:"conflict_type"`
	Category         string  `json:"category"`
	Message          *string `json:"message"`
	Status           string  `json:"status"`
	CreatedAt        *string `json:"created_at"`
	CourseID         *string `json:"course_id"`
	CourseCode       *string `json:"course_code"`
	CourseName       *string `json:"course_name"`
	LegacyCourseID   string  `json:"legacy_course_id"`
}

type skippedCourseDTO struct {
	ReasonKind    string  `json:"reason_kind"`
	ExternalID    string  `json:"external_id"`
	ConflictType  string  `json:"conflict_type"`
	ErrorCategory *string `json:"error_category"`
	Message       *string `json:"message"`
	Status        string  `json:"status"`
	CreatedAt     *string `json:"created_at"`
	CourseID      *string `json:"course_id"`
	CourseCode    *string `json:"course_code"`
	CourseName    *string `json:"course_name"`
}

type deadLetterDTO struct {
	ID            string  `json:"id"`
	JobType       string  `json:"job_type"`
	EntityType    *string `json:"entity_type"`
	ExternalID    *string `json:"external_id"`
	ErrorCategory *string `json:"error_category"`
	LastError     string  `json:"last_error"`
	Attempts      int32   `json:"attempts"`
	CreatedAt     *string `json:"created_at"`
}

type legacyAuditDTO struct {
	GeneratedAt     string               `json:"generated_at"`
	Totals          legacyAuditTotalsDTO `json:"totals"`
	Runs            legacyAuditRunsDTO   `json:"runs"`
	Skips           legacyAuditSkipsDTO  `json:"skips"`
	SkippedSessions []skippedSessionDTO  `json:"skipped_sessions"`
	SkippedCourses  []skippedCourseDTO   `json:"skipped_courses"`
	DeadLetters     []deadLetterDTO      `json:"dead_letters"`
}

func controlToDTO(control sqldb.LegacySyncControl) controlDTO {
	return controlDTO{
		DetectionEnabled: control.DetectionEnabled,
		FetchEnabled:     control.FetchEnabled,
		ApplyEnabled:     control.ApplyEnabled,
		StudentEnabled:   control.StudentEnabled,
		TombstoneEnabled: control.TombstoneEnabled,
		RealtimeEnabled:  control.RealtimeEnabled,
		ShadowMode:       control.ShadowMode,
		UpdatedAt:        timePtr(control.UpdatedAt),
	}
}

func runToDTO(run sqldb.LegacySyncRun) runDTO {
	return runDTO{
		ID:                       uuidString(run.ID),
		Mode:                     run.Mode,
		Status:                   run.Status,
		StartedAt:                timePtr(run.StartedAt),
		CompletedAt:              timePtr(run.CompletedAt),
		PagesRequested:           run.PagesRequested,
		EntitiesParsed:           run.EntitiesParsed,
		EntitiesChanged:          run.EntitiesChanged,
		EntitiesApplied:          run.EntitiesApplied,
		ParseFailures:            run.ParseFailures,
		ReconciliationMismatches: run.ReconciliationMismatches,
		SourceLatencyMs:          int32Ptr(run.SourceLatencyMs),
		LastError:                textPtr(run.LastError),
	}
}

func progressToDTO(progress sqldb.LegacySyncRunProgress) progressDTO {
	return progressDTO{
		Phase:             progress.Phase,
		CurrentEntity:     textPtr(progress.CurrentEntity),
		ProcessedEntities: progress.ProcessedEntities,
		TotalEntities:     progress.TotalEntities,
		ChangedEntities:   progress.ChangedEntities,
		AppliedEntities:   progress.AppliedEntities,
		Failures:          progress.Failures,
		UpdatedAt:         timePtr(progress.UpdatedAt),
	}
}

func conflictToDTO(conflict sqldb.LegacySyncConflict) conflictDTO {
	return conflictDTO{
		ID:            uuidString(conflict.ID),
		EntityType:    conflict.EntityType,
		ExternalID:    conflict.ExternalID,
		ConflictType:  conflict.ConflictType,
		Category:      conflict.Category,
		Message:       textPtr(conflict.Message),
		SourcePayload: jsonbPtr(conflict.SourcePayload),
		LocalPayload:  jsonbPtr(conflict.LocalPayload),
		Status:        conflict.Status,
		CreatedAt:     timePtr(conflict.CreatedAt),
		ResolvedAt:    timePtr(conflict.ResolvedAt),
	}
}

func jobToDTO(job sqldb.LegacySyncJob) jobDTO {
	return jobDTO{
		ID:          uuidString(job.ID),
		JobType:     job.JobType,
		EntityType:  textPtr(job.EntityType),
		ExternalID:  textPtr(job.ExternalID),
		UniqueKey:   textPtr(job.UniqueKey),
		Priority:    job.Priority,
		Status:      job.Status,
		DeadlineAt:  timePtr(job.DeadlineAt),
		Attempt:     job.Attempt,
		MaxAttempts: job.MaxAttempts,
		LockedUntil: timePtr(job.LockedUntil),
		HeartbeatAt: timePtr(job.HeartbeatAt),
		RunAfter:    timePtr(job.RunAfter),
		LastError:   textPtr(job.LastError),
		CreatedAt:   timePtr(job.CreatedAt),
		UpdatedAt:   timePtr(job.UpdatedAt),
	}
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return value.String()
}

func timePtr(value pgtype.Timestamptz) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func int32Ptr(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
}

func jsonbPtr(value []byte) *string {
	if len(value) == 0 {
		return nil
	}
	rendered := string(value)
	return &rendered
}
