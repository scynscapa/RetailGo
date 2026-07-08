-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, access_level, first_name, last_name)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2,
    $3
)
RETURNING *;