-- +goose Up
-- Login is removed for now (PLAN.md §5.4); sessions come back with it.
DROP TABLE sessions;

-- +goose Down
CREATE TABLE sessions (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash   bytea NOT NULL UNIQUE,
  expires_at   timestamptz NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
