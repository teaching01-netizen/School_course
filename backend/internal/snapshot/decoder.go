package snapshot

import (
	"encoding/json"
	"fmt"
)

// DecodeSessionSnapshotV1 decodes a JSON byte slice into a SessionSnapshotV1.
// It returns a controlled error for malformed data and preserves unknown fields.
func DecodeSessionSnapshotV1(data []byte) (SessionSnapshotV1, error) {
	var snapshot SessionSnapshotV1
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return SessionSnapshotV1{}, fmt.Errorf("decode snapshot: %w", err)
	}

	if err := ValidateSnapshot(snapshot); err != nil {
		return SessionSnapshotV1{}, fmt.Errorf("validate snapshot: %w", err)
	}

	return snapshot, nil
}

// DecodeSessionSnapshotV1Raw decodes JSON without validation.
// Use this for audit access to preserve unknown snapshots.
func DecodeSessionSnapshotV1Raw(data []byte) (SessionSnapshotV1, error) {
	var snapshot SessionSnapshotV1
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return SessionSnapshotV1{}, fmt.Errorf("decode snapshot: %w", err)
	}
	return snapshot, nil
}
