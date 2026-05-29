package scheduling

import (
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/series"
)

// LocalDate and Clock are type aliases for the canonical types owned by series.
// scheduling callers should only depend on scheduling.* names.
type LocalDate = series.LocalDate
type Clock = series.Clock

type FindAvailableSlotsParams struct {
	StudentID        pgtype.UUID
	CourseID         pgtype.UUID
	StartDate        LocalDate
	EndDate          LocalDate
	SlotDurationMins int
	DayStartHour     int
	DayEndHour       int
}

type AvailableSlot struct {
	Date      string            `json:"date"`
	StartTime string            `json:"start_time"`
	EndTime   string            `json:"end_time"`
	Status    string            `json:"status"`
	Kind      *ConflictKind     `json:"kind,omitempty"`
	Message   string            `json:"message,omitempty"`
	Conflicts []ConflictSession `json:"conflicts,omitempty"`
}

type FindAvailableSlotsResult struct {
	Slots []AvailableSlot `json:"slots"`
}
