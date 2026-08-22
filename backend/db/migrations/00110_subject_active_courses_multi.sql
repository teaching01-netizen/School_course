-- +goose Up
-- Allow multiple active courses per subject: a subject's parallel classes can
-- all be open for booking at the same time. The (subject_id) primary key that
-- enforced a single active pointer becomes (subject_id, course_id), and
-- unique(course_id) keeps each course paired with only its own subject.

ALTER TABLE subject_active_courses DROP CONSTRAINT subject_active_courses_pkey;
ALTER TABLE subject_active_courses ADD PRIMARY KEY (subject_id, course_id);
DROP INDEX IF EXISTS idx_subject_active_courses_course;
ALTER TABLE subject_active_courses ADD CONSTRAINT uq_subject_active_courses_course UNIQUE (course_id);

-- +goose Down
-- Restore the single-active-pointer constraint. Subjects that accumulated
-- several active courses keep the most recently updated one.

ALTER TABLE subject_active_courses DROP CONSTRAINT uq_subject_active_courses_course;
DELETE FROM subject_active_courses
WHERE course_id NOT IN (
    SELECT DISTINCT ON (subject_id) course_id
    FROM subject_active_courses
    ORDER BY subject_id, updated_at DESC, course_id DESC
);
ALTER TABLE subject_active_courses ADD PRIMARY KEY (subject_id);
CREATE INDEX idx_subject_active_courses_course ON subject_active_courses(course_id);
