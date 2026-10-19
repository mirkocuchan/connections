-- name: UpsertPasswordReset :exec
INSERT INTO password_resets (user_id, code_hash, expires_at, attempts, created_at)
VALUES (
    $1,
    $2,
    $3,
    0, 
    (now() at time zone 'utc')
) ON CONFLICT (user_id) DO UPDATE
SET code_hash  = EXCLUDED.code_hash,
    expires_at = EXCLUDED.expires_at,
    attempts   = 0,
    created_at = EXCLUDED.created_at; --poné en la fila existente el code_hash que venía en el INSERT, reemplaza lo que habia con lo nuevo que requesteaste.

-- name: GetPasswordReset :one
SELECT * FROM password_resets WHERE user_id = $1;

-- name: IncrementResetAttempts :exec
UPDATE password_resets SET attempts = attempts + 1 WHERE user_id = $1;

-- name: DeletePasswordReset :exec
DELETE FROM password_resets WHERE user_id = $1;