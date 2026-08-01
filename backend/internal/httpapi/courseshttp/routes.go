package courseshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/courseadmin"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/legacysync"
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
			teachersByCourse[cid] = append(teachersByCourse[cid], map[string]any{"id": tid, "username": te.Username, "is_primary": te.IsPrimary})
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
			Teachers:           teachersByCourse[cid],
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

		Year         int16                      `json:"year"`
		SubjectID    string                     `json:"subject_id"`
		Hour         int32                      `json:"hour"`
		StudentCount int32                      `json:"student_count"`
		CourseType   string                     `json:"course_type"`
		Teachers     []teacherAssignmentRequest `json:"teachers"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
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
		s.a.WriteErr(w, http.StatusInternalServerError, "sync_failed", "Legacy sync failed")
		return
	}
	if result.SessionsCreated > 0 {
		s.publishSessionsUpdated()
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"sessions_created": result.SessionsCreated,
		"synced_at":        result.SyncedAt,
	})
}

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
	return map[string]any{
		"id":                    cid,
		"course_no":             item.CourseNo,
		"code":                  item.Code,
		"name":                  item.Name,
		"year":                  year,
		"teacher_id":            teacherID,
		"teacher_name":          item.TeacherName,
		"subject_id":            subjectID,
		"subject_code":          item.SubjectCode,
		"subject_name":          item.SubjectName,
		"hour":                  hour,
		"student_count":         studentCount,
		"course_type":           courseType,
		"legacy_course_id":      legacyCourseID,
		"legacy_last_synced_at": legacyLastSyncedAt,
		"version":               item.Version.Int32,
		"teachers":              teachers,
	}, nil
}
