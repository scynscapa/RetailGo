-- name: AddItemTrans :many
INSERT INTO transaction_items (item_id, transaction_id, quantity, price) 
VALUES (
    $1,
    $2,
    $3,
    $4
)
RETURNING *;