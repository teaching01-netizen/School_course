package courseshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/courseadmin"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/realtime"
	"warwick-institute/internal/scheduling"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func (s *server) publishCourseUpdated(id string) {
	s.publishCourseUpdates([]string{id})
}

func (s *server) publishCourseUpdates(ids []string) {
	if s.deps.Realtime == nil {
		return
	}
	for _, id := range ids {
		if id != "" {
			s.deps.Realtime.Publish("courses:all", realtime.Event{Type: "course.updated", ID: id})
		}
	}
}

func (s *server) publishSessionsUpdated() {
	if s.deps.Realtime != nil {
		s.deps.Realtime.Publish("sessions:all", realtime.Event{Type: "sessions.updated"})
	}
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}
	registerCourseGroupRoutes(mux, deps)

	mux.HandleFunc("GET /api/v1/courses", s.handleCoursesList)
	mux.HandleFunc("POST /api/v1/courses", s.handleCoursesCreate)
	mux.HandleFunc("GET /api/v1/courses/{id}", s.handleCoursesGet)
	mux.HandleFunc("PATCH /api/v1/courses/{id}", s.handleCoursesPatch)
	mux.HandleFunc("PUT /api/v1/courses/{id}", s.handleCoursesUpdate)
	mux.HandleFunc("DELETE /api/v1/courses/{id}", s.handleCoursesDelete)
	mux.HandleFunc("GET /api/v1/courses/{id}/students", s.handleCourseStudentsList)
	mux.HandleFunc("POST /api/v1/courses/{id}/students", s.handleCourseStudentsAdd)
	mux.HandleFunc("DELETE /api/v1/courses/{id}/students/{student_id}", s.handleCourseStudentsRemove)
	mux.HandleFunc("POST /api/v1/courses/{id}/students/draft", s.handleCourseStudentsAddDraft)
	mux.HandleFunc("POST /api/v1/courses/{id}/students/{student_id}/convert", s.handleCourseStudentsConvert)
	mux.HandleFunc("GET /api/v1/courses/{id}/sessions", s.handleCourseSessionsList)
	mux.HandleFunc("POST /api/v1/courses/{id}/legacy-sync", s.handleLegacySync)
	mux.HandleFunc("POST /api/v1/courses/batch-delete", s.handleCoursesBatchDelete)
	mux.HandleFunc("GET /api/v1/courses/{id}/legacy-conflicts", s.handleCourseLegacyConflicts)
}

// handleCoursesList serves the course list with two response contracts:
//
//   - No `limit` query param → the legacy bare array of every matching course
//     (backward compatible with the lookups/dropdown consumers). Defaults to
//     live courses only so those consumers stop receiving thousands of
//     archived rows.
//   - `limit` present → the paginated envelope {"items", "total_count",
//     "offset", "limit"} used by the courses page.
//
// Filters: status (live default | archived), type (private | general, where
// general covers both the legacy 'General' and native 'Group' vocabulary),
// teacher_id (user uuid | "none" for no primary teacher), and q (substring
// search across code, name, subject, teachers, and roster membership).
func (s *server) handleCoursesList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	params := sqldb.CourseOverviewParams{
		Archived:   strings.TrimSpace(r.URL.Query().Get("status")) == "archived",
		CourseType: courseTypeFilter(r.URL.Query().Get("type")),
		Q:          s.a.SearchQuery(r.URL.Query().Get("q")),
	}
	switch teacherParam := strings.TrimSpace(r.URL.Query().Get("teacher_id")); teacherParam {
	case "":
		// No teacher filter.
	case "none":
		params.TeacherID = "none"
	default:
		teacherID, err := s.a.ParseUUID(teacherParam)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_teacher_id", "Invalid teacher_id")
			return
		}
		params.TeacherID = teacherID.String()
	}

	// The presence of the limit param selects the envelope shape.
	envelope := r.URL.Query().Get("limit") != ""
	if envelope {
		limit := int32(50)
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = int32(min(n, 200))
			}
		}
		offset := int32(0)
		if v := r.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = int32(n)
			}
		}
		params.Limit = limit
		params.Offset = offset
	} else {
		// Legacy bare-array consumers (lookups, dropdowns) omit `limit`, but the
		// query must stay bounded: LIMIT NULL would scan and ship the whole
		// table. Cap at a generous ceiling; migrate consumers to the envelope.
		params.Limit = 1000
		params.Offset = 0
	}

	items, err := s.deps.Q.CourseOverview(r.Context(), params)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	var totalCount int64
	if envelope {
		totalCount, err = s.deps.Q.CourseOverviewCount(r.Context(), params)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return
		}
	}

	courseIDs := make([]pgtype.UUID, len(items))
	for i, c := range items {
		courseIDs[i] = c.ID
	}
	// Batch-fetch teachers from course_teachers with the typed query so the
	// list endpoint reads the same teacher set (including is_primary) as every
	// other read path, and scan errors are surfaced instead of silently skipped.
	teachersByCourse := make(map[string][]map[string]any, len(items))
	if len(courseIDs) > 0 {
		teacherRows, tErr := s.deps.Q.CourseTeachersListForCourses(r.Context(), courseIDs)
		if tErr != nil {
			status, code, msg := s.a.ClassifyDBErr(tErr)
			s.a.WriteErr(w, status, code, msg)
			return
		}
		for _, te := range teacherRows {
			tid, err := s.a.UUIDString(te.TeacherID)
			if err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return
			}
			cid, err := s.a.UUIDString(te.CourseID)
			if err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return
			}
			var fullName any
			if te.FullName.Valid && te.FullName.String != "" {
				fullName = te.FullName.String
			}
			teachersByCourse[cid] = append(teachersByCourse[cid], map[string]any{"id": tid, "username": te.Username, "full_name": fullName, "is_primary": te.IsPrimary})
		}
	}

	type courseDTO struct {
		ID                 string           `json:"id"`
		CourseNo           int64            `json:"course_no"`
		Code               string           `json:"code"`
		Name               string           `json:"name"`
		Year               any              `json:"year"`
		TeacherID          any              `json:"teacher_id"`
		TeacherName        string           `json:"teacher_name"`
		SubjectID          any              `json:"subject_id"`
		SubjectCode        string           `json:"subject_code"`
		SubjectName        string           `json:"subject_name"`
		Hour               any              `json:"hour"`
		StudentCount       any              `json:"student_count"`
		CourseType         any              `json:"course_type"`
		LegacyCourseID     any              `json:"legacy_course_id"`
		LegacyLastSyncedAt any              `json:"legacy_last_synced_at"`
		CycleID            any              `json:"cycle_id"`
		CycleLabel         string           `json:"cycle_label"`
		ExpiryDays         any              `json:"expiry_days"`
		LastSessionAt      any              `json:"last_session_at"`
		ExpiresAt          any              `json:"expires_at"`
		ExpiryStatus       string           `json:"expiry_status"`
		HasOverlap         bool             `json:"has_overlap"`
		HasConflict        bool             `json:"has_conflict"`
		Teachers           []map[string]any `json:"teachers"`
	}
	out := make([]courseDTO, 0, len(items))
	for _, c := range items {
		id, err := s.a.UUIDString(c.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		var year any = nil
		if c.Year.Valid {
			year = c.Year.Int16
		}
		var teacherID any = nil
		if c.TeacherID.Valid {
			teacherID, _ = s.a.UUIDString(c.TeacherID)
		}
		var subjectID any = nil
		if c.SubjectID.Valid {
			subjectID, _ = s.a.UUIDString(c.SubjectID)
		}
		var hour any = nil
		if c.Hour.Valid {
			hour = c.Hour.Int32
		}
		var studentCount any = nil
		if c.StudentCount.Valid {
			studentCount = c.StudentCount.Int32
		}
		var courseType any = nil
		if c.CourseType.Valid {
			courseType = c.CourseType.String
		}
		var legacyCourseID any = nil
		if c.LegacyCourseID.Valid {
			legacyCourseID = c.LegacyCourseID.String
		}
		var legacyLastSyncedAt any = nil
		if c.LegacyLastSyncedAt.Valid {
			legacyLastSyncedAt, _ = s.a.TimeString(c.LegacyLastSyncedAt)
		}
		var cycleID any = nil
		if c.CycleID.Valid {
			cycleID = c.CycleID.String
		}
		var expiryDays any = nil
		if c.ExpiryDays.Valid {
			expiryDays = c.ExpiryDays.Int32
		}
		var lastSessionAt any = nil
		if c.LastSessionAt.Valid {
			lastSessionAt, _ = s.a.TimeString(c.LastSessionAt)
		}
		expiresAt, expiryStatus := courseExpiry(c.ExpiryDays, c.LastSessionAt)
		cid := id
		out = append(out, courseDTO{
			ID:                 cid,
			CourseNo:           c.CourseNo,
			Code:               c.Code,
			Name:               c.Name,
			Year:               year,
			TeacherID:          teacherID,
			TeacherName:        c.TeacherName,
			SubjectID:          subjectID,
			SubjectCode:        c.SubjectCode,
			SubjectName:        c.SubjectName,
			Hour:               hour,
			StudentCount:       studentCount,
			CourseType:         courseType,
			LegacyCourseID:     legacyCourseID,
			LegacyLastSyncedAt: legacyLastSyncedAt,
			CycleID:            cycleID,
			CycleLabel:         c.CycleLabel,
			ExpiryDays:         expiryDays,
			LastSessionAt:      lastSessionAt,
			ExpiresAt:          expiresAt,
			ExpiryStatus:       expiryStatus,
			HasOverlap:         c.HasOverlap,
			HasConflict:        c.HasConflict,
			Teachers:           teachersByCourse[cid],
		})
	}
	if envelope {
		s.a.WriteJSON(w, http.StatusOK, map[string]any{
			"items":       out,
			"total_count": totalCount,
			"offset":      params.Offset,
			"limit":       params.Limit,
		})
		return
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

// courseTypeFilter normalizes the type query param: "private" and "general"
// are the only accepted values; anything else means no type filter.
func courseTypeFilter(raw string) string {
	switch strings.TrimSpace(raw) {
	case "private", "general":
		return strings.TrimSpace(raw)
	default:
		return ""
	}
}

func (s *server) handleCourseSessionsList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	items, err := s.deps.Q.SessionListActiveByCourse(r.Context(), courseID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	conflicts, err := s.deps.Q.SessionConflictsByCourse(r.Context(), courseID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type sessionConflictDTO struct {
		Kind                  string `json:"kind"`
		Resource              string `json:"resource"`
		ConflictingSessionID  string `json:"conflicting_session_id"`
		ConflictingCourseID   string `json:"conflicting_course_id"`
		ConflictingCourseCode string `json:"conflicting_course_code"`
		ConflictingCourseName string `json:"conflicting_course_name"`
		ConflictingStartAt    string `json:"conflicting_start_at"`
		ConflictingEndAt      string `json:"conflicting_end_at"`
	}
	conflictsBySession := make(map[string][]sessionConflictDTO)
	for _, conflict := range conflicts {
		sessionID, err := s.a.UUIDString(conflict.SessionID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		conflictingSessionID, err := s.a.UUIDString(conflict.ConflictingSessionID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		conflictingCourseID, err := s.a.UUIDString(conflict.ConflictingCourseID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		startAt, _ := s.a.TimeString(conflict.ConflictingStartAt)
		endAt, _ := s.a.TimeString(conflict.ConflictingEndAt)
		resource := "teacher"
		if conflict.Kind == "room_overlap" {
			resource = "room"
		}
		conflictsBySession[sessionID] = append(conflictsBySession[sessionID], sessionConflictDTO{
			Kind: conflict.Kind, Resource: resource, ConflictingSessionID: conflictingSessionID,
			ConflictingCourseID: conflictingCourseID, ConflictingCourseCode: conflict.ConflictingCourseCode,
			ConflictingCourseName: conflict.ConflictingCourseName, ConflictingStartAt: startAt, ConflictingEndAt: endAt,
		})
	}
	type sessionDTO struct {
		ID        string               `json:"id"`
		SeriesID  *string              `json:"series_id"`
		CourseID  string               `json:"course_id"`
		RoomID    *string              `json:"room_id"`
		TeacherID string               `json:"teacher_id"`
		StartAt   string               `json:"start_at"`
		EndAt     string               `json:"end_at"`
		Version   int32                `json:"version"`
		Conflicts []sessionConflictDTO `json:"conflicts"`
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
		sessionConflicts := conflictsBySession[sid]
		if sessionConflicts == nil {
			sessionConflicts = []sessionConflictDTO{}
		}
		out = append(out, sessionDTO{ID: sid, SeriesID: seriesID, CourseID: cid, RoomID: rid, TeacherID: tid, StartAt: startS, EndAt: endS, Version: ss.Version, Conflicts: sessionConflicts})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleCoursesCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Code string `json:"code"`
		Name string `json:"name"`

		Year         int16                      `json:"year"`
		SubjectID    string                     `json:"subject_id"`
		Hour         int32                      `json:"hour"`
		StudentCount int32                      `json:"student_count"`
		CourseType   string                     `json:"course_type"`
		Teachers     []teacherAssignmentRequest `json:"teachers"`
		CycleID      *string                    `json:"cycle_id"`
		ExpiryDays   *int32                     `json:"expiry_days"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.ExpiryDays != nil && (*body.ExpiryDays < 0 || *body.ExpiryDays > maxExpiryDays) {
		s.a.WriteErr(w, http.StatusBadRequest, "invalid_expiry_days", fmt.Sprintf("expiry_days must be between 0 and %d", maxExpiryDays))
		return
	}

	teachers, err := parseTeacherAssignments(s.a, body.Teachers)
	if err != nil {
		writeCourseAdminError(w, s.a, err)
		return
	}

	command := courseadmin.CreateCourseCommand{
		ActorID:      pgtype.UUID{Bytes: user.ID, Valid: true},
		Code:         strings.TrimSpace(body.Code),
		Name:         strings.TrimSpace(body.Name),
		Teachers:     teachers,
		Year:         pgtype.Int2{Int16: body.Year, Valid: true},
		Hour:         pgtype.Int4{Int32: body.Hour, Valid: true},
		StudentCount: pgtype.Int4{Int32: body.StudentCount, Valid: true},
		CourseType:   body.CourseType,
		CycleID:      body.CycleID,
		ExpiryDays:   body.ExpiryDays,
	}
	// The course-generation variant (CourseCreateV2: code derived from
	// course_no, name kept empty) requires a subject_id and at least one
	// teacher, matching the historical branch condition (teacher_id +
	// subject_id). courses.teacher_id mirrors the flagged primary of the
	// teacher set and stays NULL when no teacher is flagged primary — an
	// optional primary is the intended contract (no first-teacher fallback).
	if len(teachers) > 0 && body.SubjectID != "" {
		subjectID, err := s.a.ParseUUID(body.SubjectID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_id", "Invalid subject_id")
			return
		}
		command.SubjectID = subjectID
	}

	createdID := ""
	if s.a.WithIdempotentTx(w, r, user.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		result, err := s.deps.CourseAdmin.CreateCourseTx(r.Context(), qtx, command)
		if err != nil {
			writeCourseAdminError(w, s.a, err)
			return 0, nil, err
		}
		current, err := s.deps.CourseAdmin.GetCourseResponse(r.Context(), qtx, result.CourseID)
		if err != nil {
			s.deps.Log.Error("load course response after create failed", "error", err, "course_id", result.CourseID.String())
			writeCourseAdminError(w, s.a, err)
			return 0, nil, err
		}
		createdID = current.ID
		return http.StatusCreated, current, nil
	}) {
		s.publishCourseUpdated(createdID)
	}
}

func (s *server) handleCourseStudentsList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	items, err := s.deps.Q.CourseStudentsListDetailedWithStatus(r.Context(), courseID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	conflicts, err := s.deps.Q.StudentConflictsByCourse(r.Context(), courseID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type studentConflictDTO struct {
		Kind                  string `json:"kind"`
		CurrentSessionID      string `json:"current_session_id"`
		CurrentStartAt        string `json:"current_start_at"`
		CurrentEndAt          string `json:"current_end_at"`
		ConflictingSessionID  string `json:"conflicting_session_id"`
		ConflictingCourseID   string `json:"conflicting_course_id"`
		ConflictingCourseCode string `json:"conflicting_course_code"`
		ConflictingCourseName string `json:"conflicting_course_name"`
		ConflictingStartAt    string `json:"conflicting_start_at"`
		ConflictingEndAt      string `json:"conflicting_end_at"`
	}
	conflictsByStudent := make(map[string][]studentConflictDTO)
	for _, conflict := range conflicts {
		studentID, err := s.a.UUIDString(conflict.StudentID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		currentSessionID, err := s.a.UUIDString(conflict.CurrentSessionID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		conflictingSessionID, err := s.a.UUIDString(conflict.ConflictingSessionID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		conflictingCourseID, err := s.a.UUIDString(conflict.ConflictingCourseID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		currentStartAt, _ := s.a.TimeString(conflict.CurrentStartAt)
		currentEndAt, _ := s.a.TimeString(conflict.CurrentEndAt)
		conflictingStartAt, _ := s.a.TimeString(conflict.ConflictingStartAt)
		conflictingEndAt, _ := s.a.TimeString(conflict.ConflictingEndAt)
		conflictsByStudent[studentID] = append(conflictsByStudent[studentID], studentConflictDTO{
			Kind: "student_overlap", CurrentSessionID: currentSessionID,
			CurrentStartAt: currentStartAt, CurrentEndAt: currentEndAt,
			ConflictingSessionID: conflictingSessionID, ConflictingCourseID: conflictingCourseID,
			ConflictingCourseCode: conflict.ConflictingCourseCode, ConflictingCourseName: conflict.ConflictingCourseName,
			ConflictingStartAt: conflictingStartAt, ConflictingEndAt: conflictingEndAt,
		})
	}
	type studentDTO struct {
		ID           string               `json:"id"`
		Wcode        string               `json:"wcode"`
		FullName     string               `json:"full_name"`
		Notes        string               `json:"notes"`
		Nickname     string               `json:"nickname"`
		School       string               `json:"school"`
		Level        string               `json:"level"`
		Year         string               `json:"year"`
		StudentPhone string               `json:"student_phone"`
		Email        string               `json:"email"`
		Status       string               `json:"status"`
		Conflicts    []studentConflictDTO `json:"conflicts"`
	}
	out := make([]studentDTO, 0, len(items))
	for _, st := range items {
		id, err := s.a.UUIDString(st.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		studentConflicts := conflictsByStudent[id]
		if studentConflicts == nil {
			studentConflicts = []studentConflictDTO{}
		}
		out = append(out, studentDTO{
			ID: id, Wcode: st.Wcode, FullName: st.FullName, Notes: st.Notes,
			Nickname: st.Nickname.String, School: st.School.String, Level: st.Level.String,
			Year: st.Year.String, StudentPhone: st.StudentPhone.String, Email: st.Email.String,
			Status: st.Status, Conflicts: studentConflicts,
		})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleCourseStudentsAdd(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}

	// Block manual roster edits when CRM filter is enabled.
	var crmEnabled bool
	_ = s.deps.DB.QueryRow(r.Context(), `SELECT crm_filter_enabled FROM courses WHERE id=$1`, courseID).Scan(&crmEnabled)
	if crmEnabled {
		s.a.WriteErr(w, http.StatusConflict, "crm_managed_roster", "Roster is managed by CRM filter. Disable CRM filter to edit manually.")
		return
	}

	var body struct {
		StudentID string `json:"student_id"`
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
	cid, _ := s.a.UUIDString(courseID)
	sid, _ := s.a.UUIDString(studentID)

	if s.a.WithIdempotentTx(w, r, actor.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := s.deps.Scheduling.AddCourseStudentTx(r.Context(), tx, qtx, courseID, studentID, scheduling.CourseStudentStatusEnrolled); err != nil {
			var se *scheduling.Err
			if errors.As(err, &se) {
				s.a.WriteErrDetails(w, http.StatusConflict, se.Code, se.Message, se.Details)
				return 0, nil, err
			}
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		actorID := pgtype.UUID{Bytes: actor.ID, Valid: true}
		_, _ = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "course_students.add",
			Payload:     map[string]any{"course_id": cid, "student_id": sid},
		})
		return http.StatusOK, map[string]any{"ok": true}, nil
	}) {
		s.publishCourseUpdated(cid)
	}
}

func (s *server) handleCourseStudentsRemove(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}

	// Block manual roster edits when CRM filter is enabled.
	var crmEnabled bool
	_ = s.deps.DB.QueryRow(r.Context(), `SELECT crm_filter_enabled FROM courses WHERE id=$1`, courseID).Scan(&crmEnabled)
	if crmEnabled {
		s.a.WriteErr(w, http.StatusConflict, "crm_managed_roster", "Roster is managed by CRM filter. Disable CRM filter to edit manually.")
		return
	}

	studentID, err := s.a.ParseUUID(r.PathValue("student_id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_student_id", "Invalid student_id")
		return
	}
	cid, _ := s.a.UUIDString(courseID)
	sid, _ := s.a.UUIDString(studentID)

	if s.a.WithIdempotentTx(w, r, actor.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := s.deps.Scheduling.RemoveCourseStudentTx(r.Context(), qtx, courseID, studentID); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		actorID := pgtype.UUID{Bytes: actor.ID, Valid: true}
		_, _ = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "course_students.remove",
			Payload:     map[string]any{"course_id": cid, "student_id": sid},
		})
		return http.StatusOK, map[string]any{"ok": true}, nil
	}) {
		s.publishCourseUpdated(cid)
	}
}

func (s *server) handleCourseStudentsAddDraft(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}

	// Block manual roster edits when CRM filter is enabled.
	var crmEnabled bool
	_ = s.deps.DB.QueryRow(r.Context(), `SELECT crm_filter_enabled FROM courses WHERE id=$1`, courseID).Scan(&crmEnabled)
	if crmEnabled {
		s.a.WriteErr(w, http.StatusConflict, "crm_managed_roster", "Roster is managed by CRM filter. Disable CRM filter to edit manually.")
		return
	}

	var body struct {
		StudentID string `json:"student_id"`
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

	// Verify student exists.
	var exists bool
	_ = s.deps.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM students WHERE id=$1)`, studentID).Scan(&exists)
	if !exists {
		s.a.WriteErr(w, http.StatusNotFound, "student_not_found", "Student not found")
		return
	}

	courseIDStr, _ := s.a.UUIDString(courseID)
	studentIDStr, _ := s.a.UUIDString(studentID)

	if s.a.WithIdempotentTx(w, r, actor.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := s.deps.Scheduling.AddCourseStudentTx(r.Context(), tx, qtx, courseID, studentID, scheduling.CourseStudentStatusDraft); err != nil {
			var se *scheduling.Err
			if errors.As(err, &se) {
				s.a.WriteErrDetails(w, http.StatusConflict, se.Code, se.Message, se.Details)
				return 0, nil, err
			}
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		actorID := pgtype.UUID{Bytes: actor.ID, Valid: true}
		if _, aErr := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "course_students.add",
			Payload:     map[string]any{"course_id": courseIDStr, "student_id": studentIDStr, "source": "draft"},
		}); aErr != nil {
			s.deps.Log.Error("audit insert failed", "error", aErr, "course_id", courseIDStr, "student_id", studentIDStr)
		}
		return http.StatusOK, map[string]any{"student_id": studentIDStr, "status": "draft"}, nil
	}) {
		s.publishCourseUpdated(courseIDStr)
	}
}

func (s *server) handleCourseStudentsConvert(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	studentID, err := s.a.ParseUUID(r.PathValue("student_id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_student_id", "Invalid student_id")
		return
	}

	// Block manual roster edits when CRM filter is enabled.
	var crmEnabled bool
	_ = s.deps.DB.QueryRow(r.Context(), `SELECT crm_filter_enabled FROM courses WHERE id=$1`, courseID).Scan(&crmEnabled)
	if crmEnabled {
		s.a.WriteErr(w, http.StatusConflict, "crm_managed_roster", "Roster is managed by CRM filter. Disable CRM filter to edit manually.")
		return
	}

	cid, _ := s.a.UUIDString(courseID)
	sid, _ := s.a.UUIDString(studentID)

	if s.a.WithIdempotentTx(w, r, actor.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)

		// Only update if currently draft.
		rows, err := s.deps.Scheduling.ConvertCourseStudentTx(r.Context(), qtx, courseID, studentID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		if rows == 0 {
			// No rows updated - student doesn't exist or wasn't draft.
			var exists bool
			_ = s.deps.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM course_students WHERE course_id=$1 AND student_id=$2)`, courseID, studentID).Scan(&exists)
			if !exists {
				s.a.WriteErr(w, http.StatusNotFound, "not_in_course", "Student is not enrolled in this course")
				return 0, nil, fmt.Errorf("student not in course")
			}
			s.a.WriteErr(w, http.StatusConflict, "not_draft", "Student is already enrolled")
			return 0, nil, fmt.Errorf("not draft")
		}

		actorID := pgtype.UUID{Bytes: actor.ID, Valid: true}
		_, _ = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "course_students.convert",
			Payload:     map[string]any{"course_id": cid, "student_id": sid, "source": "manual"},
		})
		return http.StatusOK, map[string]any{"student_id": sid, "status": "enrolled"}, nil
	}) {
		s.publishCourseUpdated(cid)
	}
}

func (s *server) handleCoursesGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	item, err := s.deps.Q.CourseGetFull(r.Context(), id)
	if err != nil {
		s.deps.Log.Error("course_get_full failed", "error", err, "course_id", r.PathValue("id"))
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	current, err := s.deps.CourseAdmin.GetCourseResponse(r.Context(), s.deps.Q, id)
	if err != nil {
		s.deps.Log.Error("course teacher response failed", "error", err, "course_id", r.PathValue("id"))
		writeCourseAdminError(w, s.a, err)
		return
	}
	out, err := s.courseOverviewResponse(item, current.Teachers)
	if err != nil {
		s.deps.Log.Error("uuid conversion failed", "error", err, "course_id", r.PathValue("id"))
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

// handleCoursesPatch is the versioned course update contract: it atomically
// replaces the course's teacher set inside one transaction, guarded by
// expected_version for optimistic concurrency. Course update is admin-only.
func (s *server) handleCoursesPatch(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	var body updateCourseRequest
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	// The versioned contract requires an explicit teacher set: an absent or
	// null `teachers` key is a client error, while `teachers: []` is the
	// explicit empty set (clears all teachers).
	if body.Teachers == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_request", "teachers is required")
		return
	}
	teachers, err := parseTeacherAssignments(s.a, *body.Teachers)
	if err != nil {
		writeCourseAdminError(w, s.a, err)
		return
	}
	command := courseadmin.UpdateCourseCommand{
		CourseID:        courseID,
		ActorID:         pgtype.UUID{Bytes: user.ID, Valid: true},
		ExpectedVersion: body.ExpectedVersion,
		Code:            strings.TrimSpace(body.Code),
		Name:            strings.TrimSpace(body.Name),
		LegacyCourseID:  normalizeLegacyCourseID(body.LegacyCourseID),
		Teachers:        teachers,
	}
	if err := applyOptionalCourseMetadata(s.a, body, &command); err != nil {
		writeCourseAdminError(w, s.a, err)
		return
	}
	lifecycle, err := parseLifecycle(body)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	command.CycleSet, command.CycleID = lifecycle.CycleSet, lifecycle.CycleID
	command.ExpirySet, command.ExpiryDays = lifecycle.ExpirySet, lifecycle.ExpiryDays
	if s.a.WithIdempotentTx(w, r, user.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		result, err := s.deps.CourseAdmin.UpdateCourseTx(r.Context(), qtx, command)
		if err != nil {
			// The stale_edit details.current the client re-seeds its form from
			// must be the same rich shape as a success response — a sparse
			// current would echo legacy_course_id: null back on the next save.
			var e *courseadmin.Error
			if errors.As(err, &e) {
				s.enrichStaleEditCurrent(r.Context(), qtx, courseID, e)
			}
			writeCourseAdminError(w, s.a, err)
			return 0, nil, err
		}
		out, err := s.loadCourseOverviewResponse(r.Context(), qtx, result.CourseID)
		if err != nil {
			s.deps.Log.Error("load course overview after patch failed", "error", err, "course_id", r.PathValue("id"))
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		return http.StatusOK, out, nil
	}) {
		s.publishCourseUpdated(r.PathValue("id"))
	}
}

func (s *server) handleCoursesUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	var body updateCourseRequest
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	var teachers []courseadmin.TeacherAssignment
	if body.Teachers != nil {
		// Versioned contract: the explicit teacher set replaces the current one.
		teachers, err = parseTeacherAssignments(s.a, *body.Teachers)
		if err != nil {
			writeCourseAdminError(w, s.a, err)
			return
		}
	}
	// With no `teachers` key the PUT is metadata-only: Teachers stays nil, so
	// the service updates code/name/legacy link and bumps the version while
	// leaving the existing teacher set untouched.

	command := courseadmin.UpdateCourseCommand{
		CourseID:        courseID,
		ActorID:         pgtype.UUID{Bytes: user.ID, Valid: true},
		ExpectedVersion: body.ExpectedVersion,
		Code:            strings.TrimSpace(body.Code),
		Name:            strings.TrimSpace(body.Name),
		LegacyCourseID:  normalizeLegacyCourseID(body.LegacyCourseID),
		Teachers:        teachers,
	}
	if err := applyOptionalCourseMetadata(s.a, body, &command); err != nil {
		writeCourseAdminError(w, s.a, err)
		return
	}
	lifecycle, err := parseLifecycle(body)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	command.CycleSet, command.CycleID = lifecycle.CycleSet, lifecycle.CycleID
	command.ExpirySet, command.ExpiryDays = lifecycle.ExpirySet, lifecycle.ExpiryDays

	if s.a.WithIdempotentTx(w, r, user.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if command.ExpectedVersion <= 0 {
			// Legacy clients don't send expected_version. Read the current
			// version under the row lock inside this same transaction and use
			// it as the precondition: the lock is held by this transaction, so
			// the value cannot change before the service's own lock+compare.
			locked, lockErr := qtx.CourseLockForTeacherUpdate(r.Context(), courseID)
			if lockErr != nil {
				status, code, msg := s.a.ClassifyDBErr(lockErr)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, lockErr
			}
			command.ExpectedVersion = locked.Version
		}
		result, err := s.deps.CourseAdmin.UpdateCourseTx(r.Context(), qtx, command)
		if err != nil {
			var e *courseadmin.Error
			if errors.As(err, &e) {
				s.enrichStaleEditCurrent(r.Context(), qtx, courseID, e)
			}
			writeCourseAdminError(w, s.a, err)
			return 0, nil, err
		}
		out, err := s.loadCourseOverviewResponse(r.Context(), qtx, result.CourseID)
		if err != nil {
			s.deps.Log.Error("load course overview after update failed", "error", err, "course_id", r.PathValue("id"))
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		return http.StatusOK, out, nil
	}) {
		s.publishCourseUpdated(r.PathValue("id"))
	}
}

func (s *server) handleCoursesDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	s.deps.Log.Debug("deleting course", "course_id", r.PathValue("id"))
	if !s.a.WithIdempotentTx(w, r, user.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.CourseDelete(r.Context(), id); err != nil {
			s.deps.Log.Error("course_delete failed", "error", err, "course_id", r.PathValue("id"))
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{"ok": true}, nil
	}) {
		s.deps.Log.Error("course_delete: idempotent tx failed", "course_id", r.PathValue("id"))
	} else {
		s.publishCourseUpdated(r.PathValue("id"))
	}
}

func (s *server) handleLegacySync(w http.ResponseWriter, r *http.Request) {
	_, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}

	var legacyCourseID pgtype.Text
	if err := s.deps.DB.QueryRow(r.Context(), `SELECT legacy_course_id FROM courses WHERE id = $1`, courseID).Scan(&legacyCourseID); err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if !legacyCourseID.Valid {
		s.a.WriteErr(w, http.StatusBadRequest, "no_legacy_link", "Course has no legacy system link")
		return
	}

	payload, _ := json.Marshal(map[string]string{"course_id": courseID.String(), "requested_by": "course_detail"})
	now := timeNowUTC()
	job, err := s.deps.Q.LegacyJobEnqueue(r.Context(), sqldb.LegacyJobEnqueueParams{
		JobType:     "legacy_refresh_course",
		EntityType:  pgtype.Text{String: "course", Valid: true},
		ExternalID:  legacyCourseID,
		Payload:     string(payload),
		UniqueKey:   pgtype.Text{String: "legacy:course:" + legacyCourseID.String, Valid: true},
		Priority:    1,
		DeadlineAt:  pgtype.Timestamptz{Time: now.Add(10 * time.Minute), Valid: true},
		MaxAttempts: 5,
		RunAfter:    pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "job_id": job.ID.String()})
}

func (s *server) handleCourseLegacyConflicts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid course id")
		return
	}
	// Verify course exists
	var legacyCourseID pgtype.Text
	if err := s.deps.DB.QueryRow(r.Context(), `SELECT legacy_course_id FROM courses WHERE id=$1`, courseID).Scan(&legacyCourseID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.a.WriteErr(w, http.StatusNotFound, "not_found", "Course not found")
			return
		}
		s.a.WriteErr(w, http.StatusInternalServerError, "db_error", "Database error")
		return
	}
	if !legacyCourseID.Valid {
		s.a.WriteJSON(w, http.StatusOK, map[string]any{
			"course_id":        courseID.String(),
			"legacy_course_id": nil,
			"open_conflicts":   []any{},
		})
		return
	}
	// Query conflicts via the custom query
	queries := sqldb.New(s.deps.DB)
	conflicts, err := queries.LegacyCourseConflicts(r.Context(), courseID)
	if err != nil {
		s.deps.Log.Error("legacy_course_conflicts failed", "error", err, "course_id", courseID.String())
		s.a.WriteErr(w, http.StatusInternalServerError, "db_error", "Database error")
		return
	}
	type conflictDTO struct {
		ID            string  `json:"id"`
		ConflictType  string  `json:"conflict_type"`
		Category      string  `json:"category"`
		Message       *string `json:"message"`
		SourcePayload *string `json:"source_payload"`
		LocalPayload  *string `json:"local_payload"`
		CreatedAt     *string `json:"created_at"`
	}
	out := make([]conflictDTO, 0, len(conflicts))
	for _, c := range conflicts {
		idStr, _ := s.a.UUIDString(c.ID)
		var msg *string
		if c.Message.Valid {
			msg = &c.Message.String
		}
		var srcPayload *string
		if len(c.SourcePayload) > 0 {
			s := string(c.SourcePayload)
			srcPayload = &s
		}
		var localPayload *string
		if len(c.LocalPayload) > 0 {
			s := string(c.LocalPayload)
			localPayload = &s
		}
		var createdAt *string
		if c.CreatedAt.Valid {
			t := c.CreatedAt.Time.UTC().Format(time.RFC3339Nano)
			createdAt = &t
		}
		out = append(out, conflictDTO{
			ID:            idStr,
			ConflictType:  c.ConflictType,
			Category:      c.Category,
			Message:       msg,
			SourcePayload: srcPayload,
			LocalPayload:  localPayload,
			CreatedAt:     createdAt,
		})
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"course_id":        courseID.String(),
		"legacy_course_id": legacyCourseID.String,
		"open_conflicts":   out,
	})
}

func timeNowUTC() time.Time { return time.Now().UTC() }

func (s *server) handleCoursesBatchDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if len(body.IDs) == 0 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_ids", "ids array is required")
		return
	}
	if len(body.IDs) > 100 {
		s.a.WriteErr(w, http.StatusBadRequest, "too_many", "Maximum 100 courses per batch")
		return
	}

	ids := make([]pgtype.UUID, 0, len(body.IDs))
	for _, raw := range body.IDs {
		id, err := s.a.ParseUUID(raw)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid course ID: "+raw)
			return
		}
		ids = append(ids, id)
	}

	s.deps.Log.Debug("batch deleting courses", "count", len(ids))
	var succeededIDs []string
	if !s.a.WithIdempotentTx(w, r, user.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		results := qtx.CourseBatchDelete(r.Context(), ids)

		succeeded := make([]string, 0, len(results))
		failed := make([]map[string]any, 0)
		for _, res := range results {
			idStr, _ := s.a.UUIDString(res.ID)
			if res.Success {
				succeeded = append(succeeded, idStr)
			} else {
				failed = append(failed, map[string]any{"id": idStr, "error": res.Error})
			}
		}
		succeededIDs = append(succeededIDs, succeeded...)
		return http.StatusOK, map[string]any{
			"succeeded":       succeeded,
			"failed":          failed,
			"total_processed": len(results),
		}, nil
	}) {
		s.deps.Log.Error("course_batch_delete: idempotent tx failed", "count", len(ids))
	} else {
		s.publishCourseUpdates(succeededIDs)
	}
}

// normalizeLegacyCourseID maps the optional legacy_course_id request field to
// the service contract: an empty (or whitespace-only) value means "no link"
// and becomes nil.
func normalizeLegacyCourseID(s *string) *string {
	if s == nil {
		return nil
	}
	if strings.TrimSpace(*s) == "" {
		return nil
	}
	return s
}

// loadCourseOverviewResponse builds the rich course payload (full overview row
// plus teacher set) inside the caller's transaction. Every mutation response —
// PATCH, PUT, and the stale_edit current — shares this shape with GET so a
// client that echoes fields like legacy_course_id back on the next edit never
// drops them: a sparse response would make the next save send null.
func (s *server) loadCourseOverviewResponse(ctx context.Context, qtx *sqldb.Queries, courseID pgtype.UUID) (map[string]any, error) {
	item, err := qtx.CourseGetFull(ctx, courseID)
	if err != nil {
		return nil, err
	}
	current, err := s.deps.CourseAdmin.GetCourseResponse(ctx, qtx, courseID)
	if err != nil {
		return nil, err
	}
	return s.courseOverviewResponse(item, current.Teachers)
}

// enrichStaleEditCurrent upgrades the sparse details.current of a stale_edit
// error to the full courseOverviewResponse shape, so the client's re-seed of
// the edit form keeps legacy_course_id and the overview fields instead of
// echoing null on the next save. It reads through the still-open transaction
// (the course row is already locked by UpdateCourseTx's version precheck, so
// the snapshot is consistent); on any read failure the original sparse current
// is left untouched rather than failing the whole request.
func (s *server) enrichStaleEditCurrent(ctx context.Context, qtx *sqldb.Queries, courseID pgtype.UUID, e *courseadmin.Error) {
	if e == nil || e.Code != "stale_edit" {
		return
	}
	rich, err := s.loadCourseOverviewResponse(ctx, qtx, courseID)
	if err != nil {
		s.deps.Log.Error("enrich stale_edit current failed", "error", err, "course_id", courseID.String())
		return
	}
	e.Details["current"] = rich
}

const maxExpiryDays int32 = 106751

func courseExpiry(expiryDays pgtype.Int4, lastSessionAt pgtype.Timestamptz) (any, string) {
	if !expiryDays.Valid || !lastSessionAt.Valid || expiryDays.Int32 < 0 || expiryDays.Int32 > maxExpiryDays {
		return nil, "not_configured"
	}
	expires := lastSessionAt.Time.UTC().Add(time.Duration(expiryDays.Int32) * 24 * time.Hour)
	if !time.Now().UTC().Before(expires) {
		return expires.Format(time.RFC3339Nano), "expired"
	}
	return expires.Format(time.RFC3339Nano), "active"
}

// courseOverviewResponse builds the legacy rich course payload (all overview
// fields plus version and the teacher set). GET and the transitional PUT share
// it so legacy clients keep receiving the full shape during the migration.
func (s *server) courseOverviewResponse(item sqldb.CourseOverviewRow, teachers []courseadmin.CourseTeacherResponse) (map[string]any, error) {
	cid, err := s.a.UUIDString(item.ID)
	if err != nil {
		return nil, err
	}
	var teacherID any = nil
	if item.TeacherID.Valid {
		tid, err := s.a.UUIDString(item.TeacherID)
		if err == nil {
			teacherID = tid
		}
	}
	var subjectID any = nil
	if item.SubjectID.Valid {
		sid, err := s.a.UUIDString(item.SubjectID)
		if err == nil {
			subjectID = sid
		}
	}
	var legacyCourseID any = nil
	if item.LegacyCourseID.Valid {
		legacyCourseID = item.LegacyCourseID.String
	}
	var legacyLastSyncedAt any = nil
	if item.LegacyLastSyncedAt.Valid {
		legacyLastSyncedAt, _ = s.a.TimeString(item.LegacyLastSyncedAt)
	}
	var year any = nil
	if item.Year.Valid {
		year = item.Year.Int16
	}
	var hour any = nil
	if item.Hour.Valid {
		hour = item.Hour.Int32
	}
	var studentCount any = nil
	if item.StudentCount.Valid {
		studentCount = item.StudentCount.Int32
	}
	var courseType any = nil
	if item.CourseType.Valid {
		courseType = item.CourseType.String
	}
	var cycleID any
	if item.CycleID.Valid {
		cycleID = item.CycleID.String
	}
	var expiryDays any
	if item.ExpiryDays.Valid {
		expiryDays = item.ExpiryDays.Int32
	}
	var lastSessionAt any
	if item.LastSessionAt.Valid {
		lastSessionAt, _ = s.a.TimeString(item.LastSessionAt)
	}
	expiresAt, expiryStatus := courseExpiry(item.ExpiryDays, item.LastSessionAt)
	return map[string]any{
		"id":                    cid,
		"course_no":             item.CourseNo,
		"code":                  item.Code,
		"name":                  item.Name,
		"year":                  year,
		"teacher_id":            teacherID,
		"primary_teacher_id":    teacherID,
		"teacher_name":          item.TeacherName,
		"subject_id":            subjectID,
		"subject_code":          item.SubjectCode,
		"subject_name":          item.SubjectName,
		"hour":                  hour,
		"student_count":         studentCount,
		"course_type":           courseType,
		"cycle_id":              cycleID,
		"cycle_label":           item.CycleLabel,
		"expiry_days":           expiryDays,
		"last_session_at":       lastSessionAt,
		"expires_at":            expiresAt,
		"expiry_status":         expiryStatus,
		"legacy_course_id":      legacyCourseID,
		"legacy_last_synced_at": legacyLastSyncedAt,
		"absence_form_visible":  item.AbsenceFormVisible,
		"version":               item.Version.Int32,
		"teachers":              teachers,
	}, nil
}
