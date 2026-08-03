-- name: CreateUser :one
INSERT INTO users (username, email, password_hash, date_of_birth)
VALUES (
    $1,
    $2,
    $3,
    $4
)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE user_id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

