package courseshttp

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/coursegroups"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

func registerCourseGroupRoutes(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}
	mux.HandleFunc("POST /api/v1/course-groups", s.handleCourseGroupCreate)
	mux.HandleFunc("GET /api/v1/course-groups", s.handleCourseGroupList)
	mux.HandleFunc("GET /api/v1/course-groups/{id}", s.handleCourseGroupGet)
	mux.HandleFunc("PATCH /api/v1/course-groups/{id}", s.handleCourseGroupUpdate)
	mux.HandleFunc("GET /api/v1/course-groups/{id}/sessions", s.handleCourseGroupSessions)
	mux.HandleFunc("DELETE /api/v1/course-groups/{id}", s.handleCourseGroupDelete)
}

func (s *server) handleCourseGroupList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	items, err := s.deps.Q.CourseMergeGroupList(r.Context())
	if err != nil {
		writeCourseGroupError(w, s.a, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		id, err := s.a.UUIDString(item.ID)
		if err != nil {
			writeCourseGroupError(w, s.a, err)
			return
		}
		out = append(out, map[string]any{
			"id":           id,
			"name":         item.Name,
			"member_count": item.MemberCount,
			"course_codes": item.CourseCodes,
		})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func writeCourseGroupError(w http.ResponseWriter, a httpadapter.Adapter, err error) {
	var domainErr *coursegroups.Error
	if errors.As(err, &domainErr) {
		a.WriteErr(w, coursegroups.HTTPStatusForError(domainErr), domainErr.Code, domainErr.Message)
		return
	}
	status, code, message := a.ClassifyDBErr(err)
	a.WriteErr(w, status, code, message)
}

func (s *server) handleCourseGroupCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Name      string   `json:"name"`
		CourseIDs []string `json:"course_ids"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if len(body.CourseIDs) != coursegroups.RequiredMemberCount {
		writeCourseGroupError(w, s.a, &coursegroups.Error{Code: "invalid_course_ids", Message: "Select exactly two different courses to merge."})
		return
	}
	courseIDs := make([]pgtype.UUID, 0, len(body.CourseIDs))
	for _, rawID := range body.CourseIDs {
		id, err := s.a.ParseUUID(rawID)
		if err != nil {
			writeCourseGroupError(w, s.a, &coursegroups.Error{Code: "invalid_course_ids", Message: "Select exactly two different courses to merge."})
			return
		}
		courseIDs = append(courseIDs, id)
	}

	var groupID string
	completed := s.a.WithIdempotentTx(w, r, user.ID, "course-groups", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		result, err := coursegroups.NewService().CreateTx(r.Context(), qtx, coursegroups.CreateCommand{
			ActorID:   pgtype.UUID{Bytes: user.ID, Valid: true},
			Name:      body.Name,
			CourseIDs: courseIDs,
		})
		if err != nil {
			writeCourseGroupError(w, s.a, err)
			return 0, nil, err
		}
		groupID = result.GroupID.String()
		return http.StatusCreated, map[string]any{
			"id":         groupID,
			"name":       body.Name,
			"course_ids": body.CourseIDs,
		}, nil
	})
	if completed {
		s.publishCourseUpdates(body.CourseIDs)
	}
}

func (s *server) handleCourseGroupGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid merged course ID")
		return
	}
	group, err := s.deps.Q.CourseMergeGroupGet(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.a.WriteErr(w, http.StatusNotFound, "not_found", "Merged course not found")
			return
		}
		writeCourseGroupError(w, s.a, err)
		return
	}
	response, err := s.courseGroupResponse(r.Context(), s.deps.Q, group)
	if err != nil {
		writeCourseGroupError(w, s.a, err)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, response)
}

func (s *server) handleCourseGroupUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid merged course ID")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	s.a.WithIdempotentTx(w, r, user.ID, "course-groups", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		result, err := coursegroups.NewService().UpdateNameTx(r.Context(), qtx, coursegroups.UpdateNameCommand{
			ActorID: pgtype.UUID{Bytes: user.ID, Valid: true},
			GroupID: id,
			Name:    body.Name,
		})
		if err != nil {
			writeCourseGroupError(w, s.a, err)
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{
			"id":   result.GroupID.String(),
			"name": result.NewName,
		}, nil
	})
}

func (s *server) handleCourseGroupDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid merged course ID")
		return
	}

	var courseIDs []string
	completed := s.a.WithIdempotentTx(w, r, user.ID, "course-groups", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		result, err := coursegroups.NewService().DeleteTx(r.Context(), qtx, coursegroups.DeleteCommand{
			ActorID: pgtype.UUID{Bytes: user.ID, Valid: true},
			GroupID: id,
		})
		if err != nil {
			writeCourseGroupError(w, s.a, err)
			return 0, nil, err
		}
		courseIDs = make([]string, 0, len(result.CourseIDs))
		for _, courseID := range result.CourseIDs {
			courseIDs = append(courseIDs, courseID.String())
		}
		return http.StatusOK, map[string]any{
			"ok":         true,
			"id":         result.GroupID.String(),
			"course_ids": courseIDs,
		}, nil
	})
	if completed {
		s.publishCourseUpdates(courseIDs)
	}
}

func (s *server) handleCourseGroupSessions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid merged course ID")
		return
	}
	if _, err := s.deps.Q.CourseMergeGroupGet(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.a.WriteErr(w, http.StatusNotFound, "not_found", "Merged course not found")
			return
		}
		writeCourseGroupError(w, s.a, err)
		return
	}
	items, err := s.deps.Q.CourseMergeGroupSessions(r.Context(), id)
	if err != nil {
		writeCourseGroupError(w, s.a, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		itemID, err := s.a.UUIDString(item.ID)
		if err != nil {
			writeCourseGroupError(w, s.a, err)
			return
		}
		courseID, err := s.a.UUIDString(item.CourseID)
		if err != nil {
			writeCourseGroupError(w, s.a, err)
			return
		}
		teacherID, err := s.a.UUIDString(item.TeacherID)
		if err != nil {
			writeCourseGroupError(w, s.a, err)
			return
		}
		start, _ := s.a.TimeString(item.StartAt)
		end, _ := s.a.TimeString(item.EndAt)
		var roomID any
		if item.RoomID.Valid {
			roomID, err = s.a.UUIDString(item.RoomID)
			if err != nil {
				writeCourseGroupError(w, s.a, err)
				return
			}
		}
		out = append(out, map[string]any{
			"id":           itemID,
			"course_id":    courseID,
			"course_code":  item.CourseCode,
			"course_name":  item.CourseName,
			"room_id":      roomID,
			"teacher_id":   teacherID,
			"teacher_name": item.TeacherName,
			"start_at":     start,
			"end_at":       end,
			"version":      item.Version,
		})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}
