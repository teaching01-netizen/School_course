package courseadmin

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// CourseTeacherResponse is one member of a course's teacher set.
type CourseTeacherResponse struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	IsPrimary bool   `json:"is_primary"`
}

// CourseResponse is the shared representation of a course used for the
// stale_edit "current" payload and as the success response of teacher-set
// updates. primary_teacher_id mirrors courses.teacher_id (the compatibility
// projection of the primary assignment).
type CourseResponse struct {
	ID               string                  `json:"id"`
	Version          int32                   `json:"version"`
	Code             string                  `json:"code"`
	Name             string                  `json:"name"`
	PrimaryTeacherID *string                 `json:"primary_teacher_id"`
	Teachers         []CourseTeacherResponse `json:"teachers"`
}

// loadCourseResponse assembles the current course representation (core row
// plus teacher set) inside the caller's transaction.
func loadCourseResponse(ctx context.Context, qtx *sqldb.Queries, courseID pgtype.UUID) (*CourseResponse, error) {
	course, err := qtx.CourseGetCoreByID(ctx, courseID)
	if err != nil {
		return nil, classifyCourseReadError(err)
	}

	rows, err := qtx.CourseTeachersList(ctx, courseID)
	if err != nil {
		return nil, err
	}

	resp := &CourseResponse{
		ID:      course.ID.String(),
		Version: course.Version,
		Code:    course.Code,
		Name:    course.Name,
	}
	if course.TeacherID.Valid {
		primary := course.TeacherID.String()
		resp.PrimaryTeacherID = &primary
	}
	resp.Teachers = make([]CourseTeacherResponse, 0, len(rows))
	for _, row := range rows {
		resp.Teachers = append(resp.Teachers, CourseTeacherResponse{
			ID:        row.TeacherID.String(),
			Username:  row.Username,
			IsPrimary: row.IsPrimary,
		})
	}
	return resp, nil
}

// GetCourseResponse returns the current course representation (core row plus
// teacher set) inside the caller's transaction. It is the exported seam that
// lets the HTTP layer assemble success payloads after a service mutation
// without duplicating the teacher-assembly SQL.
func (s *Service) GetCourseResponse(ctx context.Context, qtx *sqldb.Queries, courseID pgtype.UUID) (*CourseResponse, error) {
	return loadCourseResponse(ctx, qtx, courseID)
}
