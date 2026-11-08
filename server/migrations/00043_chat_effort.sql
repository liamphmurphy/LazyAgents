-- +goose Up
-- The reasoning effort a chat's turns run at; empty is the harness's own default.
ALTER TABLE chat_sessions ADD COLUMN effort text NOT NULL DEFAULT ''
  CHECK (effort IN ('', 'low', 'medium', 'high', 'xhigh', 'max'));

-- +goose Down
ALTER TABLE chat_sessions DROP COLUMN effort;
