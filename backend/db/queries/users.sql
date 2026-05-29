-- name: UserListByRoleActive :many
SELECT id, username, role, deleted_at, created_at, updated_at
FROM users
WHERE deleted_at IS NULL
  AND role = $1
ORDER BY username ASC;

