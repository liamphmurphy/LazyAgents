-- name: CreateTicketChangeRequest :one
INSERT INTO ticket_change_requests (workspace_id, user_id, chat_id, persona_id, agent_id, ticket_id, tool, arguments, changes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: CountPendingTicketChangeRequests :one
SELECT count(*)::int FROM ticket_change_requests WHERE chat_id = $1 AND state = 'pending';

-- name: PendingTicketChangeRequests :many
-- The cards a chat shows, with the ticket as it is now.
SELECT sqlc.embed(r), t.key AS ticket_key, t.title AS ticket_title
FROM ticket_change_requests r JOIN tickets t ON t.id = r.ticket_id
WHERE r.chat_id = $1 AND r.state = 'pending' ORDER BY r.created_at, r.id;

-- name: LockPendingTicketChangeRequest :one
-- Held while the change applies, so a second click waits and then finds it decided.
SELECT * FROM ticket_change_requests
WHERE id = $1 AND workspace_id = $2 AND user_id = $3 AND state = 'pending'
FOR UPDATE;

-- name: GetTicketChangeRequest :one
SELECT sqlc.embed(r), t.key AS ticket_key, t.title AS ticket_title
FROM ticket_change_requests r JOIN tickets t ON t.id = r.ticket_id
WHERE r.id = $1 AND r.workspace_id = $2 AND r.user_id = $3;

-- name: FinishTicketChangeRequest :one
UPDATE ticket_change_requests SET state = $2, error = $3, decided_at = now()
WHERE id = $1 AND state = 'pending' RETURNING *;
