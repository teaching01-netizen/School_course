package studentshttp

import (
	"net/http"
	"strconv"
	"strings"

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

	mux.HandleFunc("GET /api/v1/students", s.handleStudentsList)
	mux.HandleFunc("POST /api/v1/students", s.handleStudentsCreate)
	mux.HandleFunc("GET /api/v1/students/{id}", s.handleStudentsGet)
	mux.HandleFunc("GET /api/v1/students/by-wcode", s.handleStudentsGetByWCode)
	mux.HandleFunc("GET /api/v1/students/{id}/courses", s.handleStudentCoursesList)
	mux.HandleFunc("PUT /api/v1/students/{id}", s.handleStudentsUpdate)
}

// studentDTO is the API shape of a student. It carries the full profile the
// old site's directory sync and the CRM import both feed: identity (wcode,
// full name), personal fields (nickname, school, level, admission year,
// student phone), and contact (email). Values absent on the source are empty
// strings, never nulls, so clients can render them straight into inputs.
type studentDTO struct {
	ID           string `json:"id"`
	Wcode        string `json:"wcode"`
	FullName     string `json:"full_name"`
	Notes        string `json:"notes"`
	Nickname     string `json:"nickname"`
	School       string `json:"school"`
	Level        string `json:"level"`
	Year         string `json:"year"`
	StudentPhone string `json:"student_phone"`
	Email        string `json:"email"`
}

// studentDTO builds the API shape from the row-level fields shared by every
// Student*Row struct (which sqlc generates as distinct named types with the
// same shape).
func (s *server) studentDTO(id pgtype.UUID, wcode, fullName, notes string, nickname, email, emailCRM, emailSystem, studentPhone, school, level, year pgtype.Text) (studentDTO, error) {
	sid, err := s.a.UUIDString(id)
	if err != nil {
		return studentDTO{}, err
	}
	return studentDTO{
		ID:           sid,
		Wcode:        wcode,
		FullName:     fullName,
		Notes:        notes,
		Nickname:     nickname.String,
		School:       school.String,
		Level:        level.String,
		Year:         year.String,
		StudentPhone: studentPhone.String,
		Email:        resolvedStudentEmail(email, emailCRM, emailSystem),
	}, nil
}

func resolvedStudentEmail(email, emailCRM, emailSystem pgtype.Text) string {
	for _, candidate := range []pgtype.Text{emailCRM, emailSystem, email} {
		if candidate.Valid && strings.TrimSpace(candidate.String) != "" {
			return candidate.String
		}
	}
	return ""
}

func (s *server) handleStudentsList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	limit := int32(50)
	offset := int32(0)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = int32(min(n, 200))
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n)
		}
	}
	search := s.a.SearchQuery(r.URL.Query().Get("q"))
	searchParam := pgtype.Text{String: search, Valid: true}

	items, err := s.deps.Q.StudentList(r.Context(), sqldb.StudentListParams{
		Limit:   limit,
		Offset:  offset,
		Column3: searchParam,
	})
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	totalCount, err := s.deps.Q.StudentListCount(r.Context(), searchParam)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	out := make([]studentDTO, 0, len(items))
	for _, st := range items {
		dto, err := s.studentDTO(st.ID, st.Wcode, st.FullName, st.Notes, st.Nickname, st.Email, st.EmailCrm, st.EmailSystem, st.StudentPhone, st.School, st.Level, st.Year)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		out = append(out, dto)
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"items":       out,
		"total_count": totalCount,
		"offset":      offset,
		"limit":       limit,
	})
}

// studentWriteRequest is the payload for both create and update. Every
// optional field is a plain string so clients can send "" to clear a value.
type studentWriteRequest struct {
	Wcode        string `json:"wcode"`
	FullName     string `json:"full_name"`
	Notes        string `json:"notes"`
	Nickname     string `json:"nickname"`
	School       string `json:"school"`
	Level        string `json:"level"`
	Year         string `json:"year"`
	StudentPhone string `json:"student_phone"`
	Email        string `json:"email"`
}

func (req studentWriteRequest) StudentCreateParams() sqldb.StudentCreateParams {
	return sqldb.StudentCreateParams{
		Wcode:        req.Wcode,
		FullName:     req.FullName,
		Notes:        req.Notes,
		Nickname:     pgtype.Text{String: req.Nickname, Valid: req.Nickname != ""},
		Email:        pgtype.Text{String: req.Email, Valid: req.Email != ""},
		StudentPhone: pgtype.Text{String: req.StudentPhone, Valid: req.StudentPhone != ""},
		School:       pgtype.Text{String: req.School, Valid: req.School != ""},
		Level:        pgtype.Text{String: req.Level, Valid: req.Level != ""},
		Year:         pgtype.Text{String: req.Year, Valid: req.Year != ""},
	}
}

func (req studentWriteRequest) StudentUpdateParams(id pgtype.UUID) sqldb.StudentUpdateParams {
	return sqldb.StudentUpdateParams{
		ID:           id,
		Wcode:        req.Wcode,
		FullName:     req.FullName,
		Notes:        req.Notes,
		Nickname:     pgtype.Text{String: req.Nickname, Valid: req.Nickname != ""},
		Email:        pgtype.Text{String: req.Email, Valid: req.Email != ""},
		StudentPhone: pgtype.Text{String: req.StudentPhone, Valid: req.StudentPhone != ""},
		School:       pgtype.Text{String: req.School, Valid: req.School != ""},
		Level:        pgtype.Text{String: req.Level, Valid: req.Level != ""},
		Year:         pgtype.Text{String: req.Year, Valid: req.Year != ""},
	}
}

func (s *server) handleStudentsCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var req studentWriteRequest
	if err := s.a.DecodeJSON(w, r, &req); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	req.Wcode = strings.ToLower(strings.TrimSpace(req.Wcode))
	s.a.WithIdempotentTx(w, r, user.ID, "students", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		item, err := qtx.StudentCreate(r.Context(), req.StudentCreateParams())
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		dto, err := s.studentDTO(item.ID, item.Wcode, item.FullName, item.Notes, item.Nickname, item.Email, item.EmailCrm, item.EmailSystem, item.StudentPhone, item.School, item.Level, item.Year)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		return http.StatusCreated, dto, nil
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
	dto, err := s.studentDTO(item.ID, item.Wcode, item.FullName, item.Notes, item.Nickname, item.Email, item.EmailCrm, item.EmailSystem, item.StudentPhone, item.School, item.Level, item.Year)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, dto)
}

func (s *server) handleStudentsGetByWCode(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	wcode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("wcode")))
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
	dto, err := s.studentDTO(item.ID, item.Wcode, item.FullName, item.Notes, item.Nickname, item.Email, item.EmailCrm, item.EmailSystem, item.StudentPhone, item.School, item.Level, item.Year)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, dto)
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
	var req studentWriteRequest
	if err := s.a.DecodeJSON(w, r, &req); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	req.Wcode = strings.ToLower(strings.TrimSpace(req.Wcode))
	s.a.WithIdempotentTx(w, r, user.ID, "students", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		item, err := qtx.StudentUpdate(r.Context(), req.StudentUpdateParams(id))
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		dto, err := s.studentDTO(item.ID, item.Wcode, item.FullName, item.Notes, item.Nickname, item.Email, item.EmailCrm, item.EmailSystem, item.StudentPhone, item.School, item.Level, item.Year)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		return http.StatusOK, dto, nil
	})
}
