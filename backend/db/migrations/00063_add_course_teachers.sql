CREATE TABLE IF NOT EXISTS course_teachers (
  course_id uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  teacher_id uuid NOT NULL REFERENCES users(id),
  PRIMARY KEY (course_id, teacher_id)
);

CREATE INDEX IF NOT EXISTS idx_course_teachers_teacher_id ON course_teachers(teacher_id);
