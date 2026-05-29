package schedulinghttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/scheduling"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("POST /api/v1/scheduling/preflight", s.handlePreflight)
	mux.HandleFunc("POST /api/v1/scheduling/preflight_series", s.handlePreflightSeries)
	mux.HandleFunc("POST /api/v1/scheduling/find-slots", s.handleFindAvailableSlots)
}

// classifySchedulingErr maps a scheduling service error to a safe HTTP status, code, and message.
// It never exposes internal database details (column names, query fragments, etc.) to the client.
func (s *server) classifySchedulingErr(err error) (int, string, string) {
	// ClassifyDBErr handles PG errors, context cancellation, and timeout — all safe.
	status, code, msg := s.a.ClassifyDBErr(err)
	if code != "internal" {
		return status, code, msg
	}

	// Not a recognized PG/context error — check known safe validation messages.
	raw := err.Error()
	safePrefixes := []string{
		"invalid time range",
		"location required",
		"weekdays required",
		"duration_minutes must be > 0",
		"end_date or count required",
		"count must be > 0",
		"invalid start_local_time",
		"invalid start_date",
		"end_date before start_date",
		"invalid uuid",
		"end_date must be on or after start_date",
		"date range limited to",
	}
	for _, prefix := range safePrefixes {
		if strings.HasPrefix(raw, prefix) {
			return http.StatusBadRequest, "bad_input", raw
		}
	}

	// Unknown error — log full details server-side, return generic message to client.
	s.deps.Log.Error("scheduling error", "error", err)
	return http.StatusInternalServerError, "internal", "Internal error"
}

func (s *server) handlePreflight(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	var body struct {
		SessionID          *string   `json:"session_id"`
		CourseID           string    `json:"course_id"`
		RoomID             *string   `json:"room_id"`
		TeacherID          string    `json:"teacher_id"`
		StartAt            string    `json:"start_at"`
		EndAt              string    `json:"end_at"`
		IncludedStudentIDs *[]string `json:"included_student_ids"`
		ExcludedStudentIDs *[]string `json:"excluded_student_ids"`
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

	teacherID, err := s.a.ParseUUID(body.TeacherID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_teacher_id", "Invalid teacher_id")
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

	var sessionID *pgtype.UUID
	var sessionIDStr *string
	if body.SessionID != nil && *body.SessionID != "" {
		sid, err := s.a.ParseUUID(*body.SessionID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_session_id", "Invalid session_id")
			return
		}
		sessionID = &sid
		v, _ := s.a.UUIDString(sid)
		sessionIDStr = &v
	}

	// Parse included/excluded student IDs once and validate they all exist.
	var includedUUIDs, excludedUUIDs []pgtype.UUID
	var allUUIDs []pgtype.UUID
	overridesProvided := body.IncludedStudentIDs != nil || body.ExcludedStudentIDs != nil
	if overridesProvided {
		var includedRaw, excludedRaw []string
		if body.IncludedStudentIDs != nil {
			includedRaw = *body.IncludedStudentIDs
		}
		if body.ExcludedStudentIDs != nil {
			excludedRaw = *body.ExcludedStudentIDs
		}

		includedUUIDs = make([]pgtype.UUID, 0, len(includedRaw))
		for _, sid := range includedRaw {
			u, err := s.a.ParseUUID(sid)
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_student_id", "Invalid student_id")
				return
			}
			includedUUIDs = append(includedUUIDs, u)
		}
		excludedUUIDs = make([]pgtype.UUID, 0, len(excludedRaw))
		for _, sid := range excludedRaw {
			u, err := s.a.ParseUUID(sid)
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_student_id", "Invalid student_id")
				return
			}
			excludedUUIDs = append(excludedUUIDs, u)
		}

		allUUIDs = make([]pgtype.UUID, 0, len(includedUUIDs)+len(excludedUUIDs))
		allUUIDs = append(allUUIDs, includedUUIDs...)
		allUUIDs = append(allUUIDs, excludedUUIDs...)

		rows, err := s.deps.DB.Query(r.Context(), `SELECT id FROM students WHERE id = ANY($1::uuid[])`, allUUIDs)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return
		}
		defer rows.Close()
		found := map[[16]byte]bool{}
		for rows.Next() {
			var id pgtype.UUID
			if err := rows.Scan(&id); err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return
			}
			found[id.Bytes] = true
		}
		if err := rows.Err(); err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		for _, id := range allUUIDs {
			if !found[id.Bytes] {
				s.a.WriteErr(w, http.StatusBadRequest, "unknown_student_id", "Unknown student ID")
				return
			}
		}
	}

	// If client didn't provide overrides but this is an edit preflight (session_id present),
	// fall back to the persisted session_attendance rows to avoid false preflight blocks.
	if !overridesProvided && sessionID != nil && sessionID.Valid {
		rows, err := s.deps.Q.SessionAttendanceList(r.Context(), *sessionID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return
		}
		if len(rows) > 0 {
			includedUUIDs = includedUUIDs[:0]
			excludedUUIDs = excludedUUIDs[:0]
			for _, row := range rows {
				if !row.StudentID.Valid {
					continue
				}
				switch row.Status {
				case "included":
					includedUUIDs = append(includedUUIDs, row.StudentID)
				case "excluded":
					excludedUUIDs = append(excludedUUIDs, row.StudentID)
				}
			}
		}
	}

	// Build effective roster if explicit includes/excludes are present (client-provided or DB-loaded).
	var studentIDsPtr *[]pgtype.UUID
	if len(includedUUIDs) > 0 || len(excludedUUIDs) > 0 {
		base, err := s.deps.Q.CourseStudentsList(r.Context(), courseID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return
		}

		students := map[[16]byte]pgtype.UUID{}
		for _, row := range base {
			if row.StudentID.Valid {
				students[row.StudentID.Bytes] = row.StudentID
			}
		}
		for _, u := range includedUUIDs {
			students[u.Bytes] = u
		}
		for _, u := range excludedUUIDs {
			delete(students, u.Bytes)
		}

		studentIDs := make([]pgtype.UUID, 0, len(students))
		for _, u := range students {
			studentIDs = append(studentIDs, u)
		}
		studentIDsPtr = &studentIDs
	}

	courseIDStr, _ := s.a.UUIDString(courseID)
	var roomIDPtr *string
	if roomID.Valid {
		roomIDStr, err := s.a.UUIDString(roomID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		roomIDPtr = &roomIDStr
	}
	teacherIDStr, _ := s.a.UUIDString(teacherID)

	res, se, err := s.deps.Scheduling.Preflight(r.Context(), scheduling.PreflightParams{
		SessionID:  sessionID,
		CourseID:   courseID,
		RoomID:     roomID,
		TeacherID:  teacherID,
		StartAt:    startAt,
		EndAt:      endAt,
		StudentIDs: studentIDsPtr,
		Requested: scheduling.ConflictRequested{
			StartAt:   startAt.Time.UTC().Format(time.RFC3339Nano),
			EndAt:     endAt.Time.UTC().Format(time.RFC3339Nano),
			CourseID:  courseIDStr,
			RoomID:    roomIDPtr,
			TeacherID: teacherIDStr,
			SeriesID:  nil,
		},
	})
	if err != nil {
		status, code, msg := s.classifySchedulingErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if se != nil {
		s.a.WriteErrDetails(w, http.StatusConflict, se.Code, se.Message, se.Details)
		return
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     res.Status,
		"session_id": sessionIDStr,
	})
}

func (s *server) handlePreflightSeries(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
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
		SeriesID        *string `json:"series_id"`
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

	var seriesID *pgtype.UUID
	if body.SeriesID != nil && *body.SeriesID != "" {
		sid, err := s.a.ParseUUID(*body.SeriesID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_series_id", "Invalid series_id")
			return
		}
		seriesID = &sid
	}

	res, se, err := s.deps.Scheduling.PreflightSeries(r.Context(), scheduling.PreflightSeriesParams{
		CourseID:        courseID,
		RoomID:          roomID,
		TeacherID:       teacherID,
		Weekdays:        wds,
		StartLocalTime:  startClock,
		DurationMinutes: body.DurationMinutes,
		StartDate:       startDate,
		EndDate:         endDate,
		Count:           body.Count,
		SeriesID:        seriesID,
	})
	if err != nil {
		status, code, msg := s.classifySchedulingErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if se != nil {
		s.a.WriteErrDetails(w, http.StatusConflict, se.Code, se.Message, se.Details)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, res)
}

func (s *server) handleFindAvailableSlots(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	var body struct {
		StudentID        string `json:"student_id"`
		CourseID         string `json:"course_id"`
		StartDate        string `json:"start_date"`
		EndDate          string `json:"end_date"`
		SlotDurationMins int    `json:"slot_duration_minutes"`
		DayStartHour     int    `json:"day_start_hour"`
		DayEndHour       int    `json:"day_end_hour"`
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

	courseID, err := s.a.ParseUUID(body.CourseID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_course_id", "Invalid course_id")
		return
	}

	startDate, err := s.a.ParseLocalDateYYYYMMDD(body.StartDate)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_start_date", "Invalid start_date (YYYY-MM-DD)")
		return
	}
	endDate, err := s.a.ParseLocalDateYYYYMMDD(body.EndDate)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_end_date", "Invalid end_date (YYYY-MM-DD)")
		return
	}

	res, err := s.deps.Scheduling.FindAvailableSlots(r.Context(), scheduling.FindAvailableSlotsParams{
		StudentID:        studentID,
		CourseID:         courseID,
		StartDate:        startDate,
		EndDate:          endDate,
		SlotDurationMins: body.SlotDurationMins,
		DayStartHour:     body.DayStartHour,
		DayEndHour:       body.DayEndHour,
	})
	if err != nil {
		status, code, msg := s.classifySchedulingErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	s.a.WriteJSON(w, http.StatusOK, res)
}
