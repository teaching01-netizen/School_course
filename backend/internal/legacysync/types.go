package legacysync

import "time"

type ScheduleRow struct {
	Date      string
	Begin     string
	End       string
	Duration  string
	Classroom string
}

type ParsedRow struct {
	Date      time.Time
	Begin     string
	End       string
	Duration  string
	Classroom string
}

type Room struct {
	ID   string
	Name string
}
