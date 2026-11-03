-- +goose Up
ALTER TABLE users ADD COLUMN read_receipts BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE users DROP COLUMN read_receipts;