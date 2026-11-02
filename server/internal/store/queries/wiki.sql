-- Wiki pages, [[links]] and backlinks (PLAN.md §12).

-- name: GetNoteByPath :one
SELECT * FROM artifacts WHERE workspace_id = $1 AND type = 'note' AND lower(path) = lower(sqlc.arg(path));

-- name: FindNotesByTitle :many
-- Several notes can share a title in different folders; the shortest path wins.
SELECT * FROM artifacts WHERE workspace_id = $1 AND type = 'note' AND lower(title) = lower(sqlc.arg(title))
  AND current_version_id IS NOT NULL
ORDER BY length(path), path;

-- name: FindArtifactByTitle :one
SELECT * FROM artifacts WHERE workspace_id = $1 AND type <> 'note' AND lower(title) = lower(sqlc.arg(title))
ORDER BY updated_at DESC LIMIT 1;

-- name: FindNoteByAlias :one
SELECT ar.* FROM artifact_aliases al JOIN artifacts ar ON ar.id = al.artifact_id
WHERE al.workspace_id = $1 AND lower(al.path) = lower(sqlc.arg(path))
ORDER BY al.created_at DESC LIMIT 1;

-- name: AddAlias :exec
INSERT INTO artifact_aliases (workspace_id, artifact_id, path) VALUES ($1, $2, $3)
ON CONFLICT (workspace_id, path) DO UPDATE SET artifact_id = EXCLUDED.artifact_id, created_at = now();

-- name: DeleteAlias :exec
DELETE FROM artifact_aliases WHERE workspace_id = $1 AND lower(path) = lower(sqlc.arg(path));

-- name: FindEpicByName :one
SELECT * FROM epics WHERE workspace_id = $1 AND lower(name) = lower(sqlc.arg(name)) LIMIT 1;

-- name: SetNotePath :one
UPDATE artifacts SET path = $2, title = $3 WHERE id = $1 RETURNING *;

-- name: ListNotes :many
SELECT ar.id, ar.path, ar.title, ar.author_type, ar.author_id, ar.updated_at, ar.project_id,
  (ar.current_version_id IS NOT NULL)::bool AS published,
  (SELECT count(*) FROM artifact_versions v WHERE v.artifact_id = ar.id AND v.state = 'suggested')::int AS suggestions
FROM artifacts ar
WHERE ar.workspace_id = $1 AND ar.type = 'note'
ORDER BY lower(ar.path);

-- name: DeleteLinksFromSource :exec
DELETE FROM knowledge_links WHERE source_kind = $1 AND source_id = $2;

-- name: InsertLink :exec
INSERT INTO knowledge_links (workspace_id, source_kind, source_id, source_version_id, source_ticket_id,
  target_kind, target_id, target_anchor, target_text, raw_text, snippet, position)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: UnresolvedTargets :many
SELECT DISTINCT target_text FROM knowledge_links WHERE workspace_id = $1 AND target_kind = 'unresolved';

-- name: ResolveLinks :execrows
-- Points every unresolved link with this text at a target that now exists.
UPDATE knowledge_links SET
  target_kind = CASE WHEN target_anchor <> '' AND sqlc.arg(target_kind)::text = 'artifact' THEN 'section' ELSE sqlc.arg(target_kind)::text END,
  target_id = sqlc.arg(target_id)
WHERE workspace_id = $1 AND target_kind = 'unresolved' AND lower(target_text) = lower(sqlc.arg(target_text));

-- name: LinksFromSource :many
SELECT * FROM knowledge_links WHERE source_kind = $1 AND source_id = $2 ORDER BY position;

-- name: Backlinks :many
-- Links to a target (an artifact's sections count as the artifact), with
-- what the UI needs to name the source.
SELECT l.id, l.source_kind, l.source_id, l.target_kind, l.target_anchor, l.raw_text, l.snippet, l.created_at,
  ar.type AS source_type, ar.title AS source_title, ar.path AS source_path, ar.number AS source_number,
  t.key AS ticket_key, t.title AS ticket_title,
  c.author_type AS comment_author_type, c.author_id AS comment_author_id
FROM knowledge_links l
LEFT JOIN artifacts ar ON l.source_kind = 'artifact' AND ar.id = l.source_id
LEFT JOIN tickets t ON t.id = l.source_ticket_id
LEFT JOIN comments c ON l.source_kind = 'comment' AND c.id = l.source_id
WHERE l.workspace_id = sqlc.arg(workspace_id)
  AND l.target_id = sqlc.arg(target_id)
  AND (l.target_kind = sqlc.arg(target_kind) OR (sqlc.arg(target_kind) = 'artifact' AND l.target_kind = 'section'))
ORDER BY l.created_at DESC, l.position
LIMIT 200;

-- name: CountBacklinks :one
SELECT count(DISTINCT (source_kind, source_id))::int FROM knowledge_links
WHERE target_id = sqlc.arg(target_id) AND target_kind IN ('artifact', 'section');

-- name: MentionCandidatesArtifacts :many
-- Markdown artifacts whose text mentions a phrase (for unlinked mentions).
SELECT ar.id, ar.type, ar.title, ar.path, ar.number, v.body_md
FROM artifacts ar JOIN artifact_versions v ON v.id = ar.current_version_id
WHERE ar.workspace_id = $1 AND v.body_md IS NOT NULL AND ar.id <> sqlc.arg(exclude_id)
  AND (ar.search @@ phraseto_tsquery('english', sqlc.arg(phrase)) OR v.body_md ILIKE '%' || sqlc.arg(phrase) || '%')
LIMIT 50;

-- name: MentionCandidatesTickets :many
SELECT id, key, title, description_md FROM tickets
WHERE workspace_id = $1 AND id <> sqlc.arg(exclude_id)
  AND (search @@ phraseto_tsquery('english', sqlc.arg(phrase)) OR description_md ILIKE '%' || sqlc.arg(phrase) || '%')
LIMIT 50;

-- name: SearchKnowledgeArtifacts :many
-- Full-text search over artifacts, boosted by how many sources link to them.
SELECT ar.id, ar.type, ar.title, ar.path, ar.number, ar.summary, ar.project_id, ar.updated_at,
  ts_headline('english', ar.search_text, websearch_to_tsquery('english', sqlc.arg(q)),
    'MaxWords=30, MinWords=10, MaxFragments=1, StartSel=«, StopSel=»')::text AS snippet,
  (ts_rank(ar.search, websearch_to_tsquery('english', sqlc.arg(q))) *
    (1 + ln(1 + (SELECT count(DISTINCT (l.source_kind, l.source_id)) FROM knowledge_links l
                 WHERE l.target_id = ar.id AND l.target_kind IN ('artifact', 'section')))))::float8 AS score
FROM artifacts ar
WHERE ar.workspace_id = sqlc.arg(workspace_id) AND ar.current_version_id IS NOT NULL
  AND (ar.search @@ websearch_to_tsquery('english', sqlc.arg(q)) OR ar.title ILIKE '%' || sqlc.arg(q) || '%')
  AND (sqlc.narg(types)::text[] IS NULL OR ar.type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(project_id)::uuid IS NULL OR ar.project_id IS NULL OR ar.project_id = sqlc.narg(project_id))
ORDER BY score DESC, ar.updated_at DESC
LIMIT sqlc.arg(lim);

-- name: SearchKnowledgeTickets :many
SELECT t.id, t.key, t.title, t.status, t.project_id,
  ts_headline('english', t.title || E'\n' || t.description_md, websearch_to_tsquery('english', sqlc.arg(q)),
    'MaxWords=30, MinWords=10, MaxFragments=1, StartSel=«, StopSel=»')::text AS snippet,
  ts_rank(t.search, websearch_to_tsquery('english', sqlc.arg(q)))::float8 AS score
FROM tickets t
WHERE t.workspace_id = sqlc.arg(workspace_id)
  AND (t.search @@ websearch_to_tsquery('english', sqlc.arg(q)) OR t.key = upper(sqlc.arg(q)))
  AND (sqlc.narg(project_id)::uuid IS NULL OR t.project_id = sqlc.narg(project_id))
ORDER BY score DESC, t.updated_at DESC
LIMIT sqlc.arg(lim);

-- name: SuggestTargets :many
-- [[ autocomplete: notes, artifacts, tickets, epics and agents by prefix or substring.
SELECT kind, id, label, sub, sort FROM (
  SELECT 'note'::text AS kind, ar.id, ar.title AS label, ar.path AS sub, 0 AS sort, ar.updated_at AS at
    FROM artifacts ar WHERE ar.workspace_id = $1 AND ar.type = 'note' AND ar.current_version_id IS NOT NULL
      AND (ar.path ILIKE '%' || sqlc.arg(q) || '%')
  UNION ALL
  SELECT ar.type, ar.id, CASE WHEN ar.type = 'adr' THEN 'ADR-' || lpad(ar.number::text, 3, '0') ELSE ar.title END,
         CASE WHEN ar.type = 'adr' THEN ar.title ELSE upper(ar.type) END, 1, ar.updated_at
    FROM artifacts ar WHERE ar.workspace_id = $1 AND ar.type <> 'note' AND ar.current_version_id IS NOT NULL
      AND (ar.title ILIKE '%' || sqlc.arg(q) || '%'
           OR (ar.type = 'adr' AND ('ADR-' || lpad(ar.number::text, 3, '0')) ILIKE '%' || sqlc.arg(q) || '%'))
  UNION ALL
  SELECT 'ticket', t.id, t.key, t.title, 2, t.updated_at
    FROM tickets t WHERE t.workspace_id = $1 AND (t.key ILIKE sqlc.arg(q) || '%' OR t.title ILIKE '%' || sqlc.arg(q) || '%')
  UNION ALL
  SELECT 'epic', e.id, e.name, 'Epic', 3, e.created_at
    FROM epics e WHERE e.workspace_id = $1 AND e.name ILIKE '%' || sqlc.arg(q) || '%'
  UNION ALL
  SELECT 'agent', a.id, '@' || a.handle, a.name, 4, a.created_at
    FROM agents a WHERE a.workspace_id = $1 AND (a.handle ILIKE sqlc.arg(q) || '%' OR a.name ILIKE sqlc.arg(q) || '%'
                                                 OR ('@' || a.handle) ILIKE sqlc.arg(q) || '%')
) s
ORDER BY (lower(label) LIKE lower(sqlc.arg(q)) || '%') DESC, sort, at DESC
LIMIT 12;

-- name: ListSuggestions :many
-- Pending agent edits to shared notes, oldest first.
SELECT v.id, v.artifact_id, v.label, v.note, v.body_md, v.created_by_type, v.created_by_id, v.created_at,
  v.based_on_version_id, ar.path, ar.title, ar.current_version_id
FROM artifact_versions v JOIN artifacts ar ON ar.id = v.artifact_id
WHERE ar.workspace_id = $1 AND v.state = 'suggested'
  AND (sqlc.narg(artifact_id)::uuid IS NULL OR v.artifact_id = sqlc.narg(artifact_id))
ORDER BY v.created_at;

-- name: CountSuggestions :one
SELECT count(*)::int FROM artifact_versions v JOIN artifacts ar ON ar.id = v.artifact_id
WHERE ar.workspace_id = $1 AND v.state = 'suggested';

-- name: LockSuggestion :one
SELECT v.* FROM artifact_versions v JOIN artifacts ar ON ar.id = v.artifact_id
WHERE v.id = $1 AND ar.workspace_id = $2 FOR UPDATE OF v;

-- name: DecideVersion :exec
UPDATE artifact_versions SET state = $2, decided_by_id = $3, decided_at = now() WHERE id = $1;

-- name: ExportNotes :many
SELECT ar.id, ar.path, ar.title, ar.author_type, ar.author_id, ar.created_at, ar.updated_at, v.body_md, p.key AS project_key
FROM artifacts ar
JOIN artifact_versions v ON v.id = ar.current_version_id
LEFT JOIN projects p ON p.id = ar.project_id
WHERE ar.workspace_id = $1 AND ar.type = 'note'
ORDER BY lower(ar.path);

-- name: GraphEdges :many
-- Resolved links among artifacts, tickets, epics and agents: (source, target)
-- pairs for the local graph. Comment links count as their ticket's.
SELECT DISTINCT
  CASE WHEN l.source_kind = 'artifact' THEN 'artifact' ELSE 'ticket' END::text AS source_kind,
  CASE WHEN l.source_kind = 'artifact' THEN l.source_id ELSE l.source_ticket_id END::uuid AS source_id,
  CASE WHEN l.target_kind = 'section' THEN 'artifact' ELSE l.target_kind END::text AS target_kind,
  l.target_id::uuid AS target_id
FROM knowledge_links l
WHERE l.workspace_id = sqlc.arg(workspace_id) AND l.target_kind <> 'unresolved'
  AND (
    (CASE WHEN l.source_kind = 'artifact' THEN l.source_id ELSE l.source_ticket_id END) = ANY(sqlc.arg(ids)::uuid[])
    OR l.target_id = ANY(sqlc.arg(ids)::uuid[])
  );

-- name: TicketsByIDs :many
SELECT id, key, title, status FROM tickets WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ArtifactsByIDs :many
SELECT id, type, title, path, number FROM artifacts WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EpicsByIDs :many
SELECT id, name FROM epics WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: AgentsByIDs :many
SELECT id, name, handle, color_index FROM agents WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: TicketArtifactIDs :many
SELECT artifact_id FROM ticket_artifacts WHERE ticket_id = $1;

-- name: SetTicketDescription :exec
UPDATE tickets SET description_md = $2 WHERE id = $1;

-- name: DeleteArtifact :exec
DELETE FROM artifacts WHERE id = $1 AND workspace_id = $2;

-- name: RelabelVersion :exec
UPDATE artifact_versions SET label = $2 WHERE id = $1;

-- name: NoteLinksTo :one
-- Does a note already link to this artifact (memory index bookkeeping)?
SELECT EXISTS (SELECT 1 FROM knowledge_links WHERE source_kind = 'artifact' AND source_id = $1 AND target_id = $2)::bool;

-- name: LinksFromTicket :many
-- Links in a ticket's description and its comments.
SELECT * FROM knowledge_links WHERE source_ticket_id = $1 ORDER BY created_at, position;
