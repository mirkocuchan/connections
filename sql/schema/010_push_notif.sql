-- +goose Up
CREATE TABLE push_tokens (
    token      TEXT PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX push_tokens_user_id_idx ON push_tokens(user_id);

-- +goose Down
DROP TABLE push_tokens;