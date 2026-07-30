package sessionchangehttp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"warwick-institute/internal/snapshot"
)

func TestDecodeIssueDetails_ValidJSON(t *testing.T) {
	details := IssueDetails{
		Reasons:        []string{"session_version_changed", "missed_session_overlap"},
		SessionVersion: 3,
		OldStartAt:     "2026-06-10T09:00:00Z",
		NewStartAt:     "2026-06-10T10:00:00Z",
	}

	data, err := json.Marshal(details)
	if err != nil {
		t.Fatalf("failed to marshal details: %v", err)
	}

	decoded := DecodeIssueDetails(data)

	if len(decoded.Reasons) != 2 {
		t.Errorf("expected 2 reasons, got %d", len(decoded.Reasons))
	}
	if decoded.Reasons[0] != "session_version_changed" {
		t.Errorf("expected first reason to be 'session_version_changed', got %q", decoded.Reasons[0])
	}
	if decoded.SessionVersion != 3 {
		t.Errorf("expected session version 3, got %d", decoded.SessionVersion)
	}
}

func TestDecodeIssueDetails_EmptyJSON(t *testing.T) {
	decoded := DecodeIssueDetails([]byte("{}"))

	if decoded.Reasons == nil {
		t.Error("expected reasons to be empty slice, got nil")
	}
	if len(decoded.Reasons) != 0 {
		t.Errorf("expected 0 reasons, got %d", len(decoded.Reasons))
	}
}

func TestDecodeIssueDetails_MalformedJSON(t *testing.T) {
	decoded := DecodeIssueDetails([]byte("not json"))

	if decoded.Reasons == nil {
		t.Error("expected reasons to be empty slice, got nil")
	}
	if len(decoded.Reasons) != 0 {
		t.Errorf("expected 0 reasons, got %d", len(decoded.Reasons))
	}
}

func TestDecodeIssueDetails_NilReasons(t *testing.T) {
	data := []byte(`{"session_version": 2}`)
	decoded := DecodeIssueDetails(data)

	if decoded.Reasons == nil {
		t.Error("expected reasons to be empty slice, got nil")
	}
	if len(decoded.Reasons) != 0 {
		t.Errorf("expected 0 reasons, got %d", len(decoded.Reasons))
	}
}

func TestDecodeAssignmentSnapshot_EmptyData(t *testing.T) {
	view := DecodeAssignmentSnapshot(nil, "", "")

	if view.Quality != "unavailable" {
		t.Errorf("expected quality 'unavailable', got %q", view.Quality)
	}
	if view.Snapshot != nil {
		t.Error("expected nil snapshot for empty data")
	}
}

func TestDecodeAssignmentSnapshot_MalformedJSON(t *testing.T) {
	view := DecodeAssignmentSnapshot([]byte("not json"), "exact", "assignment")

	if view.Quality != "unavailable" {
		t.Errorf("expected quality 'unavailable', got %q", view.Quality)
	}
	if view.Snapshot != nil {
		t.Error("expected nil snapshot for malformed data")
	}
}

func TestDecodeAssignmentSnapshot_UnknownSchemaVersion(t *testing.T) {
	snap := map[string]interface{}{
		"schema_version": 99,
		"session_id":     "123e4567-e89b-12d3-a456-426614174000",
	}
	data, _ := json.Marshal(snap)

	view := DecodeAssignmentSnapshot(data, "exact", "assignment")

	if view.Quality != "unavailable" {
		t.Errorf("expected quality 'unavailable' for unknown schema version, got %q", view.Quality)
	}
	if view.Snapshot != nil {
		t.Error("expected nil snapshot for unknown schema version")
	}
}

func TestDecodeAssignmentSnapshot_ValidSnapshot(t *testing.T) {
	snap := snapshot.SessionSnapshotV1{
		SchemaVersion:  1,
		SessionID:      uuid.New(),
		SessionVersion: 2,
		StartAt:        time.Now(),
		EndAt:          time.Now().Add(time.Hour),
		Timezone:       "UTC",
		Course: snapshot.SnapshotEntity{
			ID:   uuid.New().String(),
			Code: "CS101",
			Name: "Computer Science 101",
		},
		Room: snapshot.NullableSnapshotEntity{
			ID:   ptrString(uuid.New().String()),
			Name: ptrString("Room A"),
		},
		Teacher: snapshot.NullableSnapshotEntity{
			ID:   ptrString(uuid.New().String()),
			Name: ptrString("Dr. Smith"),
		},
		CapturedAt: time.Now(),
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("failed to marshal snapshot: %v", err)
	}

	view := DecodeAssignmentSnapshot(data, "exact", "assignment")

	if view.Quality != "exact" {
		t.Errorf("expected quality 'exact', got %q", view.Quality)
	}
	if view.Source != "assignment" {
		t.Errorf("expected source 'assignment', got %q", view.Source)
	}
	if view.Snapshot == nil {
		t.Error("expected non-nil snapshot")
	}
	if view.Snapshot.Course.Code != "CS101" {
		t.Errorf("expected course code 'CS101', got %q", view.Snapshot.Course.Code)
	}
}

func TestDecodeAssignmentSnapshot_DefaultQuality(t *testing.T) {
	snap := snapshot.SessionSnapshotV1{
		SchemaVersion:  1,
		SessionID:      uuid.New(),
		SessionVersion: 1,
		StartAt:        time.Now(),
		EndAt:          time.Now().Add(time.Hour),
		Timezone:       "UTC",
		Course: snapshot.SnapshotEntity{
			ID:   uuid.New().String(),
			Code: "MATH201",
			Name: "Mathematics 201",
		},
		CapturedAt: time.Now(),
	}
	data, _ := json.Marshal(snap)

	view := DecodeAssignmentSnapshot(data, "", "detection")

	if view.Quality != "exact" {
		t.Errorf("expected default quality 'exact', got %q", view.Quality)
	}
	if view.Source != "detection" {
		t.Errorf("expected source 'detection', got %q", view.Source)
	}
}

func TestDecodeChangeSnapshot_EmptyData(t *testing.T) {
	snap := DecodeChangeSnapshot(nil)

	if snap != nil {
		t.Error("expected nil snapshot for empty data")
	}
}

func TestDecodeChangeSnapshot_MalformedJSON(t *testing.T) {
	snap := DecodeChangeSnapshot([]byte("not json"))

	if snap != nil {
		t.Error("expected nil snapshot for malformed data")
	}
}

func TestDecodeChangeSnapshot_UnknownSchemaVersion(t *testing.T) {
	snap := map[string]interface{}{
		"schema_version": 99,
		"session_id":     "123e4567-e89b-12d3-a456-426614174000",
	}
	data, _ := json.Marshal(snap)

	result := DecodeChangeSnapshot(data)

	if result != nil {
		t.Error("expected nil snapshot for unknown schema version")
	}
}

func TestDecodeChangeSnapshot_ValidSnapshot(t *testing.T) {
	snap := snapshot.SessionSnapshotV1{
		SchemaVersion:  1,
		SessionID:      uuid.New(),
		SessionVersion: 3,
		StartAt:        time.Now(),
		EndAt:          time.Now().Add(time.Hour),
		Timezone:       "UTC",
		Course: snapshot.SnapshotEntity{
			ID:   uuid.New().String(),
			Code: "PHY101",
			Name: "Physics 101",
		},
		CapturedAt: time.Now(),
	}
	data, _ := json.Marshal(snap)

	result := DecodeChangeSnapshot(data)

	if result == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if result.Course.Code != "PHY101" {
		t.Errorf("expected course code 'PHY101', got %q", result.Course.Code)
	}
}

func TestImpactReasonsFromCodes_Empty(t *testing.T) {
	reasons := ImpactReasonsFromCodes([]string{})

	if len(reasons) != 0 {
		t.Errorf("expected 0 reasons, got %d", len(reasons))
	}
}

func TestImpactReasonsFromCodes_MultipleCodes(t *testing.T) {
	codes := []string{"session_deleted", "missed_session_overlap", "unknown_code"}
	reasons := ImpactReasonsFromCodes(codes)

	if len(reasons) != 3 {
		t.Fatalf("expected 3 reasons, got %d", len(reasons))
	}

	if reasons[0].Code != "session_deleted" {
		t.Errorf("expected first reason code 'session_deleted', got %q", reasons[0].Code)
	}
	if reasons[0].Message != "The assigned session has been deleted" {
		t.Errorf("unexpected message for session_deleted: %q", reasons[0].Message)
	}

	if reasons[1].Code != "missed_session_overlap" {
		t.Errorf("expected second reason code 'missed_session_overlap', got %q", reasons[1].Code)
	}

	if reasons[2].Code != "unknown_code" {
		t.Errorf("expected third reason code 'unknown_code', got %q", reasons[2].Code)
	}
	if reasons[2].Message != "Impact reason: unknown_code" {
		t.Errorf("unexpected message for unknown code: %q", reasons[2].Message)
	}
}

func TestScheduleImpactIssue_MarshalJSON(t *testing.T) {
	snap := snapshot.SessionSnapshotV1{
		SchemaVersion:  1,
		SessionID:      uuid.New(),
		SessionVersion: 1,
		StartAt:        time.Now(),
		EndAt:          time.Now().Add(time.Hour),
		Timezone:       "UTC",
		Course: snapshot.SnapshotEntity{
			ID:   uuid.New().String(),
			Code: "CS101",
			Name: "Computer Science 101",
		},
		CapturedAt: time.Now(),
	}

	issue := ScheduleImpactIssue{
		ID:           "test-id",
		IssueVersion: 1,
		AssignmentContext: AssignmentContext{
			OriginalSession: OriginalSessionView{
				Quality:  "exact",
				Source:   "assignment",
				Snapshot: &snap,
			},
			CurrentSession: nil,
		},
		ChangeContext: ChangeContext{
			ChangeID: "change-id",
			Before:   &snap,
			After:    &snap,
		},
		ImpactContext: ImpactContext{
			IssueType: "sit_in_session_changed",
			Severity:  "warning",
			Reasons: []ImpactReason{
				{Code: "session_version_changed", Message: "Session version changed"},
			},
		},
	}

	data, err := json.Marshal(issue)
	if err != nil {
		t.Fatalf("failed to marshal ScheduleImpactIssue: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded["id"] != "test-id" {
		t.Errorf("expected id 'test-id', got %v", decoded["id"])
	}
	if decoded["issue_version"] != float64(1) {
		t.Errorf("expected issue_version 1, got %v", decoded["issue_version"])
	}

	assignmentContext, ok := decoded["assignment_context"].(map[string]interface{})
	if !ok {
		t.Fatal("expected assignment_context to be an object")
	}
	originalSession, ok := assignmentContext["original_session"].(map[string]interface{})
	if !ok {
		t.Fatal("expected original_session to be an object")
	}
	if originalSession["quality"] != "exact" {
		t.Errorf("expected quality 'exact', got %v", originalSession["quality"])
	}

	impactContext, ok := decoded["impact_context"].(map[string]interface{})
	if !ok {
		t.Fatal("expected impact_context to be an object")
	}
	if impactContext["issue_type"] != "sit_in_session_changed" {
		t.Errorf("expected issue_type 'sit_in_session_changed', got %v", impactContext["issue_type"])
	}
}

func TestImpactContext_AllReasonCodes(t *testing.T) {
	tests := []struct {
		code     string
		expected string
	}{
		{"session_deleted", "The assigned session has been deleted"},
		{"session_version_changed", "The session version has changed since assignment"},
		{"missed_session_overlap", "The sit-in session overlaps with a missed session"},
		{"regular_session_overlap", "The sit-in session overlaps with a regular enrolled session"},
		{"past_time", "The session time has moved into the past"},
		{"short_notice", "The change was made with short notice"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			reasons := ImpactReasonsFromCodes([]string{tt.code})
			if len(reasons) != 1 {
				t.Fatalf("expected 1 reason, got %d", len(reasons))
			}
			if reasons[0].Message != tt.expected {
				t.Errorf("expected message %q, got %q", tt.expected, reasons[0].Message)
			}
		})
	}
}

func ptrString(s string) *string {
	return &s
}
