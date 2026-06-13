package crossstudy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Processor struct {
	db     *pgxpool.Pool
	store  *Store
	logger *slog.Logger
}

func NewProcessor(db *pgxpool.Pool, store *Store, logger *slog.Logger) *Processor {
	return &Processor{db: db, store: store, logger: logger}
}

// ProcessSnapshot re-checks all cross-study assignments against the given snapshot.
// Called after a new snapshot import completes.
func (p *Processor) ProcessSnapshot(ctx context.Context, snapshotID uuid.UUID) error {
	hasAny, err := p.store.HasAnyAssignment(ctx)
	if err != nil {
		return fmt.Errorf("check assignments exist: %w", err)
	}
	if !hasAny {
		return nil
	}

	changes, err := p.store.LoadPendingChanges(ctx, snapshotID)
	if err != nil {
		return fmt.Errorf("load pending changes: %w", err)
	}

	for _, ch := range changes {
		// If there's no crm_row for this student in the new snapshot (empty course_name),
		// the student is orphaned from the source data.
		if ch.CurrentCourseName == "" {
			if err := p.store.UpdateStatus(ctx, ch.ID, string(StatusOrphaned), false); err != nil {
				p.logger.Error("update status to orphaned", "assignment_id", ch.ID, "error", err)
			}
			continue
		}

		currentHash := hashExtraNote(ch.CurrentNote)
		if currentHash != ch.StoredHash {
			if err := p.store.UpdateStatus(ctx, ch.ID, string(StatusNotesChanged), true); err != nil {
				p.logger.Error("update status to notes_changed", "assignment_id", ch.ID, "error", err)
			}
			continue
		}

		if err := p.store.UpdateStatus(ctx, ch.ID, string(StatusActive), true); err != nil {
			p.logger.Error("update status to active", "assignment_id", ch.ID, "error", err)
		}
	}

	return nil
}
