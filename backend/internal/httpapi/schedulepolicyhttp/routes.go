package schedulepolicyhttp

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/schedulepolicy"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

type policyBody struct {
	SystemEnforced     *bool `json:"system_enforced"`
	LegacySyncEnforced *bool `json:"legacy_sync_enforced"`
}

type policyStateResponse struct {
	SystemEnforced     bool `json:"system_enforced"`
	LegacySyncEnforced bool `json:"legacy_sync_enforced"`
}

type policyRuleResponse struct {
	ID          schedulepolicy.Rule `json:"id"`
	Label       string              `json:"label"`
	Description string              `json:"description"`
	Controlled  bool                `json:"controlled"`
}

type policyHistoryResponse struct {
	ID             int64               `json:"id"`
	CreatedAt      string              `json:"created_at"`
	Actor          string              `json:"actor"`
	Previous       policyStateResponse `json:"previous"`
	Next           policyStateResponse `json:"next"`
	LegacyForcedOn bool                `json:"legacy_forced_on"`
}

type policyResponse struct {
	policyStateResponse
	UpdatedAt        string                  `json:"updated_at"`
	Rules            []policyRuleResponse    `json:"rules"`
	History          []policyHistoryResponse `json:"history"`
	HistoryRetention string                  `json:"history_retention"`
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}
	mux.HandleFunc("GET /api/v1/admin/scheduling-rules", s.handleGet)
	mux.HandleFunc("PUT /api/v1/admin/scheduling-rules", s.handleUpdate)
}

func (s *server) handleGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	policy, err := s.deps.Q.ScheduleConflictPolicyGet(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	history, err := s.deps.Q.AppSettingsScheduleConflictHistory(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, s.response(policy, history))
}

func (s *server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body policyBody
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.SystemEnforced == nil || body.LegacySyncEnforced == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_policy", "system_enforced and legacy_sync_enforced are required")
		return
	}
	actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
	s.a.WithIdempotentTx(w, r, user.ID, "schedule-conflict-policy", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		previous, err := qtx.ScheduleConflictPolicyGetForUpdate(r.Context())
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		systemEnforced, legacySyncEnforced, legacyForcedOn := normalizePolicyUpdate(
			previous,
			*body.SystemEnforced,
			*body.LegacySyncEnforced,
		)
		updated, err := qtx.ScheduleConflictPolicyUpdate(r.Context(), systemEnforced, legacySyncEnforced)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		_, err = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "schedule_conflict_policy.updated",
			Payload: map[string]any{
				"previous": map[string]bool{
					"system_enforced":      previous.SystemEnforced,
					"legacy_sync_enforced": previous.LegacySyncEnforced,
				},
				"next": map[string]bool{
					"system_enforced":      updated.SystemEnforced,
					"legacy_sync_enforced": updated.LegacySyncEnforced,
				},
				"legacy_forced_on": legacyForcedOn,
			},
		})
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write audit log")
			return 0, nil, err
		}
		history, err := qtx.AppSettingsScheduleConflictHistory(r.Context())
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		return http.StatusOK, s.response(updated, history), nil
	})
}

func normalizePolicyUpdate(previous sqldb.ScheduleConflictPolicyRow, systemEnforced, legacySyncEnforced bool) (bool, bool, bool) {
	legacyForcedOn := !previous.SystemEnforced && systemEnforced && !legacySyncEnforced
	if legacyForcedOn {
		legacySyncEnforced = true
	}
	return systemEnforced, legacySyncEnforced, legacyForcedOn
}

func (s *server) response(policy sqldb.ScheduleConflictPolicyRow, history []sqldb.ScheduleConflictAuditRow) policyResponse {
	rules := make([]policyRuleResponse, 0, len(schedulepolicy.ControlledRules()))
	for _, rule := range schedulepolicy.ControlledRules() {
		rules = append(rules, policyRuleResponse{
			ID:          rule.ID,
			Label:       rule.Label,
			Description: rule.Description,
			Controlled:  true,
		})
	}
	historyResponse := make([]policyHistoryResponse, 0, len(history))
	for _, item := range history {
		if entry, ok := historyItem(item); ok {
			historyResponse = append(historyResponse, entry)
		}
	}
	updatedAt := ""
	if policy.UpdatedAt.Valid {
		updatedAt = policy.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	return policyResponse{
		policyStateResponse: policyStateResponse{
			SystemEnforced:     policy.SystemEnforced,
			LegacySyncEnforced: policy.LegacySyncEnforced,
		},
		UpdatedAt:        updatedAt,
		Rules:            rules,
		History:          historyResponse,
		HistoryRetention: "3 days",
	}
}

func historyItem(item sqldb.ScheduleConflictAuditRow) (policyHistoryResponse, bool) {
	var payload struct {
		Previous       policyStateResponse `json:"previous"`
		Next           policyStateResponse `json:"next"`
		LegacyForcedOn bool                `json:"legacy_forced_on"`
	}
	if !item.CreatedAt.Valid || json.Unmarshal(item.Payload, &payload) != nil {
		return policyHistoryResponse{}, false
	}
	return policyHistoryResponse{
		ID:             item.ID,
		CreatedAt:      item.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		Actor:          item.ActorName,
		Previous:       payload.Previous,
		Next:           payload.Next,
		LegacyForcedOn: payload.LegacyForcedOn,
	}, true
}
