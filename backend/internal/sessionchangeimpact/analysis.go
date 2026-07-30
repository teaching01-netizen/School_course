package sessionchangeimpact

import (
	"context"
	"fmt"
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
	affected, err := run.analyze(ctx)
	if err != nil {
		return err
	}
	if err := qtx.SessionChangeImpactRunSetStatus(ctx, changeID, "completed", ""); err != nil {
		return fmt.Errorf("mark impact analysis as completed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit impact analysis: %w", err)
	}
	s.publishAnalysis(change.ID, affected)
	return nil
}

func (run analysisRun) analyze(ctx context.Context) ([]sqldb.SessionChangeAffectedAbsencesRow, error) {
	settings, err := run.q.AppSettingsGetSessionChangeSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load session change settings: %w", err)
	}
	affected, err := run.q.SessionChangeAffectedAbsences(ctx, run.change.ID)
	if err != nil {
		return nil, fmt.Errorf("load affected absences: %w", err)
	}
	activeByAbsence := make(map[pgtype.UUID][]string, len(affected))
	for _, item := range affected {
		if item.ImpactRelation != "" {
			issueType := "sit_in_session_deleted"
			if item.ImpactRelation == "missed_session" {
				issueType = "missed_session_deleted"
			}
			fingerprint := issueFingerprint(item.ID, issueType, run.change.SessionID, pgtype.UUID{}, pgtype.UUID{})
			activeByAbsence[item.ID] = append(activeByAbsence[item.ID], fingerprint)
			if err := run.upsertIssue(ctx, issueInput{item: item, issueType: issueType, severity: "critical", reasons: []string{"session_deleted"}, fingerprint: fingerprint, deletionTarget: true}); err != nil {
				return nil, err
			}
			continue
		}
		if item.AssignmentID.Valid {
			validation, validationErr := run.resolver.ValidateAssignment(ctx, item.ID, item.SitInSessionID)
			if validationErr != nil {
				return nil, validationErr
			}
			issueType := "sit_in_session_changed"
			severity := "warning"
			if !validation.Valid {
				issueType = issueTypeForReason(validation.Reasons)
				severity = "critical"
			}
			fingerprint := issueFingerprint(item.ID, issueType, run.change.SessionID, item.SitInSessionID, item.MissedSessionID)
			activeByAbsence[item.ID] = append(activeByAbsence[item.ID], fingerprint)
			if err := run.upsertIssue(ctx, issueInput{item: item, issueType: issueType, severity: severity, reasons: validation.Reasons, fingerprint: fingerprint}); err != nil {
				return nil, err
			}
		}
		if item.MissedSessionID.Valid {
			fingerprint := issueFingerprint(item.ID, "missed_session_changed", run.change.SessionID, item.SitInSessionID, item.MissedSessionID)
			activeByAbsence[item.ID] = append(activeByAbsence[item.ID], fingerprint)
			if err := run.upsertIssue(ctx, issueInput{item: item, issueType: "missed_session_changed", severity: "warning", fingerprint: fingerprint}); err != nil {
				return nil, err
			}
		}
	}
	if run.change.ChangeSource != "session_delete" {
		if err := run.addTimingIssues(ctx, settings.WarningHours, settings.CriticalHours, affected, activeByAbsence); err != nil {
			return nil, err
		}
	}
	if err := run.resolveSuperseded(ctx, affected, activeByAbsence); err != nil {
		return nil, err
	}
	return affected, nil
}

func (run analysisRun) addTimingIssues(ctx context.Context, warningHours, criticalHours int32, affected []sqldb.SessionChangeAffectedAbsencesRow, activeByAbsence map[pgtype.UUID][]string) error {
	if !run.change.NewStartAt.Valid {
		return nil
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
			if err := run.upsertIssue(ctx, issueInput{item: item, issueType: "short_notice_change", severity: severity, fingerprint: fingerprint}); err != nil {
				return err
			}
		}
	}
	if !run.change.NewStartAt.Time.After(run.now()) {
		for _, item := range affected {
			fingerprint := issueFingerprint(item.ID, "past_time_change", run.change.SessionID, item.SitInSessionID, item.MissedSessionID)
			activeByAbsence[item.ID] = append(activeByAbsence[item.ID], fingerprint)
			if err := run.upsertIssue(ctx, issueInput{item: item, issueType: "past_time_change", severity: "critical", fingerprint: fingerprint}); err != nil {
				return err
			}
		}
	}
	return nil
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
