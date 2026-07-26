-- name: CreateUser :one
INSERT INTO users (user_id, username, email, password_hash, date_of_birth, created_at, updated_at)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7
)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE user_id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

