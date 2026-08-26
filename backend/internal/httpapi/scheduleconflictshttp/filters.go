package scheduleconflictshttp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	cursorNext = "next"
	cursorPrev = "prev"
)

type conflictCursor struct {
	StartAt       time.Time `json:"s"`
	ConflictType  string    `json:"t"`
	PrimaryID     uuid.UUID `json:"p"`
	ConflictingID uuid.UUID `json:"c"`
	Direction     string    `json:"d"`
}

type listFilters struct {
	Limit        int
	ConflictType string
	SubjectID    *uuid.UUID
	TeacherID    *uuid.UUID
	StudentID    *uuid.UUID
	DateFrom     *time.Time
	DateTo       *time.Time
	Query        string
	Cursor       *conflictCursor
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
		Query:        strings.TrimSpace(query.Get("q")),
	}
	if filters.ConflictType == "all" {
		filters.ConflictType = ""
	}
	if filters.ConflictType != "" && !validConflictType(filters.ConflictType) {
		return listFilters{}, invalidFilterError{field: "conflict_type", value: filters.ConflictType}
	}
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return listFilters{}, invalidFilterError{field: "limit", value: raw}
		}
		filters.Limit = min(value, 200)
	}

	var err error
	if filters.SubjectID, err = parseOptionalUUID(query.Get("subject_id")); err != nil {
		return listFilters{}, invalidFilterError{field: "subject_id", value: query.Get("subject_id")}
	}
	if filters.TeacherID, err = parseOptionalUUID(query.Get("teacher_id")); err != nil {
		return listFilters{}, invalidFilterError{field: "teacher_id", value: query.Get("teacher_id")}
	}
	if filters.StudentID, err = parseOptionalUUID(query.Get("student_id")); err != nil {
		return listFilters{}, invalidFilterError{field: "student_id", value: query.Get("student_id")}
	}
	if filters.DateFrom, err = parseOptionalDate(query.Get("date_from")); err != nil {
		return listFilters{}, invalidFilterError{field: "date_from", value: query.Get("date_from")}
	}
	if filters.DateTo, err = parseOptionalDate(query.Get("date_to")); err != nil {
		return listFilters{}, invalidFilterError{field: "date_to", value: query.Get("date_to")}
	}
	if filters.DateFrom != nil && filters.DateTo != nil && filters.DateFrom.After(*filters.DateTo) {
		return listFilters{}, invalidFilterError{field: "date_range", value: query.Get("date_from") + ".." + query.Get("date_to")}
	}
	if raw := strings.TrimSpace(query.Get("cursor")); raw != "" {
		filters.Cursor, err = decodeCursor(raw)
		if err != nil {
			return listFilters{}, invalidFilterError{field: "cursor", value: raw}
		}
	}
	return filters, nil
}

func parseOptionalUUID(raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := uuid.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func parseOptionalDate(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func validConflictType(value string) bool {
	return value == "room_overlap" || value == "teacher_overlap" || value == "student_overlap"
}

func encodeCursor(value conflictCursor) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeCursor(raw string) (*conflictCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	var value conflictCursor
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, err
	}
	if value.StartAt.IsZero() || !validConflictType(value.ConflictType) || value.PrimaryID == uuid.Nil || value.ConflictingID == uuid.Nil || (value.Direction != cursorNext && value.Direction != cursorPrev) {
		return nil, fmt.Errorf("invalid cursor payload")
	}
	return &value, nil
}

func cursorFor(item conflictDTO, direction string) (string, error) {
	startAt, err := time.Parse(time.RFC3339Nano, item.PrimarySession.StartAt)
	if err != nil {
		return "", fmt.Errorf("parse conflict cursor start: %w", err)
	}
	primaryID, err := uuid.Parse(item.PrimarySession.SessionID)
	if err != nil {
		return "", fmt.Errorf("parse conflict cursor primary session: %w", err)
	}
	conflictingID, err := uuid.Parse(item.ConflictingSessions[0].SessionID)
	if err != nil {
		return "", fmt.Errorf("parse conflict cursor conflicting session: %w", err)
	}
	return encodeCursor(conflictCursor{
		StartAt:       startAt,
		ConflictType:  item.ConflictType,
		PrimaryID:     primaryID,
		ConflictingID: conflictingID,
		Direction:     direction,
	})
}
