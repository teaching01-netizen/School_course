-- +goose Up

-- Availability blocks (hard constraints)
CREATE TABLE IF NOT EXISTS teacher_availability (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  teacher_id  uuid NOT NULL REFERENCES users(id),
  start_at    timestamptz NOT NULL,
  end_at      timestamptz NOT NULL,
  deleted_at  timestamptz NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT teacher_availability_valid_range CHECK (end_at > start_at)
);

ALTER TABLE teacher_availability
  ADD COLUMN IF NOT EXISTS time_range tstzrange
  GENERATED ALWAYS AS (tstzrange(start_at, end_at, '[)')) STORED;

CREATE TABLE IF NOT EXISTS room_availability (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  room_id     uuid NOT NULL REFERENCES rooms(id),
  start_at    timestamptz NOT NULL,
  end_at      timestamptz NOT NULL,
  deleted_at  timestamptz NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT room_availability_valid_range CHECK (end_at > start_at)
);

ALTER TABLE room_availability
  ADD COLUMN IF NOT EXISTS time_range tstzrange
  GENERATED ALWAYS AS (tstzrange(start_at, end_at, '[)')) STORED;

-- Recurrence (weekly-only; materialized occurrences)
CREATE TABLE IF NOT EXISTS session_series (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  course_id         uuid NOT NULL REFERENCES courses(id),
  room_id           uuid NOT NULL REFERENCES rooms(id),
  teacher_id        uuid NOT NULL REFERENCES users(id),
  institute_tz      text NOT NULL DEFAULT 'Asia/Bangkok',
  weekday           smallint NOT NULL CHECK (weekday BETWEEN 0 AND 6), -- 0=Sunday..6=Saturday
  start_local_time  time NOT NULL,
  duration_minutes  integer NOT NULL CHECK (duration_minutes > 0),
  start_date        date NOT NULL,
  end_date          date NULL,
  count             integer NULL CHECK (count IS NULL OR count > 0),
  deleted_at        timestamptz NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT session_series_end_bound_required CHECK (end_date IS NOT NULL OR count IS NOT NULL)
);

-- Occurrence rows (final gate for conflicts)
CREATE TABLE IF NOT EXISTS sessions (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  series_id   uuid NULL REFERENCES session_series(id),
  course_id   uuid NOT NULL REFERENCES courses(id),
  room_id     uuid NOT NULL REFERENCES rooms(id),
  teacher_id  uuid NOT NULL REFERENCES users(id),
  start_at    timestamptz NOT NULL,
  end_at      timestamptz NOT NULL,
  deleted_at  timestamptz NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sessions_valid_range CHECK (end_at > start_at)
);

ALTER TABLE sessions
  ADD COLUMN IF NOT EXISTS time_range tstzrange
  GENERATED ALWAYS AS (tstzrange(start_at, end_at, '[)')) STORED;

-- Per-session include/exclude overrides.
CREATE TABLE IF NOT EXISTS session_attendance (
  session_id  uuid NOT NULL REFERENCES sessions(id),
  student_id  uuid NOT NULL REFERENCES students(id),
  status      text NOT NULL CHECK (status IN ('included', 'excluded')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (session_id, student_id)
);

-- Student busy ranges (DB-enforced overlap guard).
CREATE TABLE IF NOT EXISTS student_busy_ranges (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  student_id  uuid NOT NULL REFERENCES students(id),
  session_id  uuid NOT NULL REFERENCES sessions(id),
  start_at    timestamptz NOT NULL,
  end_at      timestamptz NOT NULL,
  deleted_at  timestamptz NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT student_busy_ranges_valid_range CHECK (end_at > start_at)
);

ALTER TABLE student_busy_ranges
  ADD COLUMN IF NOT EXISTS time_range tstzrange
  GENERATED ALWAYS AS (tstzrange(start_at, end_at, '[)')) STORED;

-- Overlap constraints (soft-delete aware).
ALTER TABLE sessions
  ADD CONSTRAINT sessions_no_room_overlap
  EXCLUDE USING gist (room_id WITH =, time_range WITH &&)
  WHERE (deleted_at IS NULL);

ALTER TABLE sessions
  ADD CONSTRAINT sessions_no_teacher_overlap
  EXCLUDE USING gist (teacher_id WITH =, time_range WITH &&)
  WHERE (deleted_at IS NULL);

ALTER TABLE student_busy_ranges
  ADD CONSTRAINT student_busy_ranges_no_overlap
  EXCLUDE USING gist (student_id WITH =, time_range WITH &&)
  WHERE (deleted_at IS NULL);

-- +goose Down

ALTER TABLE student_busy_ranges DROP CONSTRAINT IF EXISTS student_busy_ranges_no_overlap;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_no_teacher_overlap;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_no_room_overlap;

DROP TABLE IF EXISTS student_busy_ranges;
DROP TABLE IF EXISTS session_attendance;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS session_series;
DROP TABLE IF EXISTS room_availability;
DROP TABLE IF EXISTS teacher_availability;

