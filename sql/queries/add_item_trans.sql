-- name: AddItemTrans :many
INSERT INTO transaction_items (item_id, transaction_id, quantity, price) 
VALUES (
    $1,
    $2,
    $3,
    $4
)
ON CONFLICT (item_id) DO UPDATE SET quantity = transaction_items.quantity + EXCLUDED.quantity
RETURNING *;