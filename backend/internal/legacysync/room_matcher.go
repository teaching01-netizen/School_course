package legacysync

import "strings"

func ExtractClassroomName(classroom string) string {
	classroom = strings.TrimSpace(classroom)
	if classroom == "" {
		return ""
	}
	if classroom == "[NOT SET]" {
		return ""
	}
	if len(classroom) > 0 && classroom[0] == '[' {
		// Format: [id] name
		idx := strings.Index(classroom, "] ")
		if idx >= 0 {
			return classroom[idx+2:]
		}
	}
	return classroom
}

func MatchRoom(classroom string, rooms []Room) *Room {
	name := ExtractClassroomName(classroom)
	if name == "" {
		return nil
	}

	// Exact match first
	for _, r := range rooms {
		if strings.EqualFold(r.Name, name) {
			return &r
		}
	}

	// Partial match (contains)
	var candidates []Room
	for _, r := range rooms {
		lowerName := strings.ToLower(name)
		lowerRoom := strings.ToLower(r.Name)
		if strings.Contains(lowerRoom, lowerName) || strings.Contains(lowerName, lowerRoom) {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 1 {
		return &candidates[0]
	}

	return nil
}
