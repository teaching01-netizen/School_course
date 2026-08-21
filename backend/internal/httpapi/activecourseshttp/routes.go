package activecourseshttp

import (
	"fmt"
	"net/http"
	"net/url"
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

	limit, offset, paginated, err := parsePagination(r.URL.Query())
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_pagination", err.Error())
		return
	}
	if paginated {
		subjects, coursesBySubject, totalSubjects, totalCourses, err := s.deps.Q.ActiveCoursesListPaginated(r.Context(), limit, offset)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return
		}
		out, err := outSubjectDTOs(s, subjects, coursesBySubject)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		s.a.WriteJSON(w, http.StatusOK, map[string]any{
			"subjects": out, "total_subjects": totalSubjects,
			"total_courses": totalCourses, "limit": limit, "offset": offset,
		})
		return
	}

	subjects, coursesBySubject, err := s.deps.Q.ActiveCoursesList(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	out, err := outSubjectDTOs(s, subjects, coursesBySubject)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"subjects": out})
}

func outSubjectDTOs(s *server, subjects []sqldb.ActiveCourseSubjectRow, coursesBySubject [][]sqldb.ActiveCourseCourseRow) ([]subjectDTO, error) {
	out := make([]subjectDTO, 0, len(subjects))
	for i, subj := range subjects {
		subjID, err := s.a.UUIDString(subj.SubjectID)
		if err != nil {
			return nil, err
		}
		courses := coursesBySubject[i]
		courseDTOs := make([]courseDTO, 0, len(courses))
		for _, c := range courses {
			cID, err := s.a.UUIDString(c.CourseID)
			if err != nil {
				return nil, err
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

	return out, nil
}

func parsePagination(query url.Values) (int, int, bool, error) {
	_, hasLimit := query["limit"]
	_, hasOffset := query["offset"]
	if !hasLimit && !hasOffset {
		return 0, 0, false, nil
	}
	limit := 50
	if hasLimit {
		value, err := strconv.Atoi(query.Get("limit"))
		if err != nil || value < 1 || value > 200 {
			return 0, 0, true, fmt.Errorf("limit must be between 1 and 200")
		}
		limit = value
	}
	offset := 0
	if hasOffset {
		value, err := strconv.Atoi(query.Get("offset"))
		if err != nil || value < 0 {
			return 0, 0, true, fmt.Errorf("offset must be non-negative")
		}
		offset = value
	}
	return limit, offset, true, nil
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
