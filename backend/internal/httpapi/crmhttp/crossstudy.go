package crmhttp

import (
	"net/http"

	"github.com/google/uuid"

	"warwick-institute/internal/crmimport/crossstudy"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type crossStudyServer struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
	cs   *crossstudy.Store
}

func RegisterCrossStudy(mux *http.ServeMux, deps httpdeps.Deps) {
	if deps.CrossStudy == nil {
		return
	}
	s := &crossStudyServer{deps: deps, a: httpadapter.New(deps.Auth, deps.Log), cs: deps.CrossStudy}

	mux.HandleFunc("GET /api/v1/cross-study/students/{wcode}", s.handleStudentLookup)
	mux.HandleFunc("GET /api/v1/cross-study/assignments", s.handleAssignmentList)
	mux.HandleFunc("PUT /api/v1/cross-study/assignments", s.handleAssignmentSave)
	mux.HandleFunc("DELETE /api/v1/cross-study/assignments/{id}", s.handleAssignmentDelete)
}

func (s *crossStudyServer) handleStudentLookup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	wcode := r.PathValue("wcode")
	if wcode == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_wcode", "Missing wcode")
		return
	}

	resp, err := s.cs.LookupStudent(r.Context(), wcode)
	if err != nil {
		if err.Error() == "student not found" {
			s.a.WriteErr(w, http.StatusNotFound, "not_found", "Student not found")
			return
		}
		s.deps.Log.Error("cross-study lookup failed", "wcode", wcode, "error", err)
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, resp)
}

func (s *crossStudyServer) handleAssignmentList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	statusFilter := r.URL.Query().Get("status")
	searchQuery := r.URL.Query().Get("q")

	items, err := s.cs.ListAssignmentsWithCourseInfo(r.Context(), statusFilter, searchQuery)
	if err != nil {
		s.deps.Log.Error("cross-study list failed", "error", err)
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	reviewCount, err := s.cs.CountReviewNeeded(r.Context())
	if err != nil {
		s.deps.Log.Error("cross-study review count failed", "error", err)
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	if items == nil {
		items = []crossstudy.AssignmentSummary{}
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"assignments":  items,
		"total":        len(items),
		"review_count": reviewCount,
	})
}

func (s *crossStudyServer) handleAssignmentSave(w http.ResponseWriter, r *http.Request) {
	au, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	var body struct {
		WCode               string  `json:"wcode"`
		SnapshotID          string  `json:"snapshot_id"`
		CRMCourseName       string  `json:"crm_course_name"`
		CRMRowHash          string  `json:"crm_row_hash"`
		CRMXLSXRowNumber    int32   `json:"crm_xlsx_row_number"`
		DestCourseAID       string  `json:"dest_course_a_id"`
		DestCourseBID       string  `json:"dest_course_b_id"`
		DestCourseAWeekdays []int16 `json:"dest_course_a_weekdays"`
		DestCourseBWeekdays []int16 `json:"dest_course_b_weekdays"`
		AssignedCourseID    string  `json:"assigned_course_id"`
		ExtraNoteText       string  `json:"extra_note_text"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		return
	}

	if body.WCode == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_input", "wcode is required")
		return
	}

	snapshotID, err := uuid.Parse(body.SnapshotID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_input", "invalid snapshot_id")
		return
	}
	destAID, err := uuid.Parse(body.DestCourseAID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_input", "invalid dest_course_a_id")
		return
	}
	destBID, err := uuid.Parse(body.DestCourseBID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_input", "invalid dest_course_b_id")
		return
	}
	assignedID, err := uuid.Parse(body.AssignedCourseID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_input", "invalid assigned_course_id")
		return
	}
	destAWeekdays, ok := normalizeWeekdays(body.DestCourseAWeekdays)
	if !ok {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_input", "invalid dest_course_a_weekdays")
		return
	}
	destBWeekdays, ok := normalizeWeekdays(body.DestCourseBWeekdays)
	if !ok {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_input", "invalid dest_course_b_weekdays")
		return
	}

	input := crossstudy.SaveAssignmentInput{
		WCode:               body.WCode,
		SnapshotID:          snapshotID,
		CRMCourseName:       body.CRMCourseName,
		CRMRowHash:          body.CRMRowHash,
		CRMXLSXRowNumber:    body.CRMXLSXRowNumber,
		DestCourseAID:       destAID,
		DestCourseBID:       destBID,
		DestCourseAWeekdays: destAWeekdays,
		DestCourseBWeekdays: destBWeekdays,
		AssignedCourseID:    assignedID,
		ExtraNoteText:       body.ExtraNoteText,
	}

	if err := s.cs.SaveAssignment(r.Context(), input, au.ID); err != nil {
		s.deps.Log.Error("cross-study save failed", "error", err)
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func normalizeWeekdays(input []int16) ([]int16, bool) {
	if len(input) == 0 {
		return []int16{1, 2, 3, 4, 5, 6, 7}, true
	}
	seen := map[int16]bool{}
	out := make([]int16, 0, len(input))
	for _, day := range input {
		if day < 1 || day > 7 {
			return nil, false
		}
		if seen[day] {
			continue
		}
		seen[day] = true
		out = append(out, day)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, true
}

func (s *crossStudyServer) handleAssignmentDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid assignment ID")
		return
	}

	if err := s.cs.DeleteAssignment(r.Context(), id); err != nil {
		s.deps.Log.Error("cross-study delete failed", "id", idStr, "error", err)
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
