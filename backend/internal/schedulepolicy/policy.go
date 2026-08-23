package schedulepolicy

import (
	"context"
	"fmt"
	"time"

	sqldb "warwick-institute/internal/db"
)

type Scope string

const (
	ScopeSystem     Scope = "system"
	ScopeLegacySync Scope = "legacy_sync"
)

type Rule string

const (
	RuleRoomOverlap         Rule = "room_overlap"
	RuleTeacherOverlap      Rule = "teacher_overlap"
	RuleStudentOverlap      Rule = "student_overlap"
	RuleTeacherAvailability Rule = "teacher_availability"
	RuleRoomAvailability    Rule = "room_availability"
	RuleCourseOverlap       Rule = "course_sessions_overlap"
)

type RuleInfo struct {
	ID          Rule
	Label       string
	Description string
}

var controlledRules = []RuleInfo{
	{ID: RuleRoomOverlap, Label: "Room overlap", Description: "Two active sessions use the same room at overlapping times."},
	{ID: RuleTeacherOverlap, Label: "Teacher overlap", Description: "A teacher is assigned to overlapping active sessions."},
	{ID: RuleStudentOverlap, Label: "Student overlap", Description: "A student is rostered into overlapping active sessions."},
	{ID: RuleTeacherAvailability, Label: "Teacher availability", Description: "A session falls outside the teacher's configured availability."},
	{ID: RuleRoomAvailability, Label: "Room availability", Description: "A session falls outside the room's configured availability."},
	{ID: RuleCourseOverlap, Label: "Same-course session overlap", Description: "A course contains active sessions that overlap for the same roster."},
}

func ControlledRules() []RuleInfo {
	result := make([]RuleInfo, len(controlledRules))
	copy(result, controlledRules)
	return result
}

type Policy struct {
	SystemEnforced     bool
	LegacySyncEnforced bool
	UpdatedAt          time.Time
}

func (p Policy) Enforced(scope Scope) bool {
	switch scope {
	case ScopeSystem:
		return p.SystemEnforced
	case ScopeLegacySync:
		return p.LegacySyncEnforced
	default:
		return true
	}
}

type Reader interface {
	Load(context.Context, sqldb.DBTX) (Policy, error)
}

type DBReader struct{}

func NewDBReader() DBReader {
	return DBReader{}
}

func (DBReader) Load(ctx context.Context, db sqldb.DBTX) (Policy, error) {
	var policy Policy
	if err := db.QueryRow(ctx, `
		SELECT schedule_conflict_enforcement,
		       legacy_sync_conflict_enforcement,
		       updated_at
		FROM app_settings
		WHERE id = true
	`).Scan(&policy.SystemEnforced, &policy.LegacySyncEnforced, &policy.UpdatedAt); err != nil {
		return Policy{}, fmt.Errorf("load schedule conflict policy: %w", err)
	}
	return policy, nil
}
