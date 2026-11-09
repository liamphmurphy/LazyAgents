-- +goose Up
-- Secrets a person stores for their personas' API calls (a JWT, an API key).
-- Runs never receive a value: they call Vesper's http_request tool, which adds
-- the secret where the person said, only for hosts the person allowed, and
-- masks it in the response. Workspace secrets (the secrets table) stay for
-- runners and the broker; these are private to one person, like connectors.
CREATE TABLE user_secrets (
  id            uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id  uuid NOT NULL REFERENCES workspaces(id),
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name          text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 60),
  -- The only name a persona sees.
  handle        text NOT NULL CHECK (handle ~ '^[a-z][a-z0-9-]*$' AND length(handle) <= 40),
  -- Where the value goes: Authorization: Bearer, a named header, a named
  -- query parameter, or Basic auth (the value is user:password).
  placement     text NOT NULL CHECK (placement IN ('bearer', 'header', 'query', 'basic')),
  placement_key text NOT NULL DEFAULT '',
  allowed_hosts text[] NOT NULL CHECK (cardinality(allowed_hosts) BETWEEN 1 AND 10),
  -- GET and HEAD are reads; every other method is a write.
  read_policy   text NOT NULL DEFAULT 'allow' CHECK (read_policy IN ('allow', 'ask', 'off')),
  write_policy  text NOT NULL DEFAULT 'ask' CHECK (write_policy IN ('allow', 'ask', 'off')),
  nonce         bytea NOT NULL,
  ciphertext    bytea NOT NULL,
  -- From a JWT's exp claim when the value has one; requests fail after it.
  expires_at    timestamptz,
  last_used_at  timestamptz,
  revision      integer NOT NULL DEFAULT 1,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, user_id, handle)
);

CREATE TABLE user_secret_personas (
  secret_id  uuid NOT NULL REFERENCES user_secrets(id) ON DELETE CASCADE,
  persona_id uuid NOT NULL REFERENCES personas(id) ON DELETE CASCADE,
  PRIMARY KEY (secret_id, persona_id)
);
CREATE INDEX user_secret_personas_persona ON user_secret_personas (persona_id);

-- NULL: every secret the persona may use. "Turn off for this chat" narrows it.
ALTER TABLE chat_sessions ADD COLUMN secret_ids uuid[];

-- A request waiting for the person, shown as they'll see it: the secret is a
-- placeholder here and is added only when the request is sent.
CREATE TABLE user_secret_approvals (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  secret_id    uuid NOT NULL REFERENCES user_secrets(id) ON DELETE CASCADE,
  persona_id   uuid REFERENCES personas(id) ON DELETE SET NULL,
  chat_id      uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  method       text NOT NULL,
  url          text NOT NULL,
  headers      jsonb NOT NULL DEFAULT '{}',
  body_preview text NOT NULL DEFAULT '',
  state        text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'allowed', 'denied', 'expired')),
  created_at   timestamptz NOT NULL DEFAULT now(),
  decided_at   timestamptz
);
CREATE INDEX user_secret_approvals_chat ON user_secret_approvals (chat_id, created_at) WHERE state = 'pending';

CREATE TABLE user_secret_proposals (
  id          uuid PRIMARY KEY DEFAULT uuid_v7(),
  chat_id     uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  run_id      uuid NOT NULL REFERENCES runs(id),
  proposed_by uuid REFERENCES personas(id) ON DELETE SET NULL,
  name        text NOT NULL,
  host        text NOT NULL,
  placement   text NOT NULL CHECK (placement IN ('bearer', 'header', 'query', 'basic')),
  placement_key text NOT NULL DEFAULT '',
  reason      text NOT NULL DEFAULT '',
  state       text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'approved', 'declined')),
  secret_id   uuid REFERENCES user_secrets(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  decided_at  timestamptz
);
CREATE INDEX user_secret_proposals_chat ON user_secret_proposals (chat_id, created_at);

-- Every request a run made with a secret, sent or not. Never the value, and
-- no query string: only the method, host and path.
CREATE TABLE user_secret_calls (
  id          uuid PRIMARY KEY DEFAULT uuid_v7(),
  secret_id   uuid NOT NULL REFERENCES user_secrets(id) ON DELETE CASCADE,
  chat_id     uuid REFERENCES chat_sessions(id) ON DELETE SET NULL,
  persona_id  uuid REFERENCES personas(id) ON DELETE SET NULL,
  method      text NOT NULL,
  host        text NOT NULL,
  path        text NOT NULL,
  -- auto: the policy allowed it. approved/denied: the person decided.
  -- blocked: refused before anything was sent (host, policy, expiry, address).
  decision    text NOT NULL CHECK (decision IN ('auto', 'approved', 'denied', 'blocked')),
  status      integer,
  error       text NOT NULL DEFAULT '',
  redactions  integer NOT NULL DEFAULT 0,
  duration_ms integer NOT NULL DEFAULT 0,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX user_secret_calls_secret ON user_secret_calls (secret_id, created_at DESC);
CREATE INDEX user_secret_calls_chat ON user_secret_calls (chat_id, created_at DESC);

-- +goose Down
DROP TABLE user_secret_calls;
DROP TABLE user_secret_proposals;
DROP TABLE user_secret_approvals;
ALTER TABLE chat_sessions DROP COLUMN secret_ids;
DROP TABLE user_secret_personas;
DROP TABLE user_secrets;
