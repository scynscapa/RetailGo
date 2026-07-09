-- name: GetTransById :many
WITH MultiItemSales AS (
    SELECT transaction_id 
    FROM transaction_items 
    GROUP BY transaction_id 
    HAVING COUNT(item_id) > 1
)
SELECT t.transaction_id, t.transaction_date, ti.item_id, ti.quantity 
FROM transactions t 
JOIN transaction_items ti ON t.transaction_id = ti.transaction_id 
WHERE t.transaction_id = $1;