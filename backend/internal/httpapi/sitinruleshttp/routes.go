package sitinruleshttp

import (
	"encoding/json"
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

	mux.HandleFunc("GET /api/v1/admin/sit-in-rules", s.handleList)
	mux.HandleFunc("GET /api/v1/admin/sit-in-rules/{id}", s.handleGet)
	mux.HandleFunc("POST /api/v1/admin/sit-in-rules", s.handleCreate)
	mux.HandleFunc("PUT /api/v1/admin/sit-in-rules/{id}", s.handleUpdate)
	mux.HandleFunc("DELETE /api/v1/admin/sit-in-rules/{id}", s.handleDelete)
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	items, err := s.deps.Q.SitInRulesList(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		id, err := s.a.UUIDString(item.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		out = append(out, map[string]any{
			"id":          id,
			"name":        item.Name,
			"type":        item.Type,
			"predicate":   json.RawMessage(item.Predicate),
			"description": item.Description,
			"created_at":  item.CreatedAt,
			"updated_at":  item.UpdatedAt,
		})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	item, err := s.deps.Q.SitInRuleGetByID(r.Context(), id)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
ucid, err := s.a.UUIDString(item.ID)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"id":          ucid,
		"name":        item.Name,
		"type":        item.Type,
		"predicate":   json.RawMessage(item.Predicate),
		"description": item.Description,
		"created_at":  item.CreatedAt,
		"updated_at":  item.UpdatedAt,
	})
}

func (s *server) handleCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Name        string          `json:"name"`
		Type        string          `json:"type"`
		Predicate   json.RawMessage `json:"predicate"`
		Description string          `json:"description"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	s.a.WithIdempotentTx(w, r, user.ID, "sit-in-rules", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		item, err := qtx.SitInRuleCreate(r.Context(), sqldb.SitInRuleCreateInput{
			Name:        body.Name,
			Type:        body.Type,
			Predicate:   body.Predicate,
			Description: body.Description,
		})
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		id, err := s.a.UUIDString(item.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		return http.StatusCreated, map[string]any{
			"id":          id,
			"name":        item.Name,
			"type":        item.Type,
			"predicate":   json.RawMessage(item.Predicate),
			"description": item.Description,
			"created_at":  item.CreatedAt,
			"updated_at":  item.UpdatedAt,
		}, nil
	})
}

func (s *server) handleUpdate(w http.ResponseWriter, r *http.Request) {
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
		Name        string          `json:"name"`
		Type        string          `json:"type"`
		Predicate   json.RawMessage `json:"predicate"`
		Description string          `json:"description"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	s.a.WithIdempotentTx(w, r, user.ID, "sit-in-rules", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		item, err := qtx.SitInRuleUpdate(r.Context(), id, sqldb.SitInRuleCreateInput{
			Name:        body.Name,
			Type:        body.Type,
			Predicate:   body.Predicate,
			Description: body.Description,
		})
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		ucid, err := s.a.UUIDString(item.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{
			"id":          ucid,
			"name":        item.Name,
			"type":        item.Type,
			"predicate":   json.RawMessage(item.Predicate),
			"description": item.Description,
			"created_at":  item.CreatedAt,
			"updated_at":  item.UpdatedAt,
		}, nil
	})
}

func (s *server) handleDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	s.a.WithIdempotentTx(w, r, user.ID, "sit-in-rules", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.SitInRuleDelete(r.Context(), id); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{"ok": true}, nil
	})
}
