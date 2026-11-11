-- +goose Up
-- When the person finished the first-run Welcome Wizard; NULL shows it.
ALTER TABLE users ADD COLUMN onboarded_at timestamptz;
-- Everyone already here is past first run (migrations run before a fresh
-- install's first user is created), so only new people see the wizard.
UPDATE users SET onboarded_at = now();

-- +goose Down
ALTER TABLE users DROP COLUMN onboarded_at;
