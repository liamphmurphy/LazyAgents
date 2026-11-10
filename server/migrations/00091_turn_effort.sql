-- +goose Up
-- A run's effort lives in its spec, which the next turn overwrites, so each
-- turn keeps its own copy, and a chat turn also keeps why Vesper chose it.
ALTER TABLE run_turns
  -- The reasoning effort the turn ran at; NULL is the harness's own default.
  ADD COLUMN effort text,
  -- The level an automatic chat aimed for; NULL when the person chose the effort
  -- or the turn isn't a chat turn.
  ADD COLUMN effort_level text CHECK (effort_level IN ('low', 'medium', 'high', 'xhigh')),
  -- The classifier rules that raised the level above the message's own.
  ADD COLUMN effort_reasons text[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE run_turns DROP COLUMN effort_reasons, DROP COLUMN effort_level, DROP COLUMN effort;
