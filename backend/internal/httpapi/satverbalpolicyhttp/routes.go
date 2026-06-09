package satverbalpolicyhttp

import (
	"context"
	"encoding/json"
	"errors"
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

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("GET /api/v1/admin/sat-verbal-policy/mapping", s.handleGet)
	mux.HandleFunc("POST /api/v1/admin/sat-verbal-policy/apply", s.handleApply)
	mux.HandleFunc("DELETE /api/v1/admin/sat-verbal-policy/mapping/{subject_id}", s.handleDelete)
}

func (s *server) handleGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	subjectIDRaw := strings.TrimSpace(r.URL.Query().Get("subject_id"))
	if subjectIDRaw == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "missing_subject_id", "subject_id is required")
		return
	}
	subjectID, err := s.a.ParseUUID(subjectIDRaw)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_id", "Invalid subject_id")
		return
	}
	mapping, err := s.deps.Q.SatVerbalPolicyMappingGetBySubject(r.Context(), subjectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.a.WriteJSON(w, http.StatusOK, map[string]any{
				"active":                false,
				"subject_id":            subjectIDRaw,
				"warnings":              []string{},
				"matched_courses":       []any{},
				"unmatched_policy_rows": []string{},
				"unmatched_courses":     []string{},
			})
			return
		}
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.writeMapping(w, mapping)
}

func (s *server) handleApply(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		SubjectID string          `json:"subject_id"`
		Policy    json.RawMessage `json:"policy"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	subjectID, err := s.a.ParseUUID(strings.TrimSpace(body.SubjectID))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_id", "Invalid subject_id")
		return
	}
	rules, err := satverbalpolicy.DecodeRules(body.Policy)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_policy", "Invalid SAT Verbal policy")
		return
	}

	s.a.WithIdempotentTx(w, r, user.ID, "sat-verbal-policy", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.AdvisoryLockForText(r.Context(), "sat-verbal-policy:"+body.SubjectID); err != nil {
			return 0, nil, err
		}
		subject, err := qtx.SubjectGetByID(r.Context(), subjectID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		courses, err := qtx.CoursesBySubject(r.Context(), subjectID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		report := satverbalpolicy.BuildApplyReport(rules, courses, subject.Code)
		if err := s.applyRootGroups(r.Context(), qtx, report); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}

		warnings := mustJSON(report.Warnings)
		matched := mustJSON(report.MatchedCourses)
		unmatchedRows := mustJSON(report.UnmatchedPolicyRows)
		unmatchedCourses := mustJSON(report.UnmatchedCourses)
		mapping, err := qtx.SatVerbalPolicyMappingUpsert(r.Context(), sqldb.SatVerbalPolicyMappingUpsertParams{
			SubjectID:           subjectID,
			Policy:              body.Policy,
			PolicyHash:          satverbalpolicy.HashPolicy(body.Policy),
			Warnings:            warnings,
			MatchedCourses:      matched,
			UnmatchedPolicyRows: unmatchedRows,
			UnmatchedCourses:    unmatchedCourses,
		})
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		return http.StatusOK, mappingResponse(s.a, mapping), nil
	})
}

func (s *server) handleDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	subjectID, err := s.a.ParseUUID(r.PathValue("subject_id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_id", "Invalid subject_id")
		return
	}
	s.a.WithIdempotentTx(w, r, user.ID, "sat-verbal-policy", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.SatVerbalPolicyMappingDelete(r.Context(), subjectID); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{"ok": true, "active": false}, nil
	})
}

func (s *server) applyRootGroups(ctx context.Context, q *sqldb.Queries, report satverbalpolicy.ApplyReport) error {
	groupIDs := make(map[string]pgtype.UUID)
	for _, match := range report.MatchedCourses {
		if _, ok := groupIDs[match.RootGroupName]; !ok {
			id, exists, err := q.RootCourseGroupFindByName(ctx, match.RootGroupName)
			if err != nil {
				return err
			}
			if !exists {
				id, _, _, err = q.RootCourseGroupCreate(ctx, match.RootGroupName, pgtype.UUID{})
				if err != nil {
					return err
				}
			}
			groupIDs[match.RootGroupName] = id
		}
	}
	for _, match := range report.MatchedCourses {
		courseID, err := s.a.ParseUUID(match.CourseID)
		if err != nil {
			return err
		}
		if err := q.CourseUpdateRootCourseGroup(ctx, courseID, groupIDs[match.RootGroupName]); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) writeMapping(w http.ResponseWriter, mapping *sqldb.SatVerbalPolicyMapping) {
	s.a.WriteJSON(w, http.StatusOK, mappingResponse(s.a, mapping))
}

func mappingResponse(a httpadapter.Adapter, mapping *sqldb.SatVerbalPolicyMapping) map[string]any {
	subjectID, _ := a.UUIDString(mapping.SubjectID)
	return map[string]any{
		"active":                mapping.Active,
		"subject_id":            subjectID,
		"policy_hash":           mapping.PolicyHash,
		"policy":                json.RawMessage(mapping.Policy),
		"warnings":              json.RawMessage(mapping.Warnings),
		"matched_courses":       json.RawMessage(mapping.MatchedCourses),
		"unmatched_policy_rows": json.RawMessage(mapping.UnmatchedPolicyRows),
		"unmatched_courses":     json.RawMessage(mapping.UnmatchedCourses),
		"created_at":            mapping.CreatedAt,
		"updated_at":            mapping.UpdatedAt,
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("[]")
	}
	return b
}
