package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type SatVerbalPolicyMapping struct {
	ID                  pgtype.UUID        `json:"id"`
	SubjectID           pgtype.UUID        `json:"subject_id"`
	Policy              []byte             `json:"policy"`
	PolicyHash          string             `json:"policy_hash"`
	Warnings            []byte             `json:"warnings"`
	MatchedCourses      []byte             `json:"matched_courses"`
	UnmatchedPolicyRows []byte             `json:"unmatched_policy_rows"`
	UnmatchedCourses    []byte             `json:"unmatched_courses"`
	Active              bool               `json:"active"`
	CreatedAt           pgtype.Timestamptz `json:"created_at"`
	UpdatedAt           pgtype.Timestamptz `json:"updated_at"`
}

type SatVerbalPolicyMappingUpsertParams struct {
	SubjectID           pgtype.UUID
	Policy              []byte
	PolicyHash          string
	Warnings            []byte
	MatchedCourses      []byte
	UnmatchedPolicyRows []byte
	UnmatchedCourses    []byte
}

func (q *Queries) SatVerbalPolicyMappingGetBySubject(ctx context.Context, subjectID pgtype.UUID) (*SatVerbalPolicyMapping, error) {
	var r SatVerbalPolicyMapping
	err := q.db.QueryRow(ctx, `
		SELECT id, subject_id, policy, policy_hash, warnings, matched_courses,
		       unmatched_policy_rows, unmatched_courses, active, created_at, updated_at
		FROM sat_verbal_policy_mappings
		WHERE subject_id = $1
	`, subjectID).Scan(
		&r.ID, &r.SubjectID, &r.Policy, &r.PolicyHash, &r.Warnings, &r.MatchedCourses,
		&r.UnmatchedPolicyRows, &r.UnmatchedCourses, &r.Active, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (q *Queries) SatVerbalPolicyMappingGetActiveBySubject(ctx context.Context, subjectID pgtype.UUID) (*SatVerbalPolicyMapping, error) {
	var r SatVerbalPolicyMapping
	err := q.db.QueryRow(ctx, `
		SELECT id, subject_id, policy, policy_hash, warnings, matched_courses,
		       unmatched_policy_rows, unmatched_courses, active, created_at, updated_at
		FROM sat_verbal_policy_mappings
		WHERE subject_id = $1 AND active = true
	`, subjectID).Scan(
		&r.ID, &r.SubjectID, &r.Policy, &r.PolicyHash, &r.Warnings, &r.MatchedCourses,
		&r.UnmatchedPolicyRows, &r.UnmatchedCourses, &r.Active, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (q *Queries) SatVerbalPolicyMappingUpsert(ctx context.Context, p SatVerbalPolicyMappingUpsertParams) (*SatVerbalPolicyMapping, error) {
	var r SatVerbalPolicyMapping
	err := q.db.QueryRow(ctx, `
		INSERT INTO sat_verbal_policy_mappings (
			subject_id, policy, policy_hash, warnings, matched_courses,
			unmatched_policy_rows, unmatched_courses, active
		)
		VALUES ($1, $2::jsonb, $3, $4::jsonb, $5::jsonb, $6::jsonb, $7::jsonb, true)
		ON CONFLICT (subject_id) DO UPDATE SET
			policy = EXCLUDED.policy,
			policy_hash = EXCLUDED.policy_hash,
			warnings = EXCLUDED.warnings,
			matched_courses = EXCLUDED.matched_courses,
			unmatched_policy_rows = EXCLUDED.unmatched_policy_rows,
			unmatched_courses = EXCLUDED.unmatched_courses,
			active = true,
			updated_at = now()
		RETURNING id, subject_id, policy, policy_hash, warnings, matched_courses,
		          unmatched_policy_rows, unmatched_courses, active, created_at, updated_at
	`, p.SubjectID, string(p.Policy), p.PolicyHash, string(p.Warnings), string(p.MatchedCourses), string(p.UnmatchedPolicyRows), string(p.UnmatchedCourses)).Scan(
		&r.ID, &r.SubjectID, &r.Policy, &r.PolicyHash, &r.Warnings, &r.MatchedCourses,
		&r.UnmatchedPolicyRows, &r.UnmatchedCourses, &r.Active, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (q *Queries) SatVerbalPolicyMappingDelete(ctx context.Context, subjectID pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `
		DELETE FROM sat_verbal_policy_mappings
		WHERE subject_id = $1
	`, subjectID)
	return err
}

func (q *Queries) AdvisoryLockForText(ctx context.Context, key string) error {
	_, err := q.db.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, key)
	return err
}

func (q *Queries) CoursesBySubject(ctx context.Context, subjectID pgtype.UUID) ([]SubjectCourseV2, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.code, c.name, c.subject_id, COALESCE(sub.code, ''), COALESCE(sub.name, ''),
		       c.cycle_id, c.level, c.root_course_group_id, rcg.sit_in_rule_id
		FROM courses c
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		WHERE c.subject_id = $1
		  AND c.deleted_at IS NULL
		ORDER BY c.name ASC, c.code ASC
	`, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SubjectCourseV2
	for rows.Next() {
		var r SubjectCourseV2
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.CycleID, &r.Level, &r.RootCourseGroupID, &r.SitInRuleID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queries) RootCourseGroupFindByName(ctx context.Context, name string) (pgtype.UUID, bool, error) {
	var id pgtype.UUID
	err := q.db.QueryRow(ctx, `
		SELECT id
		FROM root_course_groups
		WHERE name = $1
		ORDER BY created_at ASC
		LIMIT 1
	`, name).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return pgtype.UUID{}, false, nil
		}
		return pgtype.UUID{}, false, err
	}
	return id, true, nil
}
