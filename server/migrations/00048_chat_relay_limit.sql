-- +goose Up
-- How many replies personas may hand to each other after the person's last
-- message, when that message asked for it ("talk up to 25 messages"); 0 is
-- the default limit.
ALTER TABLE chat_sessions ADD COLUMN relay_limit integer NOT NULL DEFAULT 0 CHECK (relay_limit >= 0);

-- +goose Down
ALTER TABLE chat_sessions DROP COLUMN relay_limit;
