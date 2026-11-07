-- +goose Up
-- Personal assistant memory: one row per remembered thing, owned by exactly one
-- person and kept apart from the shared wiki. Every query filters on
-- (workspace_id, user_id); nothing reads across people.
--
-- superseded_by retires an entry in favour of a newer one: retired entries stay
-- for history but drop out of search and the index. Forgetting an entry deletes
-- it together with everything it superseded.

-- memory_search_vector is the ranked full-text document: title and tags weigh
-- most, then the summary, then the body. Declared IMMUTABLE (array_to_string
-- is only STABLE) so it can back an expression index.
-- +goose StatementBegin
CREATE FUNCTION memory_search_vector(title text, summary text, body text, tags text[]) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT setweight(to_tsvector('english'::regconfig, title), 'A')
      || setweight(to_tsvector('english'::regconfig, array_to_string(tags, ' ')), 'A')
      || setweight(to_tsvector('english'::regconfig, summary), 'B')
      || setweight(to_tsvector('english'::regconfig, body), 'C')
$$;
-- +goose StatementEnd

CREATE TABLE memories (
  id             uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id   uuid NOT NULL REFERENCES workspaces(id),
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title          text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
  summary        text NOT NULL DEFAULT '' CHECK (length(summary) <= 300),
  body           text NOT NULL DEFAULT '' CHECK (length(body) <= 20000),
  tags           text[] NOT NULL DEFAULT '{}',
  type           text NOT NULL DEFAULT 'other' CHECK (type IN ('preference', 'project', 'person', 'life', 'work', 'fact', 'other')),
  -- source says where the memory came from in words ("chat <id>", "manual");
  -- the ids link it when that chat or run still exists.
  source         text NOT NULL DEFAULT '',
  source_chat_id uuid REFERENCES chat_sessions(id) ON DELETE SET NULL,
  source_run_id  uuid REFERENCES runs(id) ON DELETE SET NULL,
  superseded_by  uuid REFERENCES memories(id) ON DELETE CASCADE,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  CHECK (superseded_by IS DISTINCT FROM id)
);
-- Serves the index and listings: one person's entries, newest change first.
CREATE INDEX memories_owner ON memories (workspace_id, user_id, updated_at DESC, id DESC);
CREATE INDEX memories_search ON memories USING gin (memory_search_vector(title, summary, body, tags));
-- Near-duplicate checks compare titles by trigram similarity.
CREATE INDEX memories_title_trgm ON memories USING gin (lower(title) gin_trgm_ops);

-- +goose Down
DROP TABLE memories;
DROP FUNCTION memory_search_vector(text, text, text, text[]);
