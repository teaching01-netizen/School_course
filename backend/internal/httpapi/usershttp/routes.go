package usershttp

import (
	"net/http"
	"strings"

	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}
	mux.HandleFunc("GET /api/v1/users", s.handleUsersList)
}

func (s *server) handleUsersList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	if role == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_role", "role is required")
		return
	}
	if role != "Admin" && role != "Teacher" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_role", "invalid role")
		return
	}
	items, err := s.deps.Q.UserListByRoleActive(r.Context(), role)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type userDTO struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	out := make([]userDTO, 0, len(items))
	for _, u := range items {
		uid, err := s.a.UUIDString(u.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		out = append(out, userDTO{ID: uid, Username: u.Username, Role: u.Role})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}
