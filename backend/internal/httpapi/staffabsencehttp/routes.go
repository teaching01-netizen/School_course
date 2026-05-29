package staffabsencehttp

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type StaffAbsencePolicies struct {
	NotifyAdminOnTeacherAbsence bool `json:"notify_admin_on_teacher_absence"`
	NotifySubstituteTeachers    bool `json:"notify_substitute_teachers"`
	AutoAssignCoverEnabled      bool `json:"auto_assign_cover_enabled"`
	CoverThresholdDays          int  `json:"cover_threshold_days"`
	DefaultCoverDurationMinutes int  `json:"default_cover_duration_minutes"`
}

func defaultPolicies() StaffAbsencePolicies {
	return StaffAbsencePolicies{
		NotifyAdminOnTeacherAbsence: true,
		NotifySubstituteTeachers:    false,
		AutoAssignCoverEnabled:      false,
		CoverThresholdDays:          3,
		DefaultCoverDurationMinutes: 60,
	}
}

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}
	mux.HandleFunc("GET /api/v1/admin/staff-absence-policies", s.handleGet)
	mux.HandleFunc("PUT /api/v1/admin/staff-absence-policies", s.handleUpdate)
}

func (s *server) handleGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	settings, err := s.deps.Q.AppSettingsGetWithPolicies(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	var policies map[string]any
	if err := json.Unmarshal(settings.AbsencePolicies, &policies); err != nil {
		policies = make(map[string]any)
	}

	staffPolicies := defaultPolicies()
	if sp, ok := policies["staff_absence_policies"]; ok && sp != nil {
		if data, err := json.Marshal(sp); err == nil {
			json.Unmarshal(data, &staffPolicies)
		}
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"staff_absence_policies": staffPolicies,
	})
}

func (s *server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		StaffAbsencePolicies StaffAbsencePolicies `json:"staff_absence_policies"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	s.a.WithIdempotentTx(w, r, user.ID, "staff-absence-policies", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		settings, err := qtx.AppSettingsGetWithPolicies(r.Context())
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		var policiesMap map[string]any
		if err := json.Unmarshal(settings.AbsencePolicies, &policiesMap); err != nil {
			policiesMap = make(map[string]any)
		}
		policiesMap["staff_absence_policies"] = body.StaffAbsencePolicies
		merged, err := json.Marshal(policiesMap)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		if err := qtx.AppSettingsUpdateAbsencePolicies(r.Context(), merged); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
		if _, err := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "staff_absence.policies_updated",
			Payload:     map[string]any{"staff_absence_policies": body.StaffAbsencePolicies},
		}); err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write audit log")
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{"staff_absence_policies": body.StaffAbsencePolicies}, nil
	})
}
