package scheduling

import (
	"testing"
	"time"
)

func TestOrdinalityToIndex_ConvertsOneBasedValues(t *testing.T) {
	for ordinal, want := range map[int64]int{1: 0, 2: 1, 4: 3} {
		got, ok := ordinalityToIndex(ordinal, 4)
		if !ok || got != want {
			t.Fatalf("ordinal=%d got=%d ok=%v", ordinal, got, ok)
		}
	}
}

func TestOrdinalityToIndex_RejectsZeroAndOutOfRange(t *testing.T) {
	for _, ordinal := range []int64{0, -1, 5} {
		if _, ok := ordinalityToIndex(ordinal, 4); ok {
			t.Fatalf("accepted %d", ordinal)
		}
	}
}

func TestGenerateHourlySlots_DoesNotEndAfterDayEnd(t *testing.T) {
	for _, duration := range []int{30, 60, 90, 120} {
		got := generateHourlySlots(time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), 8, 20, duration)
		for _, slot := range got {
			if slot.End.Hour() > 20 || (slot.End.Hour() == 20 && slot.End.Minute() > 0) {
				t.Fatalf("duration=%d slot=%v", duration, slot)
			}
		}
	}
}
