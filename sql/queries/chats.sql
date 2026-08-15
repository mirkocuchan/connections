-- name: CreateChat :one
INSERT INTO chats (user_one_id, user_two_id)
VALUES (
    $1,
    $2
)
RETURNING *;

-- name: GetChatByID :one
SELECT * FROM chats WHERE chat_id = $1;

-- name: GetChatByUserIDs :one
SELECT * FROM chats WHERE (user_one_id = $1 AND user_two_id = $2) OR (user_one_id = $2 AND user_two_id = $1);

-- name: GetChatsByUserID :many
SELECT * FROM chats WHERE user_one_id = $1 OR user_two_id = $1;

-- name: DeleteChatByID :exec
DELETE FROM chats WHERE chat_id = $1;