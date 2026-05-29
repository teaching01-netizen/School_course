package courselevelshttp

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("GET /api/v1/admin/course-levels", s.handleList)
	mux.HandleFunc("PUT /api/v1/admin/courses/{id}/level", s.handleUpdateLevel)
	mux.HandleFunc("PUT /api/v1/admin/courses/{id}/root-course-group", s.handleUpdateRootCourseGroup)
	mux.HandleFunc("GET /api/v1/admin/root-course-groups", s.handleListRootCourseGroups)
	mux.HandleFunc("POST /api/v1/admin/root-course-groups", s.handleCreateRootCourseGroup)
	mux.HandleFunc("PUT /api/v1/admin/root-course-groups/{id}", s.handleUpdateRootCourseGroupMeta)
	mux.HandleFunc("DELETE /api/v1/admin/root-course-groups/{id}", s.handleDeleteRootCourseGroup)
}

type rootCourseGroupDTO struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	CourseCount int32   `json:"course_count"`
	SitInRuleID *string `json:"sit_in_rule_id"`
}

type courseLevelDTO struct {
	ID                  string  `json:"id"`
	Code                string  `json:"code"`
	Name                string  `json:"name"`
	SubjectID           string  `json:"subject_id"`
	SubjectCode         string  `json:"subject_code"`
	SubjectName         string  `json:"subject_name"`
	CycleID             *string `json:"cycle_id"`
	CycleLabel          *string `json:"cycle_label"`
	Level               *int16  `json:"level"`
	RootCourseGroupID   *string `json:"root_course_group_id"`
	RootCourseGroupName *string `json:"root_course_group_name"`
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	rows, err := s.deps.Q.CourseLevelsListV2(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	out := make([]courseLevelDTO, 0, len(rows))
	for _, row := range rows {
		id, _ := s.a.UUIDString(row.ID)
		subjID, _ := s.a.UUIDString(row.SubjectID)
		dto := courseLevelDTO{
			ID:          id,
			Code:        row.Code,
			Name:        row.Name,
			SubjectID:   subjID,
			SubjectCode: row.SubjectCode,
			SubjectName: row.SubjectName,
		}
		if row.CycleID.Valid {
			v := row.CycleID.String
			dto.CycleID = &v
		}
		if row.CycleLabel.Valid {
			v := row.CycleLabel.String
			dto.CycleLabel = &v
		}
		if row.Level.Valid {
			v := row.Level.Int16
			dto.Level = &v
		}
		if row.RootCourseGroupID.Valid {
			v, _ := s.a.UUIDString(row.RootCourseGroupID)
			dto.RootCourseGroupID = &v
		}
		if row.RootCourseGroupName.Valid {
			v := row.RootCourseGroupName.String
			dto.RootCourseGroupName = &v
		}
		out = append(out, dto)
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleUpdateLevel(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid course ID")
		return
	}

	var body struct {
		Level   *int16  `json:"level"`
		CycleID *string `json:"cycle_id"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	if body.Level != nil && *body.Level < 1 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_level", "level must be >= 1")
		return
	}

	level := pgtype.Int2{}
	if body.Level != nil {
		level = pgtype.Int2{Int16: *body.Level, Valid: true}
	}
	cycleID := pgtype.Text{}
	if body.CycleID != nil {
		cycleID = pgtype.Text{String: *body.CycleID, Valid: true}
	}

	if err := s.deps.Q.CourseLevelUpdateV2(r.Context(), courseID, cycleID, level); err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleUpdateRootCourseGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid course ID")
		return
	}

	var body struct {
		RootCourseGroupID *string `json:"root_course_group_id"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	if body.RootCourseGroupID != nil {
		groupID, err := s.a.ParseUUID(*body.RootCourseGroupID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_root_course_group_id", "Invalid root course group ID")
			return
		}

		exists, err := s.deps.Q.RootCourseGroupExists(r.Context(), groupID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return
		}
		if !exists {
			s.a.WriteErr(w, http.StatusBadRequest, "not_found", "Root course group not found")
			return
		}

		if err := s.deps.Q.CourseUpdateRootCourseGroup(r.Context(), courseID, groupID); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return
		}
	} else {
		if err := s.deps.Q.CourseUpdateRootCourseGroup(r.Context(), courseID, pgtype.UUID{}); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return
		}
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}



func (s *server) handleListRootCourseGroups(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	rows, err := s.deps.Q.RootCourseGroupsList(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	out := make([]rootCourseGroupDTO, 0, len(rows))
	for _, row := range rows {
		id, _ := s.a.UUIDString(row.ID)
		dto := rootCourseGroupDTO{
			ID:          id,
			Name:        row.Name,
			CourseCount: row.CourseCount,
		}
		if row.SitInRuleID.Valid {
			v, _ := s.a.UUIDString(row.SitInRuleID)
			dto.SitInRuleID = &v
		}
		out = append(out, dto)
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleCreateRootCourseGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	var body struct {
		Name        string  `json:"name"`
		SitInRuleID *string `json:"sit_in_rule_id"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.Name == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "missing_fields", "name is required")
		return
	}

	sitInRuleID := pgtype.UUID{}
	if body.SitInRuleID != nil {
		sid, err := s.a.ParseUUID(*body.SitInRuleID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_rule_id", "Invalid sit-in rule ID")
			return
		}
		sitInRuleID = sid
	}

	id, name, _, err := s.deps.Q.RootCourseGroupCreate(r.Context(), body.Name, sitInRuleID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	idStr, _ := s.a.UUIDString(id)
	dto := rootCourseGroupDTO{
		ID:          idStr,
		Name:        name,
		CourseCount: 0,
	}
	if body.SitInRuleID != nil {
		dto.SitInRuleID = body.SitInRuleID
	}
	s.a.WriteJSON(w, http.StatusCreated, dto)
}

func (s *server) handleUpdateRootCourseGroupMeta(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid ID")
		return
	}

	var body struct {
		Name        string  `json:"name"`
		SitInRuleID *string `json:"sit_in_rule_id"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.Name == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "missing_fields", "name is required")
		return
	}

	exists, err := s.deps.Q.RootCourseGroupExists(r.Context(), id)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if !exists {
		s.a.WriteErr(w, http.StatusNotFound, "not_found", "Root course group not found")
		return
	}

	sitInRuleID := pgtype.UUID{}
	if body.SitInRuleID != nil {
		sid, err := s.a.ParseUUID(*body.SitInRuleID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_rule_id", "Invalid sit-in rule ID")
			return
		}
		sitInRuleID = sid
	}

	if err := s.deps.Q.RootCourseGroupUpdate(r.Context(), id, body.Name, sitInRuleID); err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleDeleteRootCourseGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid ID")
		return
	}
	if err := s.deps.Q.RootCourseGroupDelete(r.Context(), id); err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
