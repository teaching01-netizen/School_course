package backfill

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/snapshot"
)

// EvidenceFinder implements the evidence hierarchy for snapshot reconstruction.
type EvidenceFinder struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewEvidenceFinder creates a new EvidenceFinder.
func NewEvidenceFinder(pool *pgxpool.Pool, logger *slog.Logger) *EvidenceFinder {
	return &EvidenceFinder{
		pool:   pool,
		logger: logger,
	}
}

// FindEvidence attempts to find snapshot evidence using the priority hierarchy:
// 1. Exact assignment event snapshot
// 2. Immutable session revision matching stored assignment version
// 3. Matching session_changes.before_snapshot or after_snapshot
// 4. Current session only when current version equals stored assignment version
// 5. Otherwise leave unavailable
func (ef *EvidenceFinder) FindEvidence(
	ctx context.Context,
	assignment *EligibleAssignment,
) (*EvidenceResult, error) {
	// Source 1: Exact assignment event snapshot
	if result, err := ef.findAssignmentEvent(ctx, assignment); err != nil {
		return nil, fmt.Errorf("assignment event lookup: %w", err)
	} else if result != nil {
		return result, nil
	}

	// Source 2: Session revision matching stored assignment version
	if assignment.SessionVersionAtAssignment != nil {
		if result, err := ef.findSessionRevision(ctx, assignment); err != nil {
			return nil, fmt.Errorf("session revision lookup: %w", err)
		} else if result != nil {
			return result, nil
		}
	}

	// Source 3: Session changes before/after snapshot
	if assignment.SessionVersionAtAssignment != nil {
		if result, err := ef.findSessionChangeSnapshot(ctx, assignment); err != nil {
			return nil, fmt.Errorf("session change lookup: %w", err)
		} else if result != nil {
			return result, nil
		}
	}

	// Source 4: Current session (only if version matches)
	if assignment.SessionVersionAtAssignment != nil &&
		!assignment.CurrentSessionDeleted &&
		assignment.CurrentSessionVersion == *assignment.SessionVersionAtAssignment {
		if result, err := ef.findCurrentSession(ctx, assignment); err != nil {
			return nil, fmt.Errorf("current session lookup: %w", err)
		} else if result != nil {
			return result, nil
		}
	}

	// Source 5: Unavailable
	return &EvidenceResult{
		Quality: QualityUnavailable,
		Source:  SourceNone,
	}, nil
}

// findAssignmentEvent looks for an exact assignment event with snapshot data.
// This is evidence source 1.
func (ef *EvidenceFinder) findAssignmentEvent(
	ctx context.Context,
	assignment *EligibleAssignment,
) (*EvidenceResult, error) {
	// Assignment events don't store snapshots directly, but they record
	// the session version at assignment time. If we can find the event,
	// we can use it to identify which version was assigned.
	var capturedAt pgtype.Timestamptz
	var sessionVersion int32

	err := ef.pool.QueryRow(ctx, `
		SELECT ase.created_at, s.version
		FROM absence_sit_in_assignment_events ase
		JOIN sessions s ON s.id = ase.new_session_id
		WHERE ase.absence_id = $1
		  AND ase.new_session_id = $2
		  AND ase.action = 'assigned'
		ORDER BY ase.created_at DESC
		LIMIT 1
	`, assignment.AbsenceID, assignment.SessionID).Scan(&capturedAt, &sessionVersion)

	if err != nil {
		// No assignment event found
		return nil, nil
	}

	ef.logger.Debug("found assignment event",
		"assignment_id", assignment.ID,
		"session_version", sessionVersion,
	)

	return &EvidenceResult{
		Quality:    QualityExact,
		Source:     SourceAssignmentEvent,
		Version:    &sessionVersion,
		CapturedAt: &capturedAt.Time,
	}, nil
}

// findSessionRevision looks for a session_changes record matching the stored version.
// This is evidence source 2.
func (ef *EvidenceFinder) findSessionRevision(
	ctx context.Context,
	assignment *EligibleAssignment,
) (*EvidenceResult, error) {
	if assignment.SessionVersionAtAssignment == nil {
		return nil, nil
	}

	var snapshotData []byte
	var capturedAt pgtype.Timestamptz

	err := ef.pool.QueryRow(ctx, `
		SELECT sc.after_snapshot, sc.created_at
		FROM session_changes sc
		WHERE sc.session_id = $1
		  AND sc.session_version = $2
		LIMIT 1
	`, assignment.SessionID, *assignment.SessionVersionAtAssignment).Scan(&snapshotData, &capturedAt)

	if err != nil {
		// No session revision found
		return nil, nil
	}

	// Validate the snapshot
	if _, err := snapshot.DecodeSessionSnapshotV1(snapshotData); err != nil {
		ef.logger.Warn("invalid snapshot in session revision",
			"assignment_id", assignment.ID,
			"error", err,
		)
		return nil, nil
	}

	ef.logger.Debug("found session revision snapshot",
		"assignment_id", assignment.ID,
		"version", *assignment.SessionVersionAtAssignment,
	)

	return &EvidenceResult{
		Quality:    QualityExact,
		Source:     SourceSessionRevision,
		Snapshot:   snapshotData,
		Version:    assignment.SessionVersionAtAssignment,
		CapturedAt: &capturedAt.Time,
	}, nil
}

// findSessionChangeSnapshot looks for session_changes before/after snapshots.
// This is evidence source 3.
func (ef *EvidenceFinder) findSessionChangeSnapshot(
	ctx context.Context,
	assignment *EligibleAssignment,
) (*EvidenceResult, error) {
	if assignment.SessionVersionAtAssignment == nil {
		return nil, nil
	}

	// Try after_snapshot first (version matches)
	var snapshotData []byte
	var capturedAt pgtype.Timestamptz

	err := ef.pool.QueryRow(ctx, `
		SELECT sc.after_snapshot, sc.created_at
		FROM session_changes sc
		WHERE sc.session_id = $1
		  AND sc.session_version = $2
		LIMIT 1
	`, assignment.SessionID, *assignment.SessionVersionAtAssignment).Scan(&snapshotData, &capturedAt)

	if err == nil {
		if _, err := snapshot.DecodeSessionSnapshotV1(snapshotData); err == nil {
			ef.logger.Debug("found session change after_snapshot",
				"assignment_id", assignment.ID,
				"version", *assignment.SessionVersionAtAssignment,
			)
			return &EvidenceResult{
				Quality:    QualityReconstructed,
				Source:     SourceSessionChange,
				Snapshot:   snapshotData,
				Version:    assignment.SessionVersionAtAssignment,
				CapturedAt: &capturedAt.Time,
			}, nil
		}
	}

	// Try before_snapshot (version + 1 matches)
	nextVersion := *assignment.SessionVersionAtAssignment + 1
	err = ef.pool.QueryRow(ctx, `
		SELECT sc.before_snapshot, sc.created_at
		FROM session_changes sc
		WHERE sc.session_id = $1
		  AND sc.session_version = $2
		LIMIT 1
	`, assignment.SessionID, nextVersion).Scan(&snapshotData, &capturedAt)

	if err == nil {
		if _, err := snapshot.DecodeSessionSnapshotV1(snapshotData); err == nil {
			ef.logger.Debug("found session change before_snapshot",
				"assignment_id", assignment.ID,
				"version", nextVersion,
			)
			return &EvidenceResult{
				Quality:    QualityReconstructed,
				Source:     SourceSessionChange,
				Snapshot:   snapshotData,
				Version:    assignment.SessionVersionAtAssignment,
				CapturedAt: &capturedAt.Time,
			}, nil
		}
	}

	return nil, nil
}

// findCurrentSession builds a snapshot from current session data.
// This is evidence source 4 - only used when current version matches stored version.
func (ef *EvidenceFinder) findCurrentSession(
	ctx context.Context,
	assignment *EligibleAssignment,
) (*EvidenceResult, error) {
	// Fetch current session data with joined entities
	var (
		courseCode  string
		courseName  string
		teacherName string
		roomName    *string
		sessionID   pgtype.UUID
		seriesID    pgtype.UUID
		courseID    pgtype.UUID
		roomID      pgtype.UUID
		teacherID   pgtype.UUID
		startAt     pgtype.Timestamptz
		endAt       pgtype.Timestamptz
		version     int32
	)

	err := ef.pool.QueryRow(ctx, `
		SELECT s.id, s.series_id, s.course_id, s.room_id, s.teacher_id,
		       s.start_at, s.end_at, s.version,
		       COALESCE(c.code, '') AS course_code,
		       COALESCE(c.name, '') AS course_name,
		       COALESCE(u.username, '') AS teacher_name,
		       r.name AS room_name
		FROM sessions s
		JOIN courses c ON c.id = s.course_id
		JOIN users u ON u.id = s.teacher_id
		LEFT JOIN rooms r ON r.id = s.room_id
		WHERE s.id = $1 AND s.deleted_at IS NULL
	`, assignment.SessionID).Scan(
		&sessionID, &seriesID, &courseID, &roomID, &teacherID,
		&startAt, &endAt, &version,
		&courseCode, &courseName, &teacherName, &roomName,
	)

	if err != nil {
		// Session not found or deleted
		return nil, nil
	}

	// Build snapshot
	session := snapshot.AssignmentSession{
		ID:         uuidFromPgtype(sessionID),
		SeriesID:   ptrUUID(uuidFromPgtype(seriesID)),
		CourseID:   uuidFromPgtype(courseID),
		RoomID:     ptrUUID(uuidFromPgtype(roomID)),
		TeacherID:  uuidFromPgtype(teacherID),
		StartAt:    startAt.Time.UTC(),
		EndAt:      endAt.Time.UTC(),
		Version:    version,
		CourseCode: courseCode,
		CourseName: courseName,
		TeacherName: teacherName,
		RoomName:   roomName,
	}

	capturedAt := time.Now().UTC()
	snap := snapshot.BuildSessionSnapshotV1(session, capturedAt, "Asia/Bangkok")

	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}

	ef.logger.Debug("built snapshot from current session",
		"assignment_id", assignment.ID,
		"session_version", version,
	)

	return &EvidenceResult{
		Quality:    QualityReconstructed,
		Source:     SourceCurrentSession,
		Snapshot:   data,
		Version:    &version,
		CapturedAt: &capturedAt,
	}, nil
}

// Helper functions

func uuidFromPgtype(u pgtype.UUID) uuid.UUID {
	if !u.Valid {
		return uuid.Nil
	}
	parsed, err := uuid.FromBytes(u.Bytes[:])
	if err != nil {
		return uuid.Nil
	}
	return parsed
}

func ptrUUID(u uuid.UUID) *uuid.UUID {
	if u == uuid.Nil {
		return nil
	}
	return &u
}
