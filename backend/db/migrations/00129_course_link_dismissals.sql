-- +goose Up

CREATE TABLE course_link_dismissals (
  course_a uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  course_b uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  actor_user_id uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (course_a, course_b),
  CONSTRAINT course_link_dismissals_canonical_pair CHECK (course_a < course_b)
);

-- +goose Down

DROP TABLE course_link_dismissals;
