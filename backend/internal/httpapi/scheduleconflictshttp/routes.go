package scheduleconflictshttp

import (
	"net/http"

	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type server struct {
	deps  httpdeps.Deps
	a     httpadapter.Adapter
	store conflictStore
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log), store: conflictStore{db: deps.DB}}
	mux.HandleFunc("GET /api/v1/schedule-conflicts", s.handleList)
	mux.HandleFunc("GET /api/v1/schedule-conflicts/summary", s.handleSummary)
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	filters, err := parseListFilters(r)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "invalid_filters", err.Error())
		return
	}
	response, err := s.store.list(r.Context(), filters)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, response)
}

func (s *server) handleSummary(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	filters, err := parseListFilters(r)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "invalid_filters", err.Error())
		return
	}
	filters.Cursor = nil
	summary, err := s.store.summary(r.Context(), filters)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, summary)
}
