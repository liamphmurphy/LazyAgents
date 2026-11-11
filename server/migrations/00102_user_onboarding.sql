-- +goose Up
-- When the person finished the first-run Welcome Wizard; NULL shows it.
ALTER TABLE users ADD COLUMN onboarded_at timestamptz;
-- People who have already chatted are past first run: don't send them back.
UPDATE users u SET onboarded_at = now()
WHERE EXISTS (SELECT 1 FROM chat_sessions c WHERE c.created_by = u.id);

-- +goose Down
ALTER TABLE users DROP COLUMN onboarded_at;
