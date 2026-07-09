-- name: GetUserByUserName :one
SELECT *
FROM users
WHERE user_name = $1;