-- +goose Up
ALTER TABLE users ADD COLUMN detected_country TEXT;
ALTER TABLE users ADD COLUMN global_discovery BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE users DROP COLUMN detected_country;
ALTER TABLE users DROP COLUMN global_discovery;