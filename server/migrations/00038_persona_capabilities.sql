-- +goose Up
-- Persona capabilities (wiki: Architecture/ADR - Assistant personas). Only
-- these structured columns widen what a persona's chats may do; the code
-- maps them to scopes, sandbox and harness options (runs.ChatSpec), never
-- the free text.
ALTER TABLE personas
  ADD COLUMN can_code boolean NOT NULL DEFAULT false,
  ADD COLUMN can_generate_images boolean NOT NULL DEFAULT false,
  ADD COLUMN repos text[] NOT NULL DEFAULT '{}' CHECK (cardinality(repos) <= 10);

-- A coding chat works in one repository on its own branch, both fixed when
-- the chat is created.
ALTER TABLE chat_sessions
  ADD COLUMN repo_url text,
  ADD COLUMN work_branch text,
  ADD CONSTRAINT chat_sessions_repo_branch CHECK ((repo_url IS NULL) = (work_branch IS NULL));

-- Images the assistant generates are attachments on its own message.
ALTER TABLE chat_attachments
  ADD COLUMN source text NOT NULL DEFAULT 'upload' CHECK (source IN ('upload', 'generated'));

-- +goose Down
ALTER TABLE chat_attachments DROP COLUMN source;
ALTER TABLE chat_sessions DROP CONSTRAINT chat_sessions_repo_branch, DROP COLUMN work_branch, DROP COLUMN repo_url;
ALTER TABLE personas DROP COLUMN repos, DROP COLUMN can_generate_images, DROP COLUMN can_code;
