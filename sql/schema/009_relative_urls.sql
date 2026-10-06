-- +goose Up
UPDATE user_photos SET photo_url = regexp_replace(photo_url, '^https?://[^/]+', '')
WHERE photo_url ~ '^https?://';

UPDATE stories SET media_url = regexp_replace(media_url, '^https?://[^/]+', '')
WHERE media_url ~ '^https?://';

-- +goose Down
SELECT 1;