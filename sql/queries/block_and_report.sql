-- name: CreateBlock :one
INSERT INTO blocks (blocker_id, blocked_id, created_at)
VALUES (
    $1,
    $2,
    $3
)
RETURNING *;

-- name: GetBlocksByBlockerID :many
SELECT * FROM blocks WHERE blocker_id = $1;

-- name: DeleteBlockByBlockerAndBlockedID :exec
DELETE FROM blocks WHERE blocker_id = $1 AND blocked_id = $2;

-- name: existsBlockByBlockerAndBlockedID :one
SELECT * FROM blocks WHERE (blocker_id = $1 AND blocked_id = $2) OR (blocker_id = $2 AND blocked_id = $1);

-- name: CreateReport :one
INSERT INTO reports (reporter_id, reported_id, reason, details)
VALUES (
    $1,
    $2,
    $3,
    $4
)
RETURNING *;

