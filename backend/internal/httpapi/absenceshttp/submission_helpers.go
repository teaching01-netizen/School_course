package absenceshttp

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (s *server) requireStudentReason(w http.ResponseWriter, reason pgtype.Text) error {
	if reason.Valid {
		return nil
	}
	s.a.WriteErr(w, http.StatusBadRequest, "reason_required", "Please tell us why you'll be away.")
	return errors.New("student reason required")
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

var (
	errBookingScopeChanged    = errors.New("course booking scope changed while acquiring locks")
	errBookingSessionInactive = errors.New("a selected session's course is closed for student booking")
)

// Lock the requested courses and their rule sources as a single ordered set.
// A savepoint lets a topology change release that set before retrying.
func lockStudentBookingCourses(ctx context.Context, q *sqldb.Queries, tx pgx.Tx, courseIDs, sessionIDs []pgtype.UUID) (map[pgtype.UUID]bool, error) {
	for attempt := 0; attempt < 3; attempt++ {
		ids := append([]pgtype.UUID(nil), courseIDs...)
		if len(sessionIDs) > 0 {
			sessionCourses, err := q.BookingSessionCourseIDs(ctx, sessionIDs)
			if err != nil {
				return nil, err
			}
			ids = append(ids, sessionCourses...)
		}
		targets, err := q.CourseBookingTargets(ctx, ids)
		if err != nil {
			return nil, err
		}
		toLock := append([]pgtype.UUID(nil), ids...)
		for _, target := range targets {
			toLock = append(toLock, target.SourceID)
		}
		savepoint, err := tx.Begin(ctx)
		if err != nil {
			return nil, err
		}
		work := q.WithTx(savepoint)
		locked, err := work.CoursesLockOrdered(ctx, toLock)
		lockedIDs := make(map[pgtype.UUID]bool, len(locked))
		for _, course := range locked {
			lockedIDs[course] = true
		}
		if err == nil {
			var current []sqldb.CourseBookingTarget
			current, err = work.CourseBookingTargets(ctx, ids)
			if err == nil {
				for _, course := range current {
					if !lockedIDs[course.SourceID] {
						err = errBookingScopeChanged
						break
					}
				}
			}
		}
		if err == nil {
			return lockedIDs, savepoint.Commit(ctx)
		}
		if rollbackErr := savepoint.Rollback(ctx); rollbackErr != nil {
			return nil, errors.Join(err, rollbackErr)
		}
		var pgErr *pgconn.PgError
		if !errors.Is(err, errBookingScopeChanged) && !(errors.As(err, &pgErr) && pgErr.Code == "40P01") {
			return nil, err
		}
	}
	return nil, errBookingScopeChanged
}

// Check the actual session courses after locking the session rows. A session
// moved since the initial course lookup must be selected again by the student.
func validateStudentBookingSessions(ctx context.Context, q *sqldb.Queries, lockedCourses map[pgtype.UUID]bool, ids []pgtype.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	courses, err := q.BookingSessionCourseIDs(ctx, ids)
	if err != nil {
		return err
	}
	targets, err := q.CourseBookingTargets(ctx, courses)
	if err != nil {
		return err
	}
	for _, course := range targets {
		if !lockedCourses[course.CourseID] || !lockedCourses[course.SourceID] {
			return errBookingScopeChanged
		}
		if !course.SourceEnabled {
			return errBookingSessionInactive
		}
	}
	return nil
}

func (s *server) writeBookingScopeError(w http.ResponseWriter, err error) {
	if errors.Is(err, errBookingScopeChanged) {
		s.a.WriteErr(w, http.StatusServiceUnavailable, "retry_exhausted", "Course availability changed. Please retry.")
		return
	}
	if errors.Is(err, errBookingSessionInactive) {
		s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "A selected session is no longer available for student booking. Refresh the form and select again.")
		return
	}
	status, code, message := s.a.ClassifyDBErr(err)
	s.a.WriteErr(w, status, code, message)
}

func batchBookingCourseIDs(items []batchAbsenceCreateItem) []pgtype.UUID {
	ids := make([]pgtype.UUID, 0, len(items))
	for _, item := range items {
		if id, err := uuid.Parse(strings.TrimSpace(item.CourseID)); err == nil {
			ids = append(ids, pgtype.UUID{Bytes: id, Valid: true})
		}
		if item.SitInMethod != nil && item.SitInCourseID != nil {
			if id, err := uuid.Parse(strings.TrimSpace(*item.SitInCourseID)); err == nil {
				ids = append(ids, pgtype.UUID{Bytes: id, Valid: true})
			}
		}
	}
	return ids
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
	return "A selected sit-in session is already assigned or overlaps another sit-in for this student. Choose another session."
}

func ensureSitInSessionsAvailable(ctx context.Context, q *sqldb.Queries, studentID pgtype.UUID, sessionIDs []pgtype.UUID, excludeAbsenceID ...pgtype.UUID) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	var excluded pgtype.UUID
	if len(excludeAbsenceID) > 0 {
		excluded = excludeAbsenceID[0]
	}
	booked, err := q.ActiveSitInSessionCandidateConflictsForStudent(ctx, studentID, sessionIDs, excluded)
	if err != nil {
		return err
	}
	conflictingIDs := make(map[string]struct{})
	conflictInfoByCandidate := make(map[string]*sitInSessionConflictInfo)
	for _, item := range booked {
		candidateID := item.CandidateSessionID.String()
		conflictingIDs[candidateID] = struct{}{}
		if _, exists := conflictInfoByCandidate[candidateID]; !exists {
			conflictInfoByCandidate[candidateID] = sitInConflictInfo(item.Existing)
		}
	}
	intervals, err := q.SitInCandidateSessionIntervals(ctx, sessionIDs)
	if err != nil {
		return err
	}
	for i := 0; i < len(intervals); i++ {
		for j := i + 1; j < len(intervals); j++ {
			if timesOverlap(intervals[i].StartAt, intervals[i].EndAt, intervals[j].StartAt, intervals[j].EndAt) {
				sameMergedMeeting := intervals[i].ID.Bytes != intervals[j].ID.Bytes &&
					intervals[i].MergeGroupID.Valid && intervals[j].MergeGroupID.Valid &&
					intervals[i].MergeGroupID.Bytes == intervals[j].MergeGroupID.Bytes
				if sameMergedMeeting {
					continue
				}
				conflictingIDs[intervals[i].ID.String()] = struct{}{}
				conflictingIDs[intervals[j].ID.String()] = struct{}{}
			}
		}
	}
	if len(conflictingIDs) == 0 {
		return nil
	}
	conflicts := make([]string, 0, len(conflictingIDs))
	for id := range conflictingIDs {
		conflicts = append(conflicts, id)
	}
	sort.Strings(conflicts)
	conflictDetails := make([]*sitInSessionConflictInfo, 0, len(conflicts))
	for _, id := range conflicts {
		conflictDetails = append(conflictDetails, conflictInfoByCandidate[id])
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
func (s *server) recheckSitInSessionsHeld(w http.ResponseWriter, r *http.Request, qtx *sqldb.Queries, studentID pgtype.UUID, sessionUUIDs []pgtype.UUID, excludeAbsenceID ...pgtype.UUID) bool {
	if len(sessionUUIDs) == 0 {
		return false
	}
	if err := ensureSitInSessionsAvailable(r.Context(), qtx, studentID, sessionUUIDs, excludeAbsenceID...); err != nil {
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
	limitPercent int,
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
		limitPercent,
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
