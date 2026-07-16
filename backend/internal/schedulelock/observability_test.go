package schedulelock

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestResourceLockFailureLogIsBounded(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	logResourceLockFailure(context.Background(), logger, "lock_teacher", errors.New("deadline exceeded"))

	got := output.String()
	for _, want := range []string{"schedule resource lock failed", `"operation":"lock_teacher"`, "deadline exceeded"} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output missing %q: %s", want, got)
		}
	}
	for _, forbidden := range []string{"student@example.com", "SELECT * FROM", `{"student_name"`} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("log output contains sensitive payload %q: %s", forbidden, got)
		}
	}
}
