package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type SatVerbalPolicyMapping struct {
	ID         pgtype.UUID        `json:"id"`
	RuleID     string             `json:"rule_id"`
	CourseID   pgtype.UUID        `json:"course_id"`
	PolicyRule []byte             `json:"policy_rule"`
	PolicyHash string             `json:"policy_hash"`
	Active     bool               `json:"active"`
	CreatedAt  pgtype.Timestamptz `json:"created_at"`
	UpdatedAt  pgtype.Timestamptz `json:"updated_at"`
}

type SatVerbalPolicyCourseMapping struct {
	ID                pgtype.UUID        `json:"id"`
	RuleID            string             `json:"rule_id"`
	CourseID          pgtype.UUID        `json:"course_id"`
	CourseCode        string             `json:"course_code"`
	CourseName        string             `json:"course_name"`
	SubjectID         pgtype.UUID        `json:"subject_id"`
	SubjectCode       string             `json:"subject_code"`
	SubjectName       string             `json:"subject_name"`
	CycleID           pgtype.Text        `json:"cycle_id"`
	Level             pgtype.Int2        `json:"level"`
	RootCourseGroupID pgtype.UUID        `json:"root_course_group_id"`
	SitInRuleID       pgtype.UUID        `json:"sit_in_rule_id"`
	PolicyRule        []byte             `json:"policy_rule"`
	PolicyHash        string             `json:"policy_hash"`
	Active            bool               `json:"active"`
	CreatedAt         pgtype.Timestamptz `json:"created_at"`
	UpdatedAt         pgtype.Timestamptz `json:"updated_at"`
}

type SatVerbalPolicyMappingReplaceParam struct {
	RuleID     string
	CourseID   pgtype.UUID
	PolicyRule []byte
	PolicyHash string
}

func (q *Queries) SatVerbalPolicyMappingsList(ctx context.Context) ([]SatVerbalPolicyCourseMapping, error) {
	rows, err := q.db.Query(ctx, `
		SELECT m.id, m.rule_id, m.course_id, c.code, c.name, c.subject_id,
		       COALESCE(sub.code, ''), COALESCE(sub.name, ''), c.cycle_id, c.level,
		       c.root_course_group_id, rcg.sit_in_rule_id, m.policy_rule, m.policy_hash,
		       m.active, m.created_at, m.updated_at
		FROM sat_verbal_policy_mappings m
		JOIN courses c ON c.id = m.course_id
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		WHERE m.active = true
		  AND c.deleted_at IS NULL
		ORDER BY m.rule_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SatVerbalPolicyCourseMapping
	for rows.Next() {
		var r SatVerbalPolicyCourseMapping
		if err := rows.Scan(
			&r.ID, &r.RuleID, &r.CourseID, &r.CourseCode, &r.CourseName, &r.SubjectID,
			&r.SubjectCode, &r.SubjectName, &r.CycleID, &r.Level, &r.RootCourseGroupID,
			&r.SitInRuleID, &r.PolicyRule, &r.PolicyHash, &r.Active, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queries) SatVerbalPolicyMappingGetActiveByCourse(ctx context.Context, courseID pgtype.UUID) (*SatVerbalPolicyCourseMapping, error) {
	var r SatVerbalPolicyCourseMapping
	err := q.db.QueryRow(ctx, `
		SELECT m.id, m.rule_id, m.course_id, c.code, c.name, c.subject_id,
		       COALESCE(sub.code, ''), COALESCE(sub.name, ''), c.cycle_id, c.level,
		       c.root_course_group_id, rcg.sit_in_rule_id, m.policy_rule, m.policy_hash,
		       m.active, m.created_at, m.updated_at
		FROM sat_verbal_policy_mappings m
		JOIN courses c ON c.id = m.course_id
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		WHERE m.course_id = $1
		  AND m.active = true
		  AND c.deleted_at IS NULL
	`, courseID).Scan(
		&r.ID, &r.RuleID, &r.CourseID, &r.CourseCode, &r.CourseName, &r.SubjectID,
		&r.SubjectCode, &r.SubjectName, &r.CycleID, &r.Level, &r.RootCourseGroupID,
		&r.SitInRuleID, &r.PolicyRule, &r.PolicyHash, &r.Active, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (q *Queries) SatVerbalPolicyMappingsReplace(ctx context.Context, params []SatVerbalPolicyMappingReplaceParam) ([]SatVerbalPolicyMapping, error) {
	if _, err := q.db.Exec(ctx, `DELETE FROM sat_verbal_policy_mappings`); err != nil {
		return nil, err
	}
	out := make([]SatVerbalPolicyMapping, 0, len(params))
	for _, p := range params {
		var r SatVerbalPolicyMapping
		err := q.db.QueryRow(ctx, `
			INSERT INTO sat_verbal_policy_mappings (rule_id, course_id, policy_rule, policy_hash, active)
			VALUES ($1, $2, $3::jsonb, $4, true)
			RETURNING id, rule_id, course_id, policy_rule, policy_hash, active, created_at, updated_at
		`, p.RuleID, p.CourseID, string(p.PolicyRule), p.PolicyHash).Scan(
			&r.ID, &r.RuleID, &r.CourseID, &r.PolicyRule, &r.PolicyHash, &r.Active, &r.CreatedAt, &r.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (q *Queries) SatVerbalPolicyMappingDeleteByRule(ctx context.Context, ruleID string) error {
	tag, err := q.db.Exec(ctx, `
		DELETE FROM sat_verbal_policy_mappings
		WHERE rule_id = $1
	`, ruleID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (q *Queries) AdvisoryLockForText(ctx context.Context, key string) error {
	_, err := q.db.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, key)
	return err
}

func (q *Queries) CourseSubjectByID(ctx context.Context, courseID pgtype.UUID) (SubjectCourseV2, error) {
	var r SubjectCourseV2
	err := q.db.QueryRow(ctx, `
		SELECT c.id, c.code, c.name, c.subject_id, COALESCE(sub.code, ''), COALESCE(sub.name, ''),
		       c.cycle_id, c.level, c.root_course_group_id, rcg.sit_in_rule_id
		FROM courses c
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		WHERE c.id = $1
		  AND c.deleted_at IS NULL
	`, courseID).Scan(&r.ID, &r.Code, &r.Name, &r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.CycleID, &r.Level, &r.RootCourseGroupID, &r.SitInRuleID)
	if err != nil {
		return SubjectCourseV2{}, err
	}
	return r, nil
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
