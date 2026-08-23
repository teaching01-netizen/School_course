package logging

import (
	"context"
	"log/slog"
	"testing"
)

func TestNewHonorsWarnLevel(t *testing.T) {
	logger := New("warn")
	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("info logs are enabled at warn level")
	}
	if !logger.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("warn logs are disabled at warn level")
	}
	if !logger.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("error logs are disabled at warn level")
	}
}
