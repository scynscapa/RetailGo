-- name: CreateTrans :one
INSERT INTO transactions (customer_id, transaction_date)
VALUES (
    $1,
    NOW()
)
RETURNING *;