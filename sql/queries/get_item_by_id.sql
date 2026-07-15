-- name: GetItemById :one
SELECT *
FROM items
WHERE item_id = $1;