-- +goose Up
-- The conversation starters a person's empty chats offer, one row per person.
-- The runner writes them in the background so opening a chat never waits on a
-- model. attempted_at limits how often that runs and, by changing, discards a
-- generation still in flight when the person forgets something.
CREATE TABLE chat_suggestions (
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  items        jsonb NOT NULL DEFAULT '[]',
  generated_at timestamptz,
  attempted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, user_id)
);

-- +goose Down
DROP TABLE chat_suggestions;
