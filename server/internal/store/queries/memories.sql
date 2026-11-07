-- Personal assistant memory. Every query is scoped to one person:
-- (workspace_id, user_id) is always in the WHERE clause.

-- name: CreateMemory :one
INSERT INTO memories (workspace_id, user_id, title, summary, body, tags, type, source, source_chat_id, source_run_id)
VALUES (@workspace_id, @user_id, @title, @summary, @body, @tags::text[], @type, @source, sqlc.narg(source_chat_id), sqlc.narg(source_run_id))
RETURNING *;

-- name: UpdateMemory :one
-- Absent fields keep their value. A non-empty source replaces the old one (the
-- latest conversation that touched the entry).
UPDATE memories SET
  title = coalesce(sqlc.narg(title), title),
  summary = coalesce(sqlc.narg(summary), summary),
  body = coalesce(sqlc.narg(body), body),
  tags = coalesce(sqlc.narg(tags)::text[], tags),
  type = coalesce(sqlc.narg(type), type),
  source = coalesce(sqlc.narg(source), source),
  source_chat_id = CASE WHEN sqlc.narg(source)::text IS NULL THEN source_chat_id ELSE sqlc.narg(source_chat_id) END,
  source_run_id = CASE WHEN sqlc.narg(source)::text IS NULL THEN source_run_id ELSE sqlc.narg(source_run_id) END,
  updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id AND user_id = @user_id
RETURNING *;

-- name: SetMemorySupersededBy :one
-- Retires an entry in favour of another of the same person's entries, or
-- restores it when superseded_by is null.
-- Retiring records the reason and time; restoring clears them.
UPDATE memories m SET superseded_by = sqlc.narg(superseded_by),
  retired_reason = CASE WHEN sqlc.narg(superseded_by)::uuid IS NULL THEN '' ELSE @retired_reason::text END,
  retired_at = CASE WHEN sqlc.narg(superseded_by)::uuid IS NULL THEN NULL ELSE now() END,
  updated_at = now()
WHERE m.id = @id AND m.workspace_id = @workspace_id AND m.user_id = @user_id
  AND (sqlc.narg(superseded_by)::uuid IS NULL OR EXISTS (
    SELECT 1 FROM memories n WHERE n.id = sqlc.narg(superseded_by) AND n.workspace_id = @workspace_id AND n.user_id = @user_id AND n.superseded_by IS NULL))
RETURNING *;

-- name: RetireDuplicateMemory :one
-- The consolidation pass retires a live entry in favour of another live entry
-- of the same person. No row when either was retired or changed meanwhile.
UPDATE memories m SET superseded_by = @keep_id, retired_reason = @retired_reason::text, retired_at = now(), updated_at = now()
WHERE m.id = @id AND m.workspace_id = @workspace_id AND m.user_id = @user_id
  AND m.superseded_by IS NULL AND m.updated_at = @updated_at
  AND EXISTS (SELECT 1 FROM memories n WHERE n.id = @keep_id AND n.workspace_id = @workspace_id AND n.user_id = @user_id AND n.superseded_by IS NULL)
RETURNING *;

-- name: DuplicateMemoryPairs :many
-- Pairs of one person's live entries of the same type that look like the same
-- thing: a similar title or a near-identical summary. keep is the more
-- recently changed of the two. Newest keep first, best match first.
SELECT a.workspace_id, a.user_id, a.id AS keep_id, a.title AS keep_title,
  b.id AS retire_id, b.title AS retire_title, b.updated_at AS retire_updated_at,
  greatest(similarity(lower(a.title), lower(b.title)),
           CASE WHEN a.summary <> '' AND b.summary <> '' THEN similarity(lower(a.summary), lower(b.summary)) ELSE 0 END)::float8 AS score
FROM memories a
JOIN memories b ON b.workspace_id = a.workspace_id AND b.user_id = a.user_id AND b.type = a.type
  AND (b.updated_at, b.id) < (a.updated_at, a.id) AND b.superseded_by IS NULL
WHERE a.superseded_by IS NULL
  AND (similarity(lower(a.title), lower(b.title)) >= @title_threshold::float8
       OR (a.summary <> '' AND b.summary <> '' AND similarity(lower(a.summary), lower(b.summary)) >= @summary_threshold::float8))
ORDER BY a.workspace_id, a.user_id, a.updated_at DESC, a.id DESC, score DESC
LIMIT @lim;

-- name: GetMemory :one
SELECT * FROM memories WHERE id = @id AND workspace_id = @workspace_id AND user_id = @user_id;

-- name: ListMemories :many
-- Newest change first. Retired entries only when include_retired.
SELECT * FROM memories
WHERE workspace_id = @workspace_id AND user_id = @user_id
  AND (@include_retired::bool OR superseded_by IS NULL)
  AND (sqlc.narg(type)::text IS NULL OR type = sqlc.narg(type))
ORDER BY updated_at DESC, id DESC
LIMIT @lim OFFSET @off;

-- name: CountMemories :one
SELECT count(*) FROM memories
WHERE workspace_id = @workspace_id AND user_id = @user_id
  AND (@include_retired::bool OR superseded_by IS NULL)
  AND (sqlc.narg(type)::text IS NULL OR type = sqlc.narg(type));

-- name: SearchMemories :many
-- Ranked full-text search over title, tags, summary and body, plus trigram
-- title similarity so partial words and typos still find an entry.
SELECT sqlc.embed(m),
  (ts_rank(memory_search_vector(m.title, m.summary, m.body, m.tags), websearch_to_tsquery('english', @q::text))
   + similarity(lower(m.title), lower(@q::text)))::float8 AS rank
FROM memories m
WHERE m.workspace_id = @workspace_id AND m.user_id = @user_id
  AND (@include_retired::bool OR m.superseded_by IS NULL)
  AND (sqlc.narg(type)::text IS NULL OR m.type = sqlc.narg(type))
  AND (memory_search_vector(m.title, m.summary, m.body, m.tags) @@ websearch_to_tsquery('english', @q::text)
       OR similarity(lower(m.title), lower(@q::text)) >= 0.3)
ORDER BY rank DESC, m.updated_at DESC, m.id DESC
LIMIT @lim;

-- name: NearDuplicateMemory :one
-- The person's live entry most like a new one: a similar title, or (when both
-- have one) a near-identical summary.
SELECT sqlc.embed(m),
  greatest(similarity(lower(m.title), lower(@title::text)),
           CASE WHEN @summary::text <> '' AND m.summary <> '' THEN similarity(lower(m.summary), lower(@summary::text)) ELSE 0 END)::float8 AS score,
  similarity(lower(m.title), lower(@title::text))::float8 AS title_score
FROM memories m
WHERE m.workspace_id = @workspace_id AND m.user_id = @user_id AND m.superseded_by IS NULL
  AND (similarity(lower(m.title), lower(@title::text)) >= @title_threshold::float8
       OR (@summary::text <> '' AND m.summary <> '' AND similarity(lower(m.summary), lower(@summary::text)) >= @summary_threshold::float8))
ORDER BY score DESC, m.updated_at DESC
LIMIT 1;

-- name: DeleteMemory :execrows
DELETE FROM memories WHERE id = @id AND workspace_id = @workspace_id AND user_id = @user_id;

-- name: DeleteAllMemories :execrows
DELETE FROM memories WHERE workspace_id = @workspace_id AND user_id = @user_id;

-- name: ExportMemories :many
-- Everything the person has, retired entries included, oldest first.
SELECT * FROM memories WHERE workspace_id = @workspace_id AND user_id = @user_id ORDER BY created_at, id;
