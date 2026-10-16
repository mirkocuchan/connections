-- +goose Up
ALTER TABLE users
  ADD COLUMN terms_accepted_at TIMESTAMP,
  ADD COLUMN terms_version TEXT;

-- +goose Down
ALTER TABLE users
  DROP COLUMN terms_accepted_at,
  DROP COLUMN terms_version;