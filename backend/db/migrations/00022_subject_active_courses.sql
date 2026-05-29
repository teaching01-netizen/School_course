-- +goose Up

CREATE TABLE IF NOT EXISTS subject_active_courses (
  subject_id uuid NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
  course_id   uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (subject_id)
);

CREATE INDEX IF NOT EXISTS idx_subject_active_courses_course ON subject_active_courses(course_id);

-- +goose Down

DROP TABLE IF EXISTS subject_active_courses;
