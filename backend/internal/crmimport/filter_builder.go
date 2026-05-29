package crmimport

import (
	"fmt"

	"warwick-institute/internal/crmimport/crmtypes"
)

// buildSnapshotFilterConditions builds SQL WHERE conditions and args from a CourseFilter.
// Uses the cr. table alias prefix for use in queries against crm_rows.
func buildSnapshotFilterConditions(filter crmtypes.CourseFilter) ([]string, []any) {
	conds := []string{"1=1"}
	args := []any{}
	argN := 1

	addIn := func(col string, values []string) {
		if len(values) == 0 {
			return
		}
		conds = append(conds, fmt.Sprintf("cr.%s = ANY($%d)", col, argN))
		args = append(args, values)
		argN++
	}

	addBlankMode := func(col string, mode crmtypes.BlankMode) {
		switch mode {
		case crmtypes.BlankModeAny:
		case crmtypes.BlankModeOnlyBlank:
			conds = append(conds, fmt.Sprintf("(cr.%s IS NULL OR btrim(cr.%s) = '')", col, col))
		case crmtypes.BlankModeOnlyValue:
			conds = append(conds, fmt.Sprintf("(cr.%s IS NOT NULL AND btrim(cr.%s) <> '')", col, col))
		}
	}

	addIn("cycle_label", filter.CycleLabels)
	addBlankMode("cycle_label", filter.CycleBlankMode)
	addIn("course_name", filter.CourseNameValues)
	addBlankMode("course_name", filter.CourseNameBlankMode)
	addIn("academic_level", filter.AcademicLevelValues)
	addBlankMode("academic_level", filter.AcademicLevelBlankMode)
	addIn("secondary_school", filter.SecondarySchoolValues)
	addBlankMode("secondary_school", filter.SecondarySchoolBlankMode)

	if filter.TeachersContains != "" {
		conds = append(conds, fmt.Sprintf("cr.teachers_raw ILIKE $%d", argN))
		args = append(args, "%"+filter.TeachersContains+"%")
		argN++
	}
	addBlankMode("teachers_raw", filter.TeachersBlankMode)

	return conds, args
}
