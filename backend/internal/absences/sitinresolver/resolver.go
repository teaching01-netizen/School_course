package sitinresolver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type Resolver interface {
	ValidateAssignment(ctx context.Context, absenceID pgtype.UUID, sessionID pgtype.UUID) (ValidationResult, error)
	SuggestReplacements(ctx context.Context, absenceID pgtype.UUID, exclusions []pgtype.UUID, limit int) ([]Candidate, error)
}

type ValidationResult struct {
	Valid                  bool     `json:"valid"`
	Severity               string   `json:"severity"`
	Reasons                []string `json:"reasons"`
	SessionVersion         int32    `json:"session_version"`
	AssignedSessionVersion *int32   `json:"assigned_session_version,omitempty"`
}

type Candidate struct {
	SessionID pgtype.UUID        `json:"session_id"`
	CourseID  pgtype.UUID        `json:"course_id"`
	StartAt   pgtype.Timestamptz `json:"start_at"`
	EndAt     pgtype.Timestamptz `json:"end_at"`
	Occupancy int64              `json:"occupancy"`
}

type Service struct {
	q           *sqldb.Queries
	instituteTZ string
	now         func() time.Time
}

func (s *Service) WithQueries(q *sqldb.Queries) *Service {
	return &Service{q: q, instituteTZ: s.instituteTZ, now: s.now}
}

func (s *Service) ValidateCandidate(ctx context.Context, absenceID, sessionID pgtype.UUID) (ValidationResult, error) {
	var result ValidationResult
	var deletedAt, startAt, endAt pgtype.Timestamptz
	var missedOverlap, normalOverlap bool
	err := s.q.DBQueryRow(ctx, `
		SELECT sess.version, sess.deleted_at, sess.start_at, sess.end_at,
		       EXISTS (
				SELECT 1 FROM absence_missed_sessions ams
				JOIN sessions missed ON missed.id = ams.session_id
				WHERE ams.absence_id = $1 AND missed.deleted_at IS NULL
				  AND sess.start_at < missed.end_at AND sess.end_at > missed.start_at
			   ) AS missed_overlap,
		       EXISTS (
				SELECT 1
				FROM student_absences sa
				JOIN students st ON st.wcode = sa.wcode
				JOIN course_students cs ON cs.student_id = st.id AND cs.status = 'enrolled'
				JOIN sessions normal ON normal.course_id = cs.course_id AND normal.deleted_at IS NULL
				WHERE sa.id = $1 AND normal.id <> sess.id
				  AND sess.start_at < normal.end_at AND sess.end_at > normal.start_at
			   ) AS normal_overlap
		FROM sessions sess
		WHERE sess.id = $2
	`, absenceID, sessionID).Scan(&result.SessionVersion, &deletedAt, &startAt, &endAt, &missedOverlap, &normalOverlap)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			result.Reasons = []string{"session_deleted"}
			return result, nil
		}
		return result, err
	}
	assignedVersion := result.SessionVersion
	result.AssignedSessionVersion = &assignedVersion
	result.Valid = !deletedAt.Valid && !missedOverlap && !normalOverlap && endAt.Time.After(startAt.Time)
	if deletedAt.Valid {
		result.Reasons = append(result.Reasons, "session_deleted")
	}
	if missedOverlap {
		result.Reasons = append(result.Reasons, "missed_session_overlap")
	}
	if normalOverlap {
		result.Reasons = append(result.Reasons, "regular_session_overlap")
	}
	if !endAt.Time.After(startAt.Time) {
		result.Reasons = append(result.Reasons, "invalid_time_range")
	}
	return result, nil
}

func New(q *sqldb.Queries, instituteTZ string) *Service {
	return &Service{q: q, instituteTZ: instituteTZ, now: time.Now}
}

func (s *Service) ValidateAssignment(ctx context.Context, absenceID pgtype.UUID, sessionID pgtype.UUID) (ValidationResult, error) {
	facts, err := s.q.SitInAssignmentFacts(ctx, sqldb.SitInAssignmentFactsParams{AbsenceID: absenceID, SessionID: sessionID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ValidationResult{Valid: false, Severity: "critical", Reasons: []string{"assignment_missing"}}, nil
		}
		return ValidationResult{}, fmt.Errorf("load sit-in assignment: %w", err)
	}

	reasons := make([]string, 0, 5)
	if facts.DeletedAt.Valid {
		reasons = append(reasons, "session_deleted")
	}
	if facts.SessionVersionAtAssignment.Valid && facts.SessionVersionAtAssignment.Int32 != facts.Version {
		reasons = append(reasons, "session_version_changed")
	}
	if facts.MissedOverlap {
		reasons = append(reasons, "missed_session_overlap")
	}
	if facts.NormalOverlap {
		reasons = append(reasons, "regular_session_overlap")
	}
	if facts.SitInOverlap {
		reasons = append(reasons, "sit_in_overlap")
	}
	if facts.StartAt.Valid && !facts.StartAt.Time.After(s.now()) {
		reasons = append(reasons, "past_time")
	}
	severity := "warning"
	if len(reasons) > 0 {
		severity = "critical"
	}
	var assignedVersion *int32
	if facts.SessionVersionAtAssignment.Valid {
		value := facts.SessionVersionAtAssignment.Int32
		assignedVersion = &value
	}
	return ValidationResult{
		Valid:                  len(reasons) == 0,
		Severity:               severity,
		Reasons:                reasons,
		SessionVersion:         facts.Version,
		AssignedSessionVersion: assignedVersion,
	}, nil
}

func (s *Service) SuggestReplacements(ctx context.Context, absenceID pgtype.UUID, exclusions []pgtype.UUID, limit int) ([]Candidate, error) {
	if limit <= 0 {
		return []Candidate{}, nil
	}
	if limit > 50 {
		limit = 50
	}
	absence, err := s.q.ManagedAbsenceGet(ctx, absenceID)
	if err != nil {
		return nil, fmt.Errorf("load absence for sit-in candidates: %w", err)
	}
	courseID := absence.SitInCourseID
	if !courseID.Valid {
		courseID = absence.CourseID
	}
	rows, err := s.q.SitInCandidateValidationBatch(ctx, absenceID, courseID, s.instituteTZ)
	if err != nil {
		return nil, fmt.Errorf("load sit-in candidates: %w", err)
	}
	excluded := make(map[pgtype.UUID]struct{}, len(exclusions))
	for _, id := range exclusions {
		excluded[id] = struct{}{}
	}
	result := make([]Candidate, 0, limit)
	for _, row := range rows {
		if _, ok := excluded[row.ID]; ok {
			continue
		}
		validation := candidateRowToValidation(row, s.now())
		if !validation.Valid {
			continue
		}
		result = append(result, Candidate{SessionID: row.ID, CourseID: row.CourseID, StartAt: row.StartAt, EndAt: row.EndAt, Occupancy: row.Occupancy})
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func (s *Service) validateCandidate(ctx context.Context, absenceID, sessionID pgtype.UUID) (ValidationResult, error) {
	facts, err := s.q.SitInAssignmentFacts(ctx, sqldb.SitInAssignmentFactsParams{AbsenceID: absenceID, SessionID: sessionID})
	if err == nil {
		return factsToValidation(facts, s.now()), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ValidationResult{}, fmt.Errorf("validate sit-in candidate: %w", err)
	}
	return s.ValidateCandidate(ctx, absenceID, sessionID)
}

func candidateRowToValidation(row sqldb.SitInCandidateValidationRow, now time.Time) ValidationResult {
	reasons := make([]string, 0, 4)
	if row.DeletedAt.Valid {
		reasons = append(reasons, "session_deleted")
	}
	if row.MissedOverlap {
		reasons = append(reasons, "missed_session_overlap")
	}
	if row.NormalOverlap || row.SitInOverlap {
		reasons = append(reasons, "session_overlap")
	}
	if row.StartAt.Valid && !row.StartAt.Time.After(now) {
		reasons = append(reasons, "past_time")
	}
	if row.StartAt.Valid && row.EndAt.Valid && !row.EndAt.Time.After(row.StartAt.Time) {
		reasons = append(reasons, "invalid_time_range")
	}
	return ValidationResult{Valid: len(reasons) == 0, Severity: severityForReasons(reasons), Reasons: reasons, SessionVersion: row.Version}
}

func factsToValidation(facts sqldb.SitInAssignmentFactsRow, now time.Time) ValidationResult {
	reasons := make([]string, 0, 4)
	if facts.DeletedAt.Valid {
		reasons = append(reasons, "session_deleted")
	}
	if facts.MissedOverlap {
		reasons = append(reasons, "missed_session_overlap")
	}
	if facts.NormalOverlap || facts.SitInOverlap {
		reasons = append(reasons, "session_overlap")
	}
	if facts.StartAt.Valid && !facts.StartAt.Time.After(now) {
		reasons = append(reasons, "past_time")
	}
	return ValidationResult{Valid: len(reasons) == 0, Severity: severityForReasons(reasons), Reasons: reasons, SessionVersion: facts.Version}
}

func severityForReasons(reasons []string) string {
	if len(reasons) == 0 {
		return "warning"
	}
	return "critical"
}
