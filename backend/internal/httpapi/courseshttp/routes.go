package courseshttp

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/legacysync"
	"warwick-institute/internal/scheduling"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("GET /api/v1/courses", s.handleCoursesList)
	mux.HandleFunc("POST /api/v1/courses", s.handleCoursesCreate)
	mux.HandleFunc("GET /api/v1/courses/{id}", s.handleCoursesGet)
	mux.HandleFunc("PUT /api/v1/courses/{id}", s.handleCoursesUpdate)
	mux.HandleFunc("DELETE /api/v1/courses/{id}", s.handleCoursesDelete)
	mux.HandleFunc("GET /api/v1/courses/{id}/students", s.handleCourseStudentsList)
	mux.HandleFunc("POST /api/v1/courses/{id}/students", s.handleCourseStudentsAdd)
	mux.HandleFunc("DELETE /api/v1/courses/{id}/students/{student_id}", s.handleCourseStudentsRemove)
	mux.HandleFunc("POST /api/v1/courses/{id}/students/draft", s.handleCourseStudentsAddDraft)
	mux.HandleFunc("POST /api/v1/courses/{id}/students/{student_id}/convert", s.handleCourseStudentsConvert)
	mux.HandleFunc("GET /api/v1/courses/{id}/sessions", s.handleCourseSessionsList)
	mux.HandleFunc("POST /api/v1/courses/{id}/legacy-sync", s.handleLegacySync)
}

func (s *server) handleCoursesList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	includeArchived := strings.TrimSpace(r.URL.Query().Get("include_archived")) == "1"
	items, err := s.deps.Q.CourseOverview(r.Context(), sqldb.CourseOverviewParams{IncludeArchived: includeArchived})
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type courseDTO struct {
		ID                string `json:"id"`
		CourseNo          int64  `json:"course_no"`
		Code              string `json:"code"`
		Name              string `json:"name"`
		Year              any    `json:"year"`
		TeacherID         any    `json:"teacher_id"`
		TeacherName       string `json:"teacher_name"`
		SubjectID         any    `json:"subject_id"`
		SubjectCode       string `json:"subject_code"`
		SubjectName       string `json:"subject_name"`
		Hour              any    `json:"hour"`
		StudentCount      any    `json:"student_count"`
		CourseType        any    `json:"course_type"`
		LegacyCourseID    any    `json:"legacy_course_id"`
		LegacyLastSyncedAt any   `json:"legacy_last_synced_at"`
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
		out = append(out, courseDTO{
			ID:                 id,
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
		})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
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

func (s *server) handleCoursesCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Code string `json:"code"`
		Name string `json:"name"`

		Year         int16  `json:"year"`
		TeacherID    string `json:"teacher_id"`
		SubjectID    string `json:"subject_id"`
		Hour         int32  `json:"hour"`
		StudentCount int32  `json:"student_count"`
		CourseType   string `json:"course_type"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	scope := "courses"
	if body.TeacherID != "" && body.SubjectID != "" {
		teacherID, err := s.a.ParseUUID(body.TeacherID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_teacher_id", "Invalid teacher_id")
			return
		}
		subjectID, err := s.a.ParseUUID(body.SubjectID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_id", "Invalid subject_id")
			return
		}
		s.a.WithIdempotentTx(w, r, user.ID, scope, s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
			qtx := s.deps.Q.WithTx(tx)
			item, err := qtx.CourseCreateV2(r.Context(), sqldb.CourseCreateV2Params{
				Year:         pgtype.Int2{Int16: body.Year, Valid: true},
				TeacherID:    teacherID,
				SubjectID:    subjectID,
				Hour:         pgtype.Int4{Int32: body.Hour, Valid: true},
				StudentCount: pgtype.Int4{Int32: body.StudentCount, Valid: true},
				CourseType:   body.CourseType,
			})
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
			id, err := s.a.UUIDString(item.ID)
			if err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return 0, nil, err
			}
			return http.StatusCreated, map[string]any{"id": id, "course_no": item.CourseNo, "code": item.Code}, nil
		})
		return
	}

	s.a.WithIdempotentTx(w, r, user.ID, scope, s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		item, err := qtx.CourseCreate(r.Context(), sqldb.CourseCreateParams{Code: body.Code, Name: body.Name})
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		id, err := s.a.UUIDString(item.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		return http.StatusCreated, map[string]any{"id": id, "code": item.Code, "name": item.Name}, nil
	})
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
	type studentDTO struct {
		ID       string `json:"id"`
		Wcode    string `json:"wcode"`
		FullName string `json:"full_name"`
		Notes    string `json:"notes"`
		Status   string `json:"status"`
	}
	out := make([]studentDTO, 0, len(items))
	for _, st := range items {
		id, err := s.a.UUIDString(st.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		out = append(out, studentDTO{ID: id, Wcode: st.Wcode, FullName: st.FullName, Notes: st.Notes, Status: st.Status})
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

	s.a.WithIdempotentTx(w, r, actor.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.CourseStudentAdd(r.Context(), sqldb.CourseStudentAddParams{CourseID: courseID, StudentID: studentID}); err != nil {
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
	})
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

	s.a.WithIdempotentTx(w, r, actor.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.CourseStudentRemove(r.Context(), sqldb.CourseStudentRemoveParams{CourseID: courseID, StudentID: studentID}); err != nil {
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
	})
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

	// Run preflight checking this student's busy ranges against the course's existing sessions.
	// We need to preflight with just this one student's ID.
	studentIDs := []pgtype.UUID{studentID}

	courseIDStr, _ := s.a.UUIDString(courseID)
	studentIDStr, _ := s.a.UUIDString(studentID)

	// Preflight: get time bounds from the course's sessions.
	var minStart, maxEnd pgtype.Timestamptz
	err = s.deps.DB.QueryRow(r.Context(), `
		SELECT MIN(start_at), MAX(end_at) FROM sessions
		WHERE course_id = $1 AND deleted_at IS NULL
	`, courseID).Scan(&minStart, &maxEnd)
	if err != nil || !minStart.Valid || !maxEnd.Valid {
		// No sessions exist, so preflight passes automatically.
		s.a.WithIdempotentTx(w, r, actor.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
			qtx := s.deps.Q.WithTx(tx)
			if err := qtx.CourseStudentAddDraft(r.Context(), sqldb.CourseStudentAddDraftParams{CourseID: courseID, StudentID: studentID}); err != nil {
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
		})
		return
	}

	// Build conflict requested struct.
	startStr, _ := s.a.TimeString(minStart)
	endStr, _ := s.a.TimeString(maxEnd)
	requested := scheduling.ConflictRequested{
		StartAt:  startStr,
		EndAt:    endStr,
		CourseID: courseIDStr,
	}

	result, se, err := s.deps.Scheduling.Preflight(r.Context(), scheduling.PreflightParams{
		CourseID:   courseID,
		StartAt:    minStart,
		EndAt:      maxEnd,
		StudentIDs: &studentIDs,
		Requested:  requested,
	})
	_ = result
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	if se != nil {
		s.a.WriteErrDetails(w, http.StatusConflict, se.Code, se.Message, se.Details)
		return
	}

	// Preflight passed — add as draft.
	s.a.WithIdempotentTx(w, r, actor.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.CourseStudentAddDraft(r.Context(), sqldb.CourseStudentAddDraftParams{CourseID: courseID, StudentID: studentID}); err != nil {
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
	})
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

	s.a.WithIdempotentTx(w, r, actor.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)

		// Only update if currently draft.
		rows, err := qtx.CourseStudentUpdateStatusRow(r.Context(), sqldb.CourseStudentUpdateStatusRowParams{
			CourseID:  courseID,
			StudentID: studentID,
			NewStatus: "enrolled",
			OldStatus: "draft",
		})
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
	})
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
	item, err := s.deps.Q.CourseGetByID(r.Context(), id)
	if err != nil {
		s.deps.Log.Error("course_get_by_id failed", "error", err, "course_id", r.PathValue("id"))
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	// Fetch legacy fields separately since CourseGetByID doesn't include them.
	var legacyCourseID any = nil
	var legacyLastSyncedAt any = nil
	var legacyCID pgtype.Text
	var legacyLastSynced pgtype.Timestamptz
	_ = s.deps.DB.QueryRow(r.Context(), `SELECT legacy_course_id, legacy_last_synced_at FROM courses WHERE id = $1`, id).Scan(&legacyCID, &legacyLastSynced)
	if legacyCID.Valid {
		legacyCourseID = legacyCID.String
	}
	if legacyLastSynced.Valid {
		legacyLastSyncedAt, _ = s.a.TimeString(legacyLastSynced)
	}

	cid, err := s.a.UUIDString(item.ID)
	if err != nil {
		s.deps.Log.Error("uuid conversion failed", "error", err, "course_id", r.PathValue("id"))
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"id":                   cid,
		"code":                 item.Code,
		"name":                 item.Name,
		"legacy_course_id":     legacyCourseID,
		"legacy_last_synced_at": legacyLastSyncedAt,
	})
}

func (s *server) handleCoursesUpdate(w http.ResponseWriter, r *http.Request) {
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
		Code           string `json:"code"`
		Name           string `json:"name"`
		LegacyCourseID *string `json:"legacy_course_id"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	s.a.WithIdempotentTx(w, r, user.ID, "courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		item, err := qtx.CourseUpdate(r.Context(), sqldb.CourseUpdateParams{ID: id, Code: body.Code, Name: body.Name})
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		cid, err := s.a.UUIDString(item.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		// Update legacy_course_id separately (not part of sqlc-generated CourseUpdate).
		if err := qtx.CourseUpdateLegacyLink(r.Context(), id, pgtype.Text{String: strPtrOr(body.LegacyCourseID, ""), Valid: body.LegacyCourseID != nil}); err != nil {
			s.deps.Log.Error("legacy_course_id update failed", "error", err, "course_id", cid)
		}
		// Read back legacy fields for response.
		var legacyCourseID any = nil
		var legacyLastSyncedAt any = nil
		if body.LegacyCourseID != nil {
			legacyCourseID = *body.LegacyCourseID
		}
		var legacyLastSynced pgtype.Timestamptz
		_ = s.deps.DB.QueryRow(r.Context(), `SELECT legacy_last_synced_at FROM courses WHERE id = $1`, id).Scan(&legacyLastSynced)
		if legacyLastSynced.Valid {
			legacyLastSyncedAt, _ = s.a.TimeString(legacyLastSynced)
		}
		return http.StatusOK, map[string]any{
			"id":                    cid,
			"code":                  item.Code,
			"name":                  item.Name,
			"legacy_course_id":      legacyCourseID,
			"legacy_last_synced_at": legacyLastSyncedAt,
		}, nil
	})
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

	loc, err := time.LoadLocation(s.deps.InstituteTZ)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Invalid timezone configuration")
		return
	}

	client, err := legacysync.NewClient(s.deps.LegacySyncURL, s.deps.LegacySyncUsername, s.deps.LegacySyncPassword)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Failed to create legacy sync client")
		return
	}

	scraper := legacysync.NewScraper(client, s.deps.DB, s.deps.Q, s.deps.Log, loc)

	result, err := scraper.SyncCourse(r.Context(), courseID, legacyCourseID.String)
	if err != nil {
		s.deps.Log.Error("legacy sync failed", "error", err, "course_id", r.PathValue("id"), "legacy_course_id", legacyCourseID.String)
		s.a.WriteErr(w, http.StatusInternalServerError, "sync_failed", "Legacy sync failed: "+err.Error())
		return
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"sessions_created": result.SessionsCreated,
		"synced_at":        result.SyncedAt,
	})
}

func strPtrOr(s *string, fallback string) string {
	if s != nil {
		return *s
	}
	return fallback
}
