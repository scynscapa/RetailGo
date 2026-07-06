-- name: CreateItem :one
INSERT INTO items (upc, created_at, updated_at, item_name, item_desc, item_retail, item_cost, item_picture)
VALUES (
    $1,
    NOW(),
    NOW(),
    $2,
    $3,
    $4,
    $5,
    $6
)
RETURNING *;