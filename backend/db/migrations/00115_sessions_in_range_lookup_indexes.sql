-- +goose NO TRANSACTION
-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS course_students_student_status_course_idx
    ON course_students (student_id, status, course_id);

-- +goose Down

DROP INDEX CONCURRENTLY IF EXISTS course_students_student_status_course_idx;
