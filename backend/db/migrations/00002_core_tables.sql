-- +goose Up

CREATE TABLE IF NOT EXISTS rooms (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name        text NOT NULL UNIQUE,
  capacity    integer NULL CHECK (capacity IS NULL OR capacity > 0),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS students (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  wcode         text NOT NULL UNIQUE,
  full_name     text NOT NULL,
  nickname      text NULL,
  email         text NULL,
  student_phone text NULL,
  parent_phone  text NULL,
  notes         text NOT NULL DEFAULT '',
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS courses (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code        text NOT NULL UNIQUE,
  name        text NOT NULL,
  deleted_at  timestamptz NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS course_students (
  course_id   uuid NOT NULL REFERENCES courses(id),
  student_id  uuid NOT NULL REFERENCES students(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (course_id, student_id)
);

-- +goose Down

DROP TABLE IF EXISTS course_students;
DROP TABLE IF EXISTS courses;
DROP TABLE IF EXISTS students;
DROP TABLE IF EXISTS rooms;

