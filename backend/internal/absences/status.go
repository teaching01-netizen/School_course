package absences

import "strings"

// Status represents a student absence status.
// This is the single source of truth for valid status values.
// Keep in sync with: db/migrations/*_status_check, frontend types.ts
type Status string

const (
	StatusPending         Status = "pending"
	StatusReviewed        Status = "reviewed"
	StatusActioned        Status = "actioned"
	StatusCancelled       Status = "cancelled"
	StatusSpecialApproved Status = "special_approved"
)

// AllStatuses returns all valid absence statuses.
func AllStatuses() []Status {
	return []Status{
		StatusPending,
		StatusReviewed,
		StatusActioned,
		StatusCancelled,
		StatusSpecialApproved,
	}
}

// ValidStatus reports whether s is a recognized absence status.
func ValidStatus(s string) bool {
	switch Status(s) {
	case StatusPending, StatusReviewed, StatusActioned, StatusCancelled, StatusSpecialApproved:
		return true
	default:
		return false
	}
}

// ValidTransition reports whether a status change from → to is allowed.
func ValidTransition(from, to Status) bool {
	switch from {
	case StatusPending:
		return to == StatusReviewed || to == StatusCancelled || to == StatusSpecialApproved
	case StatusReviewed:
		return to == StatusActioned || to == StatusPending || to == StatusCancelled || to == StatusSpecialApproved
	case StatusActioned:
		return to == StatusReviewed || to == StatusCancelled || to == StatusSpecialApproved
	case StatusSpecialApproved:
		return to == StatusCancelled
	default:
		return false
	}
}

// StatusAuditAction returns the audit log action name for a status transition.
func StatusAuditAction(current, to Status) string {
	if current == StatusReviewed && to == StatusPending {
		return "reopened"
	}
	return string(to)
}

// StudentCancellable reports whether a student may cancel their own absence in
// status s. Staff workflows may cancel from more statuses; self-service is
// limited to pending and reviewed.
func StudentCancellable(s Status) bool {
	return s == StatusPending || s == StatusReviewed
}

// StatusesForSQL is deprecated: use parameterized ANY($n::text[]) with a []string
// derived from Status constants (e.g. AllStatuses) instead of string-interpolating
// an IN list. Kept for backwards compatibility only.
func StatusesForSQL(statuses ...Status) string {
	parts := make([]string, len(statuses))
	for i, s := range statuses {
		parts[i] = "'" + string(s) + "'"
	}
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p)
	}
	return b.String()
}
