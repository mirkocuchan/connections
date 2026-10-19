-- name: MarkChatRead :exec
INSERT INTO chat_reads (chat_id, user_id, last_read_at)
VALUES (
    $1,
    $2,
    now()
) ON CONFLICT (chat_id, user_id) DO UPDATE
SET last_read_at = EXCLUDED.last_read_at;

-- name: GetUnreadCountsByUserID :many
SELECT c.chat_id, COUNT(m.message_id) AS unread_count
FROM chats c
LEFT JOIN chat_reads cr
       ON cr.chat_id = c.chat_id AND cr.user_id = $1
LEFT JOIN messages m
       ON m.chat_id = c.chat_id
      AND m.sender_id <> $1
      AND m.created_at > COALESCE(cr.last_read_at, 'epoch'::timestamp)
WHERE c.user_one_id = $1 OR c.user_two_id = $1
GROUP BY c.chat_id;