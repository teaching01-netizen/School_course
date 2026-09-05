package absenceshttp

// Response assembly for the O(1) path (pure + bundle-backed, zero queries
// beyond the preloaded bundle).
//
// This mirrors the old per-course response loop exactly: session rows with
// institute-day dates + already_absent, per-course sit_in (bundle-resolved),
// staff all-subjects availability pools, and batched absence-day limits.
// The JSON shape (courseResponse/sessionResponse/courseSitInResponse) is
// unchanged; only the fact source differs.

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type sessionsRangeAssembled struct {
	order  []*courseGroupView
	scopes map[string]*sqldb.SessionsRangeScopeFactsRow
	counts sqldb.ScopeDayCounts
	merged map[string][2]string
	absent map[string]bool
	tz     string
}

// assembleCourseResponses builds the legacy courseResponse slice from grouped
// facts. sitInForCourse is injected so tests can stub resolution; production
// passes the bundle resolver.
func assembleCourseResponses(
	order []*courseGroupView,
	scopeByCourse map[string]*sqldb.SessionsRangeScopeFactsRow,
	counts sqldb.ScopeDayCounts,
	merged map[string][2]string,
	absent map[string]bool,
	tz string,
	sitInForCourse func(g *courseGroupView) *courseSitInJSON,
) []courseJSON {
	out := make([]courseJSON, 0, len(order))
	for _, g := range order {
		sessions := make([]sessionJSON, 0, len(g.sessions))
		for _, s := range g.sessions {
			sessions = append(sessions, sessionJSON{
				ID:            s.id,
				StartAt:       s.startAt.UTC().Format(time.RFC3339Nano),
				EndAt:         s.endAt.UTC().Format(time.RFC3339Nano),
				Date:          s.day,
				AlreadyAbsent: absent[s.id],
			})
		}
		scope := scopeByCourse[g.courseID]
		mergeID, mergeName := "", ""
		var dayCounts sqldb.AbsenceDayCounts
		if scope != nil {
			mergeName = scope.MergeGroupName
			if scope.Key.MergeGroup {
				mergeID = uuidStringOrZero(scope.Key.MergeGroupID)
			}
			dayCounts = counts[scope.Key.String()]
		}
		stats := limitStatsForScope(dayCounts)
		out = append(out, courseJSON{
			SubjectID:            g.subjectID,
			SubjectCode:          g.subjectCode,
			SubjectName:          g.subjectName,
			TeacherName:          g.teacherName,
			CourseID:             g.courseID,
			CourseCode:           g.courseCode,
			CourseName:           g.courseName,
			MergeGroupID:         mergeID,
			MergeGroupName:       mergeName,
			Sessions:             sessions,
			SitIn:                sitInForCourse(g),
			TotalCourseDays:      stats.TotalCourseDays,
			UsedAbsenceDays:      stats.UsedAbsenceDays,
			MaximumAbsenceDays:   stats.MaximumAbsenceDays,
			RemainingAbsenceDays: stats.RemainingAbsenceDays,
			AbsenceLimitReached:  stats.LimitReached,
		})
	}
	return out
}

// courseSitInJSON mirrors the legacy courseSitInResponse JSON shape.
type courseSitInJSON struct {
	RuleName             string                        `json:"rule_name,omitempty"`
	RuleType             string                        `json:"rule_type,omitempty"`
	SitInMethod          string                        `json:"sit_in_method"`
	Priorities           []SitInPriorityResult         `json:"priorities,omitempty"`
	CurrentPriorityLevel int                           `json:"current_priority_level,omitempty"`
	HasNextPriority      bool                          `json:"has_next_priority,omitempty"`
	SitInCourse          *SitInCourseInfo              `json:"sit_in_course,omitempty"`
	AvailableSessions    []sessionBrief                `json:"available_sessions,omitempty"`
	UnavailableSessions  []unavailableSessionBrief     `json:"unavailable_sessions,omitempty"`
	MissedSessions       []sessionBrief                `json:"missed_sessions,omitempty"`
	SitInByMissedSession map[string]SitInSessionResult `json:"sit_in_by_missed_session,omitempty"`
}

type sessionJSON struct {
	ID            string `json:"id"`
	StartAt       string `json:"start_at"`
	EndAt         string `json:"end_at"`
	Date          string `json:"date"`
	AlreadyAbsent bool   `json:"already_absent"`
}

type courseJSON struct {
	SubjectID            string           `json:"subject_id"`
	SubjectCode          string           `json:"subject_code"`
	SubjectName          string           `json:"subject_name"`
	TeacherName          string           `json:"teacher_name,omitempty"`
	CourseID             string           `json:"course_id"`
	CourseCode           string           `json:"course_code"`
	CourseName           string           `json:"course_name"`
	MergeGroupID         string           `json:"merge_group_id,omitempty"`
	MergeGroupName       string           `json:"merge_group_name,omitempty"`
	Sessions             []sessionJSON    `json:"sessions"`
	SitIn                *courseSitInJSON `json:"sit_in,omitempty"`
	TotalCourseDays      int32            `json:"total_course_days"`
	UsedAbsenceDays      int32            `json:"used_absence_days"`
	MaximumAbsenceDays   int32            `json:"maximum_absence_days"`
	RemainingAbsenceDays int32            `json:"remaining_absence_days"`
	AbsenceLimitReached  bool             `json:"absence_limit_reached"`
}

var _ = pgtype.UUID{}
