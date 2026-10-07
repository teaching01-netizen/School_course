package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const courseLinkSuggestionsSQL = `
WITH course_pool AS MATERIALIZED (
	SELECT c.id, c.code, c.name, c.subject_id, c.level, c.cycle_id,
	       c.root_course_group_id, sub.code AS subject_code, sub.name AS subject_name,
	       cy.label AS cycle_label, cy.start_date AS cycle_start, cy.end_date AS cycle_end,
	       -- [[:alnum:]] is locale-dependent and drops Thai under common
	       -- collations; keep ASCII letters/digits and Thai letters, vowels,
	       -- tone marks, and digits explicitly.
	       regexp_replace(lower(c.name), '[^a-z0-9ก-ฮะ-ฺเ-๎๐-๙]+', '', 'g') AS name_key
	FROM courses c
	JOIN subjects sub ON sub.id = c.subject_id
	LEFT JOIN crm_cycles cy ON cy.id = c.cycle_id
	WHERE c.legacy_archived = false
	  AND NOT EXISTS (
	      SELECT 1 FROM course_merge_group_members m WHERE m.course_id = c.id
	  )
	  AND NOT EXISTS (
	      SELECT 1 FROM sat_verbal_policy_mappings sat
	      WHERE sat.active AND sat.course_id = c.id
	  )
), raw_pairs AS MATERIALIZED (
	SELECT configured.id AS source_id, partial.id AS partial_id,
	       configured.code AS source_code, configured.name AS source_name,
	       configured.subject_id, partial.subject_id AS partial_subject_id,
	       configured.subject_code, configured.subject_name,
	       configured.level AS source_level, configured.cycle_id AS source_cycle_id,
	       configured.cycle_label AS source_cycle_label,
	       configured.cycle_start, configured.cycle_end,
	       configured.root_course_group_id AS source_root_group_id,
	       source_root.name AS source_root_group_name,
	       source_rules.sit_in_rule_id AS source_sit_in_rule_id,
	       source_sit_in_rule.name AS source_sit_in_rule_name,
	       source_sit_in_rule.type AS source_sit_in_rule_type,
	       source_sit_in_rule.description AS source_sit_in_rule_description,
	       source_rules.absence_form_visible AS source_absence_form_visible,
	       source_rules.absence_form_active AS source_absence_form_active,
	       source_sit_in_rule.predicate AS source_sit_in_rule_predicate,
	       partial.code AS partial_code, partial.name AS partial_name,
	       partial.level AS partial_level, partial.cycle_id AS partial_cycle_id,
	       partial.cycle_label AS partial_cycle_label,
	       partial.root_course_group_id AS partial_root_group_id,
	       partial_root.name AS partial_root_group_name
	FROM course_pool configured
	JOIN course_pool partial
	  ON partial.subject_id = configured.subject_id
	 AND partial.name_key = configured.name_key
	 AND partial.id <> configured.id
	LEFT JOIN root_course_groups source_root ON source_root.id = configured.root_course_group_id
	LEFT JOIN course_rule_configs source_rules ON source_rules.course_id = configured.id
	LEFT JOIN sit_in_rules source_sit_in_rule ON source_sit_in_rule.id = source_rules.sit_in_rule_id
	LEFT JOIN root_course_groups partial_root ON partial_root.id = partial.root_course_group_id
	WHERE configured.name_key <> ''
	  AND configured.level >= 1
	  AND configured.cycle_id IS NOT NULL
	  AND configured.root_course_group_id IS NOT NULL
	  AND (partial.level IS NULL OR partial.level = configured.level)
	  AND (partial.cycle_id IS NULL OR partial.cycle_id = configured.cycle_id)
	  AND (partial.root_course_group_id IS NULL OR partial.root_course_group_id = configured.root_course_group_id)
	  AND (partial.level IS NULL OR partial.cycle_id IS NULL OR partial.root_course_group_id IS NULL)
	  AND NOT EXISTS (
	      SELECT 1 FROM course_link_dismissals d
	      WHERE d.course_a = LEAST(configured.id, partial.id)
	        AND d.course_b = GREATEST(configured.id, partial.id)
	  )
), candidate_course_ids AS MATERIALIZED (
	SELECT source_id AS course_id FROM raw_pairs
	UNION
	SELECT partial_id AS course_id FROM raw_pairs
), session_rows AS MATERIALIZED (
	SELECT s.course_id, s.id, s.start_at, s.end_at,
	       (s.start_at AT TIME ZONE $1)::date AS local_date,
	       EXTRACT(ISODOW FROM (s.start_at AT TIME ZONE $1))::smallint AS iso_weekday,
	       (s.start_at AT TIME ZONE $1)::time AS local_start_time,
	       s.end_at - s.start_at AS duration
	FROM sessions s
	JOIN candidate_course_ids cc ON cc.course_id = s.course_id
	WHERE s.deleted_at IS NULL AND s.end_at > s.start_at
), session_facts AS MATERIALIZED (
	SELECT course_id,
	       count(*)::bigint AS session_count,
	       min(local_date) AS date_from,
	       max(local_date) AS date_to,
	       jsonb_agg(
	           jsonb_build_array(id::text, extract(epoch FROM start_at), extract(epoch FROM end_at))
	           ORDER BY id
	       ) AS evidence
	FROM session_rows
	GROUP BY course_id
), slot_summaries AS MATERIALIZED (
	SELECT course_id,
	       array_agg(slot_key ORDER BY slot_key) AS slots
	FROM (
	    SELECT DISTINCT course_id,
	           format('%s|%s|%s', iso_weekday, to_char(local_start_time, 'HH24:MI'), extract(epoch FROM duration)::bigint) AS slot_key
	    FROM session_rows
	) unique_slots
	GROUP BY course_id
), teacher_context_rows AS MATERIALIZED (
	SELECT ct.course_id, COALESCE(NULLIF(u.full_name, ''), u.username) AS display_name
	FROM course_teachers ct
	JOIN candidate_course_ids cc ON cc.course_id = ct.course_id
	JOIN users u ON u.id = ct.teacher_id
	UNION
	SELECT c.id, COALESCE(NULLIF(u.full_name, ''), u.username)
	FROM courses c
	JOIN candidate_course_ids cc ON cc.course_id = c.id
	JOIN users u ON u.id = c.teacher_id
), teacher_context AS MATERIALIZED (
	SELECT course_id, array_agg(display_name ORDER BY display_name) AS teacher_names
	FROM teacher_context_rows
	GROUP BY course_id
), candidates_with_sessions AS MATERIALIZED (
	SELECT p.*, source_facts.session_count AS source_session_count,
	       source_facts.date_from AS source_date_from, source_facts.date_to AS source_date_to,
	       source_facts.evidence AS source_evidence, source_slots.slots AS source_slots,
	       partial_facts.session_count AS partial_session_count,
	       partial_facts.date_from AS partial_date_from, partial_facts.date_to AS partial_date_to,
	       partial_facts.evidence AS partial_evidence, partial_slots.slots AS partial_slots,
	       COALESCE(source_teachers.teacher_names, ARRAY[]::text[]) AS source_teacher_names,
	       COALESCE(partial_teachers.teacher_names, ARRAY[]::text[]) AS partial_teacher_names
	FROM raw_pairs p
	JOIN session_facts source_facts ON source_facts.course_id = p.source_id
	JOIN slot_summaries source_slots ON source_slots.course_id = p.source_id
	JOIN session_facts partial_facts ON partial_facts.course_id = p.partial_id
	JOIN slot_summaries partial_slots ON partial_slots.course_id = p.partial_id
	LEFT JOIN teacher_context source_teachers ON source_teachers.course_id = p.source_id
	LEFT JOIN teacher_context partial_teachers ON partial_teachers.course_id = p.partial_id
), pair_evidence AS MATERIALIZED (
	SELECT p.*,
	       COALESCE((p.partial_cycle_id = p.source_cycle_id
	        AND p.cycle_start IS NOT NULL AND p.cycle_end IS NOT NULL
	        AND p.source_date_from >= p.cycle_start AND p.source_date_to <= p.cycle_end
	        AND p.partial_date_from >= p.cycle_start AND p.partial_date_to <= p.cycle_end), false) AS same_period,
	       COALESCE((p.source_cycle_id = p.partial_cycle_id
	        AND p.cycle_start IS NOT NULL AND p.cycle_end IS NOT NULL
	        AND (p.source_date_from < p.cycle_start OR p.source_date_to > p.cycle_end
	             OR p.partial_date_from < p.cycle_start OR p.partial_date_to > p.cycle_end)), false) AS outside_cycle,
	       NOT EXISTS (
	           SELECT 1 FROM session_rows partial_session
	           WHERE partial_session.course_id = p.partial_id
	             AND NOT EXISTS (
	                 SELECT 1 FROM session_rows source_session
	                 WHERE source_session.course_id = p.source_id
	                   AND source_session.iso_weekday = partial_session.iso_weekday
	                   AND source_session.local_start_time = partial_session.local_start_time
	             )
	       ) AS slots_compatible,
	       NOT EXISTS (
	           SELECT 1 FROM session_rows partial_session
	           WHERE partial_session.course_id = p.partial_id
	             AND NOT EXISTS (
	                 SELECT 1 FROM session_rows source_session
	                 WHERE source_session.course_id = p.source_id
	                   AND source_session.iso_weekday = partial_session.iso_weekday
	                   AND source_session.local_start_time = partial_session.local_start_time
	                   AND source_session.duration = partial_session.duration
	             )
	       ) AS durations_compatible,
	       EXISTS (
	           SELECT 1 FROM session_rows source_session
	           JOIN session_rows partial_session ON partial_session.course_id = p.partial_id
	           WHERE source_session.course_id = p.source_id
	             AND source_session.local_date = partial_session.local_date
	       ) AS same_local_date,
	       EXISTS (
	           SELECT 1 FROM session_rows source_session
	           JOIN session_rows partial_session ON partial_session.course_id = p.partial_id
	           WHERE source_session.course_id = p.source_id
	             AND source_session.start_at < partial_session.end_at
	             AND partial_session.start_at < source_session.end_at
	       ) AS intervals_overlap
	FROM candidates_with_sessions p
), eligible_pair_evidence AS MATERIALIZED (
	SELECT *
	FROM pair_evidence
	WHERE NOT outside_cycle
), pair_sets AS MATERIALIZED (
	SELECT source_id,
	       count(*)::bigint AS source_partner_count,
	       array_agg(partial_id::text ORDER BY partial_id) AS source_partner_ids
	FROM eligible_pair_evidence
	GROUP BY source_id
), partial_sets AS MATERIALIZED (
	SELECT partial_id,
	       count(*)::bigint AS partial_partner_count,
	       array_agg(source_id::text ORDER BY source_id) AS partial_partner_ids
	FROM eligible_pair_evidence
	GROUP BY partial_id
), scored AS MATERIALIZED (
	SELECT p.*, source_set.source_partner_count, source_set.source_partner_ids,
	       partial_set.partial_partner_count, partial_set.partial_partner_ids,
	       GREATEST(source_set.source_partner_count, partial_set.partial_partner_count) AS ambiguity_count,
	       (p.same_period AND p.slots_compatible AND p.durations_compatible
	        AND NOT p.same_local_date AND NOT p.intervals_overlap
	        AND source_set.source_partner_count = 1 AND partial_set.partial_partner_count = 1) AS is_high,
	       array_remove(ARRAY[
	           'same_subject'::text,
	           'normalized_name_match'::text,
	           CASE WHEN p.same_period THEN 'same_teaching_period' ELSE 'period_unknown' END,
	           CASE WHEN p.slots_compatible THEN 'compatible_slots' ELSE 'slot_mismatch' END,
	           CASE WHEN p.durations_compatible THEN 'equal_slot_durations' ELSE 'duration_mismatch' END,
	           CASE WHEN NOT p.same_local_date AND NOT p.intervals_overlap THEN 'complementary_dates' END,
	           CASE WHEN p.same_local_date THEN 'same_date_sessions' END,
	           CASE WHEN p.intervals_overlap THEN 'overlapping_sessions' END,
	           CASE WHEN source_set.source_partner_count = 1 AND partial_set.partial_partner_count = 1 THEN 'unique_partner' ELSE 'multiple_possible_partners' END
	       ], NULL) AS reason_codes
	FROM eligible_pair_evidence p
	JOIN pair_sets source_set ON source_set.source_id = p.source_id
	JOIN partial_sets partial_set ON partial_set.partial_id = p.partial_id
), with_fingerprint AS (
	SELECT scored.*,
	       CASE WHEN is_high THEN 'high' ELSE 'review' END AS confidence,
	       encode(digest(jsonb_build_object(
	           'detector_version', $8,
	           'institute_timezone', $1,
	           'source_id', source_id,
	           'source_code', source_code,
	           'source_subject_id', subject_id,
	           'source_name', source_name,
	           'source_level', source_level,
	           'source_cycle_id', source_cycle_id,
	           'cycle_start', cycle_start,
	           'cycle_end', cycle_end,
	           'source_root_course_group_id', source_root_group_id,
	           'source_sit_in_rule_id', source_sit_in_rule_id,
	           'source_sit_in_rule_name', source_sit_in_rule_name,
	           'source_sit_in_rule_type', source_sit_in_rule_type,
	           'source_sit_in_rule_description', source_sit_in_rule_description,
	           'source_sit_in_rule_predicate', source_sit_in_rule_predicate,
	           'source_absence_form_visible', source_absence_form_visible,
	           'source_absence_form_active', source_absence_form_active,
	           'source_sessions', source_evidence,
	           'partial_id', partial_id,
	           'partial_code', partial_code,
	           'partial_subject_id', partial_subject_id,
	           'partial_name', partial_name,
	           'partial_level', partial_level,
	           'partial_cycle_id', partial_cycle_id,
	           'partial_root_course_group_id', partial_root_group_id,
	           'partial_sessions', partial_evidence,
	           'source_partner_ids', source_partner_ids,
	           'partial_partner_ids', partial_partner_ids,
	           'same_period', same_period,
	           'slots_compatible', slots_compatible,
	           'durations_compatible', durations_compatible,
	           'same_local_date', same_local_date,
	           'intervals_overlap', intervals_overlap
	       )::text, 'sha256'), 'hex') AS evidence_fingerprint
	FROM scored
)
SELECT source_id, source_code, source_name, subject_id, subject_code, subject_name,
	   source_level, source_cycle_id, source_cycle_label, cycle_start::text, cycle_end::text,
	   source_root_group_id, source_root_group_name, source_sit_in_rule_id, source_sit_in_rule_name,
	   source_sit_in_rule_type, source_sit_in_rule_description, source_absence_form_visible,
	   source_absence_form_active, source_session_count,
	   source_date_from::text, source_date_to::text, source_slots, source_teacher_names,
	   partial_id, partial_code, partial_name, partial_level, partial_cycle_id,
	   partial_cycle_label, partial_root_group_id, partial_root_group_name, partial_session_count,
	   partial_date_from::text, partial_date_to::text, partial_slots, partial_teacher_names,
	   confidence, reason_codes, ambiguity_count, evidence_fingerprint
FROM with_fingerprint
WHERE NOT outside_cycle
	AND ($2::boolean OR confidence = 'high')
	AND ($3::uuid IS NULL OR (source_id, partial_id) > ($3::uuid, $4::uuid))
	AND ($5::uuid IS NULL OR source_id = $5::uuid)
	AND ($6::uuid IS NULL OR partial_id = $6::uuid)
	-- Search runs after ambiguity is computed so partner counts stay global.
	-- strpos avoids treating user % and _ as LIKE wildcards.
	AND ($9::text = '' OR strpos(lower(concat_ws(' ',
		source_code, source_name, partial_code, partial_name, subject_code, subject_name,
		array_to_string(source_teacher_names, ' '), array_to_string(partial_teacher_names, ' ')
	)), lower($9::text)) > 0)
ORDER BY source_id, partial_id
LIMIT $7
`

const CourseLinkSuggestionDetectorVersion = "course-link-suggestions-v1"

type CourseLinkSuggestionRow struct {
	SourceID                   pgtype.UUID
	SourceCode                 string
	SourceName                 string
	SubjectID                  pgtype.UUID
	SubjectCode                string
	SubjectName                string
	SourceLevel                pgtype.Int2
	SourceCycleID              pgtype.Text
	SourceCycleLabel           pgtype.Text
	SourceCycleStart           pgtype.Text
	SourceCycleEnd             pgtype.Text
	SourceRootCourseGroupID    pgtype.UUID
	SourceRootGroupName        pgtype.Text
	SourceSitInRuleID          pgtype.UUID
	SourceSitInRuleName        pgtype.Text
	SourceSitInRuleType        pgtype.Text
	SourceSitInRuleDescription pgtype.Text
	SourceAbsenceFormVisible   bool
	SourceAbsenceFormActive    bool
	SourceSessionCount         int64
	SourceDateFrom             pgtype.Text
	SourceDateTo               pgtype.Text
	SourceSlots                []string
	SourceTeacherNames         []string
	PartialID                  pgtype.UUID
	PartialCode                string
	PartialName                string
	PartialLevel               pgtype.Int2
	PartialCycleID             pgtype.Text
	PartialCycleLabel          pgtype.Text
	PartialRootCourseGroupID   pgtype.UUID
	PartialRootGroupName       pgtype.Text
	PartialSessionCount        int64
	PartialDateFrom            pgtype.Text
	PartialDateTo              pgtype.Text
	PartialSlots               []string
	PartialTeacherNames        []string
	Confidence                 string
	ReasonCodes                []string
	AmbiguityCount             int64
	EvidenceFingerprint        string
}

type CourseLinkSuggestionQuery struct {
	InstituteTZ     string
	IncludeReview   bool
	AfterSourceID   pgtype.UUID
	AfterPartialID  pgtype.UUID
	SourceCourseID  pgtype.UUID
	PartialCourseID pgtype.UUID
	// Search matches course codes/names, subject, and teacher names; empty means all.
	Search string
	Limit  int
}

func (q *Queries) CourseLinkSuggestions(ctx context.Context, arg CourseLinkSuggestionQuery) ([]CourseLinkSuggestionRow, error) {
	if arg.Limit < 1 {
		arg.Limit = 50
	}
	rows, err := q.db.Query(ctx, courseLinkSuggestionsSQL,
		arg.InstituteTZ,
		arg.IncludeReview,
		arg.AfterSourceID,
		arg.AfterPartialID,
		arg.SourceCourseID,
		arg.PartialCourseID,
		arg.Limit,
		CourseLinkSuggestionDetectorVersion,
		arg.Search,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]CourseLinkSuggestionRow, 0)
	for rows.Next() {
		var item CourseLinkSuggestionRow
		if err := rows.Scan(
			&item.SourceID, &item.SourceCode, &item.SourceName, &item.SubjectID,
			&item.SubjectCode, &item.SubjectName, &item.SourceLevel, &item.SourceCycleID,
			&item.SourceCycleLabel, &item.SourceCycleStart, &item.SourceCycleEnd,
			&item.SourceRootCourseGroupID, &item.SourceRootGroupName, &item.SourceSitInRuleID,
			&item.SourceSitInRuleName, &item.SourceSitInRuleType, &item.SourceSitInRuleDescription,
			&item.SourceAbsenceFormVisible, &item.SourceAbsenceFormActive, &item.SourceSessionCount,
			&item.SourceDateFrom, &item.SourceDateTo, &item.SourceSlots, &item.SourceTeacherNames,
			&item.PartialID, &item.PartialCode, &item.PartialName, &item.PartialLevel,
			&item.PartialCycleID, &item.PartialCycleLabel, &item.PartialRootCourseGroupID,
			&item.PartialRootGroupName, &item.PartialSessionCount, &item.PartialDateFrom,
			&item.PartialDateTo, &item.PartialSlots, &item.PartialTeacherNames,
			&item.Confidence, &item.ReasonCodes, &item.AmbiguityCount, &item.EvidenceFingerprint,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (q *Queries) CourseLinkDismissalExists(ctx context.Context, courseA, courseB pgtype.UUID) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM course_link_dismissals
			WHERE course_a = LEAST($1::uuid, $2::uuid)
			  AND course_b = GREATEST($1::uuid, $2::uuid)
		)
	`, courseA, courseB).Scan(&exists)
	return exists, err
}

func (q *Queries) CourseLinkPairLock(ctx context.Context, courseA, courseB pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended(
			LEAST($1::text, $2::text) || ':' || GREATEST($1::text, $2::text), 0
		))
	`, courseA, courseB)
	return err
}

func (q *Queries) CourseLinkDismissalInsert(ctx context.Context, courseA, courseB, actorID pgtype.UUID) (bool, error) {
	var inserted bool
	err := q.db.QueryRow(ctx, `
		INSERT INTO course_link_dismissals (course_a, course_b, actor_user_id)
		VALUES (LEAST($1::uuid, $2::uuid), GREATEST($1::uuid, $2::uuid), $3)
		ON CONFLICT (course_a, course_b) DO NOTHING
		RETURNING true
	`, courseA, courseB, actorID).Scan(&inserted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return inserted, nil
}
