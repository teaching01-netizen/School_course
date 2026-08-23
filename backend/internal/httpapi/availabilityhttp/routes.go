package availabilityhttp

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/scheduling"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func (s *server) writeAvailabilityMutationErr(w http.ResponseWriter, err error) {
	var scheduleErr *scheduling.Err
	if errors.As(err, &scheduleErr) && scheduleErr.Code == "availability_conflict" {
		s.a.WriteErrDetails(w, http.StatusConflict, scheduleErr.Code, scheduleErr.Message, scheduleErr.Details)
		return
	}
	status, code, msg := s.a.ClassifyDBErr(err)
	s.a.WriteErr(w, status, code, msg)
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("GET /api/v1/availability/teachers/{teacher_id}", s.handleTeacherAvailabilityList)
	mux.HandleFunc("POST /api/v1/availability/teachers/{teacher_id}", s.handleTeacherAvailabilityCreate)
	mux.HandleFunc("DELETE /api/v1/availability/teachers/{teacher_id}/{id}", s.handleTeacherAvailabilityDelete)

	mux.HandleFunc("GET /api/v1/availability/rooms/{room_id}", s.handleRoomAvailabilityList)
	mux.HandleFunc("POST /api/v1/availability/rooms/{room_id}", s.handleRoomAvailabilityCreate)
	mux.HandleFunc("DELETE /api/v1/availability/rooms/{room_id}/{id}", s.handleRoomAvailabilityDelete)
}

func (s *server) handleTeacherAvailabilityList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	teacherID, err := s.a.ParseUUID(r.PathValue("teacher_id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_teacher_id", "Invalid teacher_id")
		return
	}
	rows, err := s.deps.Q.ListTeacherAvailability(r.Context(), teacherID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type item struct {
		ID        string `json:"id"`
		TeacherID string `json:"teacher_id"`
		StartAt   string `json:"start_at"`
		EndAt     string `json:"end_at"`
	}
	out := make([]item, 0, len(rows))
	for _, row := range rows {
		id, err := s.a.UUIDString(row.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		tid, err := s.a.UUIDString(row.TeacherID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		start, ok := s.a.TimeString(row.StartAt)
		if !ok {
			continue
		}
		end, ok := s.a.TimeString(row.EndAt)
		if !ok {
			continue
		}
		out = append(out, item{ID: id, TeacherID: tid, StartAt: start, EndAt: end})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleTeacherAvailabilityCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	teacherID, err := s.a.ParseUUID(r.PathValue("teacher_id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_teacher_id", "Invalid teacher_id")
		return
	}
	var body struct {
		StartAt string `json:"start_at"`
		EndAt   string `json:"end_at"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	startAt, err := s.a.ParseTimestamptz(body.StartAt)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_start_at", "Invalid start_at")
		return
	}
	endAt, err := s.a.ParseTimestamptz(body.EndAt)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_end_at", "Invalid end_at")
		return
	}

	s.a.WithIdempotentTx(w, r, user.ID, "availability", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		row, warnings, err := s.deps.Scheduling.CreateTeacherAvailabilityWithWarningsTx(r.Context(), tx, qtx, teacherID, startAt, endAt)
		if err != nil {
			s.writeAvailabilityMutationErr(w, err)
			return 0, nil, err
		}
		id, err := s.a.UUIDString(row.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		tid, err := s.a.UUIDString(row.TeacherID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		start, _ := s.a.TimeString(row.StartAt)
		end, _ := s.a.TimeString(row.EndAt)
		return http.StatusCreated, map[string]any{
			"id":         id,
			"teacher_id": tid,
			"start_at":   start,
			"end_at":     end,
			"warnings":   warnings,
		}, nil
	})
}

func (s *server) handleTeacherAvailabilityDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	teacherID, err := s.a.ParseUUID(r.PathValue("teacher_id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_teacher_id", "Invalid teacher_id")
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	s.a.WithIdempotentTx(w, r, user.ID, "availability", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		warnings, err := s.deps.Scheduling.DeleteTeacherAvailabilityWithWarningsTx(r.Context(), tx, qtx, teacherID, id)
		if err != nil {
			s.writeAvailabilityMutationErr(w, err)
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{"ok": true, "warnings": warnings}, nil
	})
}

func (s *server) handleRoomAvailabilityList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	roomID, err := s.a.ParseUUID(r.PathValue("room_id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_room_id", "Invalid room_id")
		return
	}
	rows, err := s.deps.Q.ListRoomAvailability(r.Context(), roomID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type item struct {
		ID      string `json:"id"`
		RoomID  string `json:"room_id"`
		StartAt string `json:"start_at"`
		EndAt   string `json:"end_at"`
	}
	out := make([]item, 0, len(rows))
	for _, row := range rows {
		id, err := s.a.UUIDString(row.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		rid, err := s.a.UUIDString(row.RoomID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		start, ok := s.a.TimeString(row.StartAt)
		if !ok {
			continue
		}
		end, ok := s.a.TimeString(row.EndAt)
		if !ok {
			continue
		}
		out = append(out, item{ID: id, RoomID: rid, StartAt: start, EndAt: end})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleRoomAvailabilityCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	roomID, err := s.a.ParseUUID(r.PathValue("room_id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_room_id", "Invalid room_id")
		return
	}
	var body struct {
		StartAt string `json:"start_at"`
		EndAt   string `json:"end_at"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	startAt, err := s.a.ParseTimestamptz(body.StartAt)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_start_at", "Invalid start_at")
		return
	}
	endAt, err := s.a.ParseTimestamptz(body.EndAt)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_end_at", "Invalid end_at")
		return
	}

	s.a.WithIdempotentTx(w, r, user.ID, "availability", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		row, warnings, err := s.deps.Scheduling.CreateRoomAvailabilityWithWarningsTx(r.Context(), tx, qtx, roomID, startAt, endAt)
		if err != nil {
			s.writeAvailabilityMutationErr(w, err)
			return 0, nil, err
		}
		id, err := s.a.UUIDString(row.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		rid, err := s.a.UUIDString(row.RoomID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return 0, nil, err
		}
		start, _ := s.a.TimeString(row.StartAt)
		end, _ := s.a.TimeString(row.EndAt)
		return http.StatusCreated, map[string]any{
			"id":       id,
			"room_id":  rid,
			"start_at": start,
			"end_at":   end,
			"warnings": warnings,
		}, nil
	})
}

func (s *server) handleRoomAvailabilityDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	roomID, err := s.a.ParseUUID(r.PathValue("room_id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_room_id", "Invalid room_id")
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	s.a.WithIdempotentTx(w, r, user.ID, "availability", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		warnings, err := s.deps.Scheduling.DeleteRoomAvailabilityWithWarningsTx(r.Context(), tx, qtx, roomID, id)
		if err != nil {
			s.writeAvailabilityMutationErr(w, err)
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{"ok": true, "warnings": warnings}, nil
	})
}
