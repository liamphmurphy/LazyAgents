-- +goose Up
-- The Docs app keeps the chats behind the documents it starts in an app space
-- of its own, like Finance's Ask chats.
ALTER TABLE chat_spaces DROP CONSTRAINT chat_spaces_app_check;
ALTER TABLE chat_spaces ADD CONSTRAINT chat_spaces_app_check CHECK (app IN ('finance', 'docs'));

-- +goose Down
DELETE FROM chat_spaces WHERE app = 'docs';
ALTER TABLE chat_spaces DROP CONSTRAINT chat_spaces_app_check;
ALTER TABLE chat_spaces ADD CONSTRAINT chat_spaces_app_check CHECK (app IN ('finance'));
