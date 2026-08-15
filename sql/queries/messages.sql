-- name: CreateMessage :one
INSERT INTO messages (chat_id, sender_id, content)
VALUES (
    $1,
    $2,
    $3
)
RETURNING *;

-- name: GetMessageByID :one
SELECT * FROM messages WHERE message_id = $1;

-- name: GetMessagesByChatID :many
SELECT * FROM messages WHERE chat_id = $1 ORDER BY created_at ASC;

-- name: DeleteMessageByID :exec
DELETE FROM messages WHERE message_id = $1;

-- name: GetMessagesBySenderID :many
SELECT * FROM messages WHERE sender_id = $1 ORDER BY created_at ASC;