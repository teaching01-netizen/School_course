package scheduling

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/series"
)

// BatchPreflightConflict represents a conflict found during batch evaluation.
type BatchPreflightConflict struct {
	OccurrenceOrdinal int
	Kind              ConflictKind
	Requested         ConflictRequested
	Conflicts         []ConflictSession
	TotalConflicts    int
	Truncated         bool
}

// BatchPreflightResult wraps a batch preflight conflict. Conflict is nil when
// no conflict was found.
type BatchPreflightResult struct {
	Conflict *BatchPreflightConflict
}

// preflightSeriesBatch evaluates all occurrences of a proposed series in a
// constant number of SQL queries using the unnest batch pattern. It returns
// the earliest conflict (in conflict-priority order) or nil if all
// occurrences pass.
func (s *Service) preflightSeriesBatch(
	ctx context.Context,
	db sqldb.DBTX,
	q *sqldb.Queries,
	params PreflightSeriesParams,
	occurrences []series.Occurrence,
) (*Err, error) {
	if len(occurrences) == 0 {
		return nil, nil
	}

	ps, err := newPreflightStrings(params.CourseID, params.RoomID, params.TeacherID)
	if err != nil {
		return nil, err
	}

	// 1. Teacher membership (advisory) — checked once, same as preflightSlot.
	// Courses with NO assigned teachers return a stable conflict here; the
	// transactional write path remains authoritative.
	if params.CourseID.Valid && params.TeacherID.Valid {
		m, err := q.CourseTeacherMembershipGet(ctx, sqldb.CourseTeacherMembershipGetParams{CourseID: params.CourseID, TeacherID: params.TeacherID})
		if err != nil {
			return nil, fmt.Errorf("check teacher membership: %w", err)
		}
		if !m.CourseExists {
			return &Err{
				Code:    ErrCourseNotFound,
				Message: "Course not found.",
				Details: ConflictDetails{Kind: ConflictKindCourseNotFound, Requested: ps.conflictRequested(occurrences[0].StartUTC, occurrences[0].EndUTC, nil)},
			}, nil
		}
		if !m.HasTeachers {
			return &Err{
				Code:    ErrCourseHasNoTeachers,
				Message: "This course has no assigned teachers. Please configure teacher assignments before scheduling.",
				Details: ConflictDetails{
					Kind:      ConflictKindCourseHasNoTeachers,
					Conflicts: nil,
					Requested: ps.conflictRequested(occurrences[0].StartUTC, occurrences[0].EndUTC, nil),
				},
			}, nil
		}
		if m.HasTeachers && !m.Assigned {
			return &Err{
				Code:    ErrTeacherNotAssigned,
				Message: "The selected teacher is not assigned to this course.",
				Details: ConflictDetails{
					Kind:      ConflictKindTeacherNotAssigned,
					Conflicts: nil,
					Requested: ps.conflictRequested(occurrences[0].StartUTC, occurrences[0].EndUTC, nil),
				},
			}, nil
		}
	}

	// 1b. Resource validation — verify all referenced resources exist, same as preflightSlot.
	resources, err := q.SchedulingResourcesGet(ctx, sqldb.SchedulingResourcesGetParams{CourseID: params.CourseID, TeacherID: params.TeacherID, RoomID: params.RoomID})
	if err != nil {
		return nil, fmt.Errorf("check resources: %w", err)
	}
	if !resources.CourseExists {
		return &Err{Code: ErrCourseNotFound, Message: "Course not found.", Details: ConflictDetails{Kind: ConflictKindCourseNotFound, Requested: ps.conflictRequested(occurrences[0].StartUTC, occurrences[0].EndUTC, nil)}}, nil
	}
	if !resources.TeacherExists {
		return &Err{Code: ErrTeacherNotFound, Message: "Teacher not found.", Details: ConflictDetails{Kind: ConflictKindTeacherNotFound, Requested: ps.conflictRequested(occurrences[0].StartUTC, occurrences[0].EndUTC, nil)}}, nil
	}
	if !resources.TeacherActive {
		return &Err{Code: ErrTeacherInactive, Message: "Teacher is inactive.", Details: ConflictDetails{Kind: ConflictKindTeacherInactive, Requested: ps.conflictRequested(occurrences[0].StartUTC, occurrences[0].EndUTC, nil)}}, nil
	}
	if params.RoomID.Valid && !resources.RoomExists {
		return &Err{Code: ErrRoomNotFound, Message: "Room not found.", Details: ConflictDetails{Kind: ConflictKindRoomNotFound, Requested: ps.conflictRequested(occurrences[0].StartUTC, occurrences[0].EndUTC, nil)}}, nil
	}

	// Helper: build arrays from occurrences for unnest queries.
	ords, starts, ends := buildOccurrenceArrays(occurrences)

	// 2. Batch teacher availability.
	blockedTeacherAvail, err := s.checkTeacherAvailabilityBatch(ctx, db, params.TeacherID, ords, starts, ends)
	if err != nil {
		return nil, fmt.Errorf("batch teacher availability: %w", err)
	}
	if ord := firstBlocked(blockedTeacherAvail); ord >= 0 {
		o := occurrences[ord]
		return &Err{
			Code:    "availability_violation",
			Message: "teacher not available for requested time",
			Details: ConflictDetails{
				Kind:      ConflictKindTeacherAvailability,
				Conflicts: nil,
				Requested: ps.conflictRequested(o.StartUTC, o.EndUTC, nil),
			},
		}, nil
	}

	// 3. Batch room availability.
	if params.RoomID.Valid {
		blockedRoomAvail, err := s.checkRoomAvailabilityBatch(ctx, db, params.RoomID, ords, starts, ends)
		if err != nil {
			return nil, fmt.Errorf("batch room availability: %w", err)
		}
		if ord := firstBlocked(blockedRoomAvail); ord >= 0 {
			o := occurrences[ord]
			return &Err{
				Code:    "availability_violation",
				Message: "room not available for requested time",
				Details: ConflictDetails{
					Kind:      ConflictKindRoomAvailability,
					Conflicts: nil,
					Requested: ps.conflictRequested(o.StartUTC, o.EndUTC, nil),
				},
			}, nil
		}
	}

	// 4. Batch room overlap.
	if params.RoomID.Valid {
		overlaps, err := s.checkRoomOverlapsBatch(ctx, db, params.RoomID, ords, starts, ends, params.SeriesID)
		if err != nil {
			return nil, fmt.Errorf("batch room overlap: %w", err)
		}
		if ord, cs := firstOverlap(overlaps); ord >= 0 {
			o := occurrences[ord]
			return &Err{
				Code:    "schedule_conflict",
				Message: "Schedule conflict",
				Details: ConflictDetails{
					Kind:      ConflictKindRoomOverlap,
					Conflicts: cs,
					Requested: ps.conflictRequested(o.StartUTC, o.EndUTC, nil),
				},
			}, nil
		}
	}

	// 5. Batch teacher overlap.
	overlaps, err := s.checkTeacherOverlapsBatch(ctx, db, params.TeacherID, ords, starts, ends, params.SeriesID)
	if err != nil {
		return nil, fmt.Errorf("batch teacher overlap: %w", err)
	}
	if ord, cs := firstOverlap(overlaps); ord >= 0 {
		o := occurrences[ord]
		return &Err{
			Code:    "schedule_conflict",
			Message: "Schedule conflict",
			Details: ConflictDetails{
				Kind:      ConflictKindTeacherOverlap,
				Conflicts: cs,
				Requested: ps.conflictRequested(o.StartUTC, o.EndUTC, nil),
			},
		}, nil
	}

	// 6. Batch student overlap (via course roster).
	overlaps, err = s.checkStudentOverlapsBatch(ctx, db, params.CourseID, ords, starts, ends, params.SeriesID)
	if err != nil {
		return nil, fmt.Errorf("batch student overlap: %w", err)
	}
	if ord, cs := firstOverlap(overlaps); ord >= 0 {
		o := occurrences[ord]

		sessionIDs := make([]string, len(cs))
		for i, c := range cs {
			sessionIDs[i] = c.SessionID
		}
		conflictingStudents, detailErr := s.conflictingStudentsForOverlap(ctx, db, sessionIDs, nil, params.CourseID)
		if detailErr != nil {
			// Enrichment failed — return conflict with safe partial details.
			return &Err{
				Code:    "schedule_conflict",
				Message: "Schedule conflict",
				Details: ConflictDetails{
					Kind:                ConflictKindStudentOverlap,
					Conflicts:           cs,
					ConflictingStudents: nil,
					Requested:           ps.conflictRequested(o.StartUTC, o.EndUTC, nil),
				},
			}, nil
		}
		return &Err{
			Code:    "schedule_conflict",
			Message: "Schedule conflict",
			Details: ConflictDetails{
				Kind:                ConflictKindStudentOverlap,
				Conflicts:           cs,
				ConflictingStudents: conflictingStudents,
				Requested:           ps.conflictRequested(o.StartUTC, o.EndUTC, nil),
			},
		}, nil
	}

	return nil, nil
}

// buildOccurrenceArrays converts occurrences to parallel slices for unnest queries.
func buildOccurrenceArrays(occurrences []series.Occurrence) (ords []int64, starts, ends []time.Time) {
	n := len(occurrences)
	ords = make([]int64, n)
	starts = make([]time.Time, n)
	ends = make([]time.Time, n)
	for i, o := range occurrences {
		ords[i] = int64(i)
		starts[i] = o.StartUTC
		ends[i] = o.EndUTC
	}
	return
}

// firstBlocked returns the smallest ordinal with value true, or -1.
func firstBlocked(m map[int]bool) int {
	first := -1
	for ord := range m {
		if first < 0 || ord < first {
			first = ord
		}
	}
	return first
}

// firstOverlap returns the smallest ordinal with non-nil conflicts, along
// with its conflict list, or (-1, nil).
func firstOverlap(m map[int][]ConflictSession) (int, []ConflictSession) {
	first := -1
	for ord := range m {
		if first < 0 || ord < first {
			first = ord
		}
	}
	if first < 0 {
		return -1, nil
	}
	return first, m[first]
}

// checkTeacherAvailabilityBatch returns the set of occurrence ordinals whose
// time range is NOT covered by the union of the teacher's availability
// windows. Union containment matches the database trigger policy (00070): a
// session straddling two abutting windows is covered.
func (s *Service) checkTeacherAvailabilityBatch(
	ctx context.Context,
	db sqldb.DBTX,
	teacherID pgtype.UUID,
	ords []int64,
	starts, ends []time.Time,
) (map[int]bool, error) {
	if len(ords) == 0 || !teacherID.Valid {
		return nil, nil
	}

	var hasWindows bool
	if err := db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM teacher_availability WHERE teacher_id = $1 AND deleted_at IS NULL)`,
		teacherID,
	).Scan(&hasWindows); err != nil {
		return nil, err
	}
	if !hasWindows {
		return nil, nil
	}

	rows, err := db.Query(ctx, `
		SELECT r.ordinal
		FROM unnest($1::bigint[], $2::timestamptz[], $3::timestamptz[]) AS r(ordinal, start_at, end_at)
		WHERE NOT EXISTS (
			SELECT 1
			FROM (
				SELECT COALESCE(range_agg(a.time_range), '{}'::tstzmultirange) AS union_range
				FROM teacher_availability a
				WHERE a.teacher_id = $4 AND a.deleted_at IS NULL
			) u
			WHERE u.union_range @> tstzrange(r.start_at, r.end_at, '[)')
		)
	`, ords, starts, ends, teacherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]bool)
	for rows.Next() {
		var ordinal int64
		if err := rows.Scan(&ordinal); err != nil {
			return nil, err
		}
		result[int(ordinal)] = true
	}
	return result, rows.Err()
}

// checkRoomAvailabilityBatch returns the set of occurrence ordinals whose
// time range is NOT covered by the union of the room's availability windows.
func (s *Service) checkRoomAvailabilityBatch(
	ctx context.Context,
	db sqldb.DBTX,
	roomID pgtype.UUID,
	ords []int64,
	starts, ends []time.Time,
) (map[int]bool, error) {
	if len(ords) == 0 || !roomID.Valid {
		return nil, nil
	}

	var hasWindows bool
	if err := db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM room_availability WHERE room_id = $1 AND deleted_at IS NULL)`,
		roomID,
	).Scan(&hasWindows); err != nil {
		return nil, err
	}
	if !hasWindows {
		return nil, nil
	}

	rows, err := db.Query(ctx, `
		SELECT r.ordinal
		FROM unnest($1::bigint[], $2::timestamptz[], $3::timestamptz[]) AS r(ordinal, start_at, end_at)
		WHERE NOT EXISTS (
			SELECT 1
			FROM (
				SELECT COALESCE(range_agg(a.time_range), '{}'::tstzmultirange) AS union_range
				FROM room_availability a
				WHERE a.room_id = $4 AND a.deleted_at IS NULL
			) u
			WHERE u.union_range @> tstzrange(r.start_at, r.end_at, '[)')
		)
	`, ords, starts, ends, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]bool)
	for rows.Next() {
		var ordinal int64
		if err := rows.Scan(&ordinal); err != nil {
			return nil, err
		}
		result[int(ordinal)] = true
	}
	return result, rows.Err()
}

// checkRoomOverlapsBatch returns overlapping sessions for each occurrence
// that conflicts with an existing session in the same room.
func (s *Service) checkRoomOverlapsBatch(
	ctx context.Context,
	db sqldb.DBTX,
	roomID pgtype.UUID,
	ords []int64,
	starts, ends []time.Time,
	ignoreSeries *pgtype.UUID,
) (map[int][]ConflictSession, error) {
	if len(ords) == 0 || !roomID.Valid {
		return nil, nil
	}

	rows, err := db.Query(ctx, `
		SELECT r.ordinal, s.id, s.series_id, s.course_id, s.room_id, s.teacher_id, s.start_at, s.end_at
		FROM unnest($1::bigint[], $2::timestamptz[], $3::timestamptz[]) AS r(ordinal, start_at, end_at)
		JOIN sessions s ON s.deleted_at IS NULL
		  AND s.room_id = $4
		  AND s.time_range && tstzrange(r.start_at, r.end_at, '[)')
		  AND ($5::uuid IS NULL OR s.id <> $5)
		  AND ($6::uuid IS NULL OR s.series_id IS DISTINCT FROM $6)
		ORDER BY r.ordinal, s.start_at
		LIMIT 25
	`, ords, starts, ends, roomID, nil, ignoreUUID(ignoreSeries))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBatchConflictSessions(rows)
}

// checkTeacherOverlapsBatch returns overlapping sessions for each occurrence
// that conflicts with an existing session for the same teacher.
func (s *Service) checkTeacherOverlapsBatch(
	ctx context.Context,
	db sqldb.DBTX,
	teacherID pgtype.UUID,
	ords []int64,
	starts, ends []time.Time,
	ignoreSeries *pgtype.UUID,
) (map[int][]ConflictSession, error) {
	if len(ords) == 0 || !teacherID.Valid {
		return nil, nil
	}

	rows, err := db.Query(ctx, `
		SELECT r.ordinal, s.id, s.series_id, s.course_id, s.room_id, s.teacher_id, s.start_at, s.end_at
		FROM unnest($1::bigint[], $2::timestamptz[], $3::timestamptz[]) AS r(ordinal, start_at, end_at)
		JOIN sessions s ON s.deleted_at IS NULL
		  AND s.teacher_id = $4
		  AND s.time_range && tstzrange(r.start_at, r.end_at, '[)')
		  AND ($5::uuid IS NULL OR s.id <> $5)
		  AND ($6::uuid IS NULL OR s.series_id IS DISTINCT FROM $6)
		ORDER BY r.ordinal, s.start_at
		LIMIT 25
	`, ords, starts, ends, teacherID, nil, ignoreUUID(ignoreSeries))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBatchConflictSessions(rows)
}

// checkStudentOverlapsBatch returns overlapping sessions for each occurrence
// where any student enrolled in the course has a busy range conflict.
func (s *Service) checkStudentOverlapsBatch(
	ctx context.Context,
	db sqldb.DBTX,
	courseID pgtype.UUID,
	ords []int64,
	starts, ends []time.Time,
	ignoreSeries *pgtype.UUID,
) (map[int][]ConflictSession, error) {
	if len(ords) == 0 || !courseID.Valid {
		return nil, nil
	}

	rows, err := db.Query(ctx, `
		SELECT DISTINCT r.ordinal, s.id, s.series_id, s.course_id, s.room_id, s.teacher_id, s.start_at, s.end_at
		FROM unnest($1::bigint[], $2::timestamptz[], $3::timestamptz[]) AS r(ordinal, start_at, end_at)
		JOIN course_students cs ON cs.course_id = $4
		JOIN student_busy_ranges br ON br.student_id = cs.student_id
		JOIN sessions s ON s.id = br.session_id
		WHERE br.deleted_at IS NULL
		  AND s.deleted_at IS NULL
		  AND br.time_range && tstzrange(r.start_at, r.end_at, '[)')
		  AND ($5::uuid IS NULL OR s.id <> $5)
		  AND ($6::uuid IS NULL OR s.series_id IS DISTINCT FROM $6)
		ORDER BY r.ordinal, s.start_at
		LIMIT 25
	`, ords, starts, ends, courseID, nil, ignoreUUID(ignoreSeries))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBatchConflictSessions(rows)
}

// scanBatchConflictSessions scans rows from a batch overlap query into a
// map of ordinal → conflict sessions. The first column must be the ordinal.
func scanBatchConflictSessions(rows pgx.Rows) (map[int][]ConflictSession, error) {
	result := make(map[int][]ConflictSession)
	for rows.Next() {
		var (
			ordinal   int64
			id        pgtype.UUID
			seriesID  pgtype.UUID
			courseID  pgtype.UUID
			roomID    pgtype.UUID
			teacherID pgtype.UUID
			startAt   pgtype.Timestamptz
			endAt     pgtype.Timestamptz
		)
		if err := rows.Scan(&ordinal, &id, &seriesID, &courseID, &roomID, &teacherID, &startAt, &endAt); err != nil {
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

		cs := ConflictSession{
			SessionID: idStr,
			SeriesID:  seriesStr,
			CourseID:  courseStr,
			RoomID:    roomStr,
			TeacherID: teacherStr,
			StartAt:   startAt.Time.UTC().Format(time.RFC3339Nano),
			EndAt:     endAt.Time.UTC().Format(time.RFC3339Nano),
		}
		o := int(ordinal)
		result[o] = append(result[o], cs)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
