package scheduling

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type conflictingStudentRow struct {
	StudentID string
	FullName  string
	Status    string
}

// conflictingStudentsForConflictSessions queries for the student details that caused an overlap.
// It takes the conflicting session IDs and either explicit student IDs or a course ID.
func (s *Service) conflictingStudentsForOverlap(ctx context.Context, db sqldb.DBTX, sessionIDs []string, studentIDs []pgtype.UUID, courseID pgtype.UUID) ([]ConflictingStudent, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}

	sessionUUIDs := make([]pgtype.UUID, len(sessionIDs))
	for i, sid := range sessionIDs {
		id, err := uuidFromString(sid)
		if err != nil {
			return nil, err
		}
		sessionUUIDs[i] = id
	}

	// First, try to find overlapping students from the given session IDs and explicit student IDs.
	var rows pgx.Rows
	var err error

	if len(studentIDs) > 0 {
		rows, err = db.Query(ctx, `
			SELECT DISTINCT br.student_id, COALESCE(s.full_name, ''), COALESCE(cs.status, 'enrolled')
			FROM student_busy_ranges br
			JOIN students s ON s.id = br.student_id
			LEFT JOIN course_students cs ON cs.student_id = br.student_id AND cs.course_id = $1
			WHERE br.session_id = ANY($2::uuid[])
			  AND br.deleted_at IS NULL
			  AND br.student_id = ANY($3::uuid[])
		`, courseID, sessionUUIDs, studentIDs)
	} else {
		// Fallback: find via course roster.
		rows, err = db.Query(ctx, `
			SELECT DISTINCT br.student_id, COALESCE(s.full_name, ''), COALESCE(cs.status, 'enrolled')
			FROM student_busy_ranges br
			JOIN students s ON s.id = br.student_id
			JOIN course_students cs ON cs.student_id = br.student_id AND cs.course_id = $1
			WHERE br.session_id = ANY($2::uuid[])
			  AND br.deleted_at IS NULL
		`, courseID, sessionUUIDs)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var studentSet map[string]ConflictingStudent
	for rows.Next() {
		var stID pgtype.UUID
		var fullName, status string
		if err := rows.Scan(&stID, &fullName, &status); err != nil {
			return nil, err
		}
		idStr, err := uuidString(stID)
		if err != nil {
			continue
		}
		if studentSet == nil {
			studentSet = make(map[string]ConflictingStudent)
		}
		studentSet[idStr] = ConflictingStudent{
			StudentID: idStr,
			FullName:  fullName,
			Status:    status,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]ConflictingStudent, 0, len(studentSet))
	for _, cs := range studentSet {
		out = append(out, cs)
	}
	return out, nil
}

func uuidFromString(s string) (pgtype.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}

func (s *Service) preflightSlot(ctx context.Context, db sqldb.DBTX, q *sqldb.Queries, in preflightInput) *Err {
	ignoreSeries := in.IgnoreSeries
	if in.Requested.StartAt == "" {
		in.Requested.StartAt = in.StartUTC.UTC().Format(time.RFC3339Nano)
	}
	if in.Requested.EndAt == "" {
		in.Requested.EndAt = in.EndUTC.UTC().Format(time.RFC3339Nano)
	}

	// Availability (when windows exist).
	if ok, err := q.IsTeacherAvailable(ctx, in.TeacherID, in.StartUTC, in.EndUTC); err != nil {
		return &Err{Code: "db_error", Message: "Database error", Details: ConflictDetails{Kind: ConflictKindTeacherAvailability, Conflicts: nil, Requested: in.Requested}}
	} else if !ok {
		return &Err{
			Code:    "availability_violation",
			Message: "teacher not available for requested time",
			Details: ConflictDetails{Kind: ConflictKindTeacherAvailability, Conflicts: nil, Requested: in.Requested},
		}
	}

	if in.RoomID.Valid {
		if ok, err := q.IsRoomAvailable(ctx, in.RoomID, in.StartUTC, in.EndUTC); err != nil {
			return &Err{Code: "db_error", Message: "Database error", Details: ConflictDetails{Kind: ConflictKindRoomAvailability, Conflicts: nil, Requested: in.Requested}}
		} else if !ok {
			return &Err{
				Code:    "availability_violation",
				Message: "room not available for requested time",
				Details: ConflictDetails{Kind: ConflictKindRoomAvailability, Conflicts: nil, Requested: in.Requested},
			}
		}
	}

	// Overlaps.
	if conflicts, err := s.overlappingSessionsByRoom(ctx, db, in.RoomID, in.StartUTC, in.EndUTC, in.IgnoreSession, ignoreSeries); err != nil {
		return &Err{Code: "db_error", Message: "Database error", Details: ConflictDetails{Kind: ConflictKindRoomOverlap, Conflicts: nil, Requested: in.Requested}}
	} else if len(conflicts) > 0 {
		return &Err{
			Code:    "schedule_conflict",
			Message: "Schedule conflict",
			Details: ConflictDetails{Kind: ConflictKindRoomOverlap, Conflicts: conflicts, Requested: in.Requested},
		}
	}

	if conflicts, err := s.overlappingSessionsByTeacher(ctx, db, in.TeacherID, in.StartUTC, in.EndUTC, in.IgnoreSession, ignoreSeries); err != nil {
		return &Err{Code: "db_error", Message: "Database error", Details: ConflictDetails{Kind: ConflictKindTeacherOverlap, Conflicts: nil, Requested: in.Requested}}
	} else if len(conflicts) > 0 {
		return &Err{
			Code:    "schedule_conflict",
			Message: "Schedule conflict",
			Details: ConflictDetails{Kind: ConflictKindTeacherOverlap, Conflicts: conflicts, Requested: in.Requested},
		}
	}

	if in.StudentIDs != nil {
		if len(*in.StudentIDs) > 0 {
			if conflicts, err := s.overlappingSessionsByStudents(ctx, db, *in.StudentIDs, in.StartUTC, in.EndUTC, in.IgnoreSession, ignoreSeries); err != nil {
				return &Err{Code: "db_error", Message: "Database error", Details: ConflictDetails{Kind: ConflictKindStudentOverlap, Conflicts: nil, Requested: in.Requested}}
			} else if len(conflicts) > 0 {
				sessionIDs := make([]string, len(conflicts))
				for i, c := range conflicts {
					sessionIDs[i] = c.SessionID
				}
				conflictingStudents, _ := s.conflictingStudentsForOverlap(ctx, db, sessionIDs, *in.StudentIDs, in.CourseID)
				return &Err{
					Code:    "schedule_conflict",
					Message: "Schedule conflict",
					Details: ConflictDetails{
						Kind:               ConflictKindStudentOverlap,
						Conflicts:          conflicts,
						ConflictingStudents: conflictingStudents,
						Requested:          in.Requested,
					},
				}
			}
		}
		// Explicit non-nil StudentIDs (even if empty) means we skip the course-roster fallback.
		return nil
	}

	if conflicts, err := s.overlappingSessionsByStudentsInCourse(ctx, db, in.CourseID, in.StartUTC, in.EndUTC, in.IgnoreSession, ignoreSeries); err != nil {
		return &Err{Code: "db_error", Message: "Database error", Details: ConflictDetails{Kind: ConflictKindStudentOverlap, Conflicts: nil, Requested: in.Requested}}
	} else if len(conflicts) > 0 {
		sessionIDs := make([]string, len(conflicts))
		for i, c := range conflicts {
			sessionIDs[i] = c.SessionID
		}
		conflictingStudents, _ := s.conflictingStudentsForOverlap(ctx, db, sessionIDs, nil, in.CourseID)
		return &Err{
			Code:    "schedule_conflict",
			Message: "Schedule conflict",
			Details: ConflictDetails{
				Kind:               ConflictKindStudentOverlap,
				Conflicts:          conflicts,
				ConflictingStudents: conflictingStudents,
				Requested:          in.Requested,
			},
		}
	}

	return nil
}

func (s *Service) overlappingSessionsByStudents(ctx context.Context, db sqldb.DBTX, studentIDs []pgtype.UUID, startUTC, endUTC time.Time, ignore *pgtype.UUID, ignoreSeries *pgtype.UUID) ([]ConflictSession, error) {
	if len(studentIDs) == 0 {
		return nil, nil
	}
	rows, err := db.Query(ctx, `
		SELECT DISTINCT s.id, s.series_id, s.course_id, s.room_id, s.teacher_id, s.start_at, s.end_at
		FROM student_busy_ranges br
		JOIN sessions s ON s.id = br.session_id
		WHERE br.deleted_at IS NULL
		  AND s.deleted_at IS NULL
		  AND br.student_id = ANY($1::uuid[])
		  AND br.time_range && tstzrange($2, $3, '[)')
		  AND ($4::uuid IS NULL OR s.id <> $4)
		  AND ($5::uuid IS NULL OR s.series_id IS DISTINCT FROM $5)
		ORDER BY s.start_at ASC
		LIMIT 25
	`, studentIDs, startUTC, endUTC, ignoreUUID(ignore), ignoreUUID(ignoreSeries))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConflictSessions(rows)
}

func (s *Service) overlappingSessionsByRoom(ctx context.Context, db sqldb.DBTX, roomID pgtype.UUID, startUTC, endUTC time.Time, ignore *pgtype.UUID, ignoreSeries *pgtype.UUID) ([]ConflictSession, error) {
	if !roomID.Valid {
		return nil, nil
	}
	rows, err := db.Query(ctx, `
		SELECT id, series_id, course_id, room_id, teacher_id, start_at, end_at
		FROM sessions
		WHERE deleted_at IS NULL
		  AND room_id = $1
		  AND time_range && tstzrange($2, $3, '[)')
		  AND ($4::uuid IS NULL OR id <> $4)
		  AND ($5::uuid IS NULL OR series_id IS DISTINCT FROM $5)
		ORDER BY start_at ASC
		LIMIT 25
	`, roomID, startUTC, endUTC, ignoreUUID(ignore), ignoreUUID(ignoreSeries))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConflictSessions(rows)
}

func (s *Service) overlappingSessionsByTeacher(ctx context.Context, db sqldb.DBTX, teacherID pgtype.UUID, startUTC, endUTC time.Time, ignore *pgtype.UUID, ignoreSeries *pgtype.UUID) ([]ConflictSession, error) {
	rows, err := db.Query(ctx, `
		SELECT id, series_id, course_id, room_id, teacher_id, start_at, end_at
		FROM sessions
		WHERE deleted_at IS NULL
		  AND teacher_id = $1
		  AND time_range && tstzrange($2, $3, '[)')
		  AND ($4::uuid IS NULL OR id <> $4)
		  AND ($5::uuid IS NULL OR series_id IS DISTINCT FROM $5)
		ORDER BY start_at ASC
		LIMIT 25
	`, teacherID, startUTC, endUTC, ignoreUUID(ignore), ignoreUUID(ignoreSeries))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConflictSessions(rows)
}

func (s *Service) overlappingSessionsByStudentsInCourse(ctx context.Context, db sqldb.DBTX, courseID pgtype.UUID, startUTC, endUTC time.Time, ignore *pgtype.UUID, ignoreSeries *pgtype.UUID) ([]ConflictSession, error) {
	rows, err := db.Query(ctx, `
		WITH roster AS (
			SELECT student_id
			FROM course_students
			WHERE course_id = $1
		)
		SELECT DISTINCT s.id, s.series_id, s.course_id, s.room_id, s.teacher_id, s.start_at, s.end_at
		FROM roster r
		JOIN student_busy_ranges br ON br.student_id = r.student_id
		JOIN sessions s ON s.id = br.session_id
		WHERE br.deleted_at IS NULL
		  AND s.deleted_at IS NULL
		  AND br.time_range && tstzrange($2, $3, '[)')
		  AND ($4::uuid IS NULL OR s.id <> $4)
		  AND ($5::uuid IS NULL OR s.series_id IS DISTINCT FROM $5)
		ORDER BY s.start_at ASC
		LIMIT 25
	`, courseID, startUTC, endUTC, ignoreUUID(ignore), ignoreUUID(ignoreSeries))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConflictSessions(rows)
}

type conflictRows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}

func scanConflictSessions(rows conflictRows) ([]ConflictSession, error) {
	var out []ConflictSession
	for rows.Next() {
		var (
			id        pgtype.UUID
			seriesID  pgtype.UUID
			courseID  pgtype.UUID
			roomID    pgtype.UUID
			teacherID pgtype.UUID
			startAt   pgtype.Timestamptz
			endAt     pgtype.Timestamptz
		)
		if err := rows.Scan(&id, &seriesID, &courseID, &roomID, &teacherID, &startAt, &endAt); err != nil {
			rows.Close()
			return nil, err
		}
		idStr, err := uuidString(id)
		if err != nil {
			rows.Close()
			return nil, err
		}
		courseStr, err := uuidString(courseID)
		if err != nil {
			rows.Close()
			return nil, err
		}
		roomStr, err := uuidStringPtr(roomID)
		if err != nil {
			rows.Close()
			return nil, err
		}
		teacherStr, err := uuidString(teacherID)
		if err != nil {
			rows.Close()
			return nil, err
		}
		var seriesStr *string
		if seriesID.Valid {
			v, err := uuidString(seriesID)
			if err != nil {
				rows.Close()
				return nil, err
			}
			seriesStr = &v
		}
		if !startAt.Valid || !endAt.Valid {
			continue
		}
		out = append(out, ConflictSession{
			SessionID: idStr,
			SeriesID:  seriesStr,
			CourseID:  courseStr,
			RoomID:    roomStr,
			TeacherID: teacherStr,
			StartAt:   startAt.Time.UTC().Format(time.RFC3339Nano),
			EndAt:     endAt.Time.UTC().Format(time.RFC3339Nano),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func ignoreUUID(id *pgtype.UUID) any {
	if id == nil || !id.Valid {
		return nil
	}
	s, err := uuidString(*id)
	if err != nil {
		return nil
	}
	return s
}
