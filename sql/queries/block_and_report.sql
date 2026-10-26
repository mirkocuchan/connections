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

-- name: ExistsBlockByBlocker :one
SELECT 1 FROM blocks WHERE blocker_id = $1 AND blocked_id = $2 LIMIT 1;

-- name: CreateReportWithEvidence :one
WITH new_report AS (INSERT INTO reports (reporter_id, reported_id, chat_id, reported_username, reason, details)
    VALUES ( 
        sqlc.arg('reporter_id')::uuid,
        sqlc.arg('reported_id')::uuid,
        sqlc.narg('chat_id')::uuid,
        sqlc.arg('reported_username')::text,
        sqlc.arg('reason')::text,
        sqlc.narg('details')::text
    )
    RETURNING *
), copied_messages AS (INSERT INTO report_messages (report_id, author_role, content, sent_at)
    SELECT
        (SELECT report_id FROM new_report),
        CASE WHEN m.sender_id = sqlc.arg('reporter_id')::uuid THEN 'reporter' ELSE 'reported' END,
        m.content,
        m.created_at
    FROM (
        SELECT sender_id, content, created_at
        FROM messages
        WHERE chat_id = sqlc.narg('chat_id')::uuid
        ORDER BY created_at DESC
        LIMIT 20
    ) m
)
SELECT * FROM new_report;

-- name: CountRecentReportsByReporter :one
SELECT COUNT(*) FROM reports WHERE reporter_id = sqlc.arg('reporter_id')::uuid AND created_at > now() - interval '1 hour';

-- name: ExistsRecentReport :one
SELECT EXISTS (SELECT 1 FROM reports WHERE reporter_id = sqlc.arg('reporter_id')::uuid
      AND reported_id = sqlc.arg('reported_id')::uuid
      AND reason = sqlc.arg('reason')::text
      AND created_at > now() - interval '24 hours'
);

-- name: MarkReportsActioned :exec
UPDATE reports SET status = 'actioned', reviewed_at = now()
WHERE reported_id = $1 AND status = 'pending';