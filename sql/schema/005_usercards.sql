-- +goose Up
CREATE TABLE cards(
    card_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id UUID NOT NULL,
    CONSTRAINT fk_chat_id
    FOREIGN KEY (chat_id)
    REFERENCES chats(chat_id)
    ON DELETE CASCADE,
    creator_id UUID NOT NULL,
    CONSTRAINT fk_creator_id
    FOREIGN KEY (creator_id)
    REFERENCES users(user_id)
    ON DELETE CASCADE,
    subject_id UUID NOT NULL,
    CONSTRAINT fk_subject_id
    FOREIGN KEY (subject_id)
    REFERENCES users(user_id)
    ON DELETE CASCADE,
    nickname TEXT,
    notes_on_subject TEXT,
    display_name_visible BOOLEAN DEFAULT false,
    date_of_birth_visible BOOLEAN DEFAULT false,
    city_visible BOOLEAN DEFAULT false,
    country_visible BOOLEAN DEFAULT false,
    photo_visible BOOLEAN DEFAULT false,
    bio_visible BOOLEAN DEFAULT false,
    hobbies_visible BOOLEAN DEFAULT false,
    languages_visible BOOLEAN DEFAULT false,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);
ALTER TABLE users 
    ADD COLUMN display_name TEXT,
    ADD COLUMN bio TEXT,
    ADD COLUMN city TEXT,
    ADD COLUMN country TEXT,
    ADD COLUMN hobbies TEXT,
    ADD COLUMN languages TEXT;

CREATE TABLE user_photos(
    photo_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    CONSTRAINT fk_user_id
    FOREIGN KEY (user_id)
    REFERENCES users(user_id)
    ON DELETE CASCADE,
    photo_url TEXT NOT NULL,
    position INT NOT NULL DEFAULT 0,  -- para ordenar cuál va primero
    created_at TIMESTAMP NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE cards;
DROP TABLE user_photos;
ALTER TABLE users 
    DROP COLUMN display_name,
    DROP COLUMN bio,
    DROP COLUMN city,
    DROP COLUMN country,
    DROP COLUMN hobbies,
    DROP COLUMN languages;

