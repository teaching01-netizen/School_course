package backfill

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// BackfillQuality represents the quality of a reconstructed snapshot.
type BackfillQuality string

const (
	QualityExact        BackfillQuality = "exact"
	QualityReconstructed BackfillQuality = "reconstructed"
	QualityUnavailable  BackfillQuality = "unavailable"
)

// BackfillSource represents the source of the snapshot data.
type BackfillSource string

const (
	SourceAssignmentEvent   BackfillSource = "assignment_event"
	SourceSessionRevision   BackfillSource = "session_revision"
	SourceSessionChange     BackfillSource = "session_change"
	SourceCurrentSession    BackfillSource = "current_session"
	SourceNone              BackfillSource = ""
)

// BackfillReport tracks the progress and results of a backfill run.
type BackfillReport struct {
	TotalEligible   int           `json:"total_eligible"`
	Exact           int           `json:"exact"`
	Reconstructed   int           `json:"reconstructed"`
	Unavailable     int           `json:"unavailable"`
	Failed          int           `json:"failed"`
	Remaining       int           `json:"remaining"`
	AvgBatchDur     time.Duration `json:"avg_batch_duration"`
	BatchCount      int           `json:"batch_count"`
	StartedAt       time.Time     `json:"started_at"`
	CompletedAt     time.Time     `json:"completed_at"`
}

// EligibleAssignment represents an absence_sit_in row eligible for backfill.
type EligibleAssignment struct {
	ID                          uuid.UUID     `json:"id"`
	AbsenceID                   uuid.UUID     `json:"absence_id"`
	SessionID                   uuid.UUID     `json:"session_id"`
	SessionVersionAtAssignment *int32        `json:"session_version_at_assignment"`
	AssignedAt                  time.Time     `json:"assigned_at"`
	CurrentSessionVersion       int32         `json:"current_session_version"`
	CurrentSessionDeleted       bool          `json:"current_session_deleted"`
}

// EvidenceResult holds the outcome of evidence lookup for a single assignment.
type EvidenceResult struct {
	Quality  BackfillQuality `json:"quality"`
	Source   BackfillSource  `json:"source"`
	Snapshot json.RawMessage `json:"snapshot"`
	Version  *int32          `json:"version"`
	CapturedAt *time.Time    `json:"captured_at"`
}

// BatchResult tracks metrics for a single batch processing run.
type BatchResult struct {
	BatchSize   int           `json:"batch_size"`
	Processed   int           `json:"processed"`
	Duration    time.Duration `json:"duration"`
	Exact       int           `json:"exact"`
	Reconstructed int         `json:"reconstructed"`
	Unavailable int           `json:"unavailable"`
	Failed      int           `json:"failed"`
}

// BackfillConfig holds configuration for the backfill job.
type BackfillConfig struct {
	BatchSize    int           `json:"batch_size"`
	RateLimit    time.Duration `json:"rate_limit"`
	MaxBatches   int           `json:"max_batches"`
	DryRun       bool          `json:"dry_run"`
	SampleSize   int           `json:"sample_size"`
}
