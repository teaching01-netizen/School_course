package crossstudy

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RosterWriter applies roster changes as a direct effect of cross-study overrides.
// The cross-study module creates overrides AND their immediate roster effect atomically.
// Reconcile (on next import) converges to the same state — these writes just close
// the timing gap so preflight sees the correct roster immediately.
type RosterWriter interface {
	ExcludeStudent(ctx context.Context, tx pgx.Tx, courseID, studentID uuid.UUID) error
	IncludeStudent(ctx context.Context, tx pgx.Tx, courseID, studentID uuid.UUID) error
}
