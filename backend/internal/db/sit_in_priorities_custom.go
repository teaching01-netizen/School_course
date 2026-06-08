package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type SitInPriority struct {
	ID                pgtype.UUID `json:"id"`
	RootCourseGroupID pgtype.UUID `json:"root_course_group_id"`
	SitInRuleID       pgtype.UUID `json:"sit_in_rule_id"`
	PriorityLevel     int16       `json:"priority_level"`
	Label             string      `json:"label"`
	TargetRank        pgtype.Int2 `json:"target_rank"`
	TargetSection     pgtype.Int2 `json:"target_section"`
	CreatedAt         pgtype.Timestamptz `json:"created_at"`
}

func (q *Queries) SitInPrioritiesByRootCourseGroup(ctx context.Context, rootCourseGroupID pgtype.UUID) ([]SitInPriority, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, root_course_group_id, sit_in_rule_id, priority_level, label, target_rank, target_section, created_at
		FROM sit_in_priorities
		WHERE root_course_group_id = $1
		ORDER BY priority_level ASC
	`, rootCourseGroupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SitInPriority
	for rows.Next() {
		var p SitInPriority
		if err := rows.Scan(&p.ID, &p.RootCourseGroupID, &p.SitInRuleID, &p.PriorityLevel, &p.Label, &p.TargetRank, &p.TargetSection, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queries) SitInPrioritiesByRootCourseGroupWithRule(ctx context.Context, rootCourseGroupID pgtype.UUID) ([]SitInPriorityWithRule, error) {
	rows, err := q.db.Query(ctx, `
		SELECT
			p.id, p.root_course_group_id, p.sit_in_rule_id, p.priority_level, p.label, p.target_rank, p.target_section, p.created_at,
			r.name AS rule_name, r.type AS rule_type, r.predicate AS rule_predicate
		FROM sit_in_priorities p
		JOIN sit_in_rules r ON r.id = p.sit_in_rule_id
		WHERE p.root_course_group_id = $1
		ORDER BY p.priority_level ASC
	`, rootCourseGroupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SitInPriorityWithRule
	for rows.Next() {
		var p SitInPriorityWithRule
		if err := rows.Scan(
			&p.ID, &p.RootCourseGroupID, &p.SitInRuleID, &p.PriorityLevel, &p.Label, &p.TargetRank, &p.TargetSection, &p.CreatedAt,
			&p.RuleName, &p.RuleType, &p.RulePredicate,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type SitInPriorityWithRule struct {
	SitInPriority
	RuleName      string `json:"rule_name"`
	RuleType      string `json:"rule_type"`
	RulePredicate []byte `json:"rule_predicate"`
}
