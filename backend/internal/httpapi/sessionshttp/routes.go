package sessionshttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/realtime"
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

// roomIDFieldResult is the parsed intent of a room_id JSON field.
type roomIDFieldResult struct {
	clear bool         // JSON null: remove the room assignment.
	set   *pgtype.UUID // non-nil: assign the given room.
}

// parseRoomIDField interprets a raw room_id JSON field with three distinct
// states: absent (no change), null (clear the room), or a UUID string (assign
// the room). encoding/json collapses both "absent" and "null" into a nil
// **string, so a bare **string field cannot express "clear" — it silently
// treated room_id:null as "no change" and reported a successful no-op update.
// json.RawMessage preserves the raw tokens so the intent survives decoding.
func (s *server) parseRoomIDField(raw json.RawMessage) (roomIDFieldResult, error) {
	if len(raw) == 0 { // field absent
		return roomIDFieldResult{}, nil
	}
	if string(raw) == "null" {
		return roomIDFieldResult{clear: true}, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return roomIDFieldResult{}, fmt.Errorf("room_id must be a uuid string, null, or omitted")
	}
	if value == "" { // empty string: no change (legacy contract)
		return roomIDFieldResult{}, nil
	}
	parsed, err := s.a.ParseUUID(value)
	if err != nil {
		return roomIDFieldResult{}, fmt.Errorf("invalid room_id")
	}
	return roomIDFieldResult{set: &parsed}, nil
}

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
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

func (s *server) publishSessionUpdated(id string) {
	if s.deps.Realtime == nil || id == "" {
		return
	}
	s.deps.Realtime.Publish("sessions:all", realtime.Event{Type: "session.updated", ID: id})
}

func (s *server) sessionDTOFromFields(w http.ResponseWriter, id, seriesID, courseID, roomID, teacherID pgtype.UUID, startAt, endAt pgtype.Timestamptz, version int32) (sessionDTO, bool) {
	sid, err := s.a.UUIDString(id)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return sessionDTO{}, false
	}
	cid, err := s.a.UUIDString(courseID)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return sessionDTO{}, false
	}
	var rid *string
	if roomID.Valid {
		v, err := s.a.UUIDString(roomID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return sessionDTO{}, false
		}
		rid = &v
	}
	tid, err := s.a.UUIDString(teacherID)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return sessionDTO{}, false
	}
	startS, _ := s.a.TimeString(startAt)
	endS, _ := s.a.TimeString(endAt)
	var seriesIDOut *string
	if seriesID.Valid {
		v, err := s.a.UUIDString(seriesID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return sessionDTO{}, false
		}
		seriesIDOut = &v
	}
	return sessionDTO{ID: sid, SeriesID: seriesIDOut, CourseID: cid, RoomID: rid, TeacherID: tid, StartAt: startS, EndAt: endS, Version: version}, true
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("GET /api/v1/sessions", s.handleSessionsListByRange)
	mux.HandleFunc("POST /api/v1/sessions", s.handleSessionsCreate)
	mux.HandleFunc("DELETE /api/v1/sessions/{id}", s.handleSessionsDelete)
	mux.HandleFunc("PATCH /api/v1/sessions/{id}", s.handleSessionEditOccurrence)
	mux.HandleFunc("POST /api/v1/sessions/bulk-update", s.handleSessionsBulkUpdate)
	mux.HandleFunc("GET /api/v1/sessions/{id}/attendance", s.handleSessionAttendanceList)
	mux.HandleFunc("PUT /api/v1/sessions/{id}/attendance", s.handleSessionAttendanceUpsert)
	mux.HandleFunc("DELETE /api/v1/sessions/{id}/attendance/{student_id}", s.handleSessionAttendanceDelete)
}

func (s *server) handleSessionsListByRange(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}

	if idsRaw := strings.TrimSpace(r.URL.Query().Get("ids")); idsRaw != "" {
		values := strings.Split(idsRaw, ",")
		if len(values) > 100 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_ids", "No more than 100 session IDs can be requested")
			return
		}
		out := make([]sessionDTO, 0, len(values))
		for _, raw := range values {
			id, err := s.a.ParseUUID(strings.TrimSpace(raw))
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_ids", "Invalid session ID")
				return
			}
			row, err := s.deps.Q.SessionGetByID(r.Context(), id)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return
			}
			if row.DeletedAt.Valid {
				continue
			}
			dto, ok := s.sessionDTOFromFields(w, row.ID, row.SeriesID, row.CourseID, row.RoomID, row.TeacherID, row.StartAt, row.EndAt, row.Version)
			if !ok {
				return
			}
			out = append(out, dto)
		}
		s.a.WriteJSON(w, http.StatusOK, out)
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
	out := make([]sessionDTO, 0, len(items))
	for _, ss := range items {
		dto, ok := s.sessionDTOFromFields(w, ss.ID, ss.SeriesID, ss.CourseID, ss.RoomID, ss.TeacherID, ss.StartAt, ss.EndAt, ss.Version)
		if !ok {
			return
		}
		out = append(out, dto)
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

	var createdID string
	if s.a.WithSerializableIdempotentTx(w, r, user.ID, "sessions", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
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
				return scheduling.HTTPStatusForErr(se), map[string]any{
					"code":    se.Code,
					"message": se.Message,
					"details": se.Details,
				}, err
			}
			// Infrastructure error — return nil body so runner uses ClassifyDBErr
			return 0, nil, err
		}
		idStr, err := s.a.UUIDString(item.SessionID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		createdID = idStr
		actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
		if _, aErr := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "session.create",
			Payload:     map[string]any{"session_id": idStr, "course_id": body.CourseID, "teacher_id": body.TeacherID, "room_id": body.RoomID, "start_at": body.StartAt, "end_at": body.EndAt},
		}); aErr != nil {
			s.deps.Log.Error("audit insert failed", "error", aErr, "session_id", idStr)
		}
		return http.StatusCreated, map[string]any{"id": idStr, "warnings": item.Warnings}, nil
	}) {
		s.publishSessionUpdated(createdID)
	}
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

	deletedID := r.PathValue("id")
	if s.a.WithIdempotentTx(w, r, user.ID, "sessions", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		existing, err := s.deps.Scheduling.DeleteSessionTx(r.Context(), qtx, id, *body.ExpectedVersion)
		if err != nil {
			var scheduleErr *scheduling.Err
			if errors.As(err, &scheduleErr) && scheduleErr.Code == "stale_edit" {
				dto := map[string]any{
					"id": r.PathValue("id"), "series_id": nil,
					"course_id": mustUUIDStringOrEmpty(s.a, existing.CourseID), "room_id": uuidOrNull(s.a, existing.RoomID),
					"teacher_id": mustUUIDStringOrEmpty(s.a, existing.TeacherID), "start_at": existing.StartAt.Time.UTC().Format(time.RFC3339Nano),
					"end_at": existing.EndAt.Time.UTC().Format(time.RFC3339Nano), "version": existing.Version,
				}
				if existing.SeriesID.Valid {
					dto["series_id"] = mustUUIDStringOrEmpty(s.a, existing.SeriesID)
				}
				s.a.WriteErrDetails(w, http.StatusConflict, "stale_edit", "Stale edit", map[string]any{"current": dto})
				return 0, nil, err
			}
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
		if _, aErr := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "session.delete",
			Payload:     map[string]any{"id": r.PathValue("id"), "expected_version": *body.ExpectedVersion},
		}); aErr != nil {
			s.deps.Log.Error("audit insert failed", "error", aErr, "session_id", r.PathValue("id"))
		}
		return http.StatusOK, map[string]any{"ok": true}, nil
	}) {
		s.publishSessionUpdated(deletedID)
	}
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
		StartAt           *string         `json:"start_at"`
		EndAt             *string         `json:"end_at"`
		CourseID          *string         `json:"course_id"`
		RoomID            json.RawMessage `json:"room_id"`
		TeacherID         *string         `json:"teacher_id"`
		ExpectedVersion   *int32          `json:"expected_version"`
		AcknowledgeImpact *bool           `json:"acknowledge_impact"`
		ImpactReason      string          `json:"impact_reason"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.ExpectedVersion == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_expected_version", "expected_version required")
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
		res, err := s.parseRoomIDField(body.RoomID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_room_id", "Invalid room_id")
			return
		}
		if res.clear {
			parsed := pgtype.UUID{} // Valid=false => NULL
			roomIDPtr = &parsed
		} else if res.set != nil {
			roomIDPtr = res.set
		} else {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_room_id", "Invalid room_id")
			return
		}
	}

	var updatedID string
	if s.a.WithSerializableIdempotentTx(w, r, user.ID, "sessions", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		current, err := qtx.SessionGetByID(r.Context(), id)
		if err != nil {
			return 0, nil, err
		}
		newStartAt := current.StartAt
		if startAtPtr != nil {
			newStartAt = *startAtPtr
		}
		newEndAt := current.EndAt
		if endAtPtr != nil {
			newEndAt = *endAtPtr
		}
		newCourseID := current.CourseID
		if courseIDPtr != nil {
			newCourseID = *courseIDPtr
		}
		impact, impactErr := qtx.SessionChangePreviewImpact(r.Context(), id, newCourseID, newStartAt, newEndAt)
		if impactErr != nil {
			return 0, nil, impactErr
		}
		settings, settingsErr := qtx.AppSettingsGetSessionChangeSettings(r.Context())
		if settingsErr != nil {
			return 0, nil, settingsErr
		}
		shortNotice := newStartAt.Time.Sub(time.Now()).Hours() <= float64(settings.WarningHours)
		requiresAcknowledgement := impact.DirectSitInAssignments > 0 || impact.MissedSessionReferences > 0 || impact.PredictedStudentOverlaps > 0 || impact.PotentialEligibilityChanges > 0 || shortNotice
		if !settings.AllowMoveIntoPast && !newStartAt.Time.After(time.Now()) {
			return http.StatusConflict, map[string]any{"code": "past_time_change", "message": "Moving a session into the past is not permitted"}, fmt.Errorf("past time change")
		}
		if requiresAcknowledgement && (body.AcknowledgeImpact == nil || !*body.AcknowledgeImpact) {
			return http.StatusConflict, map[string]any{
				"code":    "impact_acknowledgement_required",
				"message": "This change affects absence plans and requires acknowledgement",
				"details": map[string]any{
					"impact_summary": map[string]any{
						"direct_sit_in_assignments":     impact.DirectSitInAssignments,
						"missed_session_references":     impact.MissedSessionReferences,
						"predicted_student_overlaps":    impact.PredictedStudentOverlaps,
						"potential_eligibility_changes": impact.PotentialEligibilityChanges,
						"short_notice":                  shortNotice,
					},
				},
			}, fmt.Errorf("impact acknowledgement required")
		}
		item, err := s.deps.Scheduling.EditOccurrenceTimeTx(r.Context(), tx, qtx, scheduling.EditOccurrenceParams{
			SessionID:       id,
			StartAt:         startAtPtr,
			EndAt:           endAtPtr,
			CourseID:        courseIDPtr,
			RoomID:          roomIDPtr,
			TeacherID:       teacherIDPtr,
			ExpectedVersion: *body.ExpectedVersion,
			ActorID:         pgtype.UUID{Bytes: user.ID, Valid: true},
			ChangeSource:    "session_edit",
		})
		if err != nil {
			var se *scheduling.Err
			if errors.As(err, &se) {
				if se.Code == "stale_edit" {
					if current, getErr := qtx.SessionGetByID(r.Context(), id); getErr == nil {
						dto := map[string]any{
							"id": r.PathValue("id"), "series_id": nil,
							"course_id": mustUUIDStringOrEmpty(s.a, current.CourseID), "room_id": uuidOrNull(s.a, current.RoomID),
							"teacher_id": mustUUIDStringOrEmpty(s.a, current.TeacherID), "start_at": current.StartAt.Time.UTC().Format(time.RFC3339Nano),
							"end_at": current.EndAt.Time.UTC().Format(time.RFC3339Nano), "version": current.Version,
						}
						if current.SeriesID.Valid {
							dto["series_id"] = mustUUIDStringOrEmpty(s.a, current.SeriesID)
						}
						return http.StatusConflict, map[string]any{"code": se.Code, "message": "Stale edit", "details": map[string]any{"current": dto}}, err
					}
				}
				return scheduling.HTTPStatusForErr(se), map[string]any{"code": se.Code, "message": se.Message, "details": se.Details}, err
			}
			// Infrastructure error — return nil body so runner uses ClassifyDBErr
			return 0, nil, err
		}
		sid, err := s.a.UUIDString(item.SessionID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		updatedID = sid

		actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
		if _, aErr := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "session.edit_occurrence",
			Payload:     map[string]any{"id": r.PathValue("id"), "start_at": body.StartAt, "end_at": body.EndAt, "course_id": body.CourseID, "room_id": string(body.RoomID), "teacher_id": body.TeacherID},
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
		changeID, _ := s.a.UUIDString(item.SessionChangeID)
		return http.StatusOK, map[string]any{"session": dto, "change_id": changeID, "warnings": item.Warnings}, nil
	}) {
		s.publishSessionUpdated(updatedID)
	}
}

func (s *server) handleSessionsBulkUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	var body struct {
		Updates []struct {
			ID              string          `json:"id"`
			ExpectedVersion int32           `json:"expected_version"`
			TeacherID       *string         `json:"teacher_id"`
			RoomID          json.RawMessage `json:"room_id"`
			StartAt         *string         `json:"start_at"`
			EndAt           *string         `json:"end_at"`
		} `json:"updates"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	if len(body.Updates) == 0 {
		s.a.WriteErr(w, http.StatusBadRequest, "no_updates", "No updates provided")
		return
	}

	if len(body.Updates) > 100 {
		s.a.WriteErr(w, http.StatusBadRequest, "too_many", "Max 100 updates per request")
		return
	}
	batchID, batchStatus, err := s.deps.Q.SessionChangeBatchCreate(r.Context(), int32(len(body.Updates)), pgtype.UUID{Bytes: user.ID, Valid: true}, r.Header.Get("Idempotency-Key"))
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if batchStatus != "open" {
		batchIDText, _ := s.a.UUIDString(batchID)
		s.a.WriteJSON(w, http.StatusOK, map[string]any{"batch_id": batchIDText, "status": batchStatus, "results": []any{}})
		return
	}

	type bulkResult struct {
		ID       string         `json:"id"`
		Status   string         `json:"status"`
		ChangeID string         `json:"change_id,omitempty"`
		Session  map[string]any `json:"session,omitempty"`
		Error    string         `json:"error,omitempty"`
		Details  any            `json:"details,omitempty"`
	}

	results := make([]bulkResult, 0, len(body.Updates))

	for _, upd := range body.Updates {
		id, err := s.a.ParseUUID(upd.ID)
		if err != nil {
			results = append(results, bulkResult{ID: upd.ID, Status: "error", Error: "Invalid session ID"})
			continue
		}

		var teacherIDPtr *pgtype.UUID
		if upd.TeacherID != nil && *upd.TeacherID != "" {
			parsed, err := s.a.ParseUUID(*upd.TeacherID)
			if err != nil {
				results = append(results, bulkResult{ID: upd.ID, Status: "error", Error: "Invalid teacher_id"})
				continue
			}
			teacherIDPtr = &parsed
		}

		var roomIDPtr *pgtype.UUID
		if upd.RoomID != nil {
			res, err := s.parseRoomIDField(upd.RoomID)
			if err != nil {
				results = append(results, bulkResult{ID: upd.ID, Status: "error", Error: err.Error()})
				continue
			}
			if res.clear {
				cleared := pgtype.UUID{}
				roomIDPtr = &cleared
			} else if res.set != nil {
				roomIDPtr = res.set
			}
		}

		var startAtPtr *pgtype.Timestamptz
		if upd.StartAt != nil {
			parsed, err := s.a.ParseTimestamptz(*upd.StartAt)
			if err != nil {
				results = append(results, bulkResult{ID: upd.ID, Status: "error", Error: "Invalid start_at"})
				continue
			}
			startAtPtr = &parsed
		}

		var endAtPtr *pgtype.Timestamptz
		if upd.EndAt != nil {
			parsed, err := s.a.ParseTimestamptz(*upd.EndAt)
			if err != nil {
				results = append(results, bulkResult{ID: upd.ID, Status: "error", Error: "Invalid end_at"})
				continue
			}
			endAtPtr = &parsed
		}

		item, err := s.deps.Scheduling.EditOccurrenceTime(r.Context(), scheduling.EditOccurrenceParams{
			SessionID:       id,
			StartAt:         startAtPtr,
			EndAt:           endAtPtr,
			CourseID:        nil,
			RoomID:          roomIDPtr,
			TeacherID:       teacherIDPtr,
			ExpectedVersion: upd.ExpectedVersion,
			ActorID:         pgtype.UUID{Bytes: user.ID, Valid: true},
			BatchID:         batchID,
			ChangeSource:    "bulk_session_edit",
		})
		if err != nil {
			var se *scheduling.Err
			if errors.As(err, &se) {
				if se.Code == "stale_edit" {
					results = append(results, bulkResult{ID: upd.ID, Status: "stale_edit", Error: err.Error()})
					continue
				}
				results = append(results, bulkResult{ID: upd.ID, Status: "conflict", Error: se.Message, Details: se.Details})
				continue
			}
			if strings.Contains(err.Error(), "stale_edit") {
				results = append(results, bulkResult{ID: upd.ID, Status: "stale_edit", Error: err.Error()})
				continue
			}
			results = append(results, bulkResult{ID: upd.ID, Status: "error", Error: err.Error()})
			continue
		}

		updated, err := s.deps.Q.SessionGetByID(r.Context(), id)
		if err != nil {
			slug, _ := s.a.UUIDString(item.SessionID)
			results = append(results, bulkResult{ID: upd.ID, Status: "updated", Session: map[string]any{"id": slug}})
			s.publishSessionUpdated(slug)
			continue
		}

		sid, _ := s.a.UUIDString(updated.ID)
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

		changeID, _ := s.a.UUIDString(item.SessionChangeID)
		results = append(results, bulkResult{ID: upd.ID, Status: "updated", ChangeID: changeID, Session: dto})
		s.publishSessionUpdated(sid)
	}

	succeededCount := int32(0)
	for _, result := range results {
		if result.Status == "updated" {
			succeededCount++
		}
	}
	if err := s.deps.Q.SessionChangeBatchComplete(r.Context(), batchID, succeededCount, int32(len(results))-succeededCount); err != nil {
		s.deps.Log.Error("session change batch completion failed", "error", err, "batch_id", batchID)
	}
	batchIDText, _ := s.a.UUIDString(batchID)
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"batch_id": batchIDText, "results": results})
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

	if s.a.WithIdempotentTx(w, r, actor.ID, "sessions", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		warnings, err := s.deps.Scheduling.UpsertSessionAttendanceWithWarningsTx(r.Context(), tx, qtx, sessionID, studentID, body.Status)
		if err != nil {
			var se *scheduling.Err
			if errors.As(err, &se) {
				s.a.WriteErrDetails(w, scheduling.HTTPStatusForErr(se), se.Code, se.Message, se.Details)
				return 0, nil, err
			}
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
		return http.StatusOK, map[string]any{"ok": true, "warnings": warnings}, nil
	}) {
		s.publishSessionUpdated(sessID)
	}
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

	if s.a.WithIdempotentTx(w, r, actor.ID, "sessions", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := s.deps.Scheduling.DeleteSessionAttendanceTx(r.Context(), qtx, sessionID, studentID); err != nil {
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
	}) {
		s.publishSessionUpdated(sessID)
	}
}
