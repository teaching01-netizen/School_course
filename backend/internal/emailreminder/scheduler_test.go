package emailreminder

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseTime(t *testing.T) {
	tests := []struct {
		input    string
		wantHour int
		wantMin  int
		wantErr  bool
	}{
		{"08:00", 8, 0, false},
		{"00:00", 0, 0, false},
		{"23:59", 23, 59, false},
		{"", 0, 0, true},
		{"invalid", 0, 0, true},
		{"25:00", 0, 0, true},
	}
	for _, tt := range tests {
		h, m, err := parseTime(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseTime(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			continue
		}
		if h != tt.wantHour || m != tt.wantMin {
			t.Errorf("parseTime(%q) = %d:%d, want %d:%d", tt.input, h, m, tt.wantHour, tt.wantMin)
		}
	}
}

func TestSchedulerSkipsWhenNotScheduledTime(t *testing.T) {
	var callCount atomic.Int32
	s := New(noopLogger(), Config{Enabled: true, Time: "08:00"}, "UTC", func(ctx context.Context) error {
		callCount.Add(1)
		return nil
	})

	// Tick at a non-matching time — should not call sendAll
	s.lastRunDate = "" // fresh start
	now := time.Date(2026, 6, 14, 10, 30, 0, 0, time.UTC)
	s.tickAt(context.Background(), now)

	if callCount.Load() != 0 {
		t.Errorf("expected 0 calls at non-scheduled time, got %d", callCount.Load())
	}
}

func TestSchedulerFiresAtScheduledTime(t *testing.T) {
	var callCount atomic.Int32
	s := New(noopLogger(), Config{Enabled: true, Time: "08:00"}, "UTC", func(ctx context.Context) error {
		callCount.Add(1)
		return nil
	})

	now := time.Date(2026, 6, 14, 8, 0, 0, 0, time.UTC)
	s.tickAt(context.Background(), now)

	if callCount.Load() != 1 {
		t.Errorf("expected 1 call at scheduled time, got %d", callCount.Load())
	}
	if s.lastRunDate != "2026-06-14" {
		t.Errorf("expected lastRunDate 2026-06-14, got %s", s.lastRunDate)
	}
}

func TestSchedulerSkipsSecondFireSameDay(t *testing.T) {
	var callCount atomic.Int32
	s := New(noopLogger(), Config{Enabled: true, Time: "08:00"}, "UTC", func(ctx context.Context) error {
		callCount.Add(1)
		return nil
	})

	// First tick at 08:00
	now := time.Date(2026, 6, 14, 8, 0, 0, 0, time.UTC)
	s.tickAt(context.Background(), now)

	// Second tick at 08:01 same day (should be skipped)
	now2 := time.Date(2026, 6, 14, 8, 1, 0, 0, time.UTC)
	s.tickAt(context.Background(), now2)

	if callCount.Load() != 1 {
		t.Errorf("expected 1 call (second skipped), got %d", callCount.Load())
	}
}

func TestSchedulerFiresNextDay(t *testing.T) {
	var callCount atomic.Int32
	s := New(noopLogger(), Config{Enabled: true, Time: "08:00"}, "UTC", func(ctx context.Context) error {
		callCount.Add(1)
		return nil
	})

	// Day 1 at 08:00
	s.tickAt(context.Background(), time.Date(2026, 6, 14, 8, 0, 0, 0, time.UTC))

	// Day 2 at 08:00
	s.tickAt(context.Background(), time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC))

	if callCount.Load() != 2 {
		t.Errorf("expected 2 calls across 2 days, got %d", callCount.Load())
	}
}

func TestSchedulerStartDisabledDoesNothing(t *testing.T) {
	var callCount atomic.Int32
	s := New(noopLogger(), Config{Enabled: false}, "UTC", func(ctx context.Context) error {
		callCount.Add(1)
		return nil
	})

	s.Start(context.Background())
	time.Sleep(50 * time.Millisecond)

	if callCount.Load() != 0 {
		t.Errorf("expected 0 calls when disabled, got %d", callCount.Load())
	}
}

func TestSchedulerDefaultsTo08_00(t *testing.T) {
	s := New(noopLogger(), Config{Enabled: true}, "UTC", func(ctx context.Context) error {
		return nil
	})
	if s.cfg.Time != "08:00" {
		t.Errorf("expected default time 08:00, got %s", s.cfg.Time)
	}
}

func TestNowInTZ(t *testing.T) {
	// UTC time should match current UTC
	before := time.Now().UTC()
	after := nowInTZ("UTC")
	// after should be within a few seconds of before
	if after.Before(before.Add(-5 * time.Second)) || after.After(before.Add(5 * time.Second)) {
		t.Errorf("nowInTZ(UTC) = %v, expected near %v", after, before)
	}

	// Falls back to UTC for unknown timezone
	fallback := nowInTZ("NotARealTimeZone")
	if fallback.Location() != time.UTC {
		t.Errorf("expected UTC fallback for unknown timezone, got %v", fallback.Location())
	}
}

func noopLogger() *slog.Logger {
	return slog.Default()
}
