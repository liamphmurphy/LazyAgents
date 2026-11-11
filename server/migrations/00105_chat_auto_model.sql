-- +goose Up
-- An automatic chat routes its model per turn (chatmodels.AutoModel); model
-- then holds the one the next turn runs on. Existing chats keep the model they
-- picked.
ALTER TABLE chat_sessions ADD COLUMN auto_model boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE chat_sessions DROP COLUMN auto_model;
