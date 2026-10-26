-- +goose Up
ALTER TABLE users ADD COLUMN banned_at TIMESTAMP;
ALTER TABLE users ADD COLUMN ban_reason TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN ban_reason;
ALTER TABLE users DROP COLUMN banned_at;