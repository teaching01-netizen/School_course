package studentshttp

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

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

	mux.HandleFunc("GET /api/v1/students", s.handleStudentsList)
	mux.HandleFunc("POST /api/v1/students", s.handleStudentsCreate)
	mux.HandleFunc("GET /api/v1/students/{id}", s.handleStudentsGet)
	mux.HandleFunc("GET /api/v1/students/by-wcode", s.handleStudentsGetByWCode)
	mux.HandleFunc("GET /api/v1/students/{id}/courses", s.handleStudentCoursesList)
	mux.HandleFunc("PUT /api/v1/students/{id}", s.handleStudentsUpdate)
}

func (s *server) handleStudentsList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	limit := int32(50)
	offset := int32(0)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = int32(n)
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n)
		}
	}
	items, err := s.deps.Q.StudentList(r.Context(), sqldb.StudentListParams{Limit: limit, Offset: offset})
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
	}
	out := make([]studentDTO, 0, len(items))
	for _, st := range items {
		sid, err := s.a.UUIDString(st.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		out = append(out, studentDTO{ID: sid, Wcode: st.Wcode, FullName: st.FullName, Notes: st.Notes})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleStudentsCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var req sqldb.StudentCreateParams
	if err := s.a.DecodeJSON(w, r, &req); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	s.a.WithIdempotentTx(w, r, user.ID, "students", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		item, err := qtx.StudentCreate(r.Context(), req)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		sid, err := s.a.UUIDString(item.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		return http.StatusCreated, map[string]any{"id": sid, "wcode": item.Wcode, "full_name": item.FullName, "notes": item.Notes}, nil
	})
}

func (s *server) handleStudentsGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	item, err := s.deps.Q.StudentGetByID(r.Context(), id)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	sid, err := s.a.UUIDString(item.ID)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"id": sid, "wcode": item.Wcode, "full_name": item.FullName, "notes": item.Notes})
}

func (s *server) handleStudentsGetByWCode(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	wcode := r.URL.Query().Get("wcode")
	if wcode == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_wcode", "Invalid wcode")
		return
	}
	item, err := s.deps.Q.StudentGetByWCode(r.Context(), wcode)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	sid, err := s.a.UUIDString(item.ID)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"id": sid, "wcode": item.Wcode, "full_name": item.FullName, "notes": item.Notes})
}

func (s *server) handleStudentCoursesList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	studentID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	items, err := s.deps.Q.StudentCoursesList(r.Context(), studentID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type courseDTO struct {
		ID           string `json:"id"`
		Code         string `json:"code"`
		Name         string `json:"name"`
		TeacherName  string `json:"teacher_name"`
		SubjectCode  string `json:"subject_code"`
		SubjectName  string `json:"subject_name"`
		StudentCount any    `json:"student_count"`
		CourseType   any    `json:"course_type"`
	}
	out := make([]courseDTO, 0, len(items))
	for _, c := range items {
		id, err := s.a.UUIDString(c.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		var studentCount any = nil
		if c.StudentCount.Valid {
			studentCount = c.StudentCount.Int32
		}
		var courseType any = nil
		if c.CourseType.Valid {
			courseType = c.CourseType.String
		}
		out = append(out, courseDTO{
			ID:           id,
			Code:         c.Code,
			Name:         c.Name,
			TeacherName:  c.TeacherName,
			SubjectCode:  c.SubjectCode,
			SubjectName:  c.SubjectName,
			StudentCount: studentCount,
			CourseType:   courseType,
		})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleStudentsUpdate(w http.ResponseWriter, r *http.Request) {
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
		Wcode    string `json:"wcode"`
		FullName string `json:"full_name"`
		Notes    string `json:"notes"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	s.a.WithIdempotentTx(w, r, user.ID, "students", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		item, err := qtx.StudentUpdate(r.Context(), sqldb.StudentUpdateParams{
			ID:       id,
			Wcode:    body.Wcode,
			FullName: body.FullName,
			Notes:    body.Notes,
		})
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		sid, err := s.a.UUIDString(item.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{"id": sid, "wcode": item.Wcode, "full_name": item.FullName, "notes": item.Notes}, nil
	})
}
