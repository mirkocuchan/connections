-- +goose Up
CREATE TABLE password_resets (
    user_id    UUID PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
    code_hash  TEXT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    attempts   INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT (now() at time zone 'utc')
);

-- +goose Down
DROP TABLE password_resets;