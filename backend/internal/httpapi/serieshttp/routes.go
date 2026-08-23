package serieshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/realtime"
	"warwick-institute/internal/scheduling"
	"warwick-institute/internal/series"
)

func mustUUIDStringOrEmptySeries(a httpadapter.Adapter, u pgtype.UUID) string {
	s, err := a.UUIDString(u)
	if err != nil {
		return ""
	}
	return s
}

func uuidOrNullSeries(a httpadapter.Adapter, u pgtype.UUID) any {
	if !u.Valid {
		return nil
	}
	s, err := a.UUIDString(u)
	if err != nil {
		return nil
	}
	return s
}

func buildStaleEditPayloadSeries(a httpadapter.Adapter, r *http.Request, ser sqldb.SeriesGetByIDRow) map[string]any {
	payload := map[string]any{
		"id":               r.PathValue("id"),
		"course_id":        mustUUIDStringOrEmptySeries(a, ser.CourseID),
		"room_id":          uuidOrNullSeries(a, ser.RoomID),
		"teacher_id":       mustUUIDStringOrEmptySeries(a, ser.TeacherID),
		"institute_tz":     ser.InstituteTz,
		"weekdays":         ser.Weekdays,
		"duration_minutes": ser.DurationMinutes,
		"start_date":       ser.StartDate.Time.UTC().Format("2006-01-02"),
		"end_date":         "",
		"count":            nil,
		"version":          ser.Version,
	}
	if ser.EndDate.Valid {
		payload["end_date"] = ser.EndDate.Time.UTC().Format("2006-01-02")
	}
	if ser.Count.Valid {
		v := ser.Count.Int32
		payload["count"] = &v
	}
	if clock, ok := a.ClockFromPgTime(ser.StartLocalTime); ok {
		payload["start_local_time"] = fmt.Sprintf("%02d:%02d", clock.Hour, clock.Minute)
	}
	return payload
}

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

// recurrenceValidationResult holds the structured error for a recurrence validation
// failure. When valid is false, the error should be returned from the fn callback
// as a structured response.
type recurrenceValidationResult struct {
	valid bool
	code  string
	msg   string
}

// checkRecurrence extracts a recurrence validation error from err and returns
// a structured result. When valid is false the caller should use the code and
// msg to build a response body.
func (s *server) checkRecurrence(ctx context.Context, err error) recurrenceValidationResult {
	var validationErr *series.ValidationError
	if !errors.As(err, &validationErr) {
		return recurrenceValidationResult{valid: true}
	}
	if s.deps.Log != nil {
		s.deps.Log.WarnContext(ctx, "schedule recurrence rejected", "code", validationErr.Code)
	}
	return recurrenceValidationResult{
		valid: false,
		code:  "invalid_recurrence",
		msg:   validationErr.Message,
	}
}

func (s *server) publishSessionsChanged(id string) {
	if s.deps.Realtime == nil || id == "" {
		return
	}
	s.deps.Realtime.Publish("sessions:all", realtime.Event{Type: "sessions.updated", ID: id})
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("POST /api/v1/series", s.handleSeriesCreate)
	mux.HandleFunc("GET /api/v1/series/{id}", s.handleSeriesGet)
	mux.HandleFunc("PATCH /api/v1/series/{id}", s.handleSeriesSplit)
	mux.HandleFunc("POST /api/v1/series/{id}/cancel", s.handleSeriesCancel)
	mux.HandleFunc("PATCH /api/v1/series/{id}/entire", s.handleSeriesEditEntire)
}

func (s *server) handleSeriesCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	var body struct {
		CourseID        string  `json:"course_id"`
		RoomID          *string `json:"room_id"`
		TeacherID       string  `json:"teacher_id"`
		Weekdays        []int   `json:"weekdays"`
		StartLocalTime  string  `json:"start_local_time"`
		DurationMinutes int     `json:"duration_minutes"`
		StartDate       string  `json:"start_date"`
		EndDate         *string `json:"end_date"`
		Count           *int    `json:"count"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
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
	if len(body.Weekdays) == 0 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_weekdays", "weekdays required")
		return
	}
	var wds []time.Weekday
	for _, n := range body.Weekdays {
		if n < 0 || n > 6 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_weekdays", "weekday must be 0..6")
			return
		}
		wds = append(wds, time.Weekday(n))
	}
	startClock, err := s.a.ParseClockHHMM(body.StartLocalTime)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_start_local_time", "Invalid start_local_time (HH:MM)")
		return
	}
	if body.DurationMinutes <= 0 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_duration", "duration_minutes must be > 0")
		return
	}
	startDate, err := s.a.ParseLocalDateYYYYMMDD(body.StartDate)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_start_date", "Invalid start_date (YYYY-MM-DD)")
		return
	}

	var endDate *scheduling.LocalDate
	if body.EndDate != nil && *body.EndDate != "" {
		d, err := s.a.ParseLocalDateYYYYMMDD(*body.EndDate)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_end_date", "Invalid end_date (YYYY-MM-DD)")
			return
		}
		endDate = &d
	}
	if endDate == nil && body.Count == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_end_bound", "end_date or count required")
		return
	}

	var changedID string
	if s.a.WithSerializableIdempotentTx(w, r, user.ID, "series", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		res, err := s.deps.Scheduling.CreateSeriesAndMaterializeTx(r.Context(), tx, qtx, scheduling.CreateSeriesParams{
			CourseID:        courseID,
			RoomID:          roomID,
			TeacherID:       teacherID,
			Weekdays:        wds,
			StartLocalTime:  startClock,
			DurationMinutes: body.DurationMinutes,
			StartDate:       startDate,
			EndDate:         endDate,
			Count:           body.Count,
		})
		if err != nil {
			if r := s.checkRecurrence(r.Context(), err); !r.valid {
				return http.StatusBadRequest, map[string]any{"code": r.code, "message": r.msg}, err
			}
			var se *scheduling.Err
			if errors.As(err, &se) {
				return scheduling.HTTPStatusForErr(se), map[string]any{"code": se.Code, "message": se.Message, "details": se.Details}, err
			}
			return 0, nil, err
		}
		seriesID, err := s.a.UUIDString(res.SeriesID)
		if err != nil {
			return http.StatusInternalServerError, map[string]any{"code": "internal", "message": "Internal error"}, err
		}
		changedID = seriesID
		actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
		if _, aErr := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "series.create",
			Payload:     map[string]any{"series_id": seriesID, "course_id": body.CourseID, "teacher_id": body.TeacherID, "room_id": body.RoomID, "weekdays": body.Weekdays, "start_local_time": body.StartLocalTime, "duration_minutes": body.DurationMinutes, "start_date": body.StartDate, "end_date": body.EndDate, "count": body.Count, "sessions_added": res.SessionsAdded},
		}); aErr != nil {
			s.deps.Log.Error("audit insert failed", "error", aErr, "series_id", seriesID)
		}
		return http.StatusCreated, map[string]any{
			"series_id":      seriesID,
			"sessions_added": res.SessionsAdded,
			"warnings":       res.Warnings,
		}, nil
	}) {
		s.publishSessionsChanged(changedID)
	}
}

func (s *server) handleSeriesGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	item, err := s.deps.Q.SeriesGetByID(r.Context(), id)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	seriesID, err := s.a.UUIDString(item.ID)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	courseID, err := s.a.UUIDString(item.CourseID)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	var roomIDPtr *string
	if item.RoomID.Valid {
		roomID, err := s.a.UUIDString(item.RoomID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		roomIDPtr = &roomID
	}
	teacherID, err := s.a.UUIDString(item.TeacherID)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	endDate := ""
	if item.EndDate.Valid {
		endDate = item.EndDate.Time.UTC().Format("2006-01-02")
	}
	var count *int32
	if item.Count.Valid {
		v := item.Count.Int32
		count = &v
	}
	startLocal, ok := s.a.ClockFromPgTime(item.StartLocalTime)
	if !ok {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"id":               seriesID,
		"course_id":        courseID,
		"room_id":          roomIDPtr,
		"teacher_id":       teacherID,
		"institute_tz":     item.InstituteTz,
		"weekdays":         item.Weekdays,
		"start_local_time": fmt.Sprintf("%02d:%02d", startLocal.Hour, startLocal.Minute),
		"duration_minutes": item.DurationMinutes,
		"start_date":       item.StartDate.Time.UTC().Format("2006-01-02"),
		"end_date":         endDate,
		"count":            count,
		"version":          item.Version,
	})
}

func (s *server) handleSeriesSplit(w http.ResponseWriter, r *http.Request) {
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
		PivotDate       string  `json:"pivot_date"`
		Weekdays        []int   `json:"weekdays"`
		StartLocalTime  *string `json:"start_local_time"`
		DurationMinutes *int    `json:"duration_minutes"`
		EndDate         *string `json:"end_date"`
		Count           *int    `json:"count"`
		ExpectedVersion *int32  `json:"expected_version"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.PivotDate == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_pivot_date", "pivot_date required")
		return
	}
	if body.ExpectedVersion == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_expected_version", "expected_version required")
		return
	}

	pivot, err := s.a.ParseLocalDateYYYYMMDD(body.PivotDate)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_pivot_date", "Invalid pivot_date (YYYY-MM-DD)")
		return
	}

	var wds []time.Weekday
	if len(body.Weekdays) > 0 {
		for _, n := range body.Weekdays {
			if n < 0 || n > 6 {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_weekdays", "weekday must be 0..6")
				return
			}
			wds = append(wds, time.Weekday(n))
		}
	}

	var clock *scheduling.Clock
	if body.StartLocalTime != nil && *body.StartLocalTime != "" {
		c, err := s.a.ParseClockHHMM(*body.StartLocalTime)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_start_local_time", "Invalid start_local_time (HH:MM)")
			return
		}
		clock = &c
	}

	var endDate *scheduling.LocalDate
	if body.EndDate != nil && *body.EndDate != "" {
		d, err := s.a.ParseLocalDateYYYYMMDD(*body.EndDate)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_end_date", "Invalid end_date (YYYY-MM-DD)")
			return
		}
		endDate = &d
	}

	var changedID string
	if s.a.WithSerializableIdempotentTx(w, r, user.ID, "series", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		existing, err := qtx.SeriesGetByID(r.Context(), id)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		if existing.Version != *body.ExpectedVersion {
			cur := buildStaleEditPayloadSeries(s.a, r, existing)
			s.a.WriteErrDetails(w, http.StatusConflict, "stale_edit", "Stale edit", map[string]any{"current": cur})
			return 0, nil, errors.New("stale_edit")
		}
		res, err := s.deps.Scheduling.SplitThisAndFutureTx(r.Context(), tx, qtx, scheduling.SplitSeriesParams{
			SeriesID:        id,
			PivotDate:       pivot,
			ExpectedVersion: *body.ExpectedVersion,
			Weekdays:        wds,
			StartLocalTime:  clock,
			DurationMinutes: body.DurationMinutes,
			EndDate:         endDate,
			Count:           body.Count,
		})
		if err != nil {
			var operationErr *series.OperationError
			if errors.As(err, &operationErr) {
				return http.StatusConflict, map[string]any{"code": operationErr.Code, "message": operationErr.Message}, err
			}
			if r := s.checkRecurrence(r.Context(), err); !r.valid {
				return http.StatusBadRequest, map[string]any{"code": r.code, "message": r.msg}, err
			}
			var se *scheduling.Err
			if errors.As(err, &se) && se.Code == "stale_edit" {
				cur, ferr := qtx.SeriesGetByID(r.Context(), id)
				if ferr == nil {
					payload := buildStaleEditPayloadSeries(s.a, r, cur)
					return http.StatusConflict, map[string]any{"code": "stale_edit", "message": "Stale edit", "details": map[string]any{"current": payload}}, err
				}
			}
			if errors.As(err, &se) {
				return scheduling.HTTPStatusForErr(se), map[string]any{"code": se.Code, "message": se.Message, "details": se.Details}, err
			}
			return 0, nil, err
		}
		oldID, err := s.a.UUIDString(res.OldSeriesID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		newID, err := s.a.UUIDString(res.NewSeriesID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		changedID = newID
		actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
		if _, aErr := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "series.split",
			Payload:     map[string]any{"old_series_id": oldID, "new_series_id": newID, "pivot_date": body.PivotDate, "weekdays": body.Weekdays, "start_local_time": body.StartLocalTime, "duration_minutes": body.DurationMinutes, "end_date": body.EndDate, "count": body.Count, "new_sessions_added": res.NewSessionsAdded},
		}); aErr != nil {
			s.deps.Log.Error("audit insert failed", "error", aErr, "series_id", r.PathValue("id"))
		}
		return http.StatusOK, map[string]any{"old_series_id": oldID, "new_series_id": newID, "new_sessions_added": res.NewSessionsAdded, "warnings": res.Warnings}, nil
	}) {
		s.publishSessionsChanged(changedID)
	}
}

func (s *server) handleSeriesCancel(w http.ResponseWriter, r *http.Request) {
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
		Scope           string `json:"scope"`
		PivotDate       string `json:"pivot_date"`
		ExpectedVersion *int32 `json:"expected_version"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.Scope == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_scope", "scope required")
		return
	}
	if body.ExpectedVersion == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_expected_version", "expected_version required")
		return
	}

	var cancelScope scheduling.CancelScope
	switch body.Scope {
	case string(scheduling.CancelScopeThisAndFuture):
		cancelScope = scheduling.CancelScopeThisAndFuture
	case string(scheduling.CancelScopeEntireSeriesFutureOnly):
		cancelScope = scheduling.CancelScopeEntireSeriesFutureOnly
	default:
		s.a.WriteErr(w, http.StatusBadRequest, "bad_scope", "Invalid scope")
		return
	}

	var pivot *scheduling.LocalDate
	if cancelScope == scheduling.CancelScopeThisAndFuture {
		if body.PivotDate == "" {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_pivot_date", "pivot_date required")
			return
		}
		d, err := s.a.ParseLocalDateYYYYMMDD(body.PivotDate)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_pivot_date", "Invalid pivot_date (YYYY-MM-DD)")
			return
		}
		pivot = &d
	}

	var changedID string
	if s.a.WithSerializableIdempotentTx(w, r, user.ID, "series", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		existing, err := qtx.SeriesGetByID(r.Context(), id)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		if existing.Version != *body.ExpectedVersion {
			cur := buildStaleEditPayloadSeries(s.a, r, existing)
			s.a.WriteErrDetails(w, http.StatusConflict, "stale_edit", "Stale edit", map[string]any{"current": cur})
			return 0, nil, errors.New("stale_edit")
		}
		res, err := s.deps.Scheduling.CancelSeriesTx(r.Context(), tx, qtx, scheduling.CancelSeriesParams{
			SeriesID:        id,
			Scope:           cancelScope,
			PivotDate:       pivot,
			ExpectedVersion: *body.ExpectedVersion,
		})
		if err != nil {
			var operationErr *series.OperationError
			if errors.As(err, &operationErr) {
				return http.StatusConflict, map[string]any{"code": operationErr.Code, "message": operationErr.Message}, err
			}
			if r := s.checkRecurrence(r.Context(), err); !r.valid {
				return http.StatusBadRequest, map[string]any{"code": r.code, "message": r.msg}, err
			}
			switch err.Error() {
			case "stale_edit":
				cur, ferr := qtx.SeriesGetByID(r.Context(), id)
				if ferr == nil {
					payload := buildStaleEditPayloadSeries(s.a, r, cur)
					return http.StatusConflict, map[string]any{"code": "stale_edit", "message": "Stale edit", "details": map[string]any{"current": payload}}, err
				}
			case "cannot_cancel_started":
				return http.StatusConflict, map[string]any{
					"code":    "cannot_cancel_started",
					"message": "Cannot cancel occurrences that have started",
					"details": map[string]any{
						"server_now": time.Now().UTC().Format(time.RFC3339Nano),
					},
				}, err
			}
			return 0, nil, err
		}
		seriesID, err := s.a.UUIDString(res.SeriesID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		changedID = seriesID
		actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
		if _, aErr := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "series.cancel",
			Payload:     map[string]any{"series_id": seriesID, "scope": body.Scope, "pivot_date": body.PivotDate, "expected_version": *body.ExpectedVersion, "sessions_canceled": res.SessionsCanceled},
		}); aErr != nil {
			s.deps.Log.Error("audit insert failed", "error", aErr, "series_id", seriesID)
		}
		return http.StatusOK, map[string]any{
			"series_id":         seriesID,
			"scope":             body.Scope,
			"canceled_from_utc": res.CanceledFromUTC.UTC().Format(time.RFC3339Nano),
			"sessions_canceled": res.SessionsCanceled,
		}, nil
	}) {
		s.publishSessionsChanged(changedID)
	}
}

func (s *server) handleSeriesEditEntire(w http.ResponseWriter, r *http.Request) {
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
		CourseID        string  `json:"course_id"`
		RoomID          *string `json:"room_id"`
		TeacherID       string  `json:"teacher_id"`
		Weekdays        []int   `json:"weekdays"`
		StartLocalTime  string  `json:"start_local_time"`
		DurationMinutes int     `json:"duration_minutes"`
		EndDate         *string `json:"end_date"`
		Count           *int    `json:"count"`
		ExpectedVersion *int32  `json:"expected_version"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.ExpectedVersion == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_expected_version", "expected_version required")
		return
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
	if len(body.Weekdays) == 0 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_weekdays", "weekdays required")
		return
	}
	var wds []time.Weekday
	for _, n := range body.Weekdays {
		if n < 0 || n > 6 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_weekdays", "weekday must be 0..6")
			return
		}
		wds = append(wds, time.Weekday(n))
	}
	startClock, err := s.a.ParseClockHHMM(body.StartLocalTime)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_start_local_time", "Invalid start_local_time (HH:MM)")
		return
	}
	if body.DurationMinutes <= 0 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_duration", "duration_minutes must be > 0")
		return
	}

	var endDate *scheduling.LocalDate
	if body.EndDate != nil && *body.EndDate != "" {
		d, err := s.a.ParseLocalDateYYYYMMDD(*body.EndDate)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_end_date", "Invalid end_date (YYYY-MM-DD)")
			return
		}
		endDate = &d
	}
	if endDate == nil && body.Count == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_end_bound", "end_date or count required")
		return
	}

	var changedID string
	if s.a.WithSerializableIdempotentTx(w, r, user.ID, "series", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		existing, err := qtx.SeriesGetByID(r.Context(), id)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		if existing.Version != *body.ExpectedVersion {
			cur := buildStaleEditPayloadSeries(s.a, r, existing)
			return http.StatusConflict, map[string]any{"code": "stale_edit", "message": "Stale edit", "details": map[string]any{"current": cur}}, errors.New("stale_edit")
		}
		res, err := s.deps.Scheduling.EditEntireSeriesFutureOnlyTx(r.Context(), tx, qtx, scheduling.EditEntireSeriesParams{
			SeriesID:        id,
			ExpectedVersion: *body.ExpectedVersion,
			NowUTC:          time.Now().UTC(),
			CourseID:        courseID,
			RoomID:          roomID,
			TeacherID:       teacherID,
			Weekdays:        wds,
			StartLocalTime:  startClock,
			DurationMinutes: body.DurationMinutes,
			EndDate:         endDate,
			Count:           body.Count,
		})
		if err != nil {
			var operationErr *series.OperationError
			if errors.As(err, &operationErr) {
				return http.StatusConflict, map[string]any{"code": operationErr.Code, "message": operationErr.Message}, err
			}
			if r := s.checkRecurrence(r.Context(), err); !r.valid {
				return http.StatusBadRequest, map[string]any{"code": r.code, "message": r.msg}, err
			}
			var se *scheduling.Err
			if errors.As(err, &se) && se.Code == "stale_edit" {
				cur, ferr := qtx.SeriesGetByID(r.Context(), id)
				if ferr == nil {
					payload := buildStaleEditPayloadSeries(s.a, r, cur)
					return http.StatusConflict, map[string]any{"code": "stale_edit", "message": "Stale edit", "details": map[string]any{"current": payload}}, err
				}
			}
			if errors.As(err, &se) {
				return scheduling.HTTPStatusForErr(se), map[string]any{"code": se.Code, "message": se.Message, "details": se.Details}, err
			}
			return 0, nil, err
		}
		seriesID, err := s.a.UUIDString(res.SeriesID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		changedID = seriesID
		actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
		if _, aErr := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "series.edit_entire",
			Payload:     map[string]any{"series_id": seriesID, "course_id": body.CourseID, "teacher_id": body.TeacherID, "room_id": body.RoomID, "weekdays": body.Weekdays, "start_local_time": body.StartLocalTime, "duration_minutes": body.DurationMinutes, "end_date": body.EndDate, "count": body.Count, "sessions_canceled": res.SessionsCanceled, "sessions_added": res.SessionsAdded},
		}); aErr != nil {
			s.deps.Log.Error("audit insert failed", "error", aErr, "series_id", seriesID)
		}
		return http.StatusOK, map[string]any{
			"series_id":         seriesID,
			"sessions_canceled": res.SessionsCanceled,
			"sessions_added":    res.SessionsAdded,
			"warnings":          res.Warnings,
		}, nil
	}) {
		s.publishSessionsChanged(changedID)
	}
}
