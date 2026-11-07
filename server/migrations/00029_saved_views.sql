-- +goose Up
-- Named ticket-list views, one set per person. query is the list's URL search
-- string without project ("view=active&group=epic"), so a view applies to
-- whichever project is current. Names are unique per person, ignoring case.
CREATE TABLE saved_views (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
  query        text NOT NULL DEFAULT '' CHECK (length(query) <= 2000),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX saved_views_user_name ON saved_views (user_id, lower(name));

-- +goose Down
DROP TABLE saved_views;
