package satverbalpolicyhttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/satverbalpolicy"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

type courseRuleMappingRequest struct {
	RuleID   string `json:"rule_id"`
	CourseID string `json:"course_id"`
}

type mappingReport struct {
	Warnings            []string            `json:"warnings"`
	MatchedCourses      []map[string]string `json:"matched_courses"`
	UnmatchedPolicyRows []string            `json:"unmatched_policy_rows"`
	Mappings            []map[string]any    `json:"mappings"`
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("GET /api/v1/admin/sat-verbal-policy/mapping", s.handleGet)
	mux.HandleFunc("POST /api/v1/admin/sat-verbal-policy/apply", s.handleApply)
	mux.HandleFunc("DELETE /api/v1/admin/sat-verbal-policy/mapping/{rule_id}", s.handleDelete)
}

func (s *server) handleGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	mappings, err := s.deps.Q.SatVerbalPolicyMappingsList(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, mappingListResponse(s.a, mappings))
}

func (s *server) handleApply(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Policy   json.RawMessage            `json:"policy"`
		Mappings []courseRuleMappingRequest `json:"mappings"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	rules, err := satverbalpolicy.DecodeRules(body.Policy)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_policy", "Invalid SAT Verbal policy")
		return
	}
	rulesByID := make(map[string]satverbalpolicy.CourseRule, len(rules))
	for _, rule := range rules {
		rulesByID[rule.ID] = rule
	}

	s.a.WithIdempotentTx(w, r, user.ID, "sat-verbal-policy", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.AdvisoryLockForText(r.Context(), "sat-verbal-policy:course-rules"); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		params, report, err := s.buildReplaceParams(r.Context(), qtx, rules, rulesByID, body.Mappings)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		if _, err := qtx.SatVerbalPolicyMappingsReplace(r.Context(), params); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		mappings, err := qtx.SatVerbalPolicyMappingsList(r.Context())
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		response := mappingListResponse(s.a, mappings)
		response["warnings"] = report.Warnings
		response["matched_courses"] = report.MatchedCourses
		response["unmatched_policy_rows"] = report.UnmatchedPolicyRows
		return http.StatusOK, response, nil
	})
}

func (s *server) buildReplaceParams(
	ctx context.Context,
	q *sqldb.Queries,
	rules []satverbalpolicy.CourseRule,
	rulesByID map[string]satverbalpolicy.CourseRule,
	requested []courseRuleMappingRequest,
) ([]sqldb.SatVerbalPolicyMappingReplaceParam, mappingReport, error) {
	var report mappingReport
	seenRules := make(map[string]struct{}, len(requested))
	seenCourses := make(map[pgtype.UUID]struct{}, len(requested))
	params := make([]sqldb.SatVerbalPolicyMappingReplaceParam, 0, len(requested))

	for _, item := range requested {
		ruleID := strings.TrimSpace(item.RuleID)
		courseIDRaw := strings.TrimSpace(item.CourseID)
		if ruleID == "" || courseIDRaw == "" {
			continue
		}
		rule, ok := rulesByID[ruleID]
		if !ok {
			return nil, report, fmt.Errorf("unknown SAT Verbal policy rule %q", ruleID)
		}
		if _, ok := seenRules[ruleID]; ok {
			return nil, report, fmt.Errorf("duplicate SAT Verbal policy rule mapping %q", ruleID)
		}
		courseID, err := s.a.ParseUUID(courseIDRaw)
		if err != nil {
			return nil, report, fmt.Errorf("invalid course_id for SAT Verbal policy rule %q", ruleID)
		}
		if _, ok := seenCourses[courseID]; ok {
			return nil, report, fmt.Errorf("same course is mapped to more than one SAT Verbal policy rule")
		}
		course, err := q.CourseSubjectByID(ctx, courseID)
		if err != nil {
			return nil, report, err
		}
		if err := ensureRootGroup(ctx, q, rule, course); err != nil {
			return nil, report, err
		}
		ruleRaw, err := json.Marshal(rule)
		if err != nil {
			return nil, report, err
		}
		seenRules[ruleID] = struct{}{}
		seenCourses[courseID] = struct{}{}
		params = append(params, sqldb.SatVerbalPolicyMappingReplaceParam{
			RuleID:     ruleID,
			CourseID:   courseID,
			PolicyRule: ruleRaw,
			PolicyHash: hashRule(ruleRaw),
		})
		courseIDStr, _ := s.a.UUIDString(course.ID)
		report.MatchedCourses = append(report.MatchedCourses, map[string]string{
			"policy_rule_id":     rule.ID,
			"policy_course_name": rule.CourseName,
			"course_id":          courseIDStr,
			"course_code":        course.Code,
			"course_name":        course.Name,
			"root_group_name":    satverbalpolicy.RootGroupName(course.SubjectCode, satverbalpolicy.RootGroupKey(rule.CourseName)),
		})
	}

	for _, rule := range rules {
		if _, ok := seenRules[rule.ID]; ok {
			continue
		}
		report.UnmatchedPolicyRows = append(report.UnmatchedPolicyRows, rule.CourseName)
		report.Warnings = append(report.Warnings, "No course selected for "+rule.CourseName)
	}
	return params, report, nil
}

func ensureRootGroup(ctx context.Context, q *sqldb.Queries, rule satverbalpolicy.CourseRule, course sqldb.SubjectCourseV2) error {
	rootName := satverbalpolicy.RootGroupName(course.SubjectCode, satverbalpolicy.RootGroupKey(rule.CourseName))
	rootID, exists, err := q.RootCourseGroupFindByName(ctx, rootName)
	if err != nil {
		return err
	}
	if !exists {
		rootID, _, _, err = q.RootCourseGroupCreate(ctx, rootName, pgtype.UUID{})
		if err != nil {
			return err
		}
	}
	return q.CourseUpdateRootCourseGroup(ctx, course.ID, rootID)
}

func (s *server) handleDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	ruleID := strings.TrimSpace(r.PathValue("rule_id"))
	if ruleID == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_rule_id", "Invalid rule_id")
		return
	}
	s.a.WithIdempotentTx(w, r, user.ID, "sat-verbal-policy", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.SatVerbalPolicyMappingDeleteByRule(r.Context(), ruleID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return http.StatusOK, map[string]any{"ok": true, "active": false}, nil
			}
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{"ok": true, "active": false}, nil
	})
}

func mappingListResponse(a httpadapter.Adapter, mappings []sqldb.SatVerbalPolicyCourseMapping) map[string]any {
	out := make([]map[string]any, 0, len(mappings))
	for _, mapping := range mappings {
		courseID, _ := a.UUIDString(mapping.CourseID)
		subjectID, _ := a.UUIDString(mapping.SubjectID)
		out = append(out, map[string]any{
			"active":       mapping.Active,
			"rule_id":      mapping.RuleID,
			"course_id":    courseID,
			"course_code":  mapping.CourseCode,
			"course_name":  mapping.CourseName,
			"subject_id":   subjectID,
			"subject_code": mapping.SubjectCode,
			"subject_name": mapping.SubjectName,
			"policy_hash":  mapping.PolicyHash,
			"policy_rule":  json.RawMessage(mapping.PolicyRule),
			"created_at":   mapping.CreatedAt,
			"updated_at":   mapping.UpdatedAt,
		})
	}
	return map[string]any{
		"active":                len(out) > 0,
		"mappings":              out,
		"warnings":              []string{},
		"matched_courses":       []map[string]string{},
		"unmatched_policy_rows": []string{},
	}
}

func hashRule(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
