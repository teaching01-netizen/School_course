-- +goose Up

CREATE TABLE IF NOT EXISTS student_absences (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  wcode            text NOT NULL,
  course_id        uuid NOT NULL REFERENCES courses(id),
  date_from        date NOT NULL,
  date_to          date NOT NULL,
  reason           text,
  sit_in_course_id uuid REFERENCES courses(id),
  created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_absences_wcode ON student_absences(wcode);
CREATE INDEX IF NOT EXISTS idx_absences_course_dates ON student_absences(course_id, date_from, date_to);
CREATE INDEX IF NOT EXISTS idx_absences_sit_in ON student_absences(sit_in_course_id);

-- +goose Down

DROP TABLE IF EXISTS student_absences;
