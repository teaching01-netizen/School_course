package legacysync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/schedulelock"
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
	SessionsCreated int
	SyncedAt        time.Time
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

		if _, err := qtx.SessionCreate(ctx, sqldb.SessionCreateParams{
			CourseID:  courseID,
			TeacherID: teacherID,
			RoomID:    roomID,
			StartAt:   startPg,
			EndAt:     endPg,
		}); err != nil {
			return nil, fmt.Errorf("create session at %s %s: %w", row.Date.Format("2006-01-02"), row.Begin, err)
		}
		created++
	}

	now := time.Now()
	if _, err := tx.Exec(ctx, `UPDATE courses SET legacy_last_synced_at = $1 WHERE id = $2`, now, courseID); err != nil {
		return nil, fmt.Errorf("update synced_at: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &SyncResult{SessionsCreated: created, SyncedAt: now}, nil
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
