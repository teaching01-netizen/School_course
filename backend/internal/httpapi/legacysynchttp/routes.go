package legacysynchttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}
	mux.HandleFunc("GET /api/v1/admin/legacy-sync/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/admin/legacy-sync/audit", s.handleAudit)
	mux.HandleFunc("GET /api/v1/admin/legacy-sync/runs", s.handleRuns)
	mux.HandleFunc("GET /api/v1/admin/legacy-sync/conflicts", s.handleConflicts)
	mux.HandleFunc("POST /api/v1/admin/legacy-sync/conflicts/{id}/resolve", s.handleConflictResolve)
	mux.HandleFunc("POST /api/v1/admin/legacy-sync/conflicts/{id}/ignore", s.handleConflictIgnore)
	mux.HandleFunc("GET /api/v1/admin/legacy-sync/jobs", s.handleJobs)
	mux.HandleFunc("POST /api/v1/admin/legacy-sync/refresh", s.handleRefresh)
	mux.HandleFunc("POST /api/v1/admin/legacy-sync/pause", s.handlePause)
	mux.HandleFunc("POST /api/v1/admin/legacy-sync/resume", s.handleResume)
	mux.HandleFunc("POST /api/v1/admin/legacy-sync/shadow", s.handleShadow)
	mux.HandleFunc("POST /api/v1/admin/legacy-sync/student-import", s.handleStudentImport)
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	control, err := s.deps.Q.LegacySyncControlGet(r.Context())
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	counts, err := s.deps.Q.LegacyJobCounts(r.Context())
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	runs, err := s.deps.Q.SyncRunListRecent(r.Context(), 20)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	conflicts, err := s.deps.Q.ConflictListOpen(r.Context())
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	var latest *runDTO
	var lastSuccessfulAt *string
	for _, run := range runs {
		if latest == nil {
			value := runToDTO(run)
			progress, progressErr := s.deps.Q.LegacySyncRunProgressGet(r.Context(), run.ID)
			if progressErr == nil {
				progressValue := progressToDTO(progress)
				value.Progress = &progressValue
			} else if !errors.Is(progressErr, pgx.ErrNoRows) {
				s.writeDBError(w, progressErr)
				return
			}
			latest = &value
		}
		if run.Status == "completed" && lastSuccessfulAt == nil {
			lastSuccessfulAt = timePtr(run.CompletedAt)
		}
	}
	paused := !control.DetectionEnabled || !control.FetchEnabled || !control.ApplyEnabled
	status := "healthy"
	if paused {
		status = "paused"
	} else if control.ShadowMode {
		status = "shadow"
	} else if counts.Queued > 0 || counts.Running > 0 || (latest != nil && latest.Status == "running") {
		status = "syncing"
	} else if latest != nil && latest.Status == "failed" {
		status = "error"
	} else if latest == nil {
		status = "waiting"
	}
	var freshness *int64
	if lastSuccessfulAt != nil {
		if parsed, parseErr := time.Parse(time.RFC3339Nano, *lastSuccessfulAt); parseErr == nil {
			seconds := int64(time.Since(parsed).Seconds())
			if seconds < 0 {
				seconds = 0
			}
			freshness = &seconds
		}
	}
	s.a.WriteJSON(w, http.StatusOK, healthDTO{
		Status:           status,
		Paused:           paused,
		ShadowMode:       control.ShadowMode,
		Control:          controlToDTO(control),
		Queue:            queueDTO{Queued: counts.Queued, Running: counts.Running, Completed: counts.Completed, Dead: counts.Dead},
		OpenConflicts:    len(conflicts),
		LatestRun:        latest,
		LastSuccessfulAt: lastSuccessfulAt,
		FreshnessSeconds: freshness,
	})
}

// handleAudit serves the migration audit snapshot: how much data the legacy
// sync brought over, what it skipped (schedule rows and courses), and the
// dead-letter ledger. Read-only aggregation of the sync tables.
func (s *server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	totals, err := s.deps.Q.LegacyAuditTotals(r.Context())
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	runs, err := s.deps.Q.LegacyAuditRuns(r.Context())
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	skips, err := s.deps.Q.LegacyAuditSkipCounts(r.Context())
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	buckets, err := s.deps.Q.LegacyAuditSkipsByCause(r.Context())
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	skippedSessions, err := s.deps.Q.LegacyAuditSkippedSessions(r.Context(), 250)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	skippedCourses, err := s.deps.Q.LegacyAuditSkippedCourses(r.Context(), 250)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	deadLetters, err := s.deps.Q.LegacyAuditDeadLetters(r.Context(), 100)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	skippedSessionDTOs := make([]skippedSessionDTO, 0, len(skippedSessions))
	for _, row := range skippedSessions {
		skippedSessionDTOs = append(skippedSessionDTOs, skippedSessionToDTO(row))
	}
	skippedCourseDTOs := make([]skippedCourseDTO, 0, len(skippedCourses))
	for _, row := range skippedCourses {
		skippedCourseDTOs = append(skippedCourseDTOs, skippedCourseToDTO(row))
	}
	deadLetterDTOs := make([]deadLetterDTO, 0, len(deadLetters))
	for _, row := range deadLetters {
		deadLetterDTOs = append(deadLetterDTOs, deadLetterToDTO(row))
	}
	s.a.WriteJSON(w, http.StatusOK, legacyAuditDTO{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Totals: legacyAuditTotalsDTO{
			LinkedCourses:       totals.LinkedCourses,
			ArchivedCourses:     totals.ArchivedCourses,
			SyncedCourses:       totals.SyncedCourses,
			LegacySessions:      totals.LegacySessions,
			ActiveSessions:      totals.ActiveSessions,
			SoftDeletedSessions: totals.SoftDeletedSessions,
			ExternalSeries:      totals.ExternalSeries,
			StudentsImported:    totals.StudentsImported,
			MappedRooms:         totals.MappedRooms,
			MappedTeachers:      totals.MappedTeachers,
			MappedSubjects:      totals.MappedSubjects,
		},
		Runs: legacyAuditRunsDTO{
			CompletedRuns:            runs.CompletedRuns,
			EntitiesParsed:           runs.EntitiesParsed,
			EntitiesApplied:          runs.EntitiesApplied,
			ParseFailures:            runs.ParseFailures,
			ReconciliationMismatches: runs.ReconciliationMismatches,
			LastSuccessfulAt:         timePtr(runs.LastSuccessfulAt),
		},
		Skips: legacyAuditSkipsDTO{
			SessionsSkippedTotal: skips.SessionsSkippedTotal,
			SessionsSkippedOpen:  skips.SessionsSkippedOpen,
			CoursesSkippedTotal:  skips.CoursesSkippedTotal,
			CoursesSkippedOpen:   skips.CoursesSkippedOpen,
			PartialSnapshots:     skips.PartialSnapshots,
			ByCause:              bucketsToDTO(buckets),
		},
		SkippedSessions: skippedSessionDTOs,
		SkippedCourses:  skippedCourseDTOs,
		DeadLetters:     deadLetterDTOs,
	})
}

func bucketsToDTO(buckets []sqldb.LegacyAuditSkipBucket) []legacyAuditBucketDTO {
	result := make([]legacyAuditBucketDTO, 0, len(buckets))
	for _, bucket := range buckets {
		result = append(result, legacyAuditBucketDTO{
			Cause:      bucket.Cause,
			EntityType: bucket.EntityType,
			Key:        bucket.Key,
			Count:      bucket.Count,
		})
	}
	return result
}

func skippedSessionToDTO(row sqldb.LegacyAuditSkippedSession) skippedSessionDTO {
	return skippedSessionDTO{
		LegacyScheduleID: row.LegacyScheduleID,
		Date:             textPtr(row.Date),
		Begin:            textPtr(row.Begin),
		End:              textPtr(row.End),
		Classroom:        textPtr(row.Classroom),
		ConflictType:     row.ConflictType,
		Category:         row.Category,
		Message:          textPtr(row.Message),
		Status:           row.Status,
		CreatedAt:        timePtr(row.CreatedAt),
		CourseID:         uuidStrPtr(row.CourseID),
		CourseCode:       textPtr(row.CourseCode),
		CourseName:       textPtr(row.CourseName),
		LegacyCourseID:   row.LegacyCourseID,
	}
}

func skippedCourseToDTO(row sqldb.LegacyAuditSkippedCourse) skippedCourseDTO {
	return skippedCourseDTO{
		ReasonKind:    row.ReasonKind,
		ExternalID:    row.ExternalID,
		ConflictType:  row.ConflictType,
		ErrorCategory: textPtr(row.ErrorCategory),
		Message:       textPtr(row.Message),
		Status:        row.Status,
		CreatedAt:     timePtr(row.CreatedAt),
		CourseID:      uuidStrPtr(row.CourseID),
		CourseCode:    textPtr(row.CourseCode),
		CourseName:    textPtr(row.CourseName),
	}
}

func deadLetterToDTO(row sqldb.LegacySyncDeadLetter) deadLetterDTO {
	return deadLetterDTO{
		ID:            uuidString(row.ID),
		JobType:       row.JobType,
		EntityType:    textPtr(row.EntityType),
		ExternalID:    textPtr(row.ExternalID),
		ErrorCategory: textPtr(row.ErrorCategory),
		LastError:     row.LastError,
		Attempts:      row.Attempts,
		CreatedAt:     timePtr(row.CreatedAt),
	}
}

func uuidStrPtr(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.String()
	return &formatted
}

func (s *server) handleRuns(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	rows, err := s.deps.Q.SyncRunListRecent(r.Context(), limitFromRequest(r, 20, 100))
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	result := make([]runDTO, 0, len(rows))
	for _, row := range rows {
		result = append(result, runToDTO(row))
	}
	s.a.WriteJSON(w, http.StatusOK, result)
}

func (s *server) handleConflicts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	rows, err := s.deps.Q.ConflictListOpen(r.Context())
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	result := make([]conflictDTO, 0, len(rows))
	for _, row := range rows {
		result = append(result, conflictToDTO(row))
	}
	s.a.WriteJSON(w, http.StatusOK, result)
}

func (s *server) handleConflictResolve(w http.ResponseWriter, r *http.Request) {
	s.handleConflictSetStatus(w, r, "resolved")
}

func (s *server) handleConflictIgnore(w http.ResponseWriter, r *http.Request) {
	s.handleConflictSetStatus(w, r, "ignored")
}

// handleConflictSetStatus closes an open conflict as resolved or ignored.
// The status change is a side effect, so the idempotency policy applies;
// a conflict that is not open anymore is a 404 (already handled).
func (s *server) handleConflictSetStatus(w http.ResponseWriter, r *http.Request, status string) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_conflict_id", "Invalid conflict id")
		return
	}
	if !s.a.WithIdempotentTx(w, r, user.ID, "legacy-sync", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		conflict, err := s.deps.Q.WithTx(tx).ConflictSetStatus(r.Context(), sqldb.ConflictSetStatusParams{ID: id, Status: status})
		if errors.Is(err, pgx.ErrNoRows) {
			s.a.WriteErr(w, http.StatusNotFound, "conflict_not_open", "Conflict is not open")
			return 0, nil, err
		}
		if err != nil {
			s.writeDBError(w, err)
			return 0, nil, err
		}
		return http.StatusOK, conflictToDTO(conflict), nil
	}) {
		return
	}
}

func (s *server) handleJobs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	rows, err := s.deps.Q.LegacyJobListRecent(r.Context(), limitFromRequest(r, 50, 200))
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	result := make([]jobDTO, 0, len(rows))
	for _, row := range rows {
		result = append(result, jobToDTO(row))
	}
	s.a.WriteJSON(w, http.StatusOK, result)
}

func (s *server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	// Enqueueing a job is a side effect, so the idempotency policy applies:
	// the accepted response is persisted and replayed on retries.
	if !s.a.WithIdempotentTx(w, r, user.ID, "legacy-sync", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		var request refreshRequest
		if err := s.a.DecodeJSON(w, r, &request); err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
			return 0, nil, err
		}
		job, err := validateRefreshRequest(request)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_refresh", err.Error())
			return 0, nil, err
		}
		payload, _ := json.Marshal(map[string]string{"requested_by": "admin"})
		now := time.Now().UTC()
		created, err := s.deps.Q.WithTx(tx).LegacyJobEnqueue(r.Context(), sqldb.LegacyJobEnqueueParams{
			JobType:     job.JobType,
			EntityType:  pgtype.Text{String: job.EntityType, Valid: true},
			ExternalID:  pgtype.Text{String: job.ExternalID, Valid: job.ExternalID != ""},
			Payload:     string(payload),
			UniqueKey:   pgtype.Text{String: job.UniqueKey, Valid: true},
			Priority:    job.Priority,
			DeadlineAt:  pgtype.Timestamptz{Time: now.Add(10 * time.Minute), Valid: true},
			MaxAttempts: 5,
			RunAfter:    pgtype.Timestamptz{Time: now, Valid: true},
		})
		if err != nil {
			s.writeDBError(w, err)
			return 0, nil, err
		}
		return http.StatusAccepted, jobToDTO(created), nil
	}) {
		return
	}
}

func (s *server) handlePause(w http.ResponseWriter, r *http.Request) {
	s.updateControl(w, r, false)
}

func (s *server) handleResume(w http.ResponseWriter, r *http.Request) {
	s.updateControl(w, r, true)
}

func (s *server) handleShadow(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	if !s.a.WithIdempotentTx(w, r, user.ID, "legacy-sync", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		var request struct {
			Enabled *bool `json:"enabled"`
		}
		if err := s.a.DecodeJSON(w, r, &request); err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
			return 0, nil, err
		}
		if request.Enabled == nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_shadow", "enabled is required")
			return 0, nil, fmt.Errorf("enabled is required")
		}
		// Only shadow_mode is supplied; every other flag stays untouched
		// because COALESCE keeps the stored value for NULL parameters.
		control, err := s.deps.Q.WithTx(tx).LegacySyncControlSet(r.Context(), sqldb.LegacySyncControlSetParams{
			ShadowMode: pgtype.Bool{Bool: *request.Enabled, Valid: true},
		})
		if err != nil {
			s.writeDBError(w, err)
			return 0, nil, err
		}
		return http.StatusOK, controlToDTO(control), nil
	}) {
		return
	}
}

func (s *server) handleStudentImport(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	if !s.a.WithIdempotentTx(w, r, user.ID, "legacy-sync", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		var request struct {
			Enabled *bool `json:"enabled"`
		}
		if err := s.a.DecodeJSON(w, r, &request); err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
			return 0, nil, err
		}
		if request.Enabled == nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_student_import", "enabled is required")
			return 0, nil, fmt.Errorf("enabled is required")
		}
		// Only student_enabled is supplied; every other flag stays untouched
		// because COALESCE keeps the stored value for NULL parameters.
		control, err := s.deps.Q.WithTx(tx).LegacySyncControlSet(r.Context(), sqldb.LegacySyncControlSetParams{
			StudentEnabled: pgtype.Bool{Bool: *request.Enabled, Valid: true},
		})
		if err != nil {
			s.writeDBError(w, err)
			return 0, nil, err
		}
		return http.StatusOK, controlToDTO(control), nil
	}) {
		return
	}
}

func (s *server) updateControl(w http.ResponseWriter, r *http.Request, enabled bool) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	if !s.a.WithIdempotentTx(w, r, user.ID, "legacy-sync", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		control, err := s.deps.Q.WithTx(tx).LegacySyncControlSet(r.Context(), sqldb.LegacySyncControlSetParams{
			DetectionEnabled: pgtype.Bool{Bool: enabled, Valid: true},
			FetchEnabled:     pgtype.Bool{Bool: enabled, Valid: true},
			ApplyEnabled:     pgtype.Bool{Bool: enabled, Valid: true},
			TombstoneEnabled: pgtype.Bool{Bool: false, Valid: true},
		})
		if err != nil {
			s.writeDBError(w, err)
			return 0, nil, err
		}
		return http.StatusOK, controlToDTO(control), nil
	}) {
		return
	}
}

func (s *server) writeDBError(w http.ResponseWriter, err error) {
	status, code, message := s.a.ClassifyDBErr(err)
	s.a.WriteErr(w, status, code, message)
}
