-- name: CreateBlock :one
INSERT INTO blocks (blocker_id, blocked_id)
VALUES (
    $1,
    $2
)
RETURNING *;

-- name: GetBlocksByBlockerID :many
SELECT * FROM blocks WHERE blocker_id = $1;

-- name: DeleteBlockByBlockerAndBlockedID :exec
DELETE FROM blocks WHERE blocker_id = $1 AND blocked_id = $2;

-- name: ExistsBlockBetweenUsers :one
SELECT 1 FROM blocks WHERE (blocker_id = $1 AND blocked_id = $2) OR (blocker_id = $2 AND blocked_id = $1) LIMIT 1;

-- name: CreateReport :one
INSERT INTO reports (reporter_id, reported_id, reason, details)
VALUES (
    $1,
    $2,
    $3,
    $4
)
RETURNING *;

