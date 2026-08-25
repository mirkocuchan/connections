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

-- name: UpdateUser :one
UPDATE users SET
    display_name = COALESCE($1, display_name),
    bio          = COALESCE($2, bio),
    city         = COALESCE($3, city),
    country      = COALESCE($4, country),
    hobbies      = COALESCE($5, hobbies),
    languages    = COALESCE($6, languages),
    updated_at   = NOW()
WHERE user_id = $7
RETURNING *;
-- si $1 tiene un valor, usalo. Si $1 es NULL, conservá display_name.