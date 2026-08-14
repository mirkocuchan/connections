-- +goose Up
CREATE TABLE chats(
    chat_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_one_id UUID NOT NULL,
    CONSTRAINT fk_user_one_id
    FOREIGN KEY (user_one_id)
    REFERENCES users(user_id)
    ON DELETE CASCADE,
    user_two_id UUID NOT NULL,
    CONSTRAINT fk_user_two_id
    FOREIGN KEY (user_two_id)
    REFERENCES users(user_id)
    ON DELETE CASCADE,
    CONSTRAINT chk_different_users
    CHECK (user_one_id <> user_two_id),
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);
CREATE INDEX idx_chats_user_one_id ON chats(user_one_id);
CREATE INDEX idx_chats_user_two_id ON chats(user_two_id);
-- why does this work? because the combination of LEAST and GREATEST ensures that the order of user IDs does not matter, effectively treating (user_one_id, user_two_id) and (user_two_id, user_one_id) as the same pair. 
-- this prevents duplicate chat entries between the same two users, regardless of their order. 
-- why is it an index? because it allows the database to quickly check for existing chats between two users, improving performance when inserting new chats or querying existing ones.
-- el índice está construido a partir de las columnas de chats, la restricción que impone el índice afecta qué filas pueden existir en chats.
CREATE UNIQUE INDEX idx_unique_chat_users
ON chats (LEAST(user_one_id, user_two_id), GREATEST(user_one_id, user_two_id));

-- +goose Down
DROP INDEX idx_unique_chat_users;
DROP INDEX idx_chats_user_one_id;
DROP INDEX idx_chats_user_two_id;
DROP TABLE chats;
