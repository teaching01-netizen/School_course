package sessionchangehttp

import (
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/snapshot"
)

// issueDTOInput groups the fields needed to build an issue DTO,
// avoiding 21 positional parameters in issueDTO.
type issueDTOInput struct {
	ID                        pgtype.UUID
	AbsenceID                 pgtype.UUID
	IssueType                 string
	Severity                  string
	Status                    string
	SourceSessionID           pgtype.UUID
	SitInSessionID            pgtype.UUID
	MissedSessionID           pgtype.UUID
	Details                   []byte
	Suggestions               []byte
	Wcode                     string
	StudentName               pgtype.Text
	StudentEmail              pgtype.Text
	StudentPhone              pgtype.Text
	StartAt                   pgtype.Timestamptz
	EndAt                     pgtype.Timestamptz
	ResolutionAction          pgtype.Text
	IssueVersion              int32
	AssignmentSnapshotJSON    []byte
	AssignmentSnapshotQuality string
	AssignmentSnapshotSource  pgtype.Text
	LatestSessionChangeID     pgtype.UUID
	AssignedAt                pgtype.Timestamptz
}

// ScheduleImpactIssue represents the extended issue response with historical context.
// Deprecated fields (start_at, end_at, room, teacher) are preserved for backward compatibility.
type ScheduleImpactIssue struct {
	ID           string `json:"id"`
	IssueVersion int32  `json:"issue_version"`

	// Deprecated: Represents current session state. New code should use assignment_context.original_session.
	StartAt *string `json:"start_at"`
	EndAt   *string `json:"end_at"`

	// AssignmentContext provides unambiguous historical and current session state.
	AssignmentContext AssignmentContext `json:"assignment_context"`

	// ChangeContext provides the session change that triggered this issue.
	ChangeContext ChangeContext `json:"change_context"`

	// ImpactContext provides the issue classification and reasoning.
	ImpactContext ImpactContext `json:"impact_context"`
}

// AssignmentContext contains when and where the sit-in assignment was made,
// along with the original session state at detection time and the current state.
type AssignmentContext struct {
	AssignedAt      *string             `json:"assigned_at"`
	OriginalSession OriginalSessionView `json:"original_session"`
	CurrentSession  *CurrentSessionView `json:"current_session"`
}

// OriginalSessionView represents the session state at the time of assignment detection.
type OriginalSessionView struct {
	Quality  string                      `json:"quality"` // "exact", "reconstructed", "unavailable"
	Source   string                      `json:"source"`  // e.g. "assignment", "detection"
	Snapshot *snapshot.SessionSnapshotV1 `json:"snapshot"`
}

// CurrentSessionView represents the current state of the session, or null if deleted.
type CurrentSessionView struct {
	Status      string  `json:"status"` // "active", "deleted", "unknown"
	SessionID   string  `json:"session_id"`
	Version     int32   `json:"version"`
	StartAt     string  `json:"start_at"`
	EndAt       string  `json:"end_at"`
	CourseCode  string  `json:"course_code"`
	CourseName  string  `json:"course_name"`
	SubjectName string  `json:"subject_name"`
	RoomName    *string `json:"room_name"`
	TeacherName string  `json:"teacher_name"`
}

// ChangeContext provides the session change that triggered this issue.
type ChangeContext struct {
	ChangeID string                      `json:"change_id"`
	Before   *snapshot.SessionSnapshotV1 `json:"before"`
	After    *snapshot.SessionSnapshotV1 `json:"after"`
}

// ImpactContext provides the issue classification and reasoning.
type ImpactContext struct {
	IssueType string         `json:"issue_type"`
	Severity  string         `json:"severity"`
	Reasons   []ImpactReason `json:"reasons"`
}

// ImpactReason represents a single reason for the impact.
type ImpactReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// IssueDetails is the internal structure stored in details_json.
type IssueDetails struct {
	Reasons          []string `json:"reasons,omitempty"`
	SessionVersion   int32    `json:"session_version,omitempty"`
	NoticeHours      float64  `json:"notice_hours,omitempty"`
	OldStartAt       string   `json:"old_start_at,omitempty"`
	NewStartAt       string   `json:"new_start_at,omitempty"`
	DeletedSessionID string   `json:"deleted_session_id,omitempty"`
}

// DecodeIssueDetails decodes the details_json bytes into IssueDetails.
// Returns empty details on malformed data rather than failing.
func DecodeIssueDetails(data []byte) IssueDetails {
	var details IssueDetails
	if err := json.Unmarshal(data, &details); err != nil {
		return IssueDetails{Reasons: []string{}}
	}
	if details.Reasons == nil {
		details.Reasons = []string{}
	}
	return details
}

// DecodeAssignmentSnapshot decodes the assignment_snapshot_at_detection JSON
// and returns an OriginalSessionView with appropriate quality assessment.
// Handles unknown schema versions and malformed data gracefully.
func DecodeAssignmentSnapshot(data []byte, quality, source string) OriginalSessionView {
	if len(data) == 0 {
		return OriginalSessionView{
			Quality:  "unavailable",
			Source:   source,
			Snapshot: nil,
		}
	}

	// Try to decode as a raw snapshot first to check schema version
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return OriginalSessionView{
			Quality:  "unavailable",
			Source:   source,
			Snapshot: nil,
		}
	}

	// Check schema version - if unknown, return unavailable
	if version, ok := raw["schema_version"].(float64); ok {
		if version != 1 {
			return OriginalSessionView{
				Quality:  "unavailable",
				Source:   source,
				Snapshot: nil,
			}
		}
	}

	// Try to decode as proper snapshot
	snap, err := snapshot.DecodeSessionSnapshotV1(data)
	if err != nil {
		// Malformed legacy record - return with unavailable quality
		return OriginalSessionView{
			Quality:  "unavailable",
			Source:   source,
			Snapshot: nil,
		}
	}

	// Use provided quality if valid, otherwise determine from data
	q := quality
	if q == "" {
		q = "exact"
	}

	return OriginalSessionView{
		Quality:  q,
		Source:   source,
		Snapshot: &snap,
	}
}

// DecodeChangeSnapshot decodes a before/after snapshot from JSON bytes.
// Returns nil for missing or malformed data rather than substituting current state.
func DecodeChangeSnapshot(data []byte) *snapshot.SessionSnapshotV1 {
	if len(data) == 0 {
		return nil
	}

	snap, err := snapshot.DecodeSessionSnapshotV1(data)
	if err != nil {
		// Unknown version or malformed data - return nil, don't substitute
		return nil
	}

	return &snap
}

// ImpactReasonsFromCodes converts reason codes to ImpactReason objects with messages.
func ImpactReasonsFromCodes(codes []string) []ImpactReason {
	reasons := make([]ImpactReason, 0, len(codes))
	for _, code := range codes {
		reasons = append(reasons, ImpactReason{
			Code:    code,
			Message: reasonMessage(code),
		})
	}
	return reasons
}

// reasonMessage returns a human-readable message for a reason code.
func reasonMessage(code string) string {
	switch code {
	case "session_deleted":
		return "The assigned session has been deleted"
	case "session_version_changed":
		return "The session version has changed since assignment"
	case "missed_session_overlap":
		return "The sit-in session overlaps with a missed session"
	case "regular_session_overlap":
		return "The sit-in session overlaps with a regular enrolled session"
	case "past_time":
		return "The session time has moved into the past"
	case "short_notice":
		return "The change was made with short notice"
	default:
		return fmt.Sprintf("Impact reason: %s", code)
	}
}
