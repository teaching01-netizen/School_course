package activecourseshttp

import (
	"net/http"

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

	mux.HandleFunc("GET /api/v1/admin/active-courses", s.handleList)
	mux.HandleFunc("PUT /api/v1/admin/active-courses", s.handleSet)
}

type courseDTO struct {
	CourseID   string `json:"course_id"`
	CourseCode string `json:"course_code"`
	CourseName string `json:"course_name"`
	CycleID    string `json:"cycle_id"`
	CycleLabel string `json:"cycle_label"`
	IsActive   bool   `json:"is_active"`
}

type subjectDTO struct {
	SubjectID   string      `json:"subject_id"`
	SubjectCode string      `json:"subject_code"`
	SubjectName string      `json:"subject_name"`
	Courses     []courseDTO `json:"courses"`
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	subjects, coursesBySubject, err := s.deps.Q.ActiveCoursesList(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	out := make([]subjectDTO, 0, len(subjects))
	for i, subj := range subjects {
		subjID, err := s.a.UUIDString(subj.SubjectID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		courses := coursesBySubject[i]
		courseDTOs := make([]courseDTO, 0, len(courses))
		for _, c := range courses {
			cID, err := s.a.UUIDString(c.CourseID)
			if err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return
			}
			cycleID := ""
			if c.CycleID.Valid {
				cycleID = c.CycleID.String
			}
			courseDTOs = append(courseDTOs, courseDTO{
				CourseID:   cID,
				CourseCode: c.CourseCode,
				CourseName: c.CourseName,
				CycleID:    cycleID,
				CycleLabel: c.CycleLabel,
				IsActive:   c.IsActive,
			})
		}
		out = append(out, subjectDTO{
			SubjectID:   subjID,
			SubjectCode: subj.SubjectCode,
			SubjectName: subj.SubjectName,
			Courses:     courseDTOs,
		})
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]any{"subjects": out})
}

func (s *server) handleSet(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	var body struct {
		SubjectID string `json:"subject_id"`
		CourseID  string `json:"course_id"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	subjectID, err := s.a.ParseUUID(body.SubjectID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_id", "Invalid subject_id")
		return
	}
	courseID, err := s.a.ParseUUID(body.CourseID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_course_id", "Invalid course_id")
		return
	}

	s.a.WithIdempotentTx(w, r, user.ID, "active-courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.ActiveCourseUpsert(r.Context(), sqldb.ActiveCourseUpsertParams{
			SubjectID: subjectID,
			CourseID:  courseID,
		}); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		return http.StatusOK, map[string]string{"status": "ok"}, nil
	})
}
