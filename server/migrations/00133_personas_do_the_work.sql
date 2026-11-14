-- +goose Up
-- One actor: the persona. A persona that takes tickets or helps on runs has a work record,
-- the agents row with the persona's own id: its handle, kind (main = takes tickets, sub =
-- helps on runs), versioned definition (harness, model, work brief, scopes, limits, helpers),
-- tokens and project overrides. Runs, tickets, approvals and every other table keyed on
-- agents keep pointing at that row, so none of them is rewritten. A chat-only persona has
-- no agents row. The Personal Assistant agent (kind assistant) is the brief every chat runs
-- with, not a persona, and stays as it is.

-- Whether the persona is offered in the chat switcher and the @ menu. Opening a chat with it
-- from its profile works either way.
ALTER TABLE personas ADD COLUMN in_chat_list boolean NOT NULL DEFAULT true;

-- +goose StatementBegin
-- Every live main and sub agent becomes a persona with the agent's id, owned by the
-- workspace's first person (the one person on an own-machine install). Its colour is the
-- agent's palette hue (light theme, web/styles/tokens.css --agent-*); its voice starts empty,
-- since the agent's instructions are its work brief; chat permissions start off. A name
-- another of that person's personas already uses gets " 2", " 3"… Archived agents stay
-- archived work records: their runs still name them, and nobody can assign them anyway.
DO $$
DECLARE
  a record;
  owner uuid;
  base text;
  candidate text;
  n int;
BEGIN
  FOR a IN
    SELECT ag.* FROM agents ag
     WHERE ag.kind IN ('main', 'sub') AND ag.archived_at IS NULL
       AND NOT EXISTS (SELECT 1 FROM personas p WHERE p.id = ag.id)
     ORDER BY ag.workspace_id, ag.created_at, ag.id
  LOOP
    SELECT u.id INTO owner FROM users u WHERE u.workspace_id = a.workspace_id ORDER BY u.created_at, u.id LIMIT 1;
    CONTINUE WHEN owner IS NULL;
    base := left(btrim(a.name), 56);
    IF base = '' THEN base := a.handle; END IF;
    candidate := base;
    n := 1;
    WHILE EXISTS (SELECT 1 FROM personas p WHERE p.workspace_id = a.workspace_id AND p.user_id = owner
                    AND lower(btrim(p.name)) = lower(candidate)) LOOP
      n := n + 1;
      candidate := base || ' ' || n;
    END LOOP;
    INSERT INTO personas (id, workspace_id, user_id, name, color, created_at, updated_at)
    VALUES (a.id, a.workspace_id, owner, candidate,
            (ARRAY['#4547c9', '#0b7a6e', '#b42f48', '#1d5fb0', '#8a6100', '#277538', '#a44a18', '#6c3bb8'])[(((a.color_index % 8) + 8) % 8) + 1],
            a.created_at, now());
  END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- The personas stay: once made, a migrated persona can't be told from one the person later
-- set to work, and both are ordinary chat personas to the older code. Their agents rows were
-- never touched.
ALTER TABLE personas DROP COLUMN in_chat_list;
