-- MCP connectors are private to one person: every query filters on (workspace_id, user_id).

-- name: CreateConnector :one
INSERT INTO connectors (workspace_id, user_id, name, slug, url, auth_kind) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: ListConnectors :many
SELECT * FROM connectors WHERE workspace_id = $1 AND user_id = $2 ORDER BY lower(name), id;

-- name: GetConnector :one
SELECT * FROM connectors WHERE id = $1 AND workspace_id = $2 AND user_id = $3;

-- name: GetConnectorBySlug :one
SELECT * FROM connectors WHERE slug = $1 AND workspace_id = $2 AND user_id = $3;

-- name: ConnectorByOAuthState :one
SELECT * FROM connectors WHERE oauth_state = $1;

-- name: UpdateConnector :one
-- Returns no row when the revision is stale.
UPDATE connectors SET name = $4, tools = $5, revision = revision + 1, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND revision = $3 RETURNING *;

-- name: SetConnectorCredential :exec
UPDATE connectors SET cred_nonce = $2, cred_ciphertext = $3, updated_at = now() WHERE id = $1;

-- name: SetConnectorOAuth :exec
UPDATE connectors SET oauth = $2, oauth_state = sqlc.narg(oauth_state), updated_at = now() WHERE id = $1;

-- name: SetConnectorHealth :one
UPDATE connectors SET status = $2, last_error = $3, tools = $4, last_checked_at = now(), revision = revision + 1, updated_at = now() WHERE id = $1 RETURNING *;

-- name: DeleteConnector :execrows
DELETE FROM connectors WHERE id = $1 AND workspace_id = $2 AND user_id = $3;

-- name: ListConnectorPersonas :many
SELECT connector_id, persona_id FROM connector_personas WHERE connector_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY connector_id, persona_id;

-- name: ClearConnectorPersonas :exec
DELETE FROM connector_personas WHERE connector_id = $1;

-- name: AddConnectorPersona :exec
INSERT INTO connector_personas (connector_id, persona_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ConnectorsForPersona :many
-- What this persona may use; a chat's list, when it has one, narrows it.
SELECT c.* FROM connectors c JOIN connector_personas cp ON cp.connector_id = c.id
WHERE c.workspace_id = $1 AND c.user_id = $2 AND cp.persona_id = $3
  AND (sqlc.narg(allowed_ids)::uuid[] IS NULL OR c.id = ANY(sqlc.narg(allowed_ids)::uuid[]))
ORDER BY c.slug;

-- name: CreateConnectorApproval :one
INSERT INTO connector_approvals (workspace_id, user_id, connector_id, persona_id, chat_id, tool, arguments) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: GetConnectorApproval :one
SELECT * FROM connector_approvals WHERE id = $1;

-- name: DecideConnectorApproval :one
UPDATE connector_approvals SET state = $2, decided_at = now()
WHERE id = $1 AND workspace_id = $3 AND user_id = $4 AND state = 'pending' RETURNING *;

-- name: ExpireConnectorApproval :exec
UPDATE connector_approvals SET state = 'expired', decided_at = now() WHERE id = $1 AND state = 'pending';

-- name: PendingConnectorApprovals :many
SELECT a.*, c.name AS connector_name FROM connector_approvals a JOIN connectors c ON c.id = a.connector_id
WHERE a.chat_id = $1 AND a.state = 'pending' ORDER BY a.created_at;

-- name: HasPendingConnectorApproval :one
SELECT EXISTS (SELECT 1 FROM connector_approvals WHERE chat_id = $1 AND state = 'pending')::bool;

-- name: CreateConnectorProposal :one
INSERT INTO connector_proposals (chat_id, run_id, proposed_by, name, url, auth_kind, reason) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: CountPendingConnectorProposals :one
SELECT count(*)::int FROM connector_proposals WHERE chat_id = $1 AND state = 'pending';

-- name: ListConnectorProposals :many
SELECT * FROM connector_proposals WHERE chat_id = $1 AND state = 'pending' ORDER BY created_at, id;

-- name: GetConnectorProposal :one
SELECT p.* FROM connector_proposals p JOIN chat_sessions c ON c.id = p.chat_id
WHERE p.id = $1 AND c.workspace_id = $2 AND c.created_by = $3;

-- name: FinishConnectorProposal :one
UPDATE connector_proposals SET state = $2, connector_id = sqlc.narg(connector_id), decided_at = now()
WHERE id = $1 AND state = 'pending' RETURNING *;
