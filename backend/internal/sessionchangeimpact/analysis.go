package sessionchangeimpact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences/sitinresolver"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/realtime"
)

type analysisRun struct {
	q        *sqldb.Queries
	resolver *sitinresolver.Service
	now      func() time.Time
	change   sqldb.SessionChangeGetByIDRow
}

type analysisResult struct {
	AffectedAbsenceIDs []string `json:"affected_absence_ids"`
	AbsenceCount       int      `json:"absence_count"`
	IssuesCreated      int      `json:"issues_created"`
}

func categorizeError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "session change settings") || strings.Contains(msg, "affected absences") || strings.Contains(msg, "load session change"):
		return "database_error"
	case strings.Contains(msg, "validate") || strings.Contains(msg, "suggest") || strings.Contains(msg, "resolver"):
		return "validation_error"
	case strings.Contains(msg, "upsert schedule issue") || strings.Contains(msg, "snapshot"):
		return "snapshot_error"
	default:
		return "analysis_error"
	}
}

func (s *Service) Analyze(ctx context.Context, changeID pgtype.UUID) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin impact analysis transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	isLatest, err := qtx.SessionChangeIsLatestForAnalysis(ctx, changeID)
	if err != nil {
		return err
	}
	if !isLatest {
		if err := qtx.SessionChangeImpactRunSetStatus(ctx, changeID, "superseded", "newer session change is already queued"); err != nil {
			return fmt.Errorf("mark stale impact analysis as superseded: %w", err)
		}
		return tx.Commit(ctx)
	}
	if err := qtx.SessionChangeImpactRunSetStatus(ctx, changeID, "processing", ""); err != nil {
		return fmt.Errorf("mark impact analysis as processing: %w", err)
	}
	change, err := qtx.SessionChangeGetByID(ctx, changeID)
	if err != nil {
		return fmt.Errorf("load session change: %w", err)
	}
	run := analysisRun{q: qtx, resolver: s.resolver.WithQueries(qtx), now: s.now, change: change}
	affected, createdIssueIDs, err := run.analyze(ctx)
	if err != nil {
		if setErr := qtx.SessionChangeImpactRunSetStatus(ctx, changeID, "failed", err.Error()); setErr != nil {
			return fmt.Errorf("mark impact analysis as failed: %w", setErr)
		}
		category := categorizeError(err)
		if setErr := qtx.SessionChangeImpactRunSetResult(ctx, changeID, nil, nil, category, true); setErr != nil {
			return fmt.Errorf("record impact analysis error: %w", setErr)
		}
		return err
	}

	// Record processing result with affected absence IDs.
	result := analysisResult{
		AffectedAbsenceIDs: make([]string, 0, len(affected)),
		AbsenceCount:       len(affected),
		IssuesCreated:      len(affected),
	}
	seen := make(map[pgtype.UUID]struct{}, len(affected))
	for _, item := range affected {
		if _, exists := seen[item.ID]; !exists {
			seen[item.ID] = struct{}{}
			result.AffectedAbsenceIDs = append(result.AffectedAbsenceIDs, uuidString(item.ID))
		}
	}
	resultJSON, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		resultJSON = []byte("{}")
	}

	if err := qtx.SessionChangeImpactRunSetStatus(ctx, changeID, "completed", ""); err != nil {
		return fmt.Errorf("mark impact analysis as completed: %w", err)
	}
	if err := qtx.SessionChangeImpactRunSetResult(ctx, changeID, resultJSON, createdIssueIDs, "", true); err != nil {
		return fmt.Errorf("record impact analysis result: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit impact analysis: %w", err)
	}
	s.publishAnalysis(change.ID, affected)
	return nil
}

func (run analysisRun) analyze(ctx context.Context) ([]sqldb.SessionChangeAffectedAbsencesRow, []pgtype.UUID, error) {
	settings, err := run.q.AppSettingsGetSessionChangeSettings(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load session change settings: %w", err)
	}
	affected, err := run.q.SessionChangeAffectedAbsences(ctx, run.change.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("load affected absences: %w", err)
	}
	var createdIssueIDs []pgtype.UUID
	activeByAbsence := make(map[pgtype.UUID][]string, len(affected))
	for _, item := range affected {
		if item.ImpactRelation != "" {
			issueType := "sit_in_session_deleted"
			if item.ImpactRelation == "missed_session" {
				issueType = "missed_session_deleted"
			}
			fingerprint := issueFingerprint(item.ID, issueType, run.change.SessionID, pgtype.UUID{}, pgtype.UUID{})
			activeByAbsence[item.ID] = append(activeByAbsence[item.ID], fingerprint)
			issueID, err := run.upsertIssue(ctx, issueInput{item: item, issueType: issueType, severity: "critical", reasons: []string{"session_deleted"}, fingerprint: fingerprint, deletionTarget: true, snapshotJSON: item.AssignmentSnapshotJson, snapshotQuality: item.AssignmentSnapshotQuality.String, snapshotSource: item.AssignmentSnapshotSource})
			if err != nil {
				return nil, nil, err
			}
			createdIssueIDs = append(createdIssueIDs, issueID)
			continue
		}
		if item.AssignmentID.Valid {
			validation, validationErr := run.resolver.ValidateAssignment(ctx, item.ID, item.SitInSessionID)
			if validationErr != nil {
				return nil, nil, fmt.Errorf("validate assignment: %w", validationErr)
			}
			issueType := "sit_in_session_changed"
			severity := "warning"
			if !validation.Valid {
				issueType = issueTypeForReason(validation.Reasons)
				severity = "critical"
			}
			fingerprint := issueFingerprint(item.ID, issueType, run.change.SessionID, item.SitInSessionID, item.MissedSessionID)
			activeByAbsence[item.ID] = append(activeByAbsence[item.ID], fingerprint)
			issueID, err := run.upsertIssue(ctx, issueInput{item: item, issueType: issueType, severity: severity, reasons: validation.Reasons, fingerprint: fingerprint, snapshotJSON: item.AssignmentSnapshotJson, snapshotQuality: item.AssignmentSnapshotQuality.String, snapshotSource: item.AssignmentSnapshotSource})
			if err != nil {
				return nil, nil, err
			}
			createdIssueIDs = append(createdIssueIDs, issueID)
		}
		if item.MissedSessionID.Valid {
			fingerprint := issueFingerprint(item.ID, "missed_session_changed", run.change.SessionID, item.SitInSessionID, item.MissedSessionID)
			activeByAbsence[item.ID] = append(activeByAbsence[item.ID], fingerprint)
			issueID, err := run.upsertIssue(ctx, issueInput{item: item, issueType: "missed_session_changed", severity: "warning", fingerprint: fingerprint, snapshotJSON: item.AssignmentSnapshotJson, snapshotQuality: item.AssignmentSnapshotQuality.String, snapshotSource: item.AssignmentSnapshotSource})
			if err != nil {
				return nil, nil, err
			}
			createdIssueIDs = append(createdIssueIDs, issueID)
		}
	}
	if run.change.ChangeSource != "session_delete" {
		timingIDs, err := run.addTimingIssues(ctx, settings.WarningHours, settings.CriticalHours, affected, activeByAbsence)
		if err != nil {
			return nil, nil, err
		}
		createdIssueIDs = append(createdIssueIDs, timingIDs...)
	}
	if err := run.resolveSuperseded(ctx, affected, activeByAbsence); err != nil {
		return nil, nil, err
	}
	return affected, createdIssueIDs, nil
}

func (run analysisRun) addTimingIssues(ctx context.Context, warningHours, criticalHours int32, affected []sqldb.SessionChangeAffectedAbsencesRow, activeByAbsence map[pgtype.UUID][]string) ([]pgtype.UUID, error) {
	var issueIDs []pgtype.UUID
	if !run.change.NewStartAt.Valid {
		return nil, nil
	}
	noticeHours := run.change.NewStartAt.Time.Sub(run.now()).Hours()
	if noticeHours <= float64(warningHours) {
		severity := "warning"
		if noticeHours <= float64(criticalHours) {
			severity = "critical"
		}
		for _, item := range affected {
			fingerprint := issueFingerprint(item.ID, "short_notice_change", run.change.SessionID, item.SitInSessionID, item.MissedSessionID)
			activeByAbsence[item.ID] = append(activeByAbsence[item.ID], fingerprint)
			issueID, err := run.upsertIssue(ctx, issueInput{item: item, issueType: "short_notice_change", severity: severity, fingerprint: fingerprint, snapshotJSON: item.AssignmentSnapshotJson, snapshotQuality: item.AssignmentSnapshotQuality.String, snapshotSource: item.AssignmentSnapshotSource})
			if err != nil {
				return nil, err
			}
			issueIDs = append(issueIDs, issueID)
		}
	}
	if !run.change.NewStartAt.Time.After(run.now()) {
		for _, item := range affected {
			fingerprint := issueFingerprint(item.ID, "past_time_change", run.change.SessionID, item.SitInSessionID, item.MissedSessionID)
			activeByAbsence[item.ID] = append(activeByAbsence[item.ID], fingerprint)
			issueID, err := run.upsertIssue(ctx, issueInput{item: item, issueType: "past_time_change", severity: "critical", fingerprint: fingerprint, snapshotJSON: item.AssignmentSnapshotJson, snapshotQuality: item.AssignmentSnapshotQuality.String, snapshotSource: item.AssignmentSnapshotSource})
			if err != nil {
				return nil, err
			}
			issueIDs = append(issueIDs, issueID)
		}
	}
	return issueIDs, nil
}

func (run analysisRun) resolveSuperseded(ctx context.Context, affected []sqldb.SessionChangeAffectedAbsencesRow, activeByAbsence map[pgtype.UUID][]string) error {
	seen := make(map[pgtype.UUID]struct{}, len(affected))
	for _, item := range affected {
		if _, exists := seen[item.ID]; exists {
			continue
		}
		seen[item.ID] = struct{}{}
		if err := run.q.AbsenceScheduleIssuesSupersede(ctx, item.ID, activeByAbsence[item.ID]); err != nil {
			return fmt.Errorf("resolve superseded schedule issues: %w", err)
		}
	}
	return nil
}

func (s *Service) publishAnalysis(changeID pgtype.UUID, affected []sqldb.SessionChangeAffectedAbsencesRow) {
	if s.realtime == nil {
		return
	}
	s.realtime.Publish("sessions:all", realtime.Event{Type: "session_change_impact.updated", ID: uuidString(changeID)})
	for _, item := range affected {
		s.realtime.Publish("absences:all", realtime.Event{Type: "absence.updated", ID: uuidString(item.ID)})
	}
}
