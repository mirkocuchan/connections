-- +goose Up
CREATE TABLE stories(
    story_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    CONSTRAINT fk_user_id
    FOREIGN KEY (user_id)
    REFERENCES users(user_id)
    ON DELETE CASCADE,
    media_url TEXT NOT NULL,
    media_type TEXT NOT NULL,  -- 'image' o 'video'
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    expires_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_stories_user_id ON stories(user_id);
CREATE INDEX idx_stories_expires_at ON stories(expires_at);

CREATE TABLE story_views(
    story_id UUID NOT NULL,
    CONSTRAINT fk_story_id
    FOREIGN KEY (story_id)
    REFERENCES stories(story_id)
    ON DELETE CASCADE,
    viewer_id UUID NOT NULL,
    CONSTRAINT fk_viewer_id
    FOREIGN KEY (viewer_id)
    REFERENCES users(user_id)
    ON DELETE CASCADE,
    viewed_at TIMESTAMP NOT NULL DEFAULT now(),
    UNIQUE (story_id, viewer_id)  -- evitamos duplicados
);

-- +goose Down
DROP TABLE story_views;
DROP INDEX idx_stories_expires_at;
DROP INDEX idx_stories_user_id;
DROP TABLE stories;