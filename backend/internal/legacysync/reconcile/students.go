package reconcile

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/legacysync/normalize"
)

// ApplyStudentProfiles converges student directory observations from the old
// site's /Admin/Students page onto the local students table. Every field is
// fill-in-if-empty: values that are currently empty locally are filled from
// the legacy observation, while non-empty values (human edits, CRM imports)
// are never clobbered. Each wcode is also recorded as a legacy external ref
// so the legacy sync can later enumerate the students it manages. It returns
// the number of students whose profile actually changed.
func (r *FullReconciler) ApplyStudentProfiles(ctx context.Context, students []normalize.LegacyStudent, opts FullReconcileOptions) (int, error) {
	if r.pool == nil || r.q == nil {
		return 0, errors.New("legacy student profile apply: pool and queries are required")
	}
	if opts.ShadowMode {
		return 0, nil
	}
	changed := 0
	for _, student := range students {
		if student.WCode == "" {
			continue
		}
		id, err := r.q.StudentProfileUpsert(ctx, sqldb.StudentProfileUpsertParams{
			Wcode:        student.WCode,
			FullName:     student.Name,
			Nickname:     pgtype.Text{String: student.Nickname, Valid: student.Nickname != ""},
			School:       pgtype.Text{String: student.School, Valid: student.School != ""},
			Level:        pgtype.Text{String: student.Level, Valid: student.Level != ""},
			Year:         pgtype.Text{String: student.Year, Valid: student.Year != ""},
			StudentPhone: pgtype.Text{String: student.Phone, Valid: student.Phone != ""},
			Email:        pgtype.Text{String: student.Email, Valid: student.Email != ""},
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// The WHERE clause skipped the update: the local profile already
			// holds every observed value (or a newer human/CRM edit). Make
			// sure the legacy ref still exists, then move on.
			if _, refErr := r.pool.Exec(ctx, `
				INSERT INTO external_refs (source, entity_type, external_id, internal_id, state)
				SELECT $1, 'student', $2, id, 'active' FROM students WHERE lower(wcode) = lower($2)
				ON CONFLICT (source, entity_type, external_id) DO NOTHING
			`, r.source, student.WCode); refErr != nil {
				return changed, fmt.Errorf("record student external ref %s: %w", student.WCode, refErr)
			}
			continue
		}
		if err != nil {
			return changed, fmt.Errorf("upsert student profile %s: %w", student.WCode, err)
		}
		if _, err := r.pool.Exec(ctx, `
			INSERT INTO external_refs (source, entity_type, external_id, internal_id, state)
			VALUES ($1, 'student', $2, $3, 'active')
			ON CONFLICT (source, entity_type, external_id) DO NOTHING
		`, r.source, student.WCode, id); err != nil {
			return changed, fmt.Errorf("record student external ref %s: %w", student.WCode, err)
		}
		changed++
	}
	return changed, nil
}
