-- +goose Up
-- How long the write-up should be and how it should read, picked on the plan.
ALTER TABLE research_jobs
  ADD COLUMN report_length text NOT NULL DEFAULT 'standard' CHECK (report_length IN ('brief', 'standard', 'detailed', 'in_depth')),
  ADD COLUMN report_tone   text NOT NULL DEFAULT 'report'   CHECK (report_tone IN ('report', 'explainer', 'essay', 'academic', 'news'));

-- +goose Down
ALTER TABLE research_jobs DROP COLUMN report_tone, DROP COLUMN report_length;
