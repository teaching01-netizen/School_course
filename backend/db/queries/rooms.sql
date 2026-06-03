-- name: RoomCreate :one
INSERT INTO rooms (name, capacity)
VALUES ($1, $2)
RETURNING id, name, capacity, created_at, updated_at;

-- name: RoomGetByID :one
SELECT id, name, capacity, created_at, updated_at
FROM rooms
WHERE id = $1;

-- name: RoomList :many
SELECT id, name, capacity, created_at, updated_at
FROM rooms
ORDER BY name ASC;

-- name: RoomUpdate :one
UPDATE rooms
SET name = $2, capacity = $3, updated_at = now()
WHERE id = $1
RETURNING id, name, capacity, created_at, updated_at;

-- name: RoomDelete :exec
DELETE FROM rooms
WHERE id = $1;
