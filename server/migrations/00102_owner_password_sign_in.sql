-- +goose Up
-- First-run sign-in: the first browser to open a fresh install picks a
-- username and password, which claims the instance. claimed_at records that
-- once, so the open setup window can't come back when sessions expire.
-- password_set_at marks a password the person chose; the seeded
-- password_hash is a random secret nobody knows until then.
ALTER TABLE users
  ADD COLUMN login text UNIQUE,
  ADD COLUMN password_set_at timestamptz,
  ADD COLUMN claimed_at timestamptz;

-- An install that already signed a browser in has an owner: keep it closed to
-- first-run setup. That browser can still set a password while signed in.
UPDATE users SET claimed_at = now() WHERE EXISTS (SELECT 1 FROM browser_sessions);

-- +goose Down
ALTER TABLE users
  DROP COLUMN claimed_at,
  DROP COLUMN password_set_at,
  DROP COLUMN login;
