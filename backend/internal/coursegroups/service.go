package coursegroups

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
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

type DeleteCommand struct {
	ActorID pgtype.UUID
	GroupID pgtype.UUID
}

type DeleteResult struct {
	GroupID   pgtype.UUID
	GroupName string
	CourseIDs []pgtype.UUID
}

type UpdateNameCommand struct {
	ActorID pgtype.UUID
	GroupID pgtype.UUID
	Name    string
}

type UpdateNameResult struct {
	GroupID pgtype.UUID
	OldName string
	NewName string
}

func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return &Error{Code: "invalid_name", Message: "A merged course name is required."}
	}
	return nil
}

func ValidateCreate(command CreateCommand) error {
	if err := ValidateName(command.Name); err != nil {
		return err
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

func (s *Service) UpdateNameTx(ctx context.Context, qtx *sqldb.Queries, command UpdateNameCommand) (UpdateNameResult, error) {
	if err := ValidateName(command.Name); err != nil {
		return UpdateNameResult{}, err
	}

	group, err := qtx.CourseMergeGroupGetForUpdate(ctx, command.GroupID)
	if errors.Is(err, pgx.ErrNoRows) {
		return UpdateNameResult{}, &Error{Code: "not_found", Message: "Merged course not found."}
	}
	if err != nil {
		return UpdateNameResult{}, err
	}

	newName := strings.TrimSpace(command.Name)
	if group.Name == newName {
		return UpdateNameResult{GroupID: group.ID, OldName: group.Name, NewName: group.Name}, nil
	}
	if err := qtx.CourseMergeGroupUpdateName(ctx, command.GroupID, newName); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return UpdateNameResult{}, &Error{Code: "duplicate_group_name", Message: "A merged course with this name already exists."}
		}
		return UpdateNameResult{}, err
	}
	if _, err := qtx.AuditInsert(ctx, sqldb.AuditInsertParams{
		ActorUserID: command.ActorID,
		Action:      "course_group.renamed",
		Payload: map[string]any{
			"group_id": command.GroupID.String(),
			"old_name": group.Name,
			"new_name": newName,
		},
	}); err != nil {
		return UpdateNameResult{}, err
	}
	return UpdateNameResult{GroupID: group.ID, OldName: group.Name, NewName: newName}, nil
}

func (s *Service) DeleteTx(ctx context.Context, qtx *sqldb.Queries, command DeleteCommand) (DeleteResult, error) {
	group, err := qtx.CourseMergeGroupGet(ctx, command.GroupID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeleteResult{}, &Error{Code: "not_found", Message: "Merged course not found."}
	}
	if err != nil {
		return DeleteResult{}, err
	}

	members, err := qtx.CourseMergeGroupMembers(ctx, command.GroupID)
	if err != nil {
		return DeleteResult{}, err
	}
	initialCourseIDs := make([]pgtype.UUID, 0, len(members))
	for _, member := range members {
		initialCourseIDs = append(initialCourseIDs, member.ID)
	}
	if len(initialCourseIDs) > 0 {
		if _, err := qtx.CourseMergeGroupLockCourses(ctx, initialCourseIDs); err != nil {
			return DeleteResult{}, err
		}
	}

	group, err = qtx.CourseMergeGroupGetForUpdate(ctx, command.GroupID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeleteResult{}, &Error{Code: "not_found", Message: "Merged course not found."}
	}
	if err != nil {
		return DeleteResult{}, err
	}
	members, err = qtx.CourseMergeGroupMembers(ctx, command.GroupID)
	if err != nil {
		return DeleteResult{}, err
	}

	courseIDs := make([]pgtype.UUID, 0, len(members))
	courseIDStrings := make([]string, 0, len(members))
	for _, member := range members {
		courseIDs = append(courseIDs, member.ID)
		courseIDStrings = append(courseIDStrings, member.ID.String())
	}
	if err := qtx.CourseMergeGroupDelete(ctx, command.GroupID); err != nil {
		return DeleteResult{}, err
	}
	if _, err := qtx.AuditInsert(ctx, sqldb.AuditInsertParams{
		ActorUserID: command.ActorID,
		Action:      "course_group.unmerged",
		Payload: map[string]any{
			"group_id":   command.GroupID.String(),
			"group_name": group.Name,
			"course_ids": courseIDStrings,
		},
	}); err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{GroupID: command.GroupID, GroupName: group.Name, CourseIDs: courseIDs}, nil
}
