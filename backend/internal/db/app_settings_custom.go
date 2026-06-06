package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

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

type AbsencePolicies struct {
	Subjects         map[string]SubjectPolicy `json:"subjects"`
	RootCourseGroups map[string]SubjectPolicy `json:"root_course_groups"`
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
		Zoom: ZoomConfig{
			Description: "Zoom session — no physical class attendance required.",
		},
		SitIn: SitInPolicyConfig{AutoResolveEnabled: &enabled},
	}
}
