-- +goose Up

-- Remove soft-delete support from courses and subjects.
-- Hard delete is now the only delete strategy.

-- Purge any previously soft-deleted rows BEFORE dropping the column.
DELETE FROM courses WHERE deleted_at IS NOT NULL;
DELETE FROM subjects WHERE deleted_at IS NOT NULL;

-- Drop the soft-delete columns.
ALTER TABLE courses DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE subjects DROP COLUMN IF EXISTS deleted_at;

-- Add ON DELETE CASCADE on child tables that should be cleaned up
-- when a course is hard-deleted.

-- course_students: remove enrollment rows when course is deleted.
ALTER TABLE course_students
  DROP CONSTRAINT IF EXISTS course_students_course_id_fkey,
  ADD CONSTRAINT course_students_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE;

-- session_series: remove series when course is deleted.
ALTER TABLE session_series
  DROP CONSTRAINT IF EXISTS session_series_course_id_fkey,
  ADD CONSTRAINT session_series_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE;

-- sessions: remove sessions when course is deleted.
ALTER TABLE sessions
  DROP CONSTRAINT IF EXISTS sessions_course_id_fkey,
  ADD CONSTRAINT sessions_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE;

-- student_absences: remove absence records when course is deleted.
ALTER TABLE student_absences
  DROP CONSTRAINT IF EXISTS student_absences_course_id_fkey,
  ADD CONSTRAINT student_absences_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE;

ALTER TABLE student_absences
  DROP CONSTRAINT IF EXISTS student_absences_sit_in_course_id_fkey,
  ADD CONSTRAINT student_absences_sit_in_course_id_fkey
    FOREIGN KEY (sit_in_course_id) REFERENCES courses(id) ON DELETE SET NULL;

-- course_roster_overrides: remove overrides when course is deleted.
ALTER TABLE course_roster_overrides
  DROP CONSTRAINT IF EXISTS course_roster_overrides_course_id_fkey,
  ADD CONSTRAINT course_roster_overrides_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE;

-- crm_pending_diffs: remove diffs when course is deleted.
ALTER TABLE crm_pending_diffs
  DROP CONSTRAINT IF EXISTS crm_pending_diffs_course_id_fkey,
  ADD CONSTRAINT crm_pending_diffs_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE;

-- Add ON DELETE CASCADE on child tables for subjects.

-- courses: remove courses when subject is deleted (already has subject_id FK).
ALTER TABLE courses
  DROP CONSTRAINT IF EXISTS courses_subject_id_fkey,
  ADD CONSTRAINT courses_subject_id_fkey
    FOREIGN KEY (subject_id) REFERENCES subjects(id) ON DELETE CASCADE;

-- root_course_groups: remove groups when subject is deleted.
ALTER TABLE root_course_groups
  DROP CONSTRAINT IF EXISTS root_course_groups_subject_id_fkey,
  ADD CONSTRAINT root_course_groups_subject_id_fkey
    FOREIGN KEY (subject_id) REFERENCES subjects(id) ON DELETE CASCADE;

-- absence_extensions: remove extensions when subject is deleted.
ALTER TABLE absence_extensions
  DROP CONSTRAINT IF EXISTS absence_extensions_subject_id_fkey,
  ADD CONSTRAINT absence_extensions_subject_id_fkey
    FOREIGN KEY (subject_id) REFERENCES subjects(id) ON DELETE CASCADE;

-- Add ON DELETE CASCADE on child tables for rooms.

-- session_series: remove series when room is deleted.
ALTER TABLE session_series
  DROP CONSTRAINT IF EXISTS session_series_room_id_fkey,
  ADD CONSTRAINT session_series_room_id_fkey
    FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE;

-- sessions: remove sessions when room is deleted.
ALTER TABLE sessions
  DROP CONSTRAINT IF EXISTS sessions_room_id_fkey,
  ADD CONSTRAINT sessions_room_id_fkey
    FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE;

-- room_availability: remove availability blocks when room is deleted.
ALTER TABLE room_availability
  DROP CONSTRAINT IF EXISTS room_availability_room_id_fkey,
  ADD CONSTRAINT room_availability_room_id_fkey
    FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE;

-- +goose Down

-- Restore soft-delete columns (data is lost, this is a best-effort rollback).
ALTER TABLE courses ADD COLUMN deleted_at timestamptz NULL;
ALTER TABLE subjects ADD COLUMN deleted_at timestamptz NULL;

-- Revert FK constraints to RESTRICT (default).
ALTER TABLE course_students
  DROP CONSTRAINT IF EXISTS course_students_course_id_fkey,
  ADD CONSTRAINT course_students_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES courses(id);

ALTER TABLE session_series
  DROP CONSTRAINT IF EXISTS session_series_course_id_fkey,
  ADD CONSTRAINT session_series_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES courses(id);

ALTER TABLE sessions
  DROP CONSTRAINT IF EXISTS sessions_course_id_fkey,
  ADD CONSTRAINT sessions_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES courses(id);

ALTER TABLE student_absences
  DROP CONSTRAINT IF EXISTS student_absences_course_id_fkey,
  ADD CONSTRAINT student_absences_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES courses(id);

ALTER TABLE student_absences
  DROP CONSTRAINT IF EXISTS student_absences_sit_in_course_id_fkey,
  ADD CONSTRAINT student_absences_sit_in_course_id_fkey
    FOREIGN KEY (sit_in_course_id) REFERENCES courses(id);

ALTER TABLE course_roster_overrides
  DROP CONSTRAINT IF EXISTS course_roster_overrides_course_id_fkey,
  ADD CONSTRAINT course_roster_overrides_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES courses(id);

ALTER TABLE crm_pending_diffs
  DROP CONSTRAINT IF EXISTS crm_pending_diffs_course_id_fkey,
  ADD CONSTRAINT crm_pending_diffs_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES courses(id);

ALTER TABLE courses
  DROP CONSTRAINT IF EXISTS courses_subject_id_fkey,
  ADD CONSTRAINT courses_subject_id_fkey
    FOREIGN KEY (subject_id) REFERENCES subjects(id);

ALTER TABLE root_course_groups
  DROP CONSTRAINT IF EXISTS root_course_groups_subject_id_fkey,
  ADD CONSTRAINT root_course_groups_subject_id_fkey
    FOREIGN KEY (subject_id) REFERENCES subjects(id);

ALTER TABLE absence_extensions
  DROP CONSTRAINT IF EXISTS absence_extensions_subject_id_fkey,
  ADD CONSTRAINT absence_extensions_subject_id_fkey
    FOREIGN KEY (subject_id) REFERENCES subjects(id);

ALTER TABLE session_series
  DROP CONSTRAINT IF EXISTS session_series_room_id_fkey,
  ADD CONSTRAINT session_series_room_id_fkey
    FOREIGN KEY (room_id) REFERENCES rooms(id);

ALTER TABLE sessions
  DROP CONSTRAINT IF EXISTS sessions_room_id_fkey,
  ADD CONSTRAINT sessions_room_id_fkey
    FOREIGN KEY (room_id) REFERENCES rooms(id);

ALTER TABLE room_availability
  DROP CONSTRAINT IF EXISTS room_availability_room_id_fkey,
  ADD CONSTRAINT room_availability_room_id_fkey
    FOREIGN KEY (room_id) REFERENCES rooms(id);
