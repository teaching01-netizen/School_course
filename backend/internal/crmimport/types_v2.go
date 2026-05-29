package crmimport

import (
	"github.com/google/uuid"
)

// ============================================================================
// Snapshot types
// ============================================================================

type SnapshotStatus string

const (
	SnapshotImporting SnapshotStatus = "importing"
	SnapshotReady     SnapshotStatus = "ready"
	SnapshotFailed    SnapshotStatus = "failed"
)

// ============================================================================
// Job payloads
// ============================================================================

type ImportSnapshotPayload struct {
	UploadID   string    `json:"upload_id"`
	SnapshotID uuid.UUID `json:"snapshot_id"`
}

type StudentSyncPayload struct {
	SnapshotID uuid.UUID `json:"snapshot_id"`
}

// ============================================================================
// Override types
// ============================================================================

type OverrideAction string

const (
	OverrideInclude OverrideAction = "include"
	OverrideExclude OverrideAction = "exclude"
)
