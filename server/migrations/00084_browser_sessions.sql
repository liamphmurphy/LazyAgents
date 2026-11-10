-- +goose Up
-- Browser sign-in for the person-facing API. Agent containers can reach the
-- API, so /api can't trust whoever connects: a browser proves it's the person
-- with a session cookie, which it gets by opening a one-time sign-in link
-- printed by `vesper sign-in-link` (a channel no container can read). Only
-- SHA-256 hashes are stored; the plaintext codes and tokens never are.
CREATE TABLE browser_sign_in_codes (
  code_hash  bytea PRIMARY KEY,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE browser_sessions (
  token_hash   bytea PRIMARY KEY,
  expires_at   timestamptz NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE browser_sessions;
DROP TABLE browser_sign_in_codes;
