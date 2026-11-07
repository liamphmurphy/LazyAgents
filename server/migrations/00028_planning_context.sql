-- +goose Up
-- What a person attached when starting a plan ([{kind, id|key, anchor?}], the
-- chat context_refs shape), delivered to the planner as context files, and how
-- many proposals the planner submitted, so the plans index can show
-- accepted / proposed after reviewers drop some.
ALTER TABLE planning_sessions ADD COLUMN context_refs jsonb NOT NULL DEFAULT '[]';
ALTER TABLE planning_sessions ADD COLUMN proposal_count integer NOT NULL DEFAULT 0;
-- Best effort: older drafts only kept the reviewer's last saved list.
UPDATE planning_sessions SET proposal_count = jsonb_array_length(proposals) WHERE state <> 'planning';

-- +goose Down
ALTER TABLE planning_sessions DROP COLUMN proposal_count;
ALTER TABLE planning_sessions DROP COLUMN context_refs;
