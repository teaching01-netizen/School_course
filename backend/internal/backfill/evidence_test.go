package backfill

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBackfillQuality_Constants(t *testing.T) {
	if QualityExact != "exact" {
		t.Errorf("QualityExact = %q, want 'exact'", QualityExact)
	}
	if QualityReconstructed != "reconstructed" {
		t.Errorf("QualityReconstructed = %q, want 'reconstructed'", QualityReconstructed)
	}
	if QualityUnavailable != "unavailable" {
		t.Errorf("QualityUnavailable = %q, want 'unavailable'", QualityUnavailable)
	}
}

func TestBackfillSource_Constants(t *testing.T) {
	if SourceAssignmentEvent != "assignment_event" {
		t.Errorf("SourceAssignmentEvent = %q, want 'assignment_event'", SourceAssignmentEvent)
	}
	if SourceSessionRevision != "session_revision" {
		t.Errorf("SourceSessionRevision = %q, want 'session_revision'", SourceSessionRevision)
	}
	if SourceSessionChange != "session_change" {
		t.Errorf("SourceSessionChange = %q, want 'session_change'", SourceSessionChange)
	}
	if SourceCurrentSession != "current_session" {
		t.Errorf("SourceCurrentSession = %q, want 'current_session'", SourceCurrentSession)
	}
	if SourceNone != "" {
		t.Errorf("SourceNone = %q, want empty string", SourceNone)
	}
}

func TestBackfillReport_Fields(t *testing.T) {
	now := time.Now().UTC()
	report := &BackfillReport{
		TotalEligible: 100,
		Exact:         50,
		Reconstructed: 30,
		Unavailable:   15,
		Failed:        5,
		Remaining:     0,
		AvgBatchDur:   100 * time.Millisecond,
		BatchCount:    5,
		StartedAt:     now,
		CompletedAt:   now.Add(5 * time.Second),
	}

	if report.TotalEligible != 100 {
		t.Errorf("TotalEligible = %d, want 100", report.TotalEligible)
	}
	if report.Exact != 50 {
		t.Errorf("Exact = %d, want 50", report.Exact)
	}
	if report.Reconstructed != 30 {
		t.Errorf("Reconstructed = %d, want 30", report.Reconstructed)
	}
	if report.Unavailable != 15 {
		t.Errorf("Unavailable = %d, want 15", report.Unavailable)
	}
	if report.Failed != 5 {
		t.Errorf("Failed = %d, want 5", report.Failed)
	}
	if report.Remaining != 0 {
		t.Errorf("Remaining = %d, want 0", report.Remaining)
	}
	if report.BatchCount != 5 {
		t.Errorf("BatchCount = %d, want 5", report.BatchCount)
	}
}

func TestEligibleAssignment_Fields(t *testing.T) {
	v := int32(5)
	assignment := &EligibleAssignment{
		ID:                          uuid.New(),
		AbsenceID:                   uuid.New(),
		SessionID:                   uuid.New(),
		SessionVersionAtAssignment: &v,
		AssignedAt:                  time.Now().UTC(),
		CurrentSessionVersion:       5,
		CurrentSessionDeleted:       false,
	}

	if assignment.SessionVersionAtAssignment == nil {
		t.Error("SessionVersionAtAssignment should not be nil")
	}
	if *assignment.SessionVersionAtAssignment != 5 {
		t.Errorf("SessionVersionAtAssignment = %d, want 5", *assignment.SessionVersionAtAssignment)
	}
	if assignment.CurrentSessionVersion != 5 {
		t.Errorf("CurrentSessionVersion = %d, want 5", assignment.CurrentSessionVersion)
	}
	if assignment.CurrentSessionDeleted {
		t.Error("CurrentSessionDeleted should be false")
	}
}

func TestEligibleAssignment_NilVersion(t *testing.T) {
	assignment := &EligibleAssignment{
		ID:                          uuid.New(),
		AbsenceID:                   uuid.New(),
		SessionID:                   uuid.New(),
		SessionVersionAtAssignment: nil,
		AssignedAt:                  time.Now().UTC(),
		CurrentSessionVersion:       3,
		CurrentSessionDeleted:       false,
	}

	if assignment.SessionVersionAtAssignment != nil {
		t.Error("SessionVersionAtAssignment should be nil")
	}
}

func TestEvidenceResult_Fields(t *testing.T) {
	now := time.Now().UTC()
	result := &EvidenceResult{
		Quality:    QualityExact,
		Source:     SourceAssignmentEvent,
		Snapshot:   []byte(`{"schema_version": 1}`),
		Version:    int32Ptr(3),
		CapturedAt: &now,
	}

	if result.Quality != QualityExact {
		t.Errorf("Quality = %q, want %q", result.Quality, QualityExact)
	}
	if result.Source != SourceAssignmentEvent {
		t.Errorf("Source = %q, want %q", result.Source, SourceAssignmentEvent)
	}
	if result.Version == nil || *result.Version != 3 {
		t.Errorf("Version = %v, want 3", result.Version)
	}
	if result.CapturedAt == nil {
		t.Error("CapturedAt should not be nil")
	}
}

func TestEvidenceResult_Unavailable(t *testing.T) {
	result := &EvidenceResult{
		Quality: QualityUnavailable,
		Source:  SourceNone,
	}

	if result.Quality != QualityUnavailable {
		t.Errorf("Quality = %q, want %q", result.Quality, QualityUnavailable)
	}
	if result.Source != SourceNone {
		t.Errorf("Source = %q, want empty string", result.Source)
	}
	if result.Snapshot != nil {
		t.Error("Snapshot should be nil for unavailable")
	}
	if result.Version != nil {
		t.Error("Version should be nil for unavailable")
	}
	if result.CapturedAt != nil {
		t.Error("CapturedAt should be nil for unavailable")
	}
}

func TestBackfillConfig_Defaults(t *testing.T) {
	config := BackfillConfig{}

	if config.BatchSize != 0 {
		t.Errorf("BatchSize = %d, want 0", config.BatchSize)
	}
	if config.RateLimit != 0 {
		t.Errorf("RateLimit = %v, want 0", config.RateLimit)
	}
	if config.MaxBatches != 0 {
		t.Errorf("MaxBatches = %d, want 0", config.MaxBatches)
	}
	if config.DryRun {
		t.Error("DryRun should be false")
	}
	if config.SampleSize != 0 {
		t.Errorf("SampleSize = %d, want 0", config.SampleSize)
	}
}

func TestBackfillConfig_CustomValues(t *testing.T) {
	config := BackfillConfig{
		BatchSize:  200,
		RateLimit:  500 * time.Millisecond,
		MaxBatches: 10,
		DryRun:     true,
		SampleSize: 5,
	}

	if config.BatchSize != 200 {
		t.Errorf("BatchSize = %d, want 200", config.BatchSize)
	}
	if config.RateLimit != 500*time.Millisecond {
		t.Errorf("RateLimit = %v, want 500ms", config.RateLimit)
	}
	if config.MaxBatches != 10 {
		t.Errorf("MaxBatches = %d, want 10", config.MaxBatches)
	}
	if !config.DryRun {
		t.Error("DryRun should be true")
	}
	if config.SampleSize != 5 {
		t.Errorf("SampleSize = %d, want 5", config.SampleSize)
	}
}

func TestBatchResult_Fields(t *testing.T) {
	result := &BatchResult{
		BatchSize:     100,
		Processed:     95,
		Duration:      1 * time.Second,
		Exact:         50,
		Reconstructed: 30,
		Unavailable:   10,
		Failed:        5,
	}

	if result.BatchSize != 100 {
		t.Errorf("BatchSize = %d, want 100", result.BatchSize)
	}
	if result.Processed != 95 {
		t.Errorf("Processed = %d, want 95", result.Processed)
	}
	if result.Exact != 50 {
		t.Errorf("Exact = %d, want 50", result.Exact)
	}
	if result.Reconstructed != 30 {
		t.Errorf("Reconstructed = %d, want 30", result.Reconstructed)
	}
	if result.Unavailable != 10 {
		t.Errorf("Unavailable = %d, want 10", result.Unavailable)
	}
	if result.Failed != 5 {
		t.Errorf("Failed = %d, want 5", result.Failed)
	}
}

func int32Ptr(v int32) *int32 {
	return &v
}
