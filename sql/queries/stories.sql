-- name: CreateStory :one
INSERT INTO stories (user_id, media_url, media_type)
VALUES (
    $1,
    $2,
    $3
)
RETURNING *;

-- name: GetActiveStories :many
SELECT * FROM stories WHERE expires_at > NOW() ORDER BY created_at DESC;

-- name: GetActiveStoriesByUserID :many
SELECT * FROM stories WHERE user_id = $1 AND expires_at > NOW() ORDER BY created_at DESC;

-- name: GetStoryByID :one
SELECT * FROM stories WHERE story_id = $1;

-- name: DeleteStoryByID :exec
DELETE FROM stories WHERE story_id = $1 AND user_id = $2;

-- name: RegisterStoryView :exec
INSERT INTO story_views (story_id, viewer_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: GetStoryViewsByUserID :many
SELECT stories.*, story_views.viewed_at FROM stories LEFT JOIN story_views ON stories.story_id = story_views.story_id AND story_views.viewer_id = $1 
WHERE stories.expires_at > NOW() ORDER BY stories.created_at DESC;
-- active stories, did i see it or did i not.