-- +goose Up
CREATE UNIQUE INDEX user_photos_user_position_idx ON user_photos (user_id, position);

-- +goose Down
DROP INDEX user_photos_user_position_idx;