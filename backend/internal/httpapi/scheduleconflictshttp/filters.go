package scheduleconflictshttp

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type listFilters struct {
	Limit        int
	Offset       int
	ConflictType string
	SubjectID    string
	TeacherID    string
	StudentID    string
	DateFrom     string
	DateTo       string
	Query        string
}

type invalidFilterError struct {
	field string
	value string
}

func (e invalidFilterError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.field, e.value)
}

func parseListFilters(r *http.Request) (listFilters, error) {
	query := r.URL.Query()
	filters := listFilters{
		Limit:        50,
		ConflictType: strings.TrimSpace(query.Get("conflict_type")),
		SubjectID:    strings.TrimSpace(query.Get("subject_id")),
		TeacherID:    strings.TrimSpace(query.Get("teacher_id")),
		StudentID:    strings.TrimSpace(query.Get("student_id")),
		DateFrom:     strings.TrimSpace(query.Get("date_from")),
		DateTo:       strings.TrimSpace(query.Get("date_to")),
		Query:        strings.TrimSpace(query.Get("q")),
	}
	if filters.ConflictType == "all" {
		filters.ConflictType = ""
	}
	if filters.ConflictType != "" && filters.ConflictType != "room_overlap" && filters.ConflictType != "teacher_overlap" && filters.ConflictType != "student_overlap" {
		return listFilters{}, invalidFilterError{field: "conflict_type", value: filters.ConflictType}
	}
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return listFilters{}, invalidFilterError{field: "limit", value: raw}
		}
		filters.Limit = min(value, 200)
	}
	if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return listFilters{}, invalidFilterError{field: "offset", value: raw}
		}
		filters.Offset = value
	}
	for field, value := range map[string]string{"subject_id": filters.SubjectID, "teacher_id": filters.TeacherID, "student_id": filters.StudentID} {
		if value != "" {
			if _, err := uuid.Parse(value); err != nil {
				return listFilters{}, invalidFilterError{field: field, value: value}
			}
		}
	}
	for field, value := range map[string]string{"date_from": filters.DateFrom, "date_to": filters.DateTo} {
		if value != "" {
			if _, err := time.Parse(time.DateOnly, value); err != nil {
				return listFilters{}, invalidFilterError{field: field, value: value}
			}
		}
	}
	if filters.DateFrom != "" && filters.DateTo != "" && filters.DateFrom > filters.DateTo {
		return listFilters{}, invalidFilterError{field: "date_range", value: filters.DateFrom + ".." + filters.DateTo}
	}
	return filters, nil
}
