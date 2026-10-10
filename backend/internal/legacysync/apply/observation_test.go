package apply

import (
	"errors"
	"testing"
)

func TestValidateScheduleObservationRequiresCompletePositiveGeneration(t *testing.T) {
	if err := validateScheduleObservation(false, 1); !errors.Is(err, ErrIncompleteScheduleSnapshot) {
		t.Fatalf("incomplete snapshot error = %v, want ErrIncompleteScheduleSnapshot", err)
	}
	if err := validateScheduleObservation(true, 0); !errors.Is(err, ErrInvalidObservationGeneration) {
		t.Fatalf("zero generation error = %v, want ErrInvalidObservationGeneration", err)
	}
	if err := validateScheduleObservation(true, 1); err != nil {
		t.Fatalf("valid observation rejected: %v", err)
	}
}
