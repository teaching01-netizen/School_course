package db

import (
	"context"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// loadBundleEnrolledAndScopeUnion loads enrollments (tag 0), scope
// courses (tag 1), AND the active SAT verbal mappings list (tag 2) in ONE
// round trip. All arms project a common superset. ORDER BY tag, code
// keeps the course arms code-ordered; every tag-2 row carries code=”, so
// their relative order after the sort is unspecified — the loader sorts
// out.SatMappings by RuleID after the scan, restoring the
// SatVerbalPolicyMappingsList contract (ORDER BY m.rule_id ASC).
// The mapping arm selects the same columns in the same order as
// SatVerbalPolicyMappingsList, so the scanned structs are identical.
func (q *Queries) loadBundleEnrolledAndScopeUnion(ctx context.Context, arg SitInBundleFactsParams, out *SitInBundleFacts) error {
	rows, err := q.db.Query(ctx, loadBundleEnrolledAndScopeSQL(), arg.StudentID, arg.MissedCourseIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var tag int
		var subjCode, subjName, mergeName string
		var visible bool
		var id pgtype.UUID
		var code, name string
		var subjectID pgtype.UUID
		var cycle pgtype.Text
		var level pgtype.Int2
		var root, rule, merge pgtype.UUID
		var m SatVerbalPolicyCourseMapping
		if err := rows.Scan(&tag, &id, &code, &name, &subjectID, &subjCode, &subjName,
			&cycle, &level, &root, &rule, &merge, &mergeName, &visible,
			&m.ID, &m.RuleID, &m.CourseID, &m.MergeGroupID, &m.CourseCode, &m.CourseName, &m.SubjectID,
			&m.SubjectCode, &m.SubjectName, &m.CycleID, &m.Level, &m.RootCourseGroupID,
			&m.SitInRuleID, &m.PolicyRule, &m.PolicyHash, &m.Active, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return err
		}
		if tag == 0 {
			out.Enrolled = append(out.Enrolled, BundleEnrolledCourse{
				CourseID: id, CourseCode: code, CourseName: name, SubjectID: subjectID,
				CycleID: cycle, Level: level, RootCourseGroupID: root,
				SitInRuleID: rule, MergeGroupID: merge, AbsenceFormVisible: visible,
			})
		} else if tag == 1 {
			c := SubjectCourseV2{
				ID: id, Code: code, Name: name, SubjectID: subjectID,
				SubjectCode: subjCode, SubjectName: subjName,
				CycleID: cycle, Level: level, RootCourseGroupID: root,
				SitInRuleID: rule, MergeGroupID: merge,
			}
			out.ScopeCourses = append(out.ScopeCourses, c)
			if merge.Valid {
				out.MergeNames[uuidBytesString(merge)] = mergeName
			}
		} else if tag == 2 {
			out.SatMappings = append(out.SatMappings, m)
		} else {
			return &bundleUnionTagError{tag: tag}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	// Restore the SatVerbalPolicyMappingsList order contract
	// (ORDER BY m.rule_id ASC): the UNION ALL ORDER BY tag, code leaves
	// tag-2 rows in unspecified relative order (all codes '').
	sort.Slice(out.SatMappings, func(i, j int) bool { return out.SatMappings[i].RuleID < out.SatMappings[j].RuleID })
	out.satMappingsLoaded = true
	return nil
}

// bundleUnionTagError surfaces an unknown UNION ALL tag instead of
// silently dropping the row into the wrong collection.
type bundleUnionTagError struct{ tag int }

func (e *bundleUnionTagError) Error() string {
	return "sessions-range: unknown bundle union tag " + strconv.Itoa(e.tag)
}
