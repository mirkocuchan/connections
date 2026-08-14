-- +goose Up
CREATE TABLE messages(
    message_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id UUID NOT NULL,
    CONSTRAINT fk_chat_id
    FOREIGN KEY (chat_id)
    REFERENCES chats(chat_id)
    ON DELETE CASCADE,
    sender_id UUID NOT NULL,
    CONSTRAINT fk_sender_id
    FOREIGN KEY (sender_id)
    REFERENCES users(user_id)
    ON DELETE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);
CREATE INDEX idx_messages_chat_id ON messages(chat_id);

-- +goose Down
DROP INDEX idx_messages_chat_id;
DROP TABLE messages;
