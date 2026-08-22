package courseshttp

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/coursegroups"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
)

func (s *server) courseGroupResponse(ctx context.Context, q *sqldb.Queries, group sqldb.CourseMergeGroupRow) (map[string]any, error) {
	members, err := q.CourseMergeGroupMembers(ctx, group.ID)
	if err != nil {
		return nil, err
	}
	if len(members) != coursegroups.RequiredMemberCount {
		return nil, &coursegroups.Error{Code: "course_not_found", Message: "This merged course no longer has two source courses."}
	}
	ids := make([]pgtype.UUID, 0, len(members))
	memberNames := make(map[string]string, len(members))
	for _, member := range members {
		id, err := s.a.UUIDString(member.ID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, member.ID)
		memberNames[id] = member.Code
	}
	teacherRows, err := q.CourseTeachersListForCourses(ctx, ids)
	if err != nil {
		return nil, err
	}
	teachersByCourse := make(map[string][]map[string]any, len(members))
	mergedTeachers := make(map[string]map[string]any)
	for _, teacher := range teacherRows {
		courseID, err := s.a.UUIDString(teacher.CourseID)
		if err != nil {
			return nil, err
		}
		teacherID, err := s.a.UUIDString(teacher.TeacherID)
		if err != nil {
			return nil, err
		}
		fullName := any(nil)
		if teacher.FullName.Valid && teacher.FullName.String != "" {
			fullName = teacher.FullName.String
		}
		entry := map[string]any{"id": teacherID, "username": teacher.Username, "full_name": fullName, "is_primary": teacher.IsPrimary}
		teachersByCourse[courseID] = append(teachersByCourse[courseID], entry)
		merged, exists := mergedTeachers[teacherID]
		if !exists {
			merged = map[string]any{"id": teacherID, "username": teacher.Username, "full_name": fullName, "course_ids": []string{courseID}, "course_codes": []string{memberNames[courseID]}}
			mergedTeachers[teacherID] = merged
		} else {
			merged["course_ids"] = append(merged["course_ids"].([]string), courseID)
			merged["course_codes"] = append(merged["course_codes"].([]string), memberNames[courseID])
		}
	}
	memberDTOs := make([]map[string]any, 0, len(members))
	for _, member := range members {
		id, err := s.a.UUIDString(member.ID)
		if err != nil {
			return nil, err
		}
		courseTeachers := teachersByCourse[id]
		if courseTeachers == nil {
			courseTeachers = make([]map[string]any, 0)
		}
		memberDTOs = append(memberDTOs, map[string]any{
			"id":                   id,
			"code":                 member.Code,
			"name":                 member.Name,
			"subject_code":         member.SubjectCode,
			"subject_name":         member.SubjectName,
			"year":                 nullableInt16(member.Year),
			"hour":                 nullableInt32(member.Hour),
			"student_count":        nullableInt32(member.StudentCount),
			"course_type":          nullableText(member.CourseType),
			"cycle_id":             nullableText(member.CycleID),
			"root_course_group_id": nullableUUID(s.a, member.RootCourseGroupID),
			"legacy_course_id":     nullableText(member.LegacyCourseID),
			"legacy_archived":      member.LegacyArchived,
			"teachers":             courseTeachers,
		})
	}
	merged := make([]map[string]any, 0, len(mergedTeachers))
	for _, teacher := range mergedTeachers {
		merged = append(merged, teacher)
	}
	return map[string]any{"id": group.ID.String(), "name": group.Name, "members": memberDTOs, "teachers": merged}, nil
}

func nullableInt16(value pgtype.Int2) any {
	if value.Valid {
		return value.Int16
	}
	return nil
}

func nullableInt32(value pgtype.Int4) any {
	if value.Valid {
		return value.Int32
	}
	return nil
}

func nullableText(value pgtype.Text) any {
	if value.Valid {
		return value.String
	}
	return nil
}

func nullableUUID(a httpadapter.Adapter, value pgtype.UUID) any {
	if !value.Valid {
		return nil
	}
	id, err := a.UUIDString(value)
	if err != nil {
		return nil
	}
	return id
}
