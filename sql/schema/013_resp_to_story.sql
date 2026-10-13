-- +goose Up
ALTER TABLE messages ADD COLUMN story_id UUID NULL REFERENCES stories(story_id) ON DELETE SET NULL;
ALTER TABLE messages ADD COLUMN is_story_reply BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE messages DROP COLUMN is_story_reply;
ALTER TABLE messages DROP COLUMN story_id;