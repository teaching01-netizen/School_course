package sessionchangeimpact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences/sitinresolver"
	sqldb "warwick-institute/internal/db"
)

type issueDetails struct {
	Reasons          []string `json:"reasons,omitempty"`
	SessionVersion   int32    `json:"session_version,omitempty"`
	NoticeHours      float64  `json:"notice_hours,omitempty"`
	OldStartAt       string   `json:"old_start_at,omitempty"`
	NewStartAt       string   `json:"new_start_at,omitempty"`
	DeletedSessionID string   `json:"deleted_session_id,omitempty"`
}

type issueInput struct {
	item           sqldb.SessionChangeAffectedAbsencesRow
	issueType      string
	severity       string
	reasons        []string
	fingerprint    string
	deletionTarget bool
}

func (run analysisRun) upsertIssue(ctx context.Context, input issueInput) error {
	details := issueDetails{Reasons: input.reasons, SessionVersion: input.item.AffectedSessionVersion.Int32, OldStartAt: run.change.OldStartAt.Time.UTC().Format(time.RFC3339Nano), NewStartAt: run.change.NewStartAt.Time.UTC().Format(time.RFC3339Nano)}
	if input.deletionTarget {
		details.DeletedSessionID = uuidString(run.change.SessionID)
	}
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("marshal issue details: %w", err)
	}
	suggestions := []sitinresolver.Candidate{}
	if len(input.reasons) > 0 && !input.deletionTarget {
		suggestions, err = run.resolver.SuggestReplacements(ctx, input.item.ID, []pgtype.UUID{input.item.SitInSessionID}, 3)
		if err != nil {
			return err
		}
	}
	suggestedJSON, err := json.Marshal(suggestions)
	if err != nil {
		return fmt.Errorf("marshal issue suggestions: %w", err)
	}
	sourceSessionID := run.change.SessionID
	sitInSessionID := input.item.SitInSessionID
	missedSessionID := input.item.MissedSessionID
	if input.deletionTarget {
		sourceSessionID = pgtype.UUID{}
		sitInSessionID = pgtype.UUID{}
		missedSessionID = pgtype.UUID{}
	}
	if err := run.q.AbsenceScheduleIssueUpsert(ctx, sqldb.AbsenceScheduleIssueUpsertParams{
		AbsenceID: input.item.ID, IssueType: input.issueType, Severity: input.severity,
		SourceSessionID: sourceSessionID, SitInSessionID: sitInSessionID,
		MissedSessionID: missedSessionID, SessionChangeID: run.change.ID,
		DetailsJson: string(detailsJSON), SuggestedResolutionJson: string(suggestedJSON), Fingerprint: input.fingerprint,
	}); err != nil {
		return fmt.Errorf("upsert schedule issue: %w", err)
	}
	return nil
}

func issueTypeForReason(reasons []string) string {
	if len(reasons) == 0 {
		return "sit_in_session_changed"
	}
	switch reasons[0] {
	case "missed_session_overlap":
		return "sit_in_overlap"
	case "regular_session_overlap":
		return "regular_session_overlap"
	case "session_version_changed":
		return "sit_in_ineligible"
	case "past_time":
		return "past_time_change"
	default:
		return "sit_in_ineligible"
	}
}

func issueFingerprint(absenceID pgtype.UUID, issueType string, source, sitIn, missed pgtype.UUID) string {
	data := []byte(fmt.Sprintf("%s|%s|%s|%s|%s", uuidString(absenceID), issueType, uuidString(source), uuidString(sitIn), uuidString(missed)))
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return value.String()
}
