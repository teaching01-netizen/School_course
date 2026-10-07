package coursegroups

import (
	"context"
	"errors"
	"strings"
	"time"

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
	// RuleSourceCourseID marks a continuation: both IDs are one real course
	// and read their rules live from this member. Zero keeps a plain merge.
	RuleSourceCourseID pgtype.UUID
	Suggestion         *SuggestionPrecondition
}

type SuggestionPrecondition struct {
	SourceCourseID      pgtype.UUID
	PartialCourseID     pgtype.UUID
	EvidenceFingerprint string
	DetectorVersion     string
	InstituteTZ         string
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
	if command.RuleSourceCourseID.Valid && command.RuleSourceCourseID != command.CourseIDs[0] && command.RuleSourceCourseID != command.CourseIDs[1] {
		return &Error{Code: "invalid_rule_source", Message: "The rule source must be one of the two linked courses."}
	}
	return nil
}

type Service struct{}

func NewService() *Service { return &Service{} }

func (s *Service) CreateTx(ctx context.Context, qtx *sqldb.Queries, command CreateCommand) (CreateResult, error) {
	if err := ValidateCreate(command); err != nil {
		return CreateResult{}, err
	}
	var suggestionEvidence *sqldb.CourseLinkSuggestionRow
	if command.Suggestion != nil {
		precondition := command.Suggestion
		if !precondition.SourceCourseID.Valid || !precondition.PartialCourseID.Valid ||
			precondition.SourceCourseID == precondition.PartialCourseID ||
			precondition.SourceCourseID != command.RuleSourceCourseID ||
			(precondition.SourceCourseID != command.CourseIDs[0] && precondition.SourceCourseID != command.CourseIDs[1]) ||
			(precondition.PartialCourseID != command.CourseIDs[0] && precondition.PartialCourseID != command.CourseIDs[1]) ||
			precondition.DetectorVersion != sqldb.CourseLinkSuggestionDetectorVersion ||
			precondition.EvidenceFingerprint == "" {
			return CreateResult{}, &Error{Code: "suggestion_changed", Message: "This suggestion changed. Refresh it and make a new decision."}
		}
		if err := qtx.CourseLinkPairLock(ctx, precondition.SourceCourseID, precondition.PartialCourseID); err != nil {
			return CreateResult{}, err
		}
	}
	if command.RuleSourceCourseID.Valid {
		if err := qtx.AdvisoryLockForText(ctx, "sat-verbal-policy:course-rules"); err != nil {
			return CreateResult{}, err
		}
	}

	courses, err := qtx.CourseMergeGroupLockCourses(ctx, command.CourseIDs)
	if err != nil {
		return CreateResult{}, err
	}
	if len(courses) != RequiredMemberCount {
		if command.Suggestion != nil {
			return CreateResult{}, changedSuggestionError()
		}
		return CreateResult{}, &Error{Code: "course_not_found", Message: "One or more selected courses could not be found."}
	}
	memberGroups, err := qtx.CourseMergeGroupMembershipsForCourses(ctx, command.CourseIDs)
	if err != nil {
		return CreateResult{}, err
	}
	if len(memberGroups) > 0 {
		if command.Suggestion != nil {
			return CreateResult{}, changedSuggestionError()
		}
		return CreateResult{}, &Error{Code: "course_already_grouped", Message: "One of the selected courses is already in a merged course."}
	}
	if command.RuleSourceCourseID.Valid {
		for _, courseID := range command.CourseIDs {
			_, err := qtx.SatVerbalPolicyMappingGetActiveByCourse(ctx, courseID)
			if err == nil {
				if command.Suggestion != nil {
					return CreateResult{}, changedSuggestionError()
				}
				return CreateResult{}, &Error{Code: "sat_verbal_course", Message: "SAT Verbal policy courses cannot be linked as a continuation."}
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return CreateResult{}, err
			}
		}
	}
	if command.Suggestion != nil {
		queryCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		rows, err := qtx.CourseLinkSuggestions(queryCtx, sqldb.CourseLinkSuggestionQuery{
			InstituteTZ:     command.Suggestion.InstituteTZ,
			IncludeReview:   true,
			SourceCourseID:  command.Suggestion.SourceCourseID,
			PartialCourseID: command.Suggestion.PartialCourseID,
			Limit:           1,
		})
		if err != nil {
			return CreateResult{}, err
		}
		if len(rows) != 1 || rows[0].Confidence != "high" || rows[0].EvidenceFingerprint != command.Suggestion.EvidenceFingerprint {
			return CreateResult{}, changedSuggestionError()
		}
		suggestionEvidence = &rows[0]
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
	auditPayload := map[string]any{
		"group_id":   group.ID.String(),
		"course_ids": []string{command.CourseIDs[0].String(), command.CourseIDs[1].String()},
	}
	if command.RuleSourceCourseID.Valid {
		if err := qtx.CourseMergeGroupSetRuleSource(ctx, group.ID, command.RuleSourceCourseID); err != nil {
			return CreateResult{}, err
		}
		auditPayload["rule_source_course_id"] = command.RuleSourceCourseID.String()
	}
	if suggestionEvidence != nil {
		auditPayload["suggestion_detector_version"] = sqldb.CourseLinkSuggestionDetectorVersion
		auditPayload["suggestion_evidence_fingerprint"] = suggestionEvidence.EvidenceFingerprint
		auditPayload["suggestion_reason_codes"] = suggestionEvidence.ReasonCodes
		auditPayload["suggestion_confidence"] = suggestionEvidence.Confidence
	}
	if _, err := qtx.AuditInsert(ctx, sqldb.AuditInsertParams{
		ActorUserID: command.ActorID,
		Action:      "course_group.created",
		Payload:     auditPayload,
	}); err != nil {
		return CreateResult{}, err
	}
	return CreateResult{GroupID: group.ID}, nil
}

func changedSuggestionError() *Error {
	return &Error{Code: "suggestion_changed", Message: "This suggestion changed. Refresh it and make a new decision."}
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
	if group.RuleSourceCourseID.Valid {
		hasAbsences, err := qtx.CourseMergeGroupHasAbsences(ctx, command.GroupID)
		if err != nil {
			return DeleteResult{}, err
		}
		if hasAbsences {
			return DeleteResult{}, &Error{Code: "continuation_in_use", Message: "Absences already use this linked course, so it cannot be unlinked."}
		}
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
	auditPayload := map[string]any{
		"group_id":   command.GroupID.String(),
		"group_name": group.Name,
		"course_ids": courseIDStrings,
	}
	if group.RuleSourceCourseID.Valid {
		auditPayload["rule_source_course_id"] = group.RuleSourceCourseID.String()
	}
	if _, err := qtx.AuditInsert(ctx, sqldb.AuditInsertParams{
		ActorUserID: command.ActorID,
		Action:      "course_group.unmerged",
		Payload:     auditPayload,
	}); err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{GroupID: command.GroupID, GroupName: group.Name, CourseIDs: courseIDs}, nil
}
