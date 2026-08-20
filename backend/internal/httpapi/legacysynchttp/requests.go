package legacysynchttp

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type refreshRequest struct {
	EntityType string `json:"entity_type"`
	ExternalID string `json:"external_id"`
	Priority   *int32 `json:"priority"`
}

type refreshJob struct {
	JobType    string
	EntityType string
	ExternalID string
	Priority   int32
	UniqueKey  string
}

func validateRefreshRequest(request refreshRequest) (refreshJob, error) {
	entityType := strings.ToLower(strings.TrimSpace(request.EntityType))
	externalID := strings.TrimSpace(request.ExternalID)
	jobType := map[string]string{
		"course":    "legacy_refresh_course",
		"schedule":  "legacy_refresh_schedule",
		"checkin":   "legacy_refresh_checkin",
		"teacher":   "legacy_refresh_teacher_list",
		"subject":   "legacy_refresh_subject_list",
		"classroom": "legacy_refresh_classroom_list",
		"student":   "legacy_refresh_student",
		"full":      "legacy_full_reconcile",
	}[entityType]
	if jobType == "" {
		return refreshJob{}, fmt.Errorf("entity_type is not refreshable")
	}
	if entityType != "full" && externalID == "" {
		return refreshJob{}, fmt.Errorf("external_id is required")
	}
	priority := int32(2)
	if entityType == "full" {
		priority = 5
	}
	if request.Priority != nil {
		priority = *request.Priority
	}
	if priority < 0 || priority > 100 {
		return refreshJob{}, fmt.Errorf("priority must be between 0 and 100")
	}
	key := "legacy:" + entityType
	if externalID != "" {
		key += ":" + externalID
	}
	return refreshJob{JobType: jobType, EntityType: entityType, ExternalID: externalID, Priority: priority, UniqueKey: key}, nil
}

func limitFromRequest(r *http.Request, defaultLimit, maxLimit int32) int32 {
	value, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 32)
	if err != nil || value <= 0 {
		return defaultLimit
	}
	if value > int64(maxLimit) {
		return maxLimit
	}
	return int32(value)
}
