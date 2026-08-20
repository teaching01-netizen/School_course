-- +goose NO TRANSACTION
-- +goose Up

-- Search indexes: every text-search predicate in the app is a leading-wildcard
-- ILIKE ('%' || $q || '%') over name-like columns. B-tree indexes cannot serve
-- those patterns; pg_trgm GIN indexes can. See docs/search-audit-2026-08-19.md.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Students list + absence-inbox name search.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_students_full_name_trgm ON students USING gin (full_name gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_students_nickname_trgm ON students USING gin (COALESCE(nickname, '') gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_students_wcode_trgm ON students USING gin (wcode gin_trgm_ops);

-- Course list search (code / name / subject / teacher).
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_courses_code_trgm ON courses USING gin (code gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_courses_name_trgm ON courses USING gin (name gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_subjects_code_trgm ON subjects USING gin (code gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_subjects_name_trgm ON subjects USING gin (name gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_full_name_trgm ON users USING gin (COALESCE(full_name, '') gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_username_trgm ON users USING gin (username gin_trgm_ops);

-- Absence inbox snapshot name search (student_absences.student_name).
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_absences_student_name_trgm ON student_absences USING gin (COALESCE(student_name, '') gin_trgm_ops);

-- Cross-study assignment search (a.wcode + joined students.full_name).
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_cross_study_assignments_wcode_trgm ON crm_cross_study_assignments USING gin (wcode gin_trgm_ops);

-- +goose Down

DROP INDEX CONCURRENTLY IF EXISTS idx_cross_study_assignments_wcode_trgm;
DROP INDEX CONCURRENTLY IF EXISTS idx_absences_student_name_trgm;
DROP INDEX CONCURRENTLY IF EXISTS idx_users_username_trgm;
DROP INDEX CONCURRENTLY IF EXISTS idx_users_full_name_trgm;
DROP INDEX CONCURRENTLY IF EXISTS idx_subjects_name_trgm;
DROP INDEX CONCURRENTLY IF EXISTS idx_subjects_code_trgm;
DROP INDEX CONCURRENTLY IF EXISTS idx_courses_name_trgm;
DROP INDEX CONCURRENTLY IF EXISTS idx_courses_code_trgm;
DROP INDEX CONCURRENTLY IF EXISTS idx_students_wcode_trgm;
DROP INDEX CONCURRENTLY IF EXISTS idx_students_nickname_trgm;
DROP INDEX CONCURRENTLY IF EXISTS idx_students_full_name_trgm;