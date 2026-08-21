package coursegroups

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

const RequiredMemberCount = 2

type CreateCommand struct {
	ActorID   pgtype.UUID
	Name      string
	CourseIDs []pgtype.UUID
}

type CreateResult struct {
	GroupID pgtype.UUID
}

func ValidateCreate(command CreateCommand) error {
	if strings.TrimSpace(command.Name) == "" {
		return &Error{Code: "invalid_name", Message: "A merged course name is required."}
	}
	if len(command.CourseIDs) != RequiredMemberCount || command.CourseIDs[0] == command.CourseIDs[1] {
		return &Error{Code: "invalid_course_ids", Message: "Select exactly two different courses to merge."}
	}
	return nil
}

type Service struct{}

func NewService() *Service { return &Service{} }

func (s *Service) CreateTx(ctx context.Context, qtx *sqldb.Queries, command CreateCommand) (CreateResult, error) {
	if err := ValidateCreate(command); err != nil {
		return CreateResult{}, err
	}

	courses, err := qtx.CourseMergeGroupLockCourses(ctx, command.CourseIDs)
	if err != nil {
		return CreateResult{}, err
	}
	if len(courses) != RequiredMemberCount {
		return CreateResult{}, &Error{Code: "course_not_found", Message: "One or more selected courses could not be found."}
	}
	for _, course := range courses {
		if course.MergeGroupID.Valid {
			return CreateResult{}, &Error{Code: "course_already_grouped", Message: "One of the selected courses is already in a merged course."}
		}
	}

	group, err := qtx.CourseMergeGroupCreate(ctx, strings.TrimSpace(command.Name), command.ActorID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return CreateResult{}, &Error{Code: "duplicate_group_name", Message: "A merged course with this name already exists."}
		}
		return CreateResult{}, err
	}
	for position, courseID := range command.CourseIDs {
		if err := qtx.CourseMergeGroupAssignCourse(ctx, group.ID, courseID, int16(position+1)); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return CreateResult{}, &Error{Code: "course_already_grouped", Message: "One of the selected courses is already in a merged course."}
			}
			return CreateResult{}, err
		}
	}
	if _, err := qtx.AuditInsert(ctx, sqldb.AuditInsertParams{
		ActorUserID: command.ActorID,
		Action:      "course_group.created",
		Payload: map[string]any{
			"group_id":   group.ID.String(),
			"course_ids": []string{command.CourseIDs[0].String(), command.CourseIDs[1].String()},
		},
	}); err != nil {
		return CreateResult{}, err
	}
	return CreateResult{GroupID: group.ID}, nil
}
