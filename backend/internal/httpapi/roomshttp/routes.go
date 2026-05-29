package roomshttp

import (
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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

	mux.HandleFunc("GET /api/v1/rooms", s.handleRoomsList)
	mux.HandleFunc("POST /api/v1/rooms", s.handleRoomsCreate)
	mux.HandleFunc("GET /api/v1/rooms/{id}", s.handleRoomsGet)
	mux.HandleFunc("PUT /api/v1/rooms/{id}", s.handleRoomsUpdate)
}

func (s *server) handleRoomsList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	items, err := s.deps.Q.RoomList(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type roomDTO struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Capacity *int32 `json:"capacity"`
	}
	out := make([]roomDTO, 0, len(items))
	for _, rm := range items {
		rid, err := s.a.UUIDString(rm.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		out = append(out, roomDTO{ID: rid, Name: rm.Name, Capacity: s.a.Int32Ptr(rm.Capacity)})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleRoomsCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var req struct {
		Name     string `json:"name"`
		Capacity *int32 `json:"capacity"`
	}
	if err := s.a.DecodeJSON(w, r, &req); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	var capVal pgtype.Int4
	if req.Capacity != nil {
		capVal = pgtype.Int4{Int32: *req.Capacity, Valid: true}
	}
	s.a.WithIdempotentTx(w, r, user.ID, "rooms", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		item, err := qtx.RoomCreate(r.Context(), sqldb.RoomCreateParams{Name: req.Name, Capacity: capVal})
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		rid, err := s.a.UUIDString(item.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		return http.StatusCreated, map[string]any{"id": rid, "name": item.Name, "capacity": s.a.Int32Ptr(item.Capacity)}, nil
	})
}

func (s *server) handleRoomsGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustUser(w, r); !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	item, err := s.deps.Q.RoomGetByID(r.Context(), id)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	rid, err := s.a.UUIDString(item.ID)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"id": rid, "name": item.Name, "capacity": s.a.Int32Ptr(item.Capacity)})
}

func (s *server) handleRoomsUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	var req struct {
		Name     string `json:"name"`
		Capacity *int32 `json:"capacity"`
	}
	if err := s.a.DecodeJSON(w, r, &req); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	var capVal pgtype.Int4
	if req.Capacity != nil {
		capVal = pgtype.Int4{Int32: *req.Capacity, Valid: true}
	}
	s.a.WithIdempotentTx(w, r, user.ID, "rooms", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		item, err := qtx.RoomUpdate(r.Context(), sqldb.RoomUpdateParams{ID: id, Name: req.Name, Capacity: capVal})
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		rid, err := s.a.UUIDString(item.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{"id": rid, "name": item.Name, "capacity": s.a.Int32Ptr(item.Capacity)}, nil
	})
}
