package legacysync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/schedulelock"
	"warwick-institute/internal/schedulepolicy"
)

type Syncer struct {
	pool *pgxpool.Pool
	q    *sqldb.Queries
	log  *slog.Logger
	loc  *time.Location
}

func NewSyncer(pool *pgxpool.Pool, q *sqldb.Queries, log *slog.Logger, loc *time.Location) *Syncer {
	return &Syncer{pool: pool, q: q, log: log, loc: loc}
}

type SyncResult struct {
	SessionsCreated  int
	ConflictWarnings int
	SyncedAt         time.Time
}

type legacySyncSlot struct {
	teacherID pgtype.UUID
	roomID    pgtype.UUID
	startAt   time.Time
	endAt     time.Time
}

func (s *Syncer) SyncCourse(ctx context.Context, courseID pgtype.UUID, rows []ParsedRow, rooms []Room) (*SyncResult, error) {
	course, err := s.q.CourseGetLegacyFields(ctx, courseID)
	if err != nil {
		return nil, fmt.Errorf("get course: %w", err)
	}

	if !course.TeacherID.Valid {
		return nil, fmt.Errorf("course has no teacher assigned")
	}
	teacherID := course.TeacherID

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.q.WithTx(tx)
	policy, err := schedulepolicy.NewDBReader().Load(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("load schedule conflict policy: %w", err)
	}
	allowConflicts := !policy.Enforced(schedulepolicy.ScopeLegacySync)

	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{CourseIDs: []pgtype.UUID{courseID}}); err != nil {
		return nil, fmt.Errorf("lock course: %w", err)
	}
	// Re-read after the course lock wait and acquire every trigger-relevant
	// resource in the shared course→student→teacher→room→session order.
	lockedCourse, err := qtx.CourseGetLegacyFields(ctx, courseID)
	if err != nil {
		return nil, fmt.Errorf("reload course: %w", err)
	}
	if !lockedCourse.TeacherID.Valid {
		return nil, fmt.Errorf("course has no teacher assigned")
	}
	teacherID = lockedCourse.TeacherID
	students, err := qtx.CourseStudentsList(ctx, courseID)
	if err != nil {
		return nil, fmt.Errorf("list course students: %w", err)
	}
	studentIDs := make([]pgtype.UUID, 0, len(students))
	for _, student := range students {
		studentIDs = append(studentIDs, student.StudentID)
	}
	existing, err := qtx.SessionListActiveByCourse(ctx, courseID)
	if err != nil {
		return nil, fmt.Errorf("list existing sessions: %w", err)
	}
	sessionIDs := make([]pgtype.UUID, 0, len(existing))
	roomIDs := make([]pgtype.UUID, 0, len(existing)+len(rows))
	for _, session := range existing {
		sessionIDs = append(sessionIDs, session.ID)
		roomIDs = append(roomIDs, session.RoomID)
	}
	for _, row := range rows {
		if matched := MatchRoom(row.Classroom, rooms); matched != nil {
			if roomID, roomErr := pgTypeUUID(matched.ID); roomErr == nil {
				roomIDs = append(roomIDs, roomID)
			}
		}
	}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{
		StudentIDs: studentIDs,
		TeacherIDs: []pgtype.UUID{teacherID},
		RoomIDs:    roomIDs,
		SessionIDs: sessionIDs,
	}); err != nil {
		return nil, fmt.Errorf("lock schedule resources: %w", err)
	}

	// Migration 00028 made session children cascade on hard delete. Keeping
	// soft-deleted rows here would retain stale attendance and busy ranges.
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE course_id = $1`, courseID); err != nil {
		return nil, fmt.Errorf("hard-delete existing sessions: %w", err)
	}

	created := 0
	conflictWarnings := 0
	materialized := make([]legacySyncSlot, 0, len(rows))
	for _, row := range rows {
		startAt, err := localToUTC(row.Date, row.Begin, s.loc)
		if err != nil {
			s.log.Warn("skipping row: invalid start time", "date", row.Date, "begin", row.Begin, "error", err)
			continue
		}
		endAt, err := localToUTC(row.Date, row.End, s.loc)
		if err != nil {
			s.log.Warn("skipping row: invalid end time", "date", row.Date, "end", row.End, "error", err)
			continue
		}

		var roomID pgtype.UUID
		if matched := MatchRoom(row.Classroom, rooms); matched != nil {
			uid, err := pgTypeUUID(matched.ID)
			if err != nil {
				s.log.Warn("invalid room UUID", "room_id", matched.ID, "error", err)
			} else {
				roomID = uid
			}
		}

		startPg := pgtype.Timestamptz{Time: startAt, Valid: true}
		endPg := pgtype.Timestamptz{Time: endAt, Valid: true}
		conflictErr := strictLegacySyncConflict(ctx, qtx, courseID, teacherID, roomID, startAt, endAt)
		if conflictErr != nil && !isLegacySyncConflict(conflictErr) {
			return nil, fmt.Errorf("legacy schedule conflict check at %s %s: %w", row.Date.Format("2006-01-02"), row.Begin, conflictErr)
		}
		if conflictErr == nil {
			conflictErr = legacySyncBatchConflict(materialized, len(studentIDs) > 0, legacySyncSlot{
				teacherID: teacherID,
				roomID:    roomID,
				startAt:   startAt,
				endAt:     endAt,
			})
		}
		if conflictErr != nil {
			if !allowConflicts {
				return nil, fmt.Errorf("legacy schedule conflict at %s %s: %w", row.Date.Format("2006-01-02"), row.Begin, conflictErr)
			}
			conflictWarnings++
			s.log.Warn("legacy schedule conflict allowed by policy", "date", row.Date.Format("2006-01-02"), "begin", row.Begin, "error", conflictErr)
		}

		if _, err := qtx.SessionCreate(ctx, sqldb.SessionCreateParams{
			CourseID:         courseID,
			TeacherID:        teacherID,
			RoomID:           roomID,
			StartAt:          startPg,
			EndAt:            endPg,
			ConflictOverride: allowConflicts,
		}); err != nil {
			return nil, fmt.Errorf("create session at %s %s: %w", row.Date.Format("2006-01-02"), row.Begin, err)
		}
		materialized = append(materialized, legacySyncSlot{teacherID: teacherID, roomID: roomID, startAt: startAt, endAt: endAt})
		created++
	}

	now := time.Now()
	if _, err := tx.Exec(ctx, `UPDATE courses SET legacy_last_synced_at = $1 WHERE id = $2`, now, courseID); err != nil {
		return nil, fmt.Errorf("update synced_at: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &SyncResult{SessionsCreated: created, ConflictWarnings: conflictWarnings, SyncedAt: now}, nil
}

func isLegacySyncConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "23P01" || pgErr.Code == "23514")
}

func legacySyncBatchConflict(slots []legacySyncSlot, hasStudents bool, candidate legacySyncSlot) error {
	for _, previous := range slots {
		if !previous.startAt.Before(candidate.endAt) || !candidate.startAt.Before(previous.endAt) {
			continue
		}
		if previous.teacherID.Valid && candidate.teacherID.Valid && previous.teacherID.Bytes == candidate.teacherID.Bytes {
			return &pgconn.PgError{Code: "23P01", ConstraintName: "sessions_no_teacher_overlap", Message: "teacher schedule overlap"}
		}
		if previous.roomID.Valid && candidate.roomID.Valid && previous.roomID.Bytes == candidate.roomID.Bytes {
			return &pgconn.PgError{Code: "23P01", ConstraintName: "sessions_no_room_overlap", Message: "room schedule overlap"}
		}
		if hasStudents {
			return &pgconn.PgError{Code: "23P01", ConstraintName: "student_busy_ranges_no_overlap", Message: "student schedule overlap"}
		}
		return &pgconn.PgError{Code: "23P01", ConstraintName: "sessions_no_course_overlap", Message: "course session overlap"}
	}
	return nil
}

func localToUTC(date time.Time, clock string, loc *time.Location) (time.Time, error) {
	if len(clock) < 5 {
		return time.Time{}, fmt.Errorf("invalid clock: %s", clock)
	}
	var hour, min int
	if _, err := fmt.Sscanf(clock, "%d:%d", &hour, &min); err != nil {
		return time.Time{}, fmt.Errorf("parse clock %s: %w", clock, err)
	}
	local := time.Date(date.Year(), date.Month(), date.Day(), hour, min, 0, 0, loc)
	return local.UTC(), nil
}

func pgTypeUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return u, nil
}

func strictLegacySyncConflict(ctx context.Context, qtx *sqldb.Queries, courseID, teacherID, roomID pgtype.UUID, start, end time.Time) error {
	startAt := pgtype.Timestamptz{Time: start, Valid: true}
	endAt := pgtype.Timestamptz{Time: end, Valid: true}
	availability, err := qtx.CheckTeacherAvailability(ctx, sqldb.CheckTeacherAvailabilityParams{TeacherID: teacherID, Column2: startAt, Column3: endAt})
	if err != nil {
		return fmt.Errorf("check teacher availability: %w", err)
	}
	if availability.HasWindows && !availability.IsAvailable {
		return &pgconn.PgError{Code: "23514", Message: "teacher not available for requested time"}
	}
	if roomID.Valid {
		availability, err := qtx.CheckRoomAvailability(ctx, sqldb.CheckRoomAvailabilityParams{RoomID: roomID, Column2: startAt, Column3: endAt})
		if err != nil {
			return fmt.Errorf("check room availability: %w", err)
		}
		if availability.HasWindows && !availability.IsAvailable {
			return &pgconn.PgError{Code: "23514", Message: "room not available for requested time"}
		}
	}
	var exists bool
	if err := qtx.DBTX().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sessions
			WHERE deleted_at IS NULL AND teacher_id = $1
			  AND time_range && tstzrange($2, $3, '[)')
		)`, teacherID, start, end).Scan(&exists); err != nil {
		return fmt.Errorf("check teacher overlap: %w", err)
	}
	if exists {
		return &pgconn.PgError{Code: "23P01", ConstraintName: "sessions_no_teacher_overlap", Message: "teacher schedule overlap"}
	}
	if roomID.Valid {
		if err := qtx.DBTX().QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM sessions
				WHERE deleted_at IS NULL AND room_id = $1
				  AND time_range && tstzrange($2, $3, '[)')
			)`, roomID, start, end).Scan(&exists); err != nil {
			return fmt.Errorf("check room overlap: %w", err)
		}
		if exists {
			return &pgconn.PgError{Code: "23P01", ConstraintName: "sessions_no_room_overlap", Message: "room schedule overlap"}
		}
	}
	if err := qtx.DBTX().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM student_busy_ranges br
			JOIN sessions other ON other.id = br.session_id
			JOIN course_students cs ON cs.student_id = br.student_id AND cs.course_id = $1
			WHERE br.deleted_at IS NULL AND other.deleted_at IS NULL
			  AND br.time_range && tstzrange($2, $3, '[)')
		)`, courseID, start, end).Scan(&exists); err != nil {
		return fmt.Errorf("check student overlap: %w", err)
	}
	if exists {
		return &pgconn.PgError{Code: "23P01", ConstraintName: "student_busy_ranges_no_overlap", Message: "student schedule overlap"}
	}
	if err := qtx.DBTX().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sessions
			WHERE deleted_at IS NULL AND course_id = $1
			  AND time_range && tstzrange($2, $3, '[)')
		)`, courseID, start, end).Scan(&exists); err != nil {
		return fmt.Errorf("check course overlap: %w", err)
	}
	if exists {
		return &pgconn.PgError{Code: "23P01", ConstraintName: "sessions_no_course_overlap", Message: "course session overlap"}
	}
	return nil
}
