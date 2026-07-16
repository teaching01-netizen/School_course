package scheduling

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestScheduleRejectionLogsAreBounded(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	resourceID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	seriesID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	logAvailabilityMutationRejected(context.Background(), logger, "teacher", resourceID, 3)
	logSeriesAttachmentRejected(context.Background(), logger, seriesID, "series_occurrence_mismatch")

	got := output.String()
	for _, want := range []string{
		"schedule availability mutation rejected",
		`"resource_type":"teacher"`,
		`"resource_id":`,
		`"conflict_count":3`,
		"schedule series attachment rejected",
		`"series_id":`,
		`"reason":"series_occurrence_mismatch"`,
	} {
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
