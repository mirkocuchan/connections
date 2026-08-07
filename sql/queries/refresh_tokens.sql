-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (token_hash, user_id, expires_at)
VALUES (
    $1,
    $2,
    $3
)
RETURNING *;

-- name: GetRefreshTokenForUser :one
SELECT * FROM refresh_tokens WHERE user_id = $1;

-- name: GetRefreshTokensForUser :many
SELECT * FROM refresh_tokens WHERE user_id = $1;

-- name: GetRefreshTokenWithHash :one
SELECT * FROM refresh_tokens WHERE token_hash = $1;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = Now(), updated_at = Now() WHERE token_hash = $1;