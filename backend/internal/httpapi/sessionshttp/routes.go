package sessionshttp

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/scheduling"
)

func mustUUIDStringOrEmpty(a httpadapter.Adapter, u pgtype.UUID) string {
	s, err := a.UUIDString(u)
	if err != nil {
		return ""
	}
	return s
}

func uuidOrNull(a httpadapter.Adapter, u pgtype.UUID) any {
	if !u.Valid {
		return nil
	}
	s, err := a.UUIDString(u)
	if err != nil {
		return nil
	}
	return s
}

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("GET /api/v1/sessions", s.handleSessionsListByRange)
	mux.HandleFunc("POST /api/v1/sessions", s.handleSessionsCreate)
	mux.HandleFunc("DELETE /api/v1/sessions/{id}", s.handleSessionsDelete)
	mux.HandleFunc("PATCH /api/v1/sessions/{id}", s.handleSessionEditOccurrence)
	mux.HandleFunc("GET /api/v1/sessions/{id}/attendance", s.handleSessionAttendanceList)
	mux.HandleFunc("PUT /api/v1/sessions/{id}/attendance", s.handleSessionAttendanceUpsert)
	mux.HandleFunc("DELETE /api/v1/sessions/{id}/attendance/{student_id}", s.handleSessionAttendanceDelete)
}

func (s *server) handleSessionsListByRange(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}

	startRaw := r.URL.Query().Get("start")
	endRaw := r.URL.Query().Get("end")
	startAt, err := s.a.ParseTimestamptz(startRaw)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_start", "Invalid start (RFC3339)")
		return
	}
	endAt, err := s.a.ParseTimestamptz(endRaw)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_end", "Invalid end (RFC3339)")
		return
	}
	if !startAt.Valid || !endAt.Valid || !endAt.Time.After(startAt.Time) {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_range", "Invalid time range")
		return
	}
	if endAt.Time.Sub(startAt.Time) > 14*24*time.Hour {
		s.a.WriteErr(w, http.StatusBadRequest, "range_too_large", "Date range must be 14 days or less")
		return
	}

	items, err := s.deps.Q.SessionListActiveByRange(r.Context(), sqldb.SessionListActiveByRangeParams{
		RangeEnd:   endAt,
		RangeStart: startAt,
	})
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type sessionDTO struct {
		ID        string  `json:"id"`
		SeriesID  *string `json:"series_id"`
		CourseID  string  `json:"course_id"`
		RoomID    *string `json:"room_id"`
		TeacherID string  `json:"teacher_id"`
		StartAt   string  `json:"start_at"`
		EndAt     string  `json:"end_at"`
		Version   int32   `json:"version"`
	}
	out := make([]sessionDTO, 0, len(items))
	for _, ss := range items {
		sid, err := s.a.UUIDString(ss.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		cid, err := s.a.UUIDString(ss.CourseID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		var rid *string
		if ss.RoomID.Valid {
			v, err := s.a.UUIDString(ss.RoomID)
			if err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return
			}
			rid = &v
		}
		tid, err := s.a.UUIDString(ss.TeacherID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		startS, _ := s.a.TimeString(ss.StartAt)
		endS, _ := s.a.TimeString(ss.EndAt)
		var seriesID *string
		if ss.SeriesID.Valid {
			v, err := s.a.UUIDString(ss.SeriesID)
			if err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return
			}
			seriesID = &v
		}
		out = append(out, sessionDTO{ID: sid, SeriesID: seriesID, CourseID: cid, RoomID: rid, TeacherID: tid, StartAt: startS, EndAt: endS, Version: ss.Version})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleSessionsCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	var body struct {
		SeriesID  *string `json:"series_id"`
		CourseID  string  `json:"course_id"`
		RoomID    *string `json:"room_id"`
		TeacherID string  `json:"teacher_id"`
		StartAt   string  `json:"start_at"`
		EndAt     string  `json:"end_at"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	var series pgtype.UUID
	if body.SeriesID != nil && *body.SeriesID != "" {
		sid, err := s.a.ParseUUID(*body.SeriesID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_series_id", "Invalid series_id")
			return
		}
		series = sid
	}

	courseID, err := s.a.ParseUUID(body.CourseID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_course_id", "Invalid course_id")
		return
	}
	var roomID pgtype.UUID
	if body.RoomID != nil {
		if *body.RoomID == "" {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_room_id", "Invalid room_id")
			return
		}
		rid, err := s.a.ParseUUID(*body.RoomID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_room_id", "Invalid room_id")
			return
		}
		roomID = rid
	}
	teacherID, err := s.a.ParseUUID(body.TeacherID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_teacher_id", "Invalid teacher_id")
		return
	}
	startAt, err := s.a.ParseTimestamptz(body.StartAt)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_start", "Invalid start_at")
		return
	}
	endAt, err := s.a.ParseTimestamptz(body.EndAt)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_end", "Invalid end_at")
		return
	}
	if !endAt.Time.After(startAt.Time) {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_range", "end_at must be after start_at")
		return
	}

	var seriesID *pgtype.UUID
	if series.Valid {
		tmp := series
		seriesID = &tmp
	}

	s.a.WithIdempotentTx(w, r, user.ID, "sessions", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		item, err := s.deps.Scheduling.CreateSessionTx(r.Context(), tx, qtx, scheduling.CreateSessionParams{
			SeriesID:  seriesID,
			CourseID:  courseID,
			RoomID:    roomID,
			TeacherID: teacherID,
			StartAt:   startAt,
			EndAt:     endAt,
		})
		if err != nil {
			var se *scheduling.Err
			if errors.As(err, &se) {
				s.a.WriteErrDetails(w, http.StatusConflict, se.Code, se.Message, se.Details)
				return 0, nil, err
			}
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		idStr, err := s.a.UUIDString(item.SessionID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
		if _, aErr := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "session.create",
			Payload:     map[string]any{"session_id": idStr, "course_id": body.CourseID, "teacher_id": body.TeacherID, "room_id": body.RoomID, "start_at": body.StartAt, "end_at": body.EndAt},
		}); aErr != nil {
			s.deps.Log.Error("audit insert failed", "error", aErr, "session_id", idStr)
		}
		return http.StatusCreated, map[string]any{"id": idStr}, nil
	})
}

func (s *server) handleSessionsDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	var body struct {
		ExpectedVersion *int32 `json:"expected_version"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.ExpectedVersion == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_expected_version", "expected_version required")
		return
	}

	// Stale edit detection (read-only, outside tx).
	existing, err := s.deps.Q.SessionGetByID(r.Context(), id)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if existing.Version != *body.ExpectedVersion {
		dto := map[string]any{
			"id":         r.PathValue("id"),
			"series_id":  nil,
			"course_id":  mustUUIDStringOrEmpty(s.a, existing.CourseID),
			"room_id":    uuidOrNull(s.a, existing.RoomID),
			"teacher_id": mustUUIDStringOrEmpty(s.a, existing.TeacherID),
			"start_at":   existing.StartAt.Time.UTC().Format(time.RFC3339Nano),
			"end_at":     existing.EndAt.Time.UTC().Format(time.RFC3339Nano),
			"version":    existing.Version,
		}
		if existing.SeriesID.Valid {
			dto["series_id"] = mustUUIDStringOrEmpty(s.a, existing.SeriesID)
		}
		s.a.WriteErrDetails(w, http.StatusConflict, "stale_edit", "Stale edit", map[string]any{"current": dto})
		return
	}

	s.a.WithIdempotentTx(w, r, user.ID, "sessions", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if _, err := qtx.SessionSoftDelete(r.Context(), sqldb.SessionSoftDeleteParams{ID: id, Version: *body.ExpectedVersion}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.a.WriteErrDetails(w, http.StatusConflict, "stale_edit", "Stale edit", map[string]any{"message": "session already deleted or version mismatch"})
			return 0, nil, err
		}
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
		if _, aErr := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "session.soft_delete",
			Payload:     map[string]any{"id": r.PathValue("id"), "expected_version": *body.ExpectedVersion},
		}); aErr != nil {
			s.deps.Log.Error("audit insert failed", "error", aErr, "session_id", r.PathValue("id"))
		}
		return http.StatusOK, map[string]any{"ok": true}, nil
	})
}

func (s *server) handleSessionEditOccurrence(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	var body struct {
		StartAt         *string  `json:"start_at"`
		EndAt           *string  `json:"end_at"`
		CourseID        *string  `json:"course_id"`
		RoomID          **string `json:"room_id"`
		TeacherID       *string  `json:"teacher_id"`
		ExpectedVersion *int32   `json:"expected_version"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.ExpectedVersion == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_expected_version", "expected_version required")
		return
	}

	// Stale edit detection + past-session immutability check (read-only, outside tx).
	existing, err := s.deps.Q.SessionGetByID(r.Context(), id)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	serverNow := time.Now().UTC()
	if existing.EndAt.Valid && existing.EndAt.Time.UTC().Before(serverNow) {
		s.a.WriteErrDetails(w, http.StatusConflict, "past_session_immutable", "Past sessions are immutable", map[string]any{
			"server_now": serverNow.Format(time.RFC3339Nano),
			"end_at":     existing.EndAt.Time.UTC().Format(time.RFC3339Nano),
		})
		return
	}

	if existing.Version != *body.ExpectedVersion {
		dto := map[string]any{
			"id":         r.PathValue("id"),
			"series_id":  nil,
			"course_id":  mustUUIDStringOrEmpty(s.a, existing.CourseID),
			"room_id":    uuidOrNull(s.a, existing.RoomID),
			"teacher_id": mustUUIDStringOrEmpty(s.a, existing.TeacherID),
			"start_at":   existing.StartAt.Time.UTC().Format(time.RFC3339Nano),
			"end_at":     existing.EndAt.Time.UTC().Format(time.RFC3339Nano),
			"version":    existing.Version,
		}
		if existing.SeriesID.Valid {
			dto["series_id"] = mustUUIDStringOrEmpty(s.a, existing.SeriesID)
		}
		s.a.WriteErrDetails(w, http.StatusConflict, "stale_edit", "Stale edit", map[string]any{"current": dto})
		return
	}

	var (
		startAtPtr   *pgtype.Timestamptz
		endAtPtr     *pgtype.Timestamptz
		courseIDPtr  *pgtype.UUID
		roomIDPtr    *pgtype.UUID
		teacherIDPtr *pgtype.UUID
	)

	if body.StartAt != nil {
		parsed, err := s.a.ParseTimestamptz(*body.StartAt)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_start", "Invalid start_at")
			return
		}
		startAtPtr = &parsed
	}
	if body.EndAt != nil {
		parsed, err := s.a.ParseTimestamptz(*body.EndAt)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_end", "Invalid end_at")
			return
		}
		endAtPtr = &parsed
	}
	if body.CourseID != nil && *body.CourseID != "" {
		parsed, err := s.a.ParseUUID(*body.CourseID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_course_id", "Invalid course_id")
			return
		}
		courseIDPtr = &parsed
	}
	if body.TeacherID != nil && *body.TeacherID != "" {
		parsed, err := s.a.ParseUUID(*body.TeacherID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_teacher_id", "Invalid teacher_id")
			return
		}
		teacherIDPtr = &parsed
	}
	if body.RoomID != nil {
		if *body.RoomID == nil {
			parsed := pgtype.UUID{} // Valid=false => NULL
			roomIDPtr = &parsed
		} else if **body.RoomID != "" {
			parsed, err := s.a.ParseUUID(**body.RoomID)
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_room_id", "Invalid room_id")
				return
			}
			roomIDPtr = &parsed
		} else {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_room_id", "Invalid room_id")
			return
		}
	}

	s.a.WithIdempotentTx(w, r, user.ID, "sessions", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		item, err := s.deps.Scheduling.EditOccurrenceTimeTx(r.Context(), tx, qtx, scheduling.EditOccurrenceParams{
			SessionID:       id,
			StartAt:         startAtPtr,
			EndAt:           endAtPtr,
			CourseID:        courseIDPtr,
			RoomID:          roomIDPtr,
			TeacherID:       teacherIDPtr,
			ExpectedVersion: *body.ExpectedVersion,
		})
		if err != nil {
			var se *scheduling.Err
			if errors.As(err, &se) {
				s.a.WriteErrDetails(w, http.StatusConflict, se.Code, se.Message, se.Details)
				return 0, nil, err
			}
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		sid, err := s.a.UUIDString(item.SessionID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}

		actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
		if _, aErr := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "session.edit_occurrence",
			Payload:     map[string]any{"id": r.PathValue("id"), "start_at": body.StartAt, "end_at": body.EndAt, "course_id": body.CourseID, "room_id": body.RoomID, "teacher_id": body.TeacherID},
		}); aErr != nil {
			s.deps.Log.Error("audit insert failed", "error", aErr, "session_id", r.PathValue("id"))
		}

		// Re-fetch updated row to include in response.
		updated, err := qtx.SessionGetByID(r.Context(), id)
		if err != nil {
			s.deps.Log.Error("re-fetch after edit failed", "error", err, "session_id", r.PathValue("id"))
			return http.StatusOK, map[string]any{"id": sid}, nil
		}
		dto := map[string]any{
			"id":         sid,
			"series_id":  nil,
			"course_id":  mustUUIDStringOrEmpty(s.a, updated.CourseID),
			"room_id":    uuidOrNull(s.a, updated.RoomID),
			"teacher_id": mustUUIDStringOrEmpty(s.a, updated.TeacherID),
			"start_at":   updated.StartAt.Time.UTC().Format(time.RFC3339Nano),
			"end_at":     updated.EndAt.Time.UTC().Format(time.RFC3339Nano),
			"version":    updated.Version,
		}
		if updated.SeriesID.Valid {
			dto["series_id"] = mustUUIDStringOrEmpty(s.a, updated.SeriesID)
		}
		return http.StatusOK, map[string]any{"session": dto}, nil
	})
}

func (s *server) handleSessionAttendanceList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	sessionID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	items, err := s.deps.Q.SessionAttendanceList(r.Context(), sessionID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type itemDTO struct {
		StudentID string `json:"student_id"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
	}
	out := make([]itemDTO, 0, len(items))
	for _, it := range items {
		sid, err := s.a.UUIDString(it.StudentID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		cs, _ := s.a.TimeString(it.CreatedAt)
		out = append(out, itemDTO{StudentID: sid, Status: it.Status, CreatedAt: cs})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleSessionAttendanceUpsert(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	sessionID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	var body struct {
		StudentID string `json:"student_id"`
		Status    string `json:"status"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	studentID, err := s.a.ParseUUID(body.StudentID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_student_id", "Invalid student_id")
		return
	}
	if body.Status != "included" && body.Status != "excluded" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_status", "status must be included or excluded")
		return
	}
	sessID, _ := s.a.UUIDString(sessionID)
	stuID, _ := s.a.UUIDString(studentID)

	s.a.WithIdempotentTx(w, r, actor.ID, "sessions", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.SessionAttendanceUpsert(r.Context(), sqldb.SessionAttendanceUpsertParams{
			SessionID: sessionID,
			StudentID: studentID,
			Status:    body.Status,
		}); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		actorID := pgtype.UUID{Bytes: actor.ID, Valid: true}
		_, _ = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "session_attendance.upsert",
			Payload:     map[string]any{"session_id": sessID, "student_id": stuID, "status": body.Status},
		})
		return http.StatusOK, map[string]any{"ok": true}, nil
	})
}

func (s *server) handleSessionAttendanceDelete(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	sessionID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	studentID, err := s.a.ParseUUID(r.PathValue("student_id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_student_id", "Invalid student_id")
		return
	}
	sessID, _ := s.a.UUIDString(sessionID)
	stuID, _ := s.a.UUIDString(studentID)

	s.a.WithIdempotentTx(w, r, actor.ID, "sessions", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.SessionAttendanceDelete(r.Context(), sqldb.SessionAttendanceDeleteParams{
			SessionID: sessionID,
			StudentID: studentID,
		}); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		actorID := pgtype.UUID{Bytes: actor.ID, Valid: true}
		_, _ = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "session_attendance.delete",
			Payload:     map[string]any{"session_id": sessID, "student_id": stuID},
		})
		return http.StatusOK, map[string]any{"ok": true}, nil
	})
}
