package absenceshttp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/snapshot"
)

func TestSitInImpactDTO(t *testing.T) {
	id := uuid.New()
	sessionID := pgtype.UUID{Bytes: id, Valid: true}
	original := snapshot.SessionSnapshotV1{
		SchemaVersion: 1, SessionID: id, SessionVersion: 1,
		Course:  snapshot.SnapshotEntity{ID: uuid.NewString(), Name: "Original class"},
		StartAt: time.Date(2026, 6, 3, 3, 0, 0, 0, time.UTC),
		EndAt:   time.Date(2026, 6, 3, 4, 30, 0, 0, time.UTC), CapturedAt: time.Now().UTC(),
	}
	evidence, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{a: httpadapter.Adapter{}}
	for _, tc := range []struct {
		name              string
		assignment        []byte
		assignmentQuality string
		issue             []byte
		issueQuality      string
		wantQuality       string
	}{
		{"assignment preferred", evidence, "exact", nil, "unavailable", "exact"},
		{"missing assignment", nil, "unavailable", evidence, "exact", "exact"},
		{"malformed assignment falls back", []byte(`{}`), "exact", evidence, "reconstructed", "reconstructed"},
		{"unknown schema", []byte(`{"schema_version":2}`), "exact", nil, "unavailable", "unavailable"},
		{"unavailable evidence", evidence, "unavailable", nil, "unavailable", "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := sqldb.AbsenceSitInImpact{SessionID: sessionID, AssignmentSnapshot: tc.assignment,
				AssignmentQuality: tc.assignmentQuality, IssueSnapshot: tc.issue, IssueQuality: tc.issueQuality}
			dto := s.sitInImpactDTO(row)
			if dto.SnapshotQuality != tc.wantQuality {
				t.Fatalf("quality = %s, want %s", dto.SnapshotQuality, tc.wantQuality)
			}
			if dto.CurrentSession != nil {
				t.Fatal("removed session must have null current context")
			}
			if tc.wantQuality != "unavailable" {
				if dto.OriginalSnapshot == nil || !dto.OriginalSnapshot.StartAt.Equal(original.StartAt) {
					t.Fatal("lost original time")
				}
			} else if dto.OriginalSnapshot != nil {
				t.Fatal("unavailable evidence exposed as original")
			}
			row.Current = sqldb.ManagedAbsenceSession{SessionID: sessionID, ID: sessionID,
				StartAt: pgtype.Timestamptz{Time: original.StartAt.Add(48 * time.Hour), Valid: true},
				EndAt:   pgtype.Timestamptz{Time: original.EndAt.Add(48 * time.Hour), Valid: true}}
			dto = s.sitInImpactDTO(row)
			if dto.CurrentSession == nil || dto.CurrentSession.StartAt == original.StartAt.Format(time.RFC3339Nano) {
				t.Fatal("expected separate moved time")
			}
		})
	}
}
