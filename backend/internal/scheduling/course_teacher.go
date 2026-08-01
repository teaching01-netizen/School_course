package scheduling

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// ErrTeacherNotAssigned is the stable error code for scheduling writes whose
// teacher is not part of the course's assigned teacher set (course_teachers).
const ErrTeacherNotAssigned = "teacher_not_assigned_to_course"

// seriesTeacherOrCourseChanged reports whether an edit-entire-series request
// moves the series to a different teacher and/or course than the current
// series row. Only changed identities need membership revalidation — the
// existing teacher may legitimately have left the set since the series was
// created (historical sessions are never backfilled).
func seriesTeacherOrCourseChanged(currentCourseID, currentTeacherID, newCourseID, newTeacherID pgtype.UUID) bool {
	teacherChanged := newTeacherID.Valid && currentTeacherID.Valid && newTeacherID.Bytes != currentTeacherID.Bytes
	courseChanged := newCourseID.Valid && currentCourseID.Valid && newCourseID.Bytes != currentCourseID.Bytes
	return teacherChanged || courseChanged
}

// enforceSeriesTeacherMembership is the shared membership gate for
// edit-entire-series writes: when the series moves to a different teacher or
// course, the NEW teacher must belong to the NEW course's teacher set,
// because every future occurrence is rewritten to those identities. Arguments
// are ordered current-then-new — currentCourseID/currentTeacherID are the
// series' existing identities, newCourseID/newTeacherID the request's
// replacements. No-op when neither identity changes: the existing teacher may
// legitimately have left the set since the series was created.
func enforceSeriesTeacherMembership(ctx context.Context, q *sqldb.Queries, currentCourseID, currentTeacherID, newCourseID, newTeacherID pgtype.UUID) error {
	if !seriesTeacherOrCourseChanged(currentCourseID, currentTeacherID, newCourseID, newTeacherID) {
		return nil
	}
	return checkCourseTeacherMembership(ctx, q, newCourseID, newTeacherID)
}

// checkCourseTeacherMembership verifies that teacherID belongs to the course's
// assigned teacher set. It must run inside the authoritative write transaction
// after the course row has been locked, so a concurrent teacher-set replacement
// cannot slip a session past the check. Returns a stable Err when the teacher
// is not assigned; an empty set rejects every teacher.
func checkCourseTeacherMembership(ctx context.Context, qtx *sqldb.Queries, courseID, teacherID pgtype.UUID) error {
	assigned, err := qtx.CourseTeacherExists(ctx, sqldb.CourseTeacherExistsParams{CourseID: courseID, TeacherID: teacherID})
	if err != nil {
		return err
	}
	if assigned {
		return nil
	}
	courseIDStr, err := uuidString(courseID)
	if err != nil {
		return err
	}
	teacherIDStr, err := uuidString(teacherID)
	if err != nil {
		return err
	}
	return &Err{
		Code:    ErrTeacherNotAssigned,
		Message: "The selected teacher is not assigned to this course.",
		Details: ConflictDetails{
			Kind: ConflictKindTeacherNotAssigned,
			// Requested start/end are omitted: the session times are not known
			// at this point in the write path (only course+teacher are checked).
			Requested: ConflictRequested{
				CourseID:  courseIDStr,
				TeacherID: teacherIDStr,
			},
		},
	}
}
