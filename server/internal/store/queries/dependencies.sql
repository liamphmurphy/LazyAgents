-- name: LockDependencyGraph :exec
SELECT pg_advisory_xact_lock(hashtextextended('ticket-dependencies:'||$1::uuid::text,0));

-- name: TicketDependencies :many
SELECT t.id,t.key,t.title,t.status FROM ticket_dependencies d JOIN tickets t ON t.id=d.blocker_id WHERE d.ticket_id=$1 ORDER BY t.key;

-- name: TicketDependents :many
SELECT t.id,t.key,t.title,t.status FROM ticket_dependencies d JOIN tickets t ON t.id=d.ticket_id WHERE d.blocker_id=$1 ORDER BY t.key;

-- name: UnmetDependencies :many
SELECT t.id,t.key,t.title,t.status FROM ticket_dependencies d JOIN tickets t ON t.id=d.blocker_id WHERE d.ticket_id=$1 AND t.status<>'done' ORDER BY t.key;

-- name: DependencyWouldCycle :one
WITH RECURSIVE upstream(id) AS (
 SELECT sqlc.arg(blocker)::uuid
 UNION
 SELECT d.blocker_id FROM ticket_dependencies d JOIN upstream u ON d.ticket_id=u.id
) SELECT EXISTS(SELECT 1 FROM upstream WHERE id=sqlc.arg(ticket)::uuid)::boolean;

-- name: ClearDependencies :exec
DELETE FROM ticket_dependencies WHERE ticket_id=$1;

-- name: AddDependency :exec
INSERT INTO ticket_dependencies(ticket_id,blocker_id) VALUES($1,$2);

-- name: BumpDependencyRevision :exec
UPDATE tickets SET dependency_revision=dependency_revision+1 WHERE id=$1;
