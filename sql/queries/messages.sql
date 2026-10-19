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

-- name: GetMessagesPage :many
SELECT m.*, s.media_url AS story_media_url, s.media_type AS story_media_type
FROM messages m
LEFT JOIN stories s --por si hay mensajes a historias
       ON s.story_id = m.story_id
      AND s.expires_at > (now() at time zone 'utc')
WHERE m.chat_id = sqlc.arg('chat_id')
  AND (
        sqlc.narg('before_created_at')::timestamp IS NULL --los 30 más nuevos, sin cursos scrolleando para arriba
        OR (m.created_at, m.message_id) <
           (sqlc.narg('before_created_at')::timestamp, sqlc.narg('before_id')::uuid) --compara de a pares, como se ordenan las palabras en un diccionario: primero por fecha, y si la fecha es la misma, desempata el ID
      )
ORDER BY m.created_at DESC, m.message_id DESC
LIMIT sqlc.arg('page_size');