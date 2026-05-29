package corehttp

import (
	"errors"
	"net/http"
	"time"

	"warwick-institute/internal/auth"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("POST /api/v1/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/logout", s.handleLogout)
	mux.HandleFunc("GET /api/v1/me", s.handleMe)
	mux.HandleFunc("GET /api/v1/meta/time", s.handleMetaTime)
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Auth.HandleLogin(w, r); err != nil {
		s.deps.Log.Info("login failed", "err", err)
		if errors.Is(err, auth.ErrInvalidCredentials) {
			s.a.WriteErr(w, http.StatusUnauthorized, "invalid_credentials", "Invalid username or password")
			return
		}
		if errors.Is(err, auth.ErrTooManyRequests) {
			s.a.WriteErr(w, http.StatusTooManyRequests, "too_many_requests", "Too many requests")
			return
		}
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
	}
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	_ = s.deps.Auth.HandleLogout(w, r)
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, ok := s.a.MustUser(w, r)
	if !ok {
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"id":       u.ID.String(),
		"username": u.Username,
		"role":     u.Role,
	})
}

func (s *server) handleMetaTime(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"institute_tz": s.deps.InstituteTZ,
		"server_now":   time.Now().UTC().Format(time.RFC3339Nano),
	})
}
