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
SELECT m.*, s.media_url AS story_media_url, s.media_type AS story_media_type
FROM messages m
LEFT JOIN stories s
       ON s.story_id = m.story_id
      AND s.expires_at > (now() at time zone 'utc')
WHERE m.chat_id = $1
ORDER BY m.created_at ASC;

-- name: DeleteMessageByID :exec
DELETE FROM messages WHERE message_id = $1;

-- name: GetMessagesBySenderID :many
SELECT * FROM messages WHERE sender_id = $1 ORDER BY created_at ASC;

-- name: GetLastMessageByChatID :one
SELECT * FROM messages WHERE chat_id = $1 ORDER BY created_at DESC LIMIT 1;

-- name: CreateStoryReplyMessage :one
INSERT INTO messages (chat_id, sender_id, content, story_id, is_story_reply)
VALUES (
    $1, 
    $2, 
    $3,
    $4,
    true
)
RETURNING *;