package absenceshttp

import (
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"warwick-institute/internal/idempotency"
)

type staffBatchCreateAbsenceRequest struct {
	Items []staffCreateAbsenceRequest `json:"items"`
}

func (s *server) handleStaffCreateAbsenceBatch(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	createdIDs := make([]string, 0)
	createdItems := make([]managedAbsenceDTO, 0)
	if s.a.WithIdempotentTx(w, r, idempotency.SystemActorUUID, "absences-staff-batch", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		var body staffBatchCreateAbsenceRequest
		if err := s.a.DecodeJSON(w, r, &body); err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
			return 0, nil, err
		}
		if len(body.Items) == 0 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_items", "At least one absence is required")
			return 0, nil, fmt.Errorf("no staff absence items")
		}

		studentWcode := ""
		for index := range body.Items {
			body.Items[index].Wcode = normalizeWCode(body.Items[index].Wcode)
			if body.Items[index].Wcode == "" {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_wcode", "wcode is required")
				return 0, nil, fmt.Errorf("wcode is required")
			}
			if studentWcode == "" {
				studentWcode = body.Items[index].Wcode
			} else if studentWcode != body.Items[index].Wcode {
				s.a.WriteErr(w, http.StatusBadRequest, "mixed_students", "All staff-created absences must belong to one student")
				return 0, nil, fmt.Errorf("mixed students in staff batch")
			}
		}

		for _, item := range body.Items {
			createdID, rawDTO, err := s.createStaffAbsenceTx(w, r, tx, qtx, user, item)
			if err != nil {
				return 0, nil, err
			}
			dto, ok := rawDTO.(managedAbsenceDTO)
			if !ok {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not build created absence response")
				return 0, nil, fmt.Errorf("staff absence response type mismatch")
			}
			createdIDs = append(createdIDs, createdID)
			createdItems = append(createdItems, dto)
		}

		return http.StatusCreated, map[string]any{"ids": createdIDs, "items": createdItems}, nil
	}) {
		return
	}

	if len(createdIDs) > 0 {
		s.publishAbsenceChanges(createdIDs)
	}
}
