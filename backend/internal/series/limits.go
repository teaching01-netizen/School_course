package series

import "fmt"

const (
	MaxOccurrences     = 1000
	MaxHorizonYears    = 5
	MaxDurationMinutes = 1440
)

type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func newValidationError(code, format string, args ...any) error {
	return &ValidationError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func validateCountAndDuration(count *int, durationMinutes int) error {
	if durationMinutes <= 0 {
		return newValidationError("invalid_duration", "duration_minutes must be > 0")
	}
	if durationMinutes > MaxDurationMinutes {
		return newValidationError("duration_exceeds_limit", "duration_minutes must be at most %d", MaxDurationMinutes)
	}
	if count != nil && *count <= 0 {
		return newValidationError("invalid_count", "count must be > 0")
	}
	if count != nil && *count > MaxOccurrences {
		return newValidationError("count_exceeds_limit", "count must be at most %d", MaxOccurrences)
	}
	return nil
}
