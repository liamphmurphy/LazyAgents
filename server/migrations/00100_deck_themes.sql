-- +goose Up
-- Slides themes a person saved from the theme editor, offered in every one of
-- their decks. A deck embeds its own copy of its theme, so changing or
-- deleting a saved theme never changes a deck.
CREATE TABLE deck_themes (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL,
  user_id      uuid NOT NULL,
  name         text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
  fonts        jsonb NOT NULL,
  colors       jsonb NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX deck_themes_owner_name ON deck_themes (workspace_id, user_id, lower(name));

-- +goose Down
DROP TABLE deck_themes;
