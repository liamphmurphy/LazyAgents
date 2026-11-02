-- name: CreateArtifact :one
INSERT INTO artifacts (workspace_id, project_id, epic_id, type, title, number, summary, author_type, author_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: NextADRNumber :one
-- Serialized by the caller's advisory lock on the workspace.
SELECT (coalesce(max(number), 0) + 1)::int FROM artifacts WHERE workspace_id = $1 AND type = 'adr';

-- name: ADRNumberTaken :one
SELECT EXISTS (SELECT 1 FROM artifacts WHERE workspace_id = $1 AND type = 'adr' AND number = $2);

-- name: LockArtifactNumbering :exec
SELECT pg_advisory_xact_lock(hashtextextended('artifact-numbering:' || $1::uuid::text, 0));

-- name: GetArtifact :one
SELECT * FROM artifacts WHERE id = $1 AND workspace_id = $2;

-- name: LockArtifact :one
SELECT * FROM artifacts WHERE id = $1 AND workspace_id = $2 FOR UPDATE;

-- name: SetArtifactCurrentVersion :exec
UPDATE artifacts SET current_version_id = $2, search_text = $3, summary = $4 WHERE id = $1;

-- name: UpdateArtifactMeta :one
UPDATE artifacts SET title = $2, summary = $3, epic_id = $4 WHERE id = $1 RETURNING *;

-- name: CreateArtifactVersion :one
INSERT INTO artifact_versions (artifact_id, seq, label, body_md, blob_key, filename, content_type, size_bytes, note, created_by_type, created_by_id)
VALUES ($1, (SELECT coalesce(max(seq), 0) + 1 FROM artifact_versions WHERE artifact_id = $1), $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetArtifactVersion :one
SELECT v.* FROM artifact_versions v JOIN artifacts a ON a.id = v.artifact_id
WHERE v.id = $1 AND v.artifact_id = $2 AND a.workspace_id = $3;

-- name: ListArtifactVersions :many
SELECT id, artifact_id, seq, label, filename, content_type, size_bytes, note, created_by_type, created_by_id, created_at
FROM artifact_versions WHERE artifact_id = $1 ORDER BY seq DESC;

-- name: InsertSection :exec
INSERT INTO artifact_sections (version_id, position, level, anchor, heading) VALUES ($1, $2, $3, $4, $5);

-- name: ListSectionsWithCitations :many
-- Citations count reads of a section by anchor across all versions, so
-- history isn't lost when a new version re-extracts the outline.
SELECT s.id, s.position, s.level, s.anchor, s.heading,
  (SELECT count(*) FROM artifact_reads r
     JOIN artifact_sections s2 ON s2.id = r.section_id
     JOIN artifact_versions v2 ON v2.id = s2.version_id
   WHERE v2.artifact_id = sqlc.arg(artifact_id) AND s2.anchor = s.anchor)::int AS citations
FROM artifact_sections s
WHERE s.version_id = sqlc.arg(version_id)
ORDER BY s.position;

-- name: GetSectionByAnchor :one
SELECT * FROM artifact_sections WHERE version_id = $1 AND anchor = $2;

-- name: InsertArtifactRead :exec
INSERT INTO artifact_reads (version_id, section_id, agent_id, run_id, read_at)
VALUES ($1, $2, $3, $4, coalesce(sqlc.narg(read_at)::timestamptz, now()));

-- name: ListArtifactReaders :many
SELECT a.id, a.name, a.handle, a.color_index, count(*)::int AS reads, max(r.read_at)::timestamptz AS last_read_at
FROM artifact_reads r
JOIN artifact_versions v ON v.id = r.version_id
JOIN agents a ON a.id = r.agent_id
WHERE v.artifact_id = $1
GROUP BY a.id ORDER BY max(r.read_at) DESC;

-- name: ListArtifacts :many
SELECT
  ar.id, ar.type, ar.title, ar.number, ar.summary, ar.author_type, ar.author_id, ar.updated_at, ar.created_at,
  p.key AS project_key, p.name AS project_name, p.color AS project_color,
  e.id AS epic_id, e.name AS epic_name,
  v.label AS version_label, v.seq AS version_seq, v.content_type, v.size_bytes, v.filename,
  (v.body_md IS NOT NULL)::bool AS is_markdown,
  (SELECT count(*) FROM artifact_sections s WHERE s.version_id = v.id)::int AS section_count,
  (SELECT count(*) FROM ticket_artifacts ta WHERE ta.artifact_id = ar.id)::int AS ticket_count
FROM artifacts ar
JOIN projects p ON p.id = ar.project_id
LEFT JOIN epics e ON e.id = ar.epic_id
LEFT JOIN artifact_versions v ON v.id = ar.current_version_id
WHERE ar.workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(type)::text IS NULL OR ar.type = sqlc.narg(type))
  AND (sqlc.narg(project_id)::uuid IS NULL OR ar.project_id = sqlc.narg(project_id))
  AND (sqlc.narg(q)::text IS NULL
       OR ar.search @@ websearch_to_tsquery('english', sqlc.narg(q))
       OR ar.title ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY ar.updated_at DESC
LIMIT 500;

-- name: CountArtifactsByType :many
SELECT type, count(*)::int AS n FROM artifacts
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(project_id)::uuid IS NULL OR project_id = sqlc.narg(project_id))
  AND (sqlc.narg(q)::text IS NULL
       OR search @@ websearch_to_tsquery('english', sqlc.narg(q))
       OR title ILIKE '%' || sqlc.narg(q) || '%')
GROUP BY type;

-- name: AttachArtifact :execrows
INSERT INTO ticket_artifacts (ticket_id, artifact_id, relation) VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: DetachArtifact :execrows
DELETE FROM ticket_artifacts WHERE ticket_id = $1 AND artifact_id = $2;

-- name: ListTicketArtifacts :many
-- Doc cards on a ticket. read_by_assignee: has the ticket's agent read any version?
SELECT ar.id, ar.type, ar.title, ar.number, ar.author_type, ar.author_id, ta.relation,
  v.label AS version_label,
  EXISTS (
    SELECT 1 FROM artifact_reads r JOIN artifact_versions v2 ON v2.id = r.version_id
    WHERE v2.artifact_id = ar.id AND r.agent_id = sqlc.narg(assignee_agent_id)
  )::bool AS read_by_assignee
FROM ticket_artifacts ta
JOIN artifacts ar ON ar.id = ta.artifact_id
LEFT JOIN artifact_versions v ON v.id = ar.current_version_id
WHERE ta.ticket_id = sqlc.arg(ticket_id)
ORDER BY ta.relation, ta.created_at;

-- name: ListArtifactTickets :many
SELECT t.key, t.title, t.status, ta.relation
FROM ticket_artifacts ta JOIN tickets t ON t.id = ta.ticket_id
WHERE ta.artifact_id = $1
ORDER BY t.updated_at DESC;

-- name: GetADRByNumber :one
SELECT * FROM artifacts WHERE workspace_id = $1 AND type = 'adr' AND number = $2;
