package adminusershttp

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/users"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("GET /api/v1/admin/users", s.handleAdminUsersList)
	mux.HandleFunc("POST /api/v1/admin/users", s.handleAdminUsersCreate)
	mux.HandleFunc("POST /api/v1/admin/users/{id}/reset_password", s.handleAdminUsersResetPassword)
	mux.HandleFunc("DELETE /api/v1/admin/users/{id}", s.handleAdminUsersDeactivate)
}

func (s *server) handleAdminUsersList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	includeDeleted := strings.TrimSpace(r.URL.Query().Get("include_deleted")) == "true"
	items, err := s.deps.Q.AdminUserList(r.Context(), sqldb.AdminUserListParams{IncludeDeleted: includeDeleted})
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type dto struct {
		ID        string  `json:"id"`
		Username  string  `json:"username"`
		Role      string  `json:"role"`
		DeletedAt *string `json:"deleted_at"`
		CreatedAt string  `json:"created_at"`
	}
	out := make([]dto, 0, len(items))
	for _, u := range items {
		id, err := s.a.UUIDString(u.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		created, _ := s.a.TimeString(u.CreatedAt)
		var deletedAt *string
		if u.DeletedAt.Valid {
			ds, _ := s.a.TimeString(u.DeletedAt)
			deletedAt = &ds
		}
		out = append(out, dto{ID: id, Username: u.Username, Role: u.Role, DeletedAt: deletedAt, CreatedAt: created})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleAdminUsersCreate(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	var body struct {
		Username string `json:"username"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	s.a.WithIdempotentTx(w, r, actor.ID, "admin-users", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		id, err := s.deps.AdminUsers.ProvisionUser(
			r.Context(),
			users.Actor{ID: actor.ID, Role: actor.Role},
			body.Username,
			body.Role,
			body.Password,
		)
		if err != nil {
			var vErr users.ValidationError
			if errors.As(err, &vErr) {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_input", vErr.Message)
				return 0, nil, err
			}
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		idStr := id.String()
		actorID := pgtype.UUID{Bytes: actor.ID, Valid: true}
		_, _ = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "user.create",
			Payload:     map[string]any{"user_id": idStr, "username": strings.TrimSpace(body.Username), "role": strings.TrimSpace(body.Role)},
		})
		return http.StatusCreated, map[string]any{"id": idStr}, nil
	})
}

func (s *server) handleAdminUsersResetPassword(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	s.a.WithIdempotentTx(w, r, actor.ID, "admin-users", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := s.deps.AdminUsers.ResetPassword(
			r.Context(),
			users.Actor{ID: actor.ID, Role: actor.Role},
			id,
			body.Password,
		); err != nil {
			var vErr users.ValidationError
			if errors.As(err, &vErr) {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_input", vErr.Message)
				return 0, nil, err
			}
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		idStr := id.String()
		actorID := pgtype.UUID{Bytes: actor.ID, Valid: true}
		_, _ = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "user.reset_password",
			Payload:     map[string]any{"user_id": idStr},
		})
		return http.StatusOK, map[string]any{"ok": true}, nil
	})
}

func (s *server) handleAdminUsersDeactivate(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}

	s.a.WithIdempotentTx(w, r, actor.ID, "admin-users", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := s.deps.AdminUsers.Deactivate(
			r.Context(),
			users.Actor{ID: actor.ID, Role: actor.Role},
			id,
		); err != nil {
			var vErr users.ValidationError
			if errors.As(err, &vErr) {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_input", vErr.Message)
				return 0, nil, err
			}
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		idStr := id.String()
		actorID := pgtype.UUID{Bytes: actor.ID, Valid: true}
		_, _ = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "user.deactivate",
			Payload:     map[string]any{"user_id": idStr},
		})
		return http.StatusOK, map[string]any{"ok": true}, nil
	})
}
