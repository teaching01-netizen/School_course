package crmhttp

import (
	"reflect"
	"testing"
)

func TestNormalizeWeekdays(t *testing.T) {
	t.Run("defaults missing input to all days for older callers", func(t *testing.T) {
		got, ok := normalizeWeekdays(nil)
		if !ok {
			t.Fatal("expected nil weekdays to be accepted")
		}
		want := []int16{1, 2, 3, 4, 5, 6, 7}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("expected %#v, got %#v", want, got)
		}
	})

	t.Run("deduplicates and sorts provided weekdays", func(t *testing.T) {
		got, ok := normalizeWeekdays([]int16{6, 2, 2})
		if !ok {
			t.Fatal("expected valid weekdays to be accepted")
		}
		want := []int16{2, 6}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("expected %#v, got %#v", want, got)
		}
	})

	t.Run("rejects values outside ISO weekday range", func(t *testing.T) {
		if _, ok := normalizeWeekdays([]int16{0}); ok {
			t.Fatal("expected weekday 0 to be rejected")
		}
		if _, ok := normalizeWeekdays([]int16{8}); ok {
			t.Fatal("expected weekday 8 to be rejected")
		}
	})
}
