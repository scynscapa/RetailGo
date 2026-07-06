-- name: GetItemByUpc :one
SELECT *
FROM items
WHERE upc = $1;