-- name: BackfillEligibleAssignments :many
-- Find absence_sit_ins rows that need backfill (snapshot is unavailable).
-- Uses FOR UPDATE SKIP LOCKED for concurrent safety.
SELECT
  asi.id,
  asi.absence_id,
  asi.session_id,
  asi.session_version_at_assignment,
  asi.assigned_at,
  COALESCE(s.version, 0) AS current_session_version,
  (s.deleted_at IS NOT NULL) AS current_session_deleted
FROM absence_sit_ins asi
LEFT JOIN sessions s ON s.id = asi.session_id
WHERE asi.snapshot_quality = 'unavailable'
ORDER BY asi.assigned_at, asi.id
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: BackfillCountEligible :one
-- Count total eligible rows for reporting.
SELECT count(*)::int4 AS total
FROM absence_sit_ins
WHERE snapshot_quality = 'unavailable';

-- name: BackfillAssignmentEventSnapshot :one
-- Evidence source 1: Find exact assignment event with snapshot data.
SELECT
  ase.created_at AS captured_at,
  s.version AS session_version
FROM absence_sit_in_assignment_events ase
JOIN sessions s ON s.id = ase.new_session_id
WHERE ase.absence_id = $1
  AND ase.new_session_id = $2
  AND ase.action = 'assigned'
  AND s.version = $3
ORDER BY ase.created_at DESC
LIMIT 1;

-- name: BackfillSessionRevisionSnapshot :one
-- Evidence source 2: Find session_changes record matching stored assignment version.
SELECT
  sc.after_snapshot AS snapshot,
  sc.created_at AS captured_at
FROM session_changes sc
WHERE sc.session_id = $1
  AND sc.session_version = $2
LIMIT 1;

-- name: BackfillSessionChangeBeforeSnapshot :one
-- Evidence source 3a: Find session_changes where this assignment's version was the before state.
SELECT
  sc.before_snapshot AS snapshot,
  sc.created_at AS captured_at
FROM session_changes sc
WHERE sc.session_id = $1
  AND sc.session_version = ($2 + 1)
LIMIT 1;

-- name: BackfillSessionChangeAfterSnapshot :one
-- Evidence source 3b: Find session_changes where this version is the after state.
SELECT
  sc.after_snapshot AS snapshot,
  sc.created_at AS captured_at
FROM session_changes sc
WHERE sc.session_id = $1
  AND sc.session_version = $2
LIMIT 1;

-- name: BackfillUpdateSnapshot :exec
-- Update a single assignment with reconstructed snapshot data.
UPDATE absence_sit_ins
SET
  session_snapshot_at_assignment = $2,
  snapshot_schema_version = $3,
  snapshot_captured_at = $4,
  snapshot_quality = $5,
  snapshot_source = $6
WHERE id = $1;

-- name: BackfillSampleByQuality :many
-- Sample records from each quality category for manual validation.
SELECT
  asi.id,
  asi.absence_id,
  asi.session_id,
  asi.snapshot_quality,
  asi.snapshot_source,
  asi.snapshot_captured_at,
  asi.session_snapshot_at_assignment
FROM absence_sit_ins asi
WHERE asi.snapshot_quality = $1
ORDER BY RANDOM()
LIMIT $2;
