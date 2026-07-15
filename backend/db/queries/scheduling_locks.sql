-- name: CoursesLockOrdered :many
SELECT id FROM courses WHERE id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;

-- name: StudentsLockOrdered :many
SELECT id FROM students WHERE id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;

-- name: UsersLockOrdered :many
SELECT id FROM users WHERE id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;

-- name: RoomsLockOrdered :many
SELECT id FROM rooms WHERE id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;

-- name: SessionsLockOrdered :many
SELECT id FROM sessions WHERE id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;

-- name: SeriesLockOrdered :many
SELECT id FROM session_series WHERE id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;
