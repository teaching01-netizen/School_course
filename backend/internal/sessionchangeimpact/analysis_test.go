package sessionchangeimpact

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestIssueInput_HasSnapshotFields(t *testing.T) {
	input := issueInput{
		snapshotJSON:    []byte(`{"schema_version":1}`),
		snapshotQuality: "exact",
		snapshotSource:  pgtype.Text{String: "assignment", Valid: true},
	}

	if string(input.snapshotJSON) != `{"schema_version":1}` {
		t.Errorf("expected snapshot JSON, got %q", string(input.snapshotJSON))
	}
	if input.snapshotQuality != "exact" {
		t.Errorf("expected snapshot quality 'exact', got %q", input.snapshotQuality)
	}
	if !input.snapshotSource.Valid || input.snapshotSource.String != "assignment" {
		t.Errorf("expected snapshot source 'assignment', got %v", input.snapshotSource)
	}
}

func TestIssueFingerprint_Stability(t *testing.T) {
	absenceID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	source := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	sitIn := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	missed := pgtype.UUID{Bytes: [16]byte{4}, Valid: true}

	// Same inputs should produce the same fingerprint
	fp1 := issueFingerprint(absenceID, "sit_in_session_changed", source, sitIn, missed)
	fp2 := issueFingerprint(absenceID, "sit_in_session_changed", source, sitIn, missed)
	if fp1 != fp2 {
		t.Errorf("fingerprint not stable: %s != %s", fp1, fp2)
	}

	// Different issue type should produce different fingerprint
	fp3 := issueFingerprint(absenceID, "missed_session_changed", source, sitIn, missed)
	if fp1 == fp3 {
		t.Error("different issue types should produce different fingerprints")
	}

	// Different absence ID should produce different fingerprint
	otherAbsence := pgtype.UUID{Bytes: [16]byte{99}, Valid: true}
	fp4 := issueFingerprint(otherAbsence, "sit_in_session_changed", source, sitIn, missed)
	if fp1 == fp4 {
		t.Error("different absence IDs should produce different fingerprints")
	}
}

func TestIssueFingerprint_EmptyUUID(t *testing.T) {
	empty := pgtype.UUID{}
	fp := issueFingerprint(empty, "test", empty, empty, empty)
	if fp == "" {
		t.Error("expected non-empty fingerprint even with empty UUIDs")
	}
}

func TestIssueTypeForReason(t *testing.T) {
	tests := []struct {
		reasons  []string
		expected string
	}{
		{[]string{}, "sit_in_session_changed"},
		{[]string{"missed_session_overlap"}, "sit_in_overlap"},
		{[]string{"regular_session_overlap"}, "regular_session_overlap"},
		{[]string{"session_version_changed"}, "sit_in_ineligible"},
		{[]string{"past_time"}, "past_time_change"},
		{[]string{"unknown_reason"}, "sit_in_ineligible"},
	}

	for _, tt := range tests {
		result := issueTypeForReason(tt.reasons)
		if result != tt.expected {
			t.Errorf("issueTypeForReason(%v) = %q, want %q", tt.reasons, result, tt.expected)
		}
	}
}

func TestAnalysisResult_MarshalUnmarshal(t *testing.T) {
	original := analysisResult{
		AffectedAbsenceIDs: []string{"absence-1", "absence-2"},
		AbsenceCount:       2,
		IssuesCreated:      2,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal analysis result: %v", err)
	}

	var decoded analysisResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal analysis result: %v", err)
	}

	if decoded.AbsenceCount != original.AbsenceCount {
		t.Errorf("absence count mismatch: %d != %d", decoded.AbsenceCount, original.AbsenceCount)
	}
	if len(decoded.AffectedAbsenceIDs) != len(original.AffectedAbsenceIDs) {
		t.Errorf("affected absence IDs length mismatch: %d != %d", len(decoded.AffectedAbsenceIDs), len(original.AffectedAbsenceIDs))
	}
	for i, id := range decoded.AffectedAbsenceIDs {
		if id != original.AffectedAbsenceIDs[i] {
			t.Errorf("affected absence ID[%d] mismatch: %s != %s", i, id, original.AffectedAbsenceIDs[i])
		}
	}
}

func TestAnalysisResult_Empty(t *testing.T) {
	original := analysisResult{
		AffectedAbsenceIDs: []string{},
		AbsenceCount:       0,
		IssuesCreated:      0,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal empty analysis result: %v", err)
	}

	var decoded analysisResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal empty analysis result: %v", err)
	}

	if decoded.AbsenceCount != 0 {
		t.Errorf("expected absence count 0, got %d", decoded.AbsenceCount)
	}
	if len(decoded.AffectedAbsenceIDs) != 0 {
		t.Errorf("expected empty affected absence IDs, got %v", decoded.AffectedAbsenceIDs)
	}
}

func TestIssueDetails_WithSnapshotContext(t *testing.T) {
	details := issueDetails{
		Reasons:        []string{"session_version_changed"},
		SessionVersion: 3,
		OldStartAt:     "2026-06-10T09:00:00Z",
		NewStartAt:     "2026-06-10T10:00:00Z",
	}

	data, err := json.Marshal(details)
	if err != nil {
		t.Fatalf("failed to marshal issue details: %v", err)
	}

	var decoded issueDetails
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal issue details: %v", err)
	}

	if decoded.SessionVersion != 3 {
		t.Errorf("expected session version 3, got %d", decoded.SessionVersion)
	}
	if len(decoded.Reasons) != 1 || decoded.Reasons[0] != "session_version_changed" {
		t.Errorf("expected reasons [session_version_changed], got %v", decoded.Reasons)
	}
}

func TestIssueDetails_DeletionTarget(t *testing.T) {
	details := issueDetails{
		Reasons:          []string{"session_deleted"},
		DeletedSessionID: "session-123",
	}

	data, err := json.Marshal(details)
	if err != nil {
		t.Fatalf("failed to marshal deletion details: %v", err)
	}

	var decoded issueDetails
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal deletion details: %v", err)
	}

	if decoded.DeletedSessionID != "session-123" {
		t.Errorf("expected deleted session ID 'session-123', got %q", decoded.DeletedSessionID)
	}
}
