package absenceshttp

import (
	"context"
	"errors"
	"net/http"
	"strings"
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

// lockSessionRowsForSubmission fences the session rows a submission is about
// to validate and snapshot (Step 9, gap G2). SessionGetByIDForSnapshot takes
// no row lock, so without this a session edit can commit between the snapshot
// read and the assignment insert. SessionsLockOrdered takes FOR UPDATE in
// immutable-ID order, compatible with the session editor (EditOccurrenceTimeTx
// locks the same session row): the two writers serialize on the row.
// Empty input is a no-op (SessionsLockOrdered on an empty set locks nothing).
func lockSessionsForSubmission(ctx context.Context, q *sqldb.Queries, sessionIDs []pgtype.UUID) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	_, err := q.SessionsLockOrdered(ctx, sessionIDs)
	return err
}

func setAbsenceMergeGroupForCourse(ctx context.Context, q *sqldb.Queries, absenceID, courseID pgtype.UUID) error {
	if err := lockCourseForMergeScope(ctx, q, courseID); err != nil {
		return err
	}
	return setAbsenceMergeGroupIDOnly(ctx, q, absenceID, courseID)
}

// setAbsenceMergeGroupIDOnly writes the merge-group id WITHOUT locking.
// Callers must already hold the course lock (Step-9 F1 takes it pre-student).
// Keeping the post-create call on the locking variant would take course AFTER
// student and reintroduce the F1 AB-BA deadlock with the session editor.
func setAbsenceMergeGroupIDOnly(ctx context.Context, q *sqldb.Queries, absenceID, courseID pgtype.UUID) error {
	scope, found, err := mergeGroupScopeForCourse(ctx, q, courseID)
	if err != nil || !found {
		return err
	}
	return q.AbsenceSetMergeGroupID(ctx, absenceID, scope.ID)
}

// lockBatchCourseSet collects every distinct course_id across batch items and
// locks them in immutable-ID order BEFORE item 1 (Step-9 G1). Batch items
// carry course_id as a raw string; unparseable entries are skipped here and
// rejected per item later with the specific bad_course_id error - pre-locking
// must not change validation semantics, only lock acquisition order.
func lockBatchCourseSet(ctx context.Context, q *sqldb.Queries, items []batchAbsenceCreateItem) error {
	seen := make(map[[16]byte]struct{}, len(items))
	courseIDs := make([]pgtype.UUID, 0, len(items))
	for _, item := range items {
		raw := strings.TrimSpace(item.CourseID)
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		pgID := pgtype.UUID{Bytes: id, Valid: true}
		if _, ok := seen[pgID.Bytes]; ok {
			continue
		}
		seen[pgID.Bytes] = struct{}{}
		courseIDs = append(courseIDs, pgID)
	}
	if len(courseIDs) == 0 {
		return nil
	}
	_, err := q.CourseMergeGroupLockCourses(ctx, courseIDs)
	return err
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

type sitInSessionAlreadyUsedError struct {
	SessionIDs []string
	Conflicts  []*sitInSessionConflictInfo
}

func (e *sitInSessionAlreadyUsedError) Error() string {
	return "This sit-in session is already assigned to this student's absence. Choose another session."
}

func ensureSitInSessionsAvailable(ctx context.Context, q *sqldb.Queries, studentID pgtype.UUID, sessionIDs []pgtype.UUID) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	used, err := q.ActiveSitInSessionIDsForStudentCandidates(ctx, studentID, sessionIDs)
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
	details, detailErr := q.ActiveSitInSessionConflictsForStudent(ctx, studentID)
	if detailErr != nil {
		return detailErr
	}
	bySession := make(map[string]*sitInSessionConflictInfo, len(details))
	for _, detail := range details {
		bySession[detail.SessionID.String()] = sitInConflictInfo(detail)
	}
	conflictDetails := make([]*sitInSessionConflictInfo, 0, len(conflicts))
	for _, sessionID := range conflicts {
		conflictDetails = append(conflictDetails, bySession[sessionID])
	}
	return &sitInSessionAlreadyUsedError{SessionIDs: conflicts, Conflicts: conflictDetails}
}

// writeSessionSnapshotResult maps snapshot-insertion outcomes to the public
// contract (Step 7: one application error boundary for stale versions).
// Stale versions - missed or sit-in - are 409 session_version_conflict;
// missing sessions surface through ClassifyDBErr; anything else is internal.
// Every absence writer (staff, public, batch, staff-tx) must use this instead
// of ad-hoc ClassifyDBErr on snapshot errors.
func (s *server) writeSessionSnapshotResult(w http.ResponseWriter, err error) {
	var versionErr *sqldb.SessionVersionConflictError
	if errors.As(err, &versionErr) {
		s.a.WriteErr(w, http.StatusConflict, "session_version_conflict", "Session has been modified since you last loaded it. Please reload and try again.")
		return
	}
	status, code, msg := s.a.ClassifyDBErr(err)
	s.a.WriteErr(w, status, code, msg)
}

func (s *server) writeSitInSessionConflict(w http.ResponseWriter, err error) bool {
	var conflict *sitInSessionAlreadyUsedError
	if !errors.As(err, &conflict) {
		return false
	}
	s.a.WriteErrDetails(w, http.StatusConflict, "sit_in_session_already_used", conflict.Error(), map[string]any{
		"session_ids": conflict.SessionIDs,
		"conflicts":   conflict.Conflicts,
	})
	return true
}

// recheckSitInSessionsHeld re-runs the same-student conflict check AFTER the
// session-row fence is held (Step 9.4). The pre-lock ensureSitInSessionsAvailable
// call races with a concurrent submission: both can read "free" before either
// inserts. Re-checking after SessionsLockOrdered (inside the snapshot fns)
// does not fully serialize two submitters (different sessions = different rows),
// but the student row lock held since F1 serializes same-student writers, so
// by the time we re-check here, any concurrent same-student insert has either
// committed (we see it and 409) or is blocked behind our student lock.
// Returns true when the caller should abort with the conflict already written.
func (s *server) recheckSitInSessionsHeld(w http.ResponseWriter, r *http.Request, qtx *sqldb.Queries, studentID pgtype.UUID, sessionUUIDs []pgtype.UUID) bool {
	if len(sessionUUIDs) == 0 {
		return false
	}
	if err := ensureSitInSessionsAvailable(r.Context(), qtx, studentID, sessionUUIDs); err != nil {
		if s.writeSitInSessionConflict(w, err) {
			return true
		}
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not re-check sit-in session availability")
		return true
	}
	return false
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
