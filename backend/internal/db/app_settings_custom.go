package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

func (q *Queries) DBTX() DBTX {
	return q.db
}

type SessionChangeSettings struct {
	SmsEnabled          bool
	SmsTemplate         string
	EmailEnabled        bool
	EmailSubject        string
	EmailBody           string
	AutoNotifySafeMoves bool
	WarningHours        int32
	CriticalHours       int32
	AllowMoveIntoPast   bool
}

func (q *Queries) AppSettingsGetSessionChangeSettings(ctx context.Context) (SessionChangeSettings, error) {
	var settings SessionChangeSettings
	err := q.db.QueryRow(ctx, `
		SELECT sit_in_change_sms_enabled, sit_in_change_sms_template,
		       sit_in_change_email_enabled, sit_in_change_email_subject,
		       sit_in_change_email_body, sit_in_change_auto_notify_safe_moves,
		       sit_in_change_warning_hours, sit_in_change_critical_hours,
		       allow_move_into_past
		FROM app_settings
		WHERE id = true
	`).Scan(
		&settings.SmsEnabled,
		&settings.SmsTemplate,
		&settings.EmailEnabled,
		&settings.EmailSubject,
		&settings.EmailBody,
		&settings.AutoNotifySafeMoves,
		&settings.WarningHours,
		&settings.CriticalHours,
		&settings.AllowMoveIntoPast,
	)
	return settings, err
}

type AppSettingWithPolicies struct {
	ID              bool               `json:"id"`
	InstituteTz     string             `json:"institute_tz"`
	AbsencePolicies []byte             `json:"absence_policies"`
	CreatedAt       pgtype.Timestamptz `json:"created_at"`
	UpdatedAt       pgtype.Timestamptz `json:"updated_at"`
}

func (q *Queries) AppSettingsGetWithPolicies(ctx context.Context) (AppSettingWithPolicies, error) {
	var s AppSettingWithPolicies
	err := q.db.QueryRow(ctx, `
		SELECT id, institute_tz, absence_policies, created_at, updated_at
		FROM app_settings
		WHERE id = true
	`).Scan(&s.ID, &s.InstituteTz, &s.AbsencePolicies, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (q *Queries) AppSettingsUpdateAbsencePolicies(ctx context.Context, policies []byte) error {
	_, err := q.db.Exec(ctx, `
		UPDATE app_settings
		SET absence_policies = $1::jsonb, updated_at = now()
		WHERE id = true
	`, string(policies))
	return err
}

type ScheduleConflictAuditRow struct {
	ID          int64
	CreatedAt   pgtype.Timestamptz
	ActorUserID pgtype.UUID
	ActorName   string
	Action      string
	Payload     []byte
}

type ScheduleConflictPolicyRow struct {
	SystemEnforced     bool
	LegacySyncEnforced bool
	UpdatedAt          pgtype.Timestamptz
}

func (q *Queries) ScheduleConflictPolicyGet(ctx context.Context) (ScheduleConflictPolicyRow, error) {
	var row ScheduleConflictPolicyRow
	err := q.db.QueryRow(ctx, `
		SELECT schedule_conflict_enforcement, legacy_sync_conflict_enforcement, updated_at
		FROM app_settings
		WHERE id = true
	`).Scan(&row.SystemEnforced, &row.LegacySyncEnforced, &row.UpdatedAt)
	return row, err
}

func (q *Queries) ScheduleConflictPolicyGetForUpdate(ctx context.Context) (ScheduleConflictPolicyRow, error) {
	var row ScheduleConflictPolicyRow
	err := q.db.QueryRow(ctx, `
		SELECT schedule_conflict_enforcement, legacy_sync_conflict_enforcement, updated_at
		FROM app_settings
		WHERE id = true
		FOR UPDATE
	`).Scan(&row.SystemEnforced, &row.LegacySyncEnforced, &row.UpdatedAt)
	return row, err
}

func (q *Queries) ScheduleConflictPolicyUpdate(ctx context.Context, systemEnforced, legacySyncEnforced bool) (ScheduleConflictPolicyRow, error) {
	var row ScheduleConflictPolicyRow
	err := q.db.QueryRow(ctx, `
		UPDATE app_settings
		SET schedule_conflict_enforcement = $1,
		    legacy_sync_conflict_enforcement = $2,
		    updated_at = now()
		WHERE id = true
		RETURNING schedule_conflict_enforcement, legacy_sync_conflict_enforcement, updated_at
	`, systemEnforced, legacySyncEnforced).Scan(&row.SystemEnforced, &row.LegacySyncEnforced, &row.UpdatedAt)
	return row, err
}

func (q *Queries) AppSettingsScheduleConflictHistory(ctx context.Context) ([]ScheduleConflictAuditRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT a.id, a.created_at, a.actor_user_id, COALESCE(u.username, 'system'), a.action, a.payload
		FROM audit_log a
		LEFT JOIN users u ON u.id = a.actor_user_id
		WHERE a.action = 'schedule_conflict_policy.updated'
		  AND a.created_at >= now() - interval '3 days'
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ScheduleConflictAuditRow
	for rows.Next() {
		var row ScheduleConflictAuditRow
		if err := rows.Scan(&row.ID, &row.CreatedAt, &row.ActorUserID, &row.ActorName, &row.Action, &row.Payload); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

type AbsencePolicies struct {
	Subjects         map[string]SubjectPolicy `json:"subjects"`
	RootCourseGroups map[string]SubjectPolicy `json:"root_course_groups"`
	MergeGroups      map[string]SubjectPolicy `json:"merge_groups"`
	Zoom             ZoomConfig               `json:"zoom"`
	SitIn            SitInPolicyConfig        `json:"sit_in"`
}

type SubjectPolicy struct {
	AutoSitInEnabled bool              `json:"auto_sit_in_enabled"`
	SitInWindowWeeks int               `json:"sit_in_window_weeks"`
	LevelActionMap   map[string]string `json:"level_action_map"`
}

type ZoomConfig struct {
	Description string `json:"description"`
}

type SitInPolicyConfig struct {
	AutoResolveEnabled *bool  `json:"auto_resolve_enabled"`
	ZoomDescription    string `json:"zoom_description"`
}

func DefaultAbsencePolicies() AbsencePolicies {
	enabled := true
	return AbsencePolicies{
		Subjects:         map[string]SubjectPolicy{},
		RootCourseGroups: map[string]SubjectPolicy{},
		MergeGroups:      map[string]SubjectPolicy{},
		Zoom: ZoomConfig{
			Description: "Zoom session — no physical class attendance required.",
		},
		SitIn: SitInPolicyConfig{AutoResolveEnabled: &enabled},
	}
}
