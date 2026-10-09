-- name: UpsertPushToken :exec
INSERT INTO push_tokens (token, user_id)
VALUES (
    $1, 
    $2
)
ON CONFLICT (token)
DO UPDATE SET user_id = EXCLUDED.user_id, updated_at = NOW();

-- name: GetPushTokensByUserID :many
SELECT token FROM push_tokens WHERE user_id = $1;

-- name: DeletePushToken :exec
DELETE FROM push_tokens WHERE token = $1;

-- name: DeletePushTokenForUser :exec
DELETE FROM push_tokens WHERE token = $1 AND user_id = $2;