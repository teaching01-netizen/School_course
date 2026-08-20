package detector

import (
	"sort"
	"strings"

	"warwick-institute/internal/legacysync/normalize"
)

type ScheduleRow struct {
	CourseID   string `json:"course_id"`
	ScheduleID string `json:"schedule_id"`
	Start      string `json:"start"`
	End        string `json:"end"`
	RoomID     string `json:"room_id"`
	Confirmed  bool   `json:"confirmed"`
}

type ScheduleDetector struct {
	lastHash string
	lastRows map[string]ScheduleRow
}

func NewScheduleDetector() *ScheduleDetector {
	return &ScheduleDetector{lastRows: make(map[string]ScheduleRow)}
}

func (d *ScheduleDetector) Observe(rows []ScheduleRow) bool {
	return len(d.ObserveTargets(rows)) > 0
}

func (d *ScheduleDetector) ObserveTargets(rows []ScheduleRow) []Target {
	canonical := append([]ScheduleRow(nil), rows...)
	sort.Slice(canonical, func(i, j int) bool {
		if canonical[i].CourseID != canonical[j].CourseID {
			return canonical[i].CourseID < canonical[j].CourseID
		}
		return scheduleKey(canonical[i]) < scheduleKey(canonical[j])
	})
	hash, err := normalize.HashCanonical(canonical)
	if err != nil || hash == d.lastHash {
		return nil
	}
	previous := d.lastRows
	current := make(map[string]ScheduleRow, len(rows))
	changedCourses := make(map[string]struct{})
	for _, row := range rows {
		key := scheduleKey(row)
		current[key] = row
		if old, ok := previous[key]; !ok || old != row {
			if row.CourseID != "" {
				changedCourses[row.CourseID] = struct{}{}
			}
		}
	}
	for key, old := range previous {
		if _, ok := current[key]; !ok && old.CourseID != "" {
			changedCourses[old.CourseID] = struct{}{}
		}
	}
	d.lastHash = hash
	d.lastRows = current
	targets := make([]Target, 0, len(changedCourses))
	for courseID := range changedCourses {
		targets = append(targets, Target{Kind: EntityCourse, ExternalID: courseID, UniqueKey: "legacy:course:" + courseID, Priority: priority(EntityCourse), Reason: "today schedule changed"})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].UniqueKey < targets[j].UniqueKey })
	return targets
}

func scheduleKey(row ScheduleRow) string {
	if row.ScheduleID != "" {
		return row.ScheduleID
	}
	return strings.Join([]string{row.CourseID, row.Start, row.End, row.RoomID}, "|")
}
