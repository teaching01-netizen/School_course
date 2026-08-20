package legacysynchttp

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func TestControlToDTOCarriesStudentEnabled(t *testing.T) {
	control := sqldb.LegacySyncControl{
		DetectionEnabled: true,
		FetchEnabled:     true,
		ApplyEnabled:     true,
		StudentEnabled:   true,
		TombstoneEnabled: false,
		RealtimeEnabled:  true,
		ShadowMode:       false,
	}
	dto := controlToDTO(control)
	if !dto.StudentEnabled {
		t.Fatal("controlToDTO must surface student_enabled")
	}
	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(encoded) {
		t.Fatalf("controlDTO must be valid JSON, got %s", encoded)
	}
}

func TestConflictToDTOCarriesPayloads(t *testing.T) {
	conflict := sqldb.LegacySyncConflict{
		ID:            pgtype.UUID{Bytes: uuid.New(), Valid: true},
		EntityType:    "course",
		ExternalID:    "7306",
		ConflictType:  "code_claimed",
		Category:      "data",
		SourcePayload: []byte(`{"code":"CS101","title":"Computing"}`),
		LocalPayload:  []byte(`{"code":"CS101"}`),
		Message:       pgtype.Text{String: "code already claimed", Valid: true},
		Status:        "open",
	}
	dto := conflictToDTO(conflict)
	if dto.SourcePayload == nil || *dto.SourcePayload != `{"code":"CS101","title":"Computing"}` {
		t.Fatalf("source_payload = %v, want the raw JSON string", dto.SourcePayload)
	}
	if dto.LocalPayload == nil || *dto.LocalPayload != `{"code":"CS101"}` {
		t.Fatalf("local_payload = %v, want the raw JSON string", dto.LocalPayload)
	}
	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["source_payload"]; !ok {
		t.Fatalf("DTO JSON must include source_payload, got %s", encoded)
	}
	if _, ok := decoded["local_payload"]; !ok {
		t.Fatalf("DTO JSON must include local_payload, got %s", encoded)
	}
}

func TestConflictToDTONilPayloads(t *testing.T) {
	conflict := sqldb.LegacySyncConflict{
		ID:           pgtype.UUID{Bytes: uuid.New(), Valid: true},
		EntityType:   "course",
		ExternalID:   "7307",
		ConflictType: "code_claimed",
		Status:       "open",
	}
	dto := conflictToDTO(conflict)
	if dto.SourcePayload != nil || dto.LocalPayload != nil {
		t.Fatalf("empty payloads must map to nil, got %v / %v", dto.SourcePayload, dto.LocalPayload)
	}
}
