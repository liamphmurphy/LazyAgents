-- +goose Up
-- A person's profile picture, stored like persona avatars: a blob key plus the
-- content type sniffed at upload. Both set or both null.
ALTER TABLE users
  ADD COLUMN avatar_key text,
  ADD COLUMN avatar_content_type text,
  ADD CONSTRAINT users_avatar_pair CHECK ((avatar_key IS NULL) = (avatar_content_type IS NULL));

-- +goose Down
ALTER TABLE users
  DROP CONSTRAINT users_avatar_pair,
  DROP COLUMN avatar_content_type,
  DROP COLUMN avatar_key;
