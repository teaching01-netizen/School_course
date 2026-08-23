package absenceshttp

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func (s *server) autoArchiveExpiredSitIns(ctx context.Context, actorID pgtype.UUID) ([]string, error) {
	if s.deps.DB == nil || s.deps.Q == nil {
		return nil, fmt.Errorf("absence auto-archive dependencies are not configured")
	}

	tx, err := s.deps.DB.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin absence auto-archive transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.deps.Q.WithTx(tx)
	ids, err := qtx.AutoArchiveExpiredSitIns(ctx, s.deps.InstituteTZ, actorID)
	if err != nil {
		return nil, err
	}

	archivedIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		idText, err := s.a.UUIDString(id)
		if err != nil {
			return nil, fmt.Errorf("format auto-archived absence ID: %w", err)
		}
		if err := qtx.AbsenceAuditInsert(ctx, sqldb.AbsenceAuditInsertParams{
			AbsenceID: id,
			Action:    "actioned",
			ActorID:   actorID,
			Details: map[string]any{
				"auto_archived": true,
				"reason":        "latest assigned sit-in date is before today",
			},
		}); err != nil {
			return nil, fmt.Errorf("write auto-archive absence timeline: %w", err)
		}
		if _, err := qtx.AuditInsert(ctx, sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "absence.auto_archived",
			Payload: map[string]any{
				"absence_id": idText,
			},
		}); err != nil {
			return nil, fmt.Errorf("write auto-archive audit log: %w", err)
		}
		archivedIDs = append(archivedIDs, idText)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit absence auto-archive transaction: %w", err)
	}
	return archivedIDs, nil
}
