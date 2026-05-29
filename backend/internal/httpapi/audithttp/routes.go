package audithttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
	mux.HandleFunc("GET /api/v1/audit", s.handleAuditList)
}

func (s *server) handleAuditList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	limit := int32(100)
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		var v int
		if _, err := fmt.Sscanf(raw, "%d", &v); err == nil {
			limit = int32(v)
		}
	}
	var beforeID *int64
	if raw := strings.TrimSpace(r.URL.Query().Get("before_id")); raw != "" {
		var v int64
		if _, err := fmt.Sscanf(raw, "%d", &v); err == nil {
			beforeID = &v
		}
	}
	items, err := s.deps.Q.AuditList(r.Context(), sqldb.AuditListParams{BeforeID: beforeID, Limit: limit})
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type dto struct {
		ID          int64           `json:"id"`
		CreatedAt   string          `json:"created_at"`
		ActorUserID *string         `json:"actor_user_id"`
		Action      string          `json:"action"`
		Payload     json.RawMessage `json:"payload"`
	}
	out := make([]dto, 0, len(items))
	for _, a := range items {
		created, _ := s.a.TimeString(a.CreatedAt)
		var actorID *string
		if a.ActorUserID.Valid {
			uid, err := s.a.UUIDString(a.ActorUserID)
			if err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return
			}
			actorID = &uid
		}
		out = append(out, dto{ID: a.ID, CreatedAt: created, ActorUserID: actorID, Action: a.Action, Payload: json.RawMessage(a.Payload)})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}
