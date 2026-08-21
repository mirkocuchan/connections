-- name: CreateUserCard :one
INSERT INTO cards (chat_id, creator_id, subject_id)
VALUES (
    $1,
    $2,
    $3
)
RETURNING *;

-- name: GetUserCardByChatAndUsers :one
SELECT * FROM cards WHERE card_id = $1 AND creator_id = $2 AND subject_id = $3;

-- name: GetUserCardsByCreator :many
SELECT * FROM cards WHERE creator_id = $1;

-- name: DeleteCard :exec
DELETE FROM cards WHERE card_id = $1;

-- name: ResetCard :exec
UPDATE cards SET nickname = NULL,
    notes_on_subject = NULL,
    display_name_visible = false,
    date_of_birth_visible = false,
    ciudad_visible = false,
    pais_visible = false,
    foto_visible = false,
    bio_visible = false,
    intereses_visible = false,
    idiomas_visible = false,
    updated_at = Now()
WHERE card_id = $1;

-- name: UpdateNickname :exec
UPDATE cards SET nickname = $1, updated_at = Now()  WHERE card_id = $2;

-- name: RevealFields :exec
UPDATE cards SET display_name_visible = true, date_of_birth_visible = true, ciudad_visible = true,
    pais_visible = true,
    foto_visible = true,
    bio_visible = true,
    intereses_visible = true,
    idiomas_visible = true
WHERE card_id = $1;

-- name: GetUserPhotos :many
SELECT * FROM user_photos WHERE user_id = $1 ORDER BY position;

-- name: GetCardWithSubjectData :one
SELECT 
    c.card_id,
    c.chat_id,
    c.creator_id,
    c.subject_id,
    c.nickname,
    c.notes_on_subject,
    c.display_name_visible,
    c.date_of_birth_visible,
    c.ciudad_visible,
    c.pais_visible,
    c.foto_visible,
    c.bio_visible,
    c.intereses_visible,
    c.idiomas_visible,
    u.display_name,
    u.date_of_birth,
    u.ciudad,
    u.pais,
    u.bio,
    u.intereses,
    u.idiomas
FROM cards c
JOIN users u ON u.user_id = c.subject_id
WHERE c.card_id = $1;