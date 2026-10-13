-- +goose Up
ALTER TABLE users ADD COLUMN push_mode TEXT NOT NULL DEFAULT 'name' CHECK (push_mode IN ('off', 'name', 'full'));

-- +goose Down
ALTER TABLE users DROP COLUMN push_mode;