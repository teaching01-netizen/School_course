package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// loadBundleEnrolledAndScopeUnion loads enrollments AND scope courses in ONE
// round trip (UNION ALL with a tag column). Both arms project a common
// superset; per-arm order matches the old loaders (ORDER BY tag, code keeps
// each arm code-ordered).
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
		if err := rows.Scan(&tag, &id, &code, &name, &subjectID, &subjCode, &subjName,
			&cycle, &level, &root, &rule, &merge, &mergeName, &visible); err != nil {
			return err
		}
		if tag == 0 {
			out.Enrolled = append(out.Enrolled, BundleEnrolledCourse{
				CourseID: id, CourseCode: code, CourseName: name, SubjectID: subjectID,
				CycleID: cycle, Level: level, RootCourseGroupID: root,
				SitInRuleID: rule, MergeGroupID: merge, AbsenceFormVisible: visible,
			})
		} else {
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
		}
	}
	return rows.Err()
}
