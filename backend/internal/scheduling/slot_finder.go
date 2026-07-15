package scheduling

import "time"

func ordinalityToIndex(ordinal int64, size int) (int, bool) {
	idx := int(ordinal - 1)
	return idx, ordinal > 0 && idx >= 0 && idx < size
}

type candidateSlot struct {
	Start time.Time
	End   time.Time
}

func generateHourlySlots(day time.Time, startHour, endHour, durationMinutes int) []candidateSlot {
	dayEnd := time.Date(day.Year(), day.Month(), day.Day(), endHour, 0, 0, 0, day.Location())
	var out []candidateSlot
	for hour := startHour; hour < endHour; hour++ {
		start := time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, day.Location())
		end := start.Add(time.Duration(durationMinutes) * time.Minute)
		if end.After(dayEnd) {
			continue
		}
		out = append(out, candidateSlot{Start: start, End: end})
	}
	return out
}
