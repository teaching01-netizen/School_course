package courselevelshttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
)

const (
	suggestionDefaultLimit = 50
	suggestionMaxLimit     = 100
	suggestionQueryTimeout = time.Second
)

type suggestionCourseDTO struct {
	ID                   string   `json:"id"`
	Code                 string   `json:"code"`
	Name                 string   `json:"name"`
	SubjectID            string   `json:"subject_id"`
	SubjectCode          string   `json:"subject_code"`
	SubjectName          string   `json:"subject_name"`
	Level                *int16   `json:"level"`
	CycleID              *string  `json:"cycle_id"`
	CycleLabel           *string  `json:"cycle_label"`
	CycleStartDate       *string  `json:"cycle_start_date"`
	CycleEndDate         *string  `json:"cycle_end_date"`
	RootCourseGroupID    *string  `json:"root_course_group_id"`
	RootCourseGroup      *string  `json:"root_course_group_name"`
	SitInRuleID          *string  `json:"sit_in_rule_id"`
	SitInRuleName        *string  `json:"sit_in_rule_name"`
	SitInRuleType        *string  `json:"sit_in_rule_type"`
	SitInRuleDescription *string  `json:"sit_in_rule_description"`
	AbsenceFormVisible   *bool    `json:"absence_form_visible"`
	AbsenceFormActive    *bool    `json:"absence_form_active"`
	SessionCount         int64    `json:"session_count"`
	SessionDateFrom      *string  `json:"session_date_from"`
	SessionDateTo        *string  `json:"session_date_to"`
	Slots                []string `json:"slots"`
	Teachers             []string `json:"teachers"`
}

type courseLinkSuggestionDTO struct {
	ConfiguredSource suggestionCourseDTO `json:"configured_source"`
	Unconfigured     suggestionCourseDTO `json:"unconfigured_course"`
	Confidence       string              `json:"confidence"`
	ReasonCodes      []string            `json:"reason_codes"`
	AmbiguityCount   int64               `json:"ambiguity_count"`
	Fingerprint      string              `json:"evidence_fingerprint"`
}

func (s *server) handleCourseLinkSuggestions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	mode := courseLinkSuggestionMode(s.deps.CourseLinkSuggestionsMode)
	if mode == "disabled" {
		s.a.WriteJSON(w, http.StatusOK, map[string]any{
			"enabled": false, "mode": mode, "items": []courseLinkSuggestionDTO{},
			"has_more": false, "next_cursor": nil,
			"evaluated_at":       time.Now().UTC().Format(time.RFC3339Nano),
			"detector_version":   sqldb.CourseLinkSuggestionDetectorVersion,
			"institute_timezone": s.instituteTimezone(),
		})
		return
	}

	limit, err := parseSuggestionLimit(r.URL.Query().Get("limit"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_pagination", err.Error())
		return
	}
	includeReview, err := parseSuggestionBool(r.URL.Query().Get("include_review"), false)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_filter", "include_review must be true or false")
		return
	}
	afterSource, afterPartial, err := s.parseSuggestionCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_cursor", "Invalid suggestion cursor")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), suggestionQueryTimeout)
	defer cancel()
	rows, err := s.deps.Q.CourseLinkSuggestions(ctx, sqldb.CourseLinkSuggestionQuery{
		InstituteTZ:    s.instituteTimezone(),
		IncludeReview:  includeReview,
		AfterSourceID:  afterSource,
		AfterPartialID: afterPartial,
		Search:         s.a.SearchQuery(r.URL.Query().Get("q")),
		Limit:          limit + 1,
	})
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]courseLinkSuggestionDTO, 0, len(rows))
	for _, row := range rows {
		item, err := s.courseLinkSuggestionDTO(row)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		items = append(items, item)
	}
	var nextCursor *string
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		sourceID, _ := s.a.UUIDString(last.SourceID)
		partialID, _ := s.a.UUIDString(last.PartialID)
		cursor := encodeSuggestionCursor(sourceID, partialID)
		nextCursor = &cursor
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled":              true,
		"mode":                 mode,
		"confirmation_enabled": mode == "confirmation",
		"items":                items,
		"has_more":             hasMore,
		"next_cursor":          nextCursor,
		"limit":                limit,
		"evaluated_at":         time.Now().UTC().Format(time.RFC3339Nano),
		"detector_version":     sqldb.CourseLinkSuggestionDetectorVersion,
		"institute_timezone":   s.instituteTimezone(),
	})
}

func (s *server) courseLinkSuggestionDTO(row sqldb.CourseLinkSuggestionRow) (courseLinkSuggestionDTO, error) {
	source, err := s.a.UUIDString(row.SourceID)
	if err != nil {
		return courseLinkSuggestionDTO{}, err
	}
	partial, err := s.a.UUIDString(row.PartialID)
	if err != nil {
		return courseLinkSuggestionDTO{}, err
	}
	subjectID, err := s.a.UUIDString(row.SubjectID)
	if err != nil {
		return courseLinkSuggestionDTO{}, err
	}
	sourceRoot, err := s.optionalUUID(row.SourceRootCourseGroupID)
	if err != nil {
		return courseLinkSuggestionDTO{}, err
	}
	partialRoot, err := s.optionalUUID(row.PartialRootCourseGroupID)
	if err != nil {
		return courseLinkSuggestionDTO{}, err
	}
	sitInRuleID, err := s.optionalUUID(row.SourceSitInRuleID)
	if err != nil {
		return courseLinkSuggestionDTO{}, err
	}
	return courseLinkSuggestionDTO{
		ConfiguredSource: suggestionCourseDTO{
			ID: source, Code: row.SourceCode, Name: row.SourceName, SubjectID: subjectID,
			SubjectCode: row.SubjectCode, SubjectName: row.SubjectName,
			Level: optionalInt16(row.SourceLevel), CycleID: optionalText(row.SourceCycleID),
			CycleLabel: optionalText(row.SourceCycleLabel), CycleStartDate: optionalText(row.SourceCycleStart),
			CycleEndDate: optionalText(row.SourceCycleEnd), RootCourseGroupID: sourceRoot,
			RootCourseGroup: optionalText(row.SourceRootGroupName), SitInRuleID: sitInRuleID,
			SitInRuleName: optionalText(row.SourceSitInRuleName), SitInRuleType: optionalText(row.SourceSitInRuleType),
			SitInRuleDescription: optionalText(row.SourceSitInRuleDescription),
			AbsenceFormVisible:   optionalBool(row.SourceAbsenceFormVisible), AbsenceFormActive: optionalBool(row.SourceAbsenceFormActive),
			SessionCount:    row.SourceSessionCount,
			SessionDateFrom: optionalText(row.SourceDateFrom), SessionDateTo: optionalText(row.SourceDateTo),
			Slots: nonNilStrings(row.SourceSlots), Teachers: nonNilStrings(row.SourceTeacherNames),
		},
		Unconfigured: suggestionCourseDTO{
			ID: partial, Code: row.PartialCode, Name: row.PartialName, SubjectID: subjectID,
			SubjectCode: row.SubjectCode, SubjectName: row.SubjectName,
			Level: optionalInt16(row.PartialLevel), CycleID: optionalText(row.PartialCycleID),
			CycleLabel: optionalText(row.PartialCycleLabel), RootCourseGroupID: partialRoot,
			RootCourseGroup: optionalText(row.PartialRootGroupName), SessionCount: row.PartialSessionCount,
			SessionDateFrom: optionalText(row.PartialDateFrom), SessionDateTo: optionalText(row.PartialDateTo),
			Slots: nonNilStrings(row.PartialSlots), Teachers: nonNilStrings(row.PartialTeacherNames),
		},
		Confidence: row.Confidence, ReasonCodes: nonNilStrings(row.ReasonCodes),
		AmbiguityCount: row.AmbiguityCount, Fingerprint: row.EvidenceFingerprint,
	}, nil
}

func (s *server) optionalUUID(id pgtype.UUID) (*string, error) {
	if !id.Valid {
		return nil, nil
	}
	value, err := s.a.UUIDString(id)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func optionalText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func optionalInt16(value pgtype.Int2) *int16 {
	if !value.Valid {
		return nil
	}
	n := value.Int16
	return &n
}

func optionalBool(value bool) *bool {
	return &value
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (s *server) instituteTimezone() string {
	if strings.TrimSpace(s.deps.InstituteTZ) == "" {
		return "Asia/Bangkok"
	}
	return s.deps.InstituteTZ
}

func courseLinkSuggestionMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "disabled":
		return "disabled"
	case "confirmation":
		return "confirmation"
	default:
		return "discovery"
	}
}

func parseSuggestionLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return suggestionDefaultLimit, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > suggestionMaxLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", suggestionMaxLimit)
	}
	return value, nil
}

func parseSuggestionBool(raw string, fallback bool) (bool, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, err
	}
	return value, nil
}

func encodeSuggestionCursor(sourceID, partialID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(sourceID + "." + partialID))
}

func (s *server) parseSuggestionCursor(raw string) (pgtype.UUID, pgtype.UUID, error) {
	if raw == "" {
		return pgtype.UUID{}, pgtype.UUID{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	parts := strings.Split(string(decoded), ".")
	if len(parts) != 2 {
		return pgtype.UUID{}, pgtype.UUID{}, errors.New("invalid cursor shape")
	}
	source, err := s.a.ParseUUID(parts[0])
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	partial, err := s.a.ParseUUID(parts[1])
	return source, partial, err
}

type dismissSuggestionRequest struct {
	ConfiguredCourseID   string `json:"configured_course_id"`
	UnconfiguredCourseID string `json:"unconfigured_course_id"`
	EvidenceFingerprint  string `json:"evidence_fingerprint"`
	DetectorVersion      string `json:"detector_version"`
}

func (s *server) handleCourseLinkSuggestionDismiss(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	if courseLinkSuggestionMode(s.deps.CourseLinkSuggestionsMode) != "confirmation" {
		s.a.WriteErr(w, http.StatusConflict, "suggestions_read_only", "Course link suggestions are currently read only.")
		return
	}
	var body dismissSuggestionRequest
	if err := decodeSuggestionRequest(r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid dismissal request")
		return
	}
	sourceID, err := s.a.ParseUUID(body.ConfiguredCourseID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid configured course ID")
		return
	}
	partialID, err := s.a.ParseUUID(body.UnconfiguredCourseID)
	if err != nil || sourceID == partialID {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid unconfigured course ID")
		return
	}
	if body.DetectorVersion != sqldb.CourseLinkSuggestionDetectorVersion || len(body.EvidenceFingerprint) != 64 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_evidence", "The suggestion evidence is invalid")
		return
	}
	if decoded, err := hex.DecodeString(body.EvidenceFingerprint); err != nil || len(decoded) != 32 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_evidence", "The suggestion evidence is invalid")
		return
	}

	actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
	s.a.WithIdempotentTx(w, r, user.ID, "course-link-suggestions", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.CourseLinkPairLock(r.Context(), sourceID, partialID); err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		locked, err := qtx.CourseMergeGroupLockCourses(r.Context(), []pgtype.UUID{sourceID, partialID})
		if err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		if len(locked) != 2 {
			return suggestionChanged(w, s, "One or both courses changed. Refresh the suggestions and try again.")
		}
		dismissed, err := qtx.CourseLinkDismissalExists(r.Context(), sourceID, partialID)
		if err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		if dismissed {
			return http.StatusOK, map[string]any{"dismissed": true, "already_dismissed": true}, nil
		}
		members, err := qtx.CourseMergeGroupMembershipsForCourses(r.Context(), []pgtype.UUID{sourceID, partialID})
		if err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		if len(members) > 0 {
			return suggestionChanged(w, s, "This pair is already linked. Refresh the suggestions.")
		}
		current, err := s.currentSuggestion(r.Context(), qtx, sourceID, partialID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return suggestionChanged(w, s, "This suggestion changed. Refresh it before deciding.")
			}
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		if current.EvidenceFingerprint != body.EvidenceFingerprint {
			return suggestionChanged(w, s, "This suggestion changed. Refresh it before deciding.")
		}
		inserted, err := qtx.CourseLinkDismissalInsert(r.Context(), sourceID, partialID, actorID)
		if err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		if !inserted {
			return http.StatusOK, map[string]any{"dismissed": true, "already_dismissed": true}, nil
		}
		courseA, courseB := sourceID, partialID
		if bytes.Compare(courseA.Bytes[:], courseB.Bytes[:]) > 0 {
			courseA, courseB = courseB, courseA
		}
		if _, err := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: actorID,
			Action:      "course_link_suggestion.dismissed",
			Payload: map[string]any{
				"course_ids":           []string{courseA.String(), courseB.String()},
				"detector_version":     sqldb.CourseLinkSuggestionDetectorVersion,
				"evidence_fingerprint": current.EvidenceFingerprint,
				"confidence":           current.Confidence,
				"reason_codes":         current.ReasonCodes,
			},
		}); err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{"dismissed": true, "already_dismissed": false}, nil
	})
}

func (s *server) currentSuggestion(ctx context.Context, q *sqldb.Queries, sourceID, partialID pgtype.UUID) (sqldb.CourseLinkSuggestionRow, error) {
	queryCtx, cancel := context.WithTimeout(ctx, suggestionQueryTimeout)
	defer cancel()
	items, err := q.CourseLinkSuggestions(queryCtx, sqldb.CourseLinkSuggestionQuery{
		InstituteTZ: s.instituteTimezone(), IncludeReview: true,
		SourceCourseID: sourceID, PartialCourseID: partialID, Limit: 1,
	})
	if err != nil {
		return sqldb.CourseLinkSuggestionRow{}, err
	}
	if len(items) != 1 {
		return sqldb.CourseLinkSuggestionRow{}, pgx.ErrNoRows
	}
	return items[0], nil
}

func suggestionChanged(w http.ResponseWriter, s *server, message string) (int, any, error) {
	s.a.WriteErr(w, http.StatusConflict, "suggestion_changed", message)
	return 0, nil, errors.New("course link suggestion changed")
}

func decodeSuggestionRequest(r *http.Request, value any) error {
	body, err := httpadapter.ReadBodyWithLimit(r, httpadapter.MaxJSONBodyBytes)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}
