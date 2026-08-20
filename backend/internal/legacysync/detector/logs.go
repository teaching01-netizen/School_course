package detector

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"time"
)

type EntityKind string

const (
	EntityCourse    EntityKind = "course"
	EntitySchedule  EntityKind = "schedule"
	EntityTeacher   EntityKind = "teacher"
	EntitySubject   EntityKind = "subject"
	EntityRoom      EntityKind = "room"
	EntityReconcile EntityKind = "reconcile"
)

type LogEntry struct {
	ID         string
	Action     string
	ObservedAt time.Time
}

type Target struct {
	Kind       EntityKind
	ExternalID string
	UniqueKey  string
	Priority   int
	Reason     string
}

var bracketID = regexp.MustCompile(`\[([^\]]+)\]`)

func ClassifyLogAction(action string) Target {
	normalized := strings.ToLower(strings.TrimSpace(action))
	parts := bracketID.FindStringSubmatch(action)
	externalID := ""
	if len(parts) > 1 {
		for _, value := range strings.Split(parts[1], ",") {
			value = strings.TrimSpace(value)
			if value != "" && !strings.HasPrefix(strings.ToLower(value), "w") {
				externalID = value
				break
			}
		}
	}
	kind := EntityReconcile
	switch {
	case strings.Contains(normalized, "teacher"):
		kind = EntityTeacher
	case strings.Contains(normalized, "subject"):
		kind = EntitySubject
	case strings.Contains(normalized, "classroom") || strings.Contains(normalized, "room"):
		kind = EntityRoom
	case strings.Contains(normalized, "schedule") || strings.Contains(normalized, "check in") || strings.Contains(normalized, "check-in"):
		kind = EntitySchedule
	case strings.Contains(normalized, "course"):
		kind = EntityCourse
	}
	if externalID == "" || kind == EntityReconcile {
		kind = EntityReconcile
		externalID = ""
	}
	return Target{Kind: kind, ExternalID: externalID, UniqueKey: targetKey(kind, externalID, action), Priority: priority(kind), Reason: action}
}

type LogDetector struct{ seenIDs map[string]struct{} }

func NewLogDetector() *LogDetector { return &LogDetector{seenIDs: make(map[string]struct{})} }

func (d *LogDetector) Observe(entries []LogEntry) []Target {
	targets := make([]Target, 0, len(entries))
	for _, entry := range entries {
		if entry.ID != "" {
			if _, seen := d.seenIDs[entry.ID]; seen {
				continue
			}
			d.seenIDs[entry.ID] = struct{}{}
		}
		targets = append(targets, ClassifyLogAction(entry.Action))
	}
	return targets
}

func CoalesceTargets(targets []Target, _ time.Duration) []Target {
	byKey := make(map[string]Target, len(targets))
	for _, target := range targets {
		current, ok := byKey[target.UniqueKey]
		if !ok || target.Priority < current.Priority {
			byKey[target.UniqueKey] = target
		}
	}
	result := make([]Target, 0, len(byKey))
	for _, target := range byKey {
		result = append(result, target)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UniqueKey < result[j].UniqueKey })
	return result
}

func targetKey(kind EntityKind, externalID, action string) string {
	if kind != EntityReconcile && externalID != "" {
		return "legacy:" + string(kind) + ":" + externalID
	}
	hash := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(action))))
	return "legacy:reconcile:" + hex.EncodeToString(hash[:8])
}

func priority(kind EntityKind) int {
	if kind == EntitySchedule {
		return 0
	}
	if kind == EntityCourse {
		return 1
	}
	return 2
}
