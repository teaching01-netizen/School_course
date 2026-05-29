package crmtypes

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ============================================================================
// Filter types
// ============================================================================

type BlankMode string

const (
	BlankModeAny       BlankMode = "any"
	BlankModeOnlyBlank BlankMode = "only_blank"
	BlankModeOnlyValue BlankMode = "only_value"
)

func (m BlankMode) Valid() bool {
	return m == BlankModeAny || m == BlankModeOnlyBlank || m == BlankModeOnlyValue
}

type CourseFilter struct {
	CycleLabels              []string  `json:"cycle_labels"`
	CycleBlankMode           BlankMode `json:"cycle_blank_mode"`
	CourseNameValues         []string  `json:"course_name_values"`
	CourseNameBlankMode      BlankMode `json:"course_name_blank_mode"`
	AcademicLevelValues      []string  `json:"academic_level_values"`
	AcademicLevelBlankMode   BlankMode `json:"academic_level_blank_mode"`
	SecondarySchoolValues    []string  `json:"secondary_school_values"`
	SecondarySchoolBlankMode BlankMode `json:"secondary_school_blank_mode"`
	TeachersContains         string    `json:"teachers_contains"`
	TeachersBlankMode        BlankMode `json:"teachers_blank_mode"`
}

func (f *CourseFilter) Normalize() {
	trimList := func(in []string) []string {
		out := make([]string, 0, len(in))
		seen := map[string]struct{}{}
		for _, v := range in {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
		sort.Strings(out)
		return out
	}
	f.CycleLabels = trimList(f.CycleLabels)
	f.CourseNameValues = trimList(f.CourseNameValues)
	f.AcademicLevelValues = trimList(f.AcademicLevelValues)
	f.SecondarySchoolValues = trimList(f.SecondarySchoolValues)
	f.TeachersContains = strings.TrimSpace(f.TeachersContains)
	if f.CycleBlankMode == "" {
		f.CycleBlankMode = BlankModeAny
	}
	if f.CourseNameBlankMode == "" {
		f.CourseNameBlankMode = BlankModeAny
	}
	if f.AcademicLevelBlankMode == "" {
		f.AcademicLevelBlankMode = BlankModeAny
	}
	if f.SecondarySchoolBlankMode == "" {
		f.SecondarySchoolBlankMode = BlankModeAny
	}
	if f.TeachersBlankMode == "" {
		f.TeachersBlankMode = BlankModeAny
	}
}

func (f CourseFilter) Validate() error {
	if !f.CycleBlankMode.Valid() || !f.CourseNameBlankMode.Valid() || !f.AcademicLevelBlankMode.Valid() || !f.SecondarySchoolBlankMode.Valid() || !f.TeachersBlankMode.Valid() {
		return fmt.Errorf("invalid blank mode")
	}
	return nil
}

// ============================================================================
// Reconcile types
// ============================================================================

type DiffAction string

const (
	DiffAdd    DiffAction = "add"
	DiffRemove DiffAction = "remove"
)

type PendingDiffRow struct {
	CourseID   pgtype.UUID `json:"course_id"`
	SnapshotID pgtype.UUID `json:"snapshot_id"`
	Action     DiffAction  `json:"diff_action"`
	Seq        int         `json:"seq"`
	StudentID  pgtype.UUID `json:"student_id,omitempty"`
	WCode      string      `json:"wcode"`
	FullName   string      `json:"full_name"`
}

type ReviewSummary struct {
	PendingSnapshotID string           `json:"pending_snapshot_id"`
	AddCount          int              `json:"add_count"`
	RemoveCount       int              `json:"remove_count"`
	FirstPage         []PendingDiffRow `json:"first_page,omitempty"`
}

type CourseReconcilePayload struct {
	SnapshotID            uuid.UUID `json:"snapshot_id"`
	CourseID              uuid.UUID `json:"course_id"`
	ExpectedFilterVersion int       `json:"expected_filter_version"`
	ReenqueueCount        int       `json:"reenqueue_count,omitempty"`
}
