package absenceshttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences"
	sqldb "warwick-institute/internal/db"
)

func normalizeWCode(raw string) string {
	return absences.NormalizeWCode(raw)
}

func normalizeSubmissionSitInMethod(raw *string) (pgtype.Text, error) {
	return absences.NormalizeSubmissionSitInMethod(raw)
}

func absenceDayLimitLockKey(wcode, courseID string) string {
	return "absence-limit:" + normalizeWCode(wcode) + ":" + courseID
}

func absenceDayLimitLockKeyForMergeGroup(wcode, mergeGroupID string) string {
	return "absence-limit:" + normalizeWCode(wcode) + ":merge:" + mergeGroupID
}

func mergeGroupScopeForCourse(ctx context.Context, q *sqldb.Queries, courseID pgtype.UUID) (sqldb.CourseMergeGroupScopeForCourseRow, bool, error) {
	scope, err := q.CourseMergeGroupScopeForCourse(ctx, courseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqldb.CourseMergeGroupScopeForCourseRow{}, false, nil
	}
	if err != nil {
		return sqldb.CourseMergeGroupScopeForCourseRow{}, false, err
	}
	return scope, true, nil
}

func lockCourseForMergeScope(ctx context.Context, q *sqldb.Queries, courseID pgtype.UUID) error {
	_, err := q.CourseMergeGroupLockCourses(ctx, []pgtype.UUID{courseID})
	return err
}

func setAbsenceMergeGroupForCourse(ctx context.Context, q *sqldb.Queries, absenceID, courseID pgtype.UUID) error {
	if err := lockCourseForMergeScope(ctx, q, courseID); err != nil {
		return err
	}
	scope, found, err := mergeGroupScopeForCourse(ctx, q, courseID)
	if err != nil || !found {
		return err
	}
	return q.AbsenceSetMergeGroupID(ctx, absenceID, scope.ID)
}

func parseUUIDStrings(values []string) ([]pgtype.UUID, error) {
	parsed := make([]pgtype.UUID, 0, len(values))
	for _, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, pgtype.UUID{Bytes: id, Valid: true})
	}
	return parsed, nil
}

type sitInDayAlreadyUsedError struct {
	SessionIDs []string
}

func (e *sitInDayAlreadyUsedError) Error() string {
	return "This sit-in day is already assigned to this student's absence. Choose another day."
}

func ensureSitInDatesAvailable(ctx context.Context, q *sqldb.Queries, studentID pgtype.UUID, sessionIDs []pgtype.UUID, instituteTZ string) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	used, err := q.ActiveSitInSessionIDsForStudentOnDates(ctx, studentID, sessionIDs, instituteTZ)
	if err != nil {
		return err
	}
	if len(used) == 0 {
		return nil
	}
	conflicts := make([]string, 0, len(used))
	for _, sessionID := range used {
		conflicts = append(conflicts, sessionID.String())
	}
	return &sitInDayAlreadyUsedError{SessionIDs: conflicts}
}

func (s *server) writeSitInDayConflict(w http.ResponseWriter, err error) bool {
	var conflict *sitInDayAlreadyUsedError
	if !errors.As(err, &conflict) {
		return false
	}
	s.a.WriteErrDetails(w, http.StatusConflict, "sit_in_day_already_used", conflict.Error(), map[string]any{
		"session_ids": conflict.SessionIDs,
	})
	return true
}

func courseAvailableToStudents(ctx context.Context, q *sqldb.Queries, courseID pgtype.UUID) (bool, error) {
	id, err := sUUIDString(courseID)
	if err != nil {
		return false, err
	}
	visible, err := q.CourseIDsVisible(ctx, []string{id})
	if err != nil {
		return false, err
	}
	_, ok := visible[id]
	return ok, nil
}

func projectedAbsenceDayStats(
	ctx context.Context,
	q *sqldb.Queries,
	wcode string,
	courseID pgtype.UUID,
	missedSessionIDs []pgtype.UUID,
	dateFrom pgtype.Date,
	dateTo pgtype.Date,
	instituteTZ string,
) (absences.AbsenceDayLimitStats, int32, error) {
	courseIDString, err := sUUIDString(courseID)
	if err != nil {
		return absences.AbsenceDayLimitStats{}, 0, err
	}
	if err := lockCourseForMergeScope(ctx, q, courseID); err != nil {
		return absences.AbsenceDayLimitStats{}, 0, err
	}
	scope, found, err := mergeGroupScopeForCourse(ctx, q, courseID)
	if err != nil {
		return absences.AbsenceDayLimitStats{}, 0, err
	}
	lockKey := absenceDayLimitLockKey(wcode, courseIDString)
	if found {
		mergeGroupID, err := sUUIDString(scope.ID)
		if err != nil {
			return absences.AbsenceDayLimitStats{}, 0, err
		}
		lockKey = absenceDayLimitLockKeyForMergeGroup(wcode, mergeGroupID)
	}
	if err := q.AdvisoryLockForText(ctx, lockKey); err != nil {
		return absences.AbsenceDayLimitStats{}, 0, err
	}
	var counts sqldb.AbsenceDayCounts
	if found {
		counts, err = q.AbsenceDayCountsForMergeGroup(ctx, sqldb.AbsenceDayCountsForMergeGroupParams{
			Wcode:               wcode,
			MergeGroupID:        scope.ID,
			CandidateSessionIDs: missedSessionIDs,
			DateFrom:            dateFrom,
			DateTo:              dateTo,
			InstituteTZ:         instituteTZ,
		})
	} else {
		counts, err = q.AbsenceDayCountsForCourse(ctx, sqldb.AbsenceDayCountsForCourseParams{
			Wcode:               wcode,
			CourseID:            courseID,
			CandidateSessionIDs: missedSessionIDs,
			DateFrom:            dateFrom,
			DateTo:              dateTo,
			InstituteTZ:         instituteTZ,
		})
	}
	if err != nil {
		return absences.AbsenceDayLimitStats{}, 0, err
	}
	return absences.NewAbsenceDayLimitStats(
		counts.TotalCourseDays,
		counts.UsedAbsenceDays,
		counts.ProjectedAbsenceDays,
	), counts.CandidateAbsenceDays, nil
}

func resolveClientStudentEmail(raw *string, emailCRM, emailSystem pgtype.Text) (pgtype.Text, bool, error) {
	return absences.ResolveClientStudentEmail(raw, emailCRM, emailSystem)
}

func clientStudentEmailProvided(raw *string) bool {
	return absences.ClientStudentEmailProvided(raw)
}

type sessionTimingInfo struct {
	StartAt pgtype.Timestamptz
	EndAt   pgtype.Timestamptz
}

type sessionTimingError struct {
	code    string
	message string
}

func (e *sessionTimingError) Error() string {
	return e.message
}

func validateSessionTiming(settings absenceFormSettings, now time.Time, sessions []sessionTimingInfo) *sessionTimingError {
	return toSessionTimingError(absences.ValidateSessionTiming(timingSettings(settings), now, domainSessionTimingInfos(sessions)))
}

func sessionAllowedByTimingPolicy(settings absenceFormSettings, now time.Time, session sessionTimingInfo) bool {
	return absences.SessionAllowedByTimingPolicy(timingSettings(settings), now, absences.SessionTimingInfo{StartAt: session.StartAt, EndAt: session.EndAt})
}

func sessionTimingInfos(rows []sqldb.MissedSessionTimingRow) []sessionTimingInfo {
	out := make([]sessionTimingInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionTimingInfo{StartAt: row.StartAt, EndAt: row.EndAt})
	}
	return out
}

func timingSettings(settings absenceFormSettings) absences.TimingSettings {
	return absences.TimingSettings{
		MinHoursBeforeSession: settings.MinHoursBeforeSession,
		MaxHoursAfterSession:  settings.MaxHoursAfterSession,
	}
}

func domainSessionTimingInfos(sessions []sessionTimingInfo) []absences.SessionTimingInfo {
	out := make([]absences.SessionTimingInfo, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, absences.SessionTimingInfo{StartAt: session.StartAt, EndAt: session.EndAt})
	}
	return out
}

func toSessionTimingError(err *absences.SessionTimingError) *sessionTimingError {
	if err == nil {
		return nil
	}
	return &sessionTimingError{code: err.Code, message: err.Message}
}
