-- name: CreateWorkflow :one
INSERT INTO workflows (workspace_id, user_id, name, description, draft) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetWorkflow :one
SELECT * FROM workflows WHERE id = $1 AND workspace_id = $2 AND user_id = $3;

-- name: LockWorkflow :one
SELECT * FROM workflows WHERE id = $1 AND workspace_id = $2 AND user_id = $3 FOR UPDATE;

-- name: ListWorkflows :many
SELECT * FROM workflows WHERE workspace_id = $1 AND user_id = $2 ORDER BY updated_at DESC, id LIMIT 200;

-- name: ListPublishedWorkflows :many
SELECT * FROM workflows WHERE workspace_id = $1 AND user_id = $2 AND published IS NOT NULL ORDER BY name, id LIMIT 30;

-- name: SaveWorkflow :one
UPDATE workflows SET name = $2, description = $3, draft = $4, updated_at = now() WHERE id = $1 RETURNING *;

-- name: PublishWorkflow :one
UPDATE workflows SET published = draft, published_version = published_version + 1, updated_at = now() WHERE id = $1 RETURNING *;

-- name: DeleteWorkflow :execrows
DELETE FROM workflows WHERE id = $1 AND workspace_id = $2 AND user_id = $3;

-- name: CreateWorkflowProposal :one
INSERT INTO workflow_proposals (workflow_id, chat_id, summary, ops, graph) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ListPendingWorkflowProposals :many
SELECT * FROM workflow_proposals WHERE workflow_id = $1 AND state = 'pending' ORDER BY created_at;

-- name: LockWorkflowProposal :one
SELECT * FROM workflow_proposals WHERE id = $1 AND workflow_id = $2 FOR UPDATE;

-- name: SetWorkflowProposalState :exec
UPDATE workflow_proposals SET state = $2 WHERE id = $1;

-- name: SupersedeWorkflowProposals :exec
-- Only one suggestion is open at a time: a newer one replaces the rest.
UPDATE workflow_proposals SET state = 'discarded' WHERE workflow_id = $1 AND state = 'pending';

-- name: CreateWorkflowRun :one
INSERT INTO workflow_runs (workspace_id, user_id, chat_id, project_id, workflow_id, message_id, name, version, graph, input)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

-- name: GetWorkflowRun :one
SELECT * FROM workflow_runs WHERE id = $1 AND workspace_id = $2 AND user_id = $3;

-- name: LockWorkflowRun :one
SELECT * FROM workflow_runs WHERE id = $1 FOR UPDATE;

-- name: WorkflowRunForMessage :one
SELECT * FROM workflow_runs WHERE message_id = $1;

-- name: ListChatWorkflowRuns :many
SELECT * FROM workflow_runs WHERE chat_id = $1 ORDER BY created_at;

-- name: ListWorkflowRuns :many
SELECT * FROM workflow_runs WHERE workspace_id = $1 AND user_id = $2 ORDER BY created_at DESC, id DESC LIMIT 100;

-- name: ActiveWorkflowRuns :many
SELECT id, chat_id FROM workflow_runs WHERE state IN ('running', 'waiting') ORDER BY created_at LIMIT 100;

-- name: LiveWorkflowRunsForChat :many
SELECT * FROM workflow_runs WHERE chat_id = $1 AND state IN ('running', 'waiting');

-- name: SetWorkflowRunState :one
UPDATE workflow_runs SET state = $2, error = $3,
  finished_at = CASE WHEN $2 IN ('done', 'failed', 'stopped') THEN now() ELSE finished_at END
WHERE id = $1 RETURNING *;

-- name: SetWorkflowRevisionNote :exec
UPDATE workflow_runs SET revision_note = $2 WHERE id = $1;

-- name: WorkflowRunForAgentRun :one
SELECT r.* FROM workflow_runs r JOIN workflow_steps s ON s.run_id = r.id WHERE s.agent_run_id = $1;

-- name: OwnsWorkflowAgentRun :one
SELECT EXISTS (SELECT 1 FROM workflow_runs r JOIN workflow_steps s ON s.run_id = r.id WHERE s.agent_run_id = $1 AND r.user_id = $2)::boolean;

-- name: CreateWorkflowStep :one
INSERT INTO workflow_steps (run_id, node_id, attempt, state, output, agent_run_id)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: LatestWorkflowSteps :many
-- The newest attempt of each node, with the state of its agent run.
SELECT DISTINCT ON (s.node_id) s.*, coalesce(r.state, '')::text AS agent_run_state, coalesce(r.outcome, '')::text AS agent_run_outcome
FROM workflow_steps s LEFT JOIN runs r ON r.id = s.agent_run_id
WHERE s.run_id = $1 ORDER BY s.node_id, s.attempt DESC;

-- name: FinishWorkflowStep :exec
UPDATE workflow_steps SET state = $2, output = $3, decision = $4, note = $5, finished_at = now() WHERE id = $1;

-- name: SetWorkflowStepOutput :exec
UPDATE workflow_steps SET output = $2 WHERE id = $1;

-- name: WorkflowStepForAgentRun :one
SELECT * FROM workflow_steps WHERE agent_run_id = $1 FOR UPDATE;

-- name: CreateWorkflowLibraryItem :one
INSERT INTO library_items (workspace_id, user_id, kind, title, filename, content_type, size_bytes, blob_key, chat_id)
VALUES ($1, $2, 'document', $3, $4, 'text/markdown; charset=utf-8', $5, $6, $7) RETURNING *;
