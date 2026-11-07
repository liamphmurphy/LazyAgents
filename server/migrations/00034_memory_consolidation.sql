-- +goose Up
-- The consolidation pass retires near-duplicate memories in favour of the
-- newest one. retired_reason and retired_at say why and when an entry was
-- retired, so the person can see what was merged; restoring clears both.
ALTER TABLE memories
  ADD COLUMN retired_reason text NOT NULL DEFAULT '' CHECK (length(retired_reason) <= 300),
  ADD COLUMN retired_at timestamptz;
UPDATE memories SET retired_at = updated_at WHERE superseded_by IS NOT NULL;

-- +goose Down
ALTER TABLE memories DROP COLUMN retired_reason, DROP COLUMN retired_at;
