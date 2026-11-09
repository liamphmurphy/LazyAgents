-- +goose Up
-- A coding chat no longer gets one branch when it is created: its persona
-- makes a branch (feat/…, fix/…, chore/…) when a change is asked for, and may
-- make several. work_branches lists the ones it pushed, oldest first.
ALTER TABLE chat_sessions ADD COLUMN work_branches text[] NOT NULL DEFAULT '{}';
UPDATE chat_sessions SET work_branches = ARRAY[work_branch] WHERE work_branch IS NOT NULL;
ALTER TABLE chat_sessions
  DROP CONSTRAINT chat_sessions_repo_branch,
  DROP COLUMN work_branch,
  ADD CONSTRAINT chat_sessions_repo_branches CHECK (repo_url IS NOT NULL OR cardinality(work_branches) = 0);

-- +goose Down
ALTER TABLE chat_sessions ADD COLUMN work_branch text;
UPDATE chat_sessions SET work_branch = COALESCE(work_branches[1], 'vesper/chat-' || right(replace(id::text, '-', ''), 8)) WHERE repo_url IS NOT NULL;
ALTER TABLE chat_sessions
  DROP CONSTRAINT chat_sessions_repo_branches,
  DROP COLUMN work_branches,
  ADD CONSTRAINT chat_sessions_repo_branch CHECK ((repo_url IS NULL) = (work_branch IS NULL));
