-- +goose Up
CREATE TABLE chat_reads (
    chat_id      UUID NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    last_read_at TIMESTAMP NOT NULL,
    PRIMARY KEY (chat_id, user_id)
);

-- sirve para contar no leídos y, en la etapa 2, para paginar mensajes
CREATE INDEX idx_messages_chat_created
    ON messages (chat_id, created_at DESC, message_id DESC);

-- +goose Down
DROP INDEX idx_messages_chat_created;
DROP TABLE chat_reads;