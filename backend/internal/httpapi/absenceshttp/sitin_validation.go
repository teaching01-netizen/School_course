package absenceshttp

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func (s *server) validateSitInCandidates(ctx context.Context, q *sqldb.Queries, absenceID pgtype.UUID, sessionIDs []pgtype.UUID) error {
	if s.deps.SitInResolver == nil {
		return nil
	}
	resolver := s.deps.SitInResolver.WithQueries(q)
	for _, sessionID := range sessionIDs {
		validation, err := resolver.ValidateCandidate(ctx, absenceID, sessionID)
		if err != nil {
			return err
		}
		if !validation.Valid {
			return fmt.Errorf("sit-in session is not currently eligible: %s", strings.Join(validation.Reasons, ", "))
		}
	}
	return nil
}
