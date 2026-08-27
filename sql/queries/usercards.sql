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
    city_visible = false,
    country_visible = false,
    photos_visible = false,
    bio_visible = false,
    hobbies_visible = false,
    languages_visible = false,
    updated_at = Now()
WHERE card_id = $1;

-- name: UpdateNickname :one
UPDATE cards SET nickname = $1, updated_at = Now() WHERE card_id = $2
RETURNING *;

-- name: UpdateNotesOnSubject :one
UPDATE cards SET notes_on_subject = $1, updated_at = Now() WHERE card_id = $2
RETURNING *;

-- name: RevealFields :exec
UPDATE cards SET display_name_visible = true, date_of_birth_visible = true, city_visible = true,
    country_visible = true,
    photos_visible = true,
    bio_visible = true,
    hobbies_visible = true,
    languages_visible = true
WHERE card_id = $1;

-- name: RevealCityField :exec
UPDATE cards SET city_visible = true WHERE card_id = $1;

-- name: RevealCountryField :exec
UPDATE cards SET country_visible = true WHERE card_id = $1;

-- name: RevealPhotosField :exec
UPDATE cards SET photos_visible = true WHERE card_id = $1;

-- name: RevealBioField :exec
UPDATE cards SET bio_visible = true WHERE card_id = $1;

-- name: RevealHobbiesField :exec
UPDATE cards SET hobbies_visible = true WHERE card_id = $1;

-- name: RevealLanguagesField :exec
UPDATE cards SET languages_visible = true WHERE card_id = $1;

-- name: RevealNameField :exec
UPDATE cards SET display_name_visible = true WHERE card_id = $1;

-- name: RevealBirthField :exec
UPDATE cards SET date_of_birth_visible = true WHERE card_id = $1;

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
    c.city_visible,
    c.country_visible,
    c.photos_visible,
    c.bio_visible,
    c.hobbies_visible,
    c.languages_visible,
    u.display_name,
    u.date_of_birth,
    u.city,
    u.country,
    u.bio,
    u.hobbies,
    u.languages
FROM cards c
JOIN users u ON u.user_id = c.subject_id
WHERE c.card_id = $1;

-- name: GetCardWithChatCreatorAndSubject :one
SELECT * FROM cards WHERE chat_id = $1 AND creator_id = $2 AND subject_id = $3;