-- name: CreateChatSession :one
-- Without a persona_id the chat gets its creator's default persona (none
-- when they have no default). Callers check that a given persona_id is the
-- creator's own.
INSERT INTO chat_sessions (workspace_id, project_id, title, created_by, agent_id, harness, model, effort, persona_id)
VALUES (sqlc.arg(workspace_id), sqlc.arg(project_id), sqlc.arg(title), sqlc.arg(created_by), sqlc.arg(agent_id), sqlc.arg(harness), sqlc.arg(model), sqlc.arg(effort),
  COALESCE(sqlc.narg(persona_id)::uuid, (SELECT p.id FROM personas p WHERE p.workspace_id = sqlc.arg(workspace_id) AND p.user_id = sqlc.arg(created_by) AND p.is_default)))
RETURNING *;

-- name: SetChatRepo :one
-- A coding chat's repository and work branch, set once right after creation
-- (the branch carries the chat id) and never changed.
UPDATE chat_sessions SET repo_url = $2, work_branch = $3 WHERE id = $1 RETURNING *;

-- name: ListChatSessions :many
SELECT * FROM chat_sessions
WHERE workspace_id = $1 AND project_id = $2 AND created_by = $3 AND NOT archived
ORDER BY COALESCE(last_message_at, created_at) DESC, id DESC
LIMIT $4;

-- name: GetChatSession :one
SELECT * FROM chat_sessions WHERE id = $1 AND workspace_id = $2;

-- name: RenameChatSession :one
-- Returns no row when the revision is stale.
UPDATE chat_sessions SET title = $3, revision = revision + 1, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND revision = $4 RETURNING *;

-- name: ArchiveChatSession :one
UPDATE chat_sessions SET archived = true, revision = revision + 1, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND revision = $3 RETURNING *;

-- name: AppendChatMessage :one
-- seq is allocated in the statement; concurrent appenders to one session collide on
-- UNIQUE (session_id, seq) and the loser must retry.
WITH next AS (
  SELECT COALESCE(MAX(m.seq), 0) + 1 AS seq FROM chat_messages m WHERE m.session_id = sqlc.arg(session_id)
), ins AS (
  INSERT INTO chat_messages (session_id, seq, role, body_md, harness, model, run_id, run_turn_id, context_refs, citations, usage, persona_id)
  SELECT sqlc.arg(session_id), next.seq, sqlc.arg(role), sqlc.arg(body_md), sqlc.narg(harness), sqlc.narg(model), sqlc.narg(run_id), sqlc.narg(run_turn_id),
         sqlc.arg(context_refs), sqlc.arg(citations), sqlc.narg(usage), sqlc.narg(persona_id)
  FROM next RETURNING *
), touch AS (
  UPDATE chat_sessions SET last_message_at = (SELECT created_at FROM ins), updated_at = now()
  WHERE id = sqlc.arg(session_id)
)
SELECT * FROM ins;

-- name: SetChatMessageCodeChanges :exec
UPDATE chat_messages SET code_changes = $2 WHERE id = $1;

-- name: ListChatMessages :many
SELECT * FROM chat_messages WHERE session_id = $1 AND seq > $2 ORDER BY seq LIMIT $3;

-- name: LockChatSession :one
SELECT * FROM chat_sessions WHERE id = $1 AND workspace_id = $2 FOR UPDATE;

-- name: LockChatSessionByID :one
SELECT * FROM chat_sessions WHERE id = $1 FOR UPDATE;

-- name: ChatSessionForRun :one
SELECT * FROM chat_sessions WHERE current_run_id = $1;

-- name: SetChatRunState :one
UPDATE chat_sessions SET current_run_id = sqlc.narg(current_run_id), state = sqlc.arg(state),
 revision = revision + 1, updated_at = now()
WHERE id = $1 RETURNING *;

-- name: HasRunningChatTurn :one
SELECT EXISTS (SELECT 1 FROM run_turns WHERE run_id = $1 AND status = 'running')::bool;

-- name: FinishUnfinishedChatTurns :exec
UPDATE run_turns SET status = 'done', ended_at = coalesce(ended_at, now()), exit_code = 1
WHERE run_id = $1 AND status IN ('pending', 'running');

-- name: RecoverChatCancellations :exec
-- Cancel may have marked the run paused just before its runner stopped.
-- A replacement runner has a new id. Also recover cancelled turns whose
-- owner no longer has a live heartbeat, including still-running turn records.
WITH locked AS MATERIALIZED (
 SELECT c.id, c.current_run_id FROM chat_sessions c JOIN runs r ON r.id = c.current_run_id
 WHERE r.purpose = 'chat' AND r.state = 'paused' AND (r.runner_id = $1 OR r.runner_id IS NULL OR NOT EXISTS (
   SELECT 1 FROM runners owner WHERE owner.id = r.runner_id
     AND owner.heartbeat_at > now() - interval '45 seconds'
 ))
 ORDER BY c.id FOR UPDATE OF c
), cancelled AS (
 UPDATE runs SET state = 'waiting', wait_reason = 'human_reply'
 WHERE id IN (SELECT current_run_id FROM locked) AND state = 'paused'
 RETURNING id
), turns AS (
 UPDATE run_turns SET status = 'done', ended_at = coalesce(ended_at, now()), exit_code = 130
 WHERE run_id IN (SELECT id FROM cancelled) AND status IN ('pending', 'running')
)
UPDATE chat_sessions SET state = 'idle', revision = revision + 1, updated_at = now()
WHERE current_run_id IN (SELECT id FROM cancelled);

-- name: ChatTurnCitations :many
-- What a chat run read since a turn began, one row per (artifact, version, section).
SELECT a.id AS artifact_id, a.type, a.title, a.path, a.number,
       v.id AS version_id, v.label AS version, s.anchor AS section, max(r.read_at)::timestamptz AS read_at
FROM artifact_reads r
JOIN artifact_versions v ON v.id = r.version_id
JOIN artifacts a ON a.id = v.artifact_id
LEFT JOIN artifact_sections s ON s.id = r.section_id
WHERE r.run_id = sqlc.arg(run_id) AND r.read_at >= sqlc.arg(since)
GROUP BY a.id, v.id, s.anchor
ORDER BY min(r.read_at), a.id, s.anchor NULLS FIRST;

-- name: LinkSourceInProject :one
-- Whether a backlink source (artifact, ticket or comment) is visible to a chat on this project:
-- project-less artifacts are workspace-wide.
SELECT CASE sqlc.arg(source_kind)::text
  WHEN 'artifact' THEN EXISTS (SELECT 1 FROM artifacts x WHERE x.id = sqlc.arg(source_id) AND (x.project_id IS NULL OR x.project_id = sqlc.arg(project_id)))
  WHEN 'ticket' THEN EXISTS (SELECT 1 FROM tickets x WHERE x.id = sqlc.arg(source_id) AND x.project_id = sqlc.arg(project_id))
  WHEN 'comment' THEN EXISTS (SELECT 1 FROM comments c JOIN tickets t ON t.id = c.ticket_id WHERE c.id = sqlc.arg(source_id) AND t.project_id = sqlc.arg(project_id))
  ELSE false END::bool AS visible;

-- name: SetChatSelection :one
-- Stores the harness/model/effort the next segment turn should use. Callers
-- validate them and hold the chat row lock; the runner applies them on the next message.
UPDATE chat_sessions SET harness = $2, model = $3, effort = $4, revision = revision + 1, updated_at = now()
WHERE id = $1 RETURNING *;

-- name: ListRecentChatMessages :many
-- Newest rows first, bounded by limit; bodies are clipped so the read stays small.
-- persona_name is who wrote an assistant message when the chat has several
-- personas ('' for the host or a deleted persona).
SELECT m.seq, m.role, m.harness, m.model, left(m.body_md, sqlc.arg(max_chars)::int)::text AS body_md, COALESCE(p.name, '')::text AS persona_name
FROM chat_messages m LEFT JOIN personas p ON p.id = m.persona_id
WHERE m.session_id = sqlc.arg(session_id) AND m.role IN ('user', 'assistant')
ORDER BY m.seq DESC LIMIT sqlc.arg(row_limit);

-- name: CountChatHistory :one
SELECT count(*)::bigint FROM chat_messages WHERE session_id = $1 AND role IN ('user', 'assistant');

-- name: ListChatsForUser :many
-- One page of a person's chats: archived or not, optionally one project.
SELECT c.* FROM chat_sessions c JOIN projects p ON p.id = c.project_id
WHERE c.workspace_id = $1 AND c.created_by = $2 AND c.archived = sqlc.arg(archived)
  AND (sqlc.narg(project_key)::text IS NULL OR p.key = sqlc.narg(project_key)::text)
  -- persona_filter: 'all', 'none' (chats without a persona) or 'id' (persona_id).
  AND CASE sqlc.arg(persona_filter)::text
    WHEN 'all' THEN true
    WHEN 'none' THEN c.persona_id IS NULL
    ELSE c.persona_id = sqlc.narg(persona_id)::uuid END
ORDER BY COALESCE(c.last_message_at, c.created_at) DESC, c.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: ChatMessageCounts :many
SELECT session_id, count(*)::int AS n FROM chat_messages WHERE session_id = ANY(sqlc.arg(ids)::uuid[]) GROUP BY session_id;

-- name: ChatLastErrors :many
-- The newest system message of each chat; the API shows it as `error` while the chat is failed.
SELECT DISTINCT ON (session_id) session_id, body_md FROM chat_messages
WHERE session_id = ANY(sqlc.arg(ids)::uuid[]) AND role = 'system'
ORDER BY session_id, seq DESC;

-- name: PageChatMessages :many
-- Messages with after < seq < before (either bound optional). newest_first pages
-- back through history; the caller reverses the rows to oldest first.
SELECT * FROM chat_messages
WHERE session_id = sqlc.arg(session_id)
  AND (sqlc.narg(after_seq)::bigint IS NULL OR seq > sqlc.narg(after_seq)::bigint)
  AND (sqlc.narg(before_seq)::bigint IS NULL OR seq < sqlc.narg(before_seq)::bigint)
ORDER BY CASE WHEN sqlc.arg(newest_first)::bool THEN -seq ELSE seq END
LIMIT sqlc.arg(row_limit);

-- name: ChatMessageForTurn :one
SELECT * FROM chat_messages WHERE run_turn_id = $1 AND role = 'user';

-- name: UpdateChatTitleArchived :one
UPDATE chat_sessions SET title = $2, archived = $3, revision = revision + 1, updated_at = now()
WHERE id = $1 RETURNING *;

-- name: SetChatTitle :one
UPDATE chat_sessions SET title = $2, revision = revision + 1, updated_at = now()
WHERE id = $1 RETURNING *;

-- name: FirstChatUserMessageBody :one
SELECT body_md FROM chat_messages WHERE session_id = $1 AND role = 'user' ORDER BY seq LIMIT 1;

-- name: DeleteChatSession :exec
DELETE FROM chat_sessions WHERE id = $1;

-- name: DefaultChatAgent :one
-- The oldest main agent, ties broken by name then id so the choice is
-- deterministic. A new Personal Assistant starts on its harness and model.
SELECT * FROM agents WHERE workspace_id = $1 AND kind = 'main' AND archived_at IS NULL ORDER BY created_at, name, id LIMIT 1;

-- name: AssistantAgent :one
-- The workspace's Personal Assistant: the agent every chat runs as.
SELECT * FROM agents WHERE workspace_id = $1 AND kind = 'assistant';

-- name: OwnsChatRun :one
SELECT EXISTS (
  SELECT 1 FROM chat_sessions s
  WHERE s.workspace_id = sqlc.arg(workspace_id) AND s.created_by = sqlc.arg(viewer)
    AND (s.current_run_id = sqlc.arg(run_id)::uuid OR EXISTS (
      SELECT 1 FROM chat_messages m WHERE m.session_id = s.id AND m.run_id = sqlc.arg(run_id)::uuid
    ))
)::boolean;

-- name: CreateAssistantAgent :one
-- Creates the workspace's Personal Assistant. Returns no row when a concurrent
-- call already made it (or the handle is taken).
INSERT INTO agents (workspace_id, name, handle, color_index, role, specialty, model, status, kind)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'idle', 'assistant')
ON CONFLICT DO NOTHING RETURNING *;

-- name: SetChatAgent :exec
UPDATE chat_sessions SET agent_id = $2 WHERE id = $1;

-- name: CreateChatAttachment :one
INSERT INTO chat_attachments (workspace_id, user_id, filename, content_type, size_bytes, blob_key)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetChatAttachment :one
SELECT * FROM chat_attachments WHERE id = $1 AND workspace_id = $2 AND user_id = $3;

-- name: AttachChatAttachments :many
-- Binds the caller's own unattached uploads to a message; ids that don't
-- qualify are simply not returned, so the caller compares counts.
UPDATE chat_attachments SET message_id = sqlc.arg(message_id)
WHERE id = ANY(sqlc.arg(ids)::uuid[]) AND workspace_id = sqlc.arg(workspace_id) AND user_id = sqlc.arg(user_id) AND message_id IS NULL
RETURNING *;

-- name: CreateGeneratedChatAttachment :one
-- An image the assistant generated, bound to its own message.
INSERT INTO chat_attachments (workspace_id, user_id, message_id, filename, content_type, size_bytes, blob_key, source)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'generated') RETURNING *;

-- name: ChatAttachmentBytes :one
-- What a person has stored in uploads, for the per-person cap (generated
-- images don't count against what they may upload).
SELECT COALESCE(sum(size_bytes), 0)::bigint AS total FROM chat_attachments WHERE workspace_id = $1 AND user_id = $2 AND source = 'upload';

-- name: ChatGeneratedImageBytes :one
-- What a person's assistant chats have generated, for the separate cap on
-- generated images: each stored object once, whether a chat or the Library
-- (which keeps them after their chat is deleted) holds it.
SELECT COALESCE(sum(size_bytes), 0)::bigint AS total FROM (
  SELECT blob_key, size_bytes FROM chat_attachments WHERE chat_attachments.workspace_id = $1 AND chat_attachments.user_id = $2 AND source = 'generated'
  UNION
  SELECT blob_key, size_bytes FROM library_items WHERE library_items.workspace_id = $1 AND library_items.user_id = $2
) stored;

-- name: DeleteStaleChatAttachments :many
-- Removes up to row_limit uploads never sent (or detached by a chat's
-- deletion) that are older than the cutoff, returning their blob keys. SKIP
-- LOCKED lets several runners sweep at once; a row a message is binding at
-- the same moment is re-checked after its lock and survives. Keys the
-- Library still holds are not returned: their objects stay.
WITH d AS (
  DELETE FROM chat_attachments
  WHERE id IN (
    SELECT id FROM chat_attachments
    WHERE message_id IS NULL AND created_at < sqlc.arg(cutoff)::timestamptz
    ORDER BY created_at LIMIT sqlc.arg(row_limit) FOR UPDATE SKIP LOCKED
  ) AND message_id IS NULL
  RETURNING blob_key
)
SELECT d.blob_key FROM d WHERE NOT EXISTS (SELECT 1 FROM library_items l WHERE l.blob_key = d.blob_key);

-- name: ListChatAttachmentsForMessages :many
-- Images generated in one turn share a created_at (one transaction), so name
-- order breaks the tie.
SELECT * FROM chat_attachments WHERE message_id = ANY(sqlc.arg(message_ids)::uuid[]) ORDER BY created_at, filename, id;

-- name: ChatUserMentions :one
-- Whether the person wrote text in this run's conversation (any segment).
SELECT EXISTS (
  SELECT 1 FROM chat_messages m
  WHERE m.role = 'user' AND strpos(m.body_md, sqlc.arg(text)::text) > 0
    AND m.session_id IN (SELECT c.session_id FROM chat_messages c WHERE c.run_id = sqlc.arg(run_id))
) AS mentioned;

-- name: RunWebSearchEvents :many
-- A run's WebSearch calls and results, for the WebFetch allowlist.
SELECT type, payload FROM run_events
WHERE run_id = $1 AND type IN ('tool_call', 'tool_result') AND payload->>'tool' = 'WebSearch'
ORDER BY seq;

-- name: SetChatSpeaker :one
-- Who answers the next turn, who is queued after them, and the relay count.
-- Not a revision bump: it changes together with the turn's state.
UPDATE chat_sessions SET speaker_persona_id = sqlc.narg(speaker_persona_id), queued_persona_ids = sqlc.arg(queued_persona_ids)::uuid[], relay_turns = sqlc.arg(relay_turns)
WHERE id = sqlc.arg(id) RETURNING *;

-- name: AddChatParticipant :exec
INSERT INTO chat_participants (chat_id, persona_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ChatParticipants :many
-- Personas brought into these chats besides their hosts, in the order they joined.
SELECT chat_id, persona_id FROM chat_participants WHERE chat_id = ANY(sqlc.arg(chat_ids)::uuid[]) ORDER BY joined_at, persona_id;

-- name: CreatePersonaProposal :one
INSERT INTO persona_proposals (chat_id, run_id, proposed_by, name, tone, color, custom_instructions, reason)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: CountPendingPersonaProposals :one
SELECT count(*)::int FROM persona_proposals WHERE chat_id = $1 AND state = 'pending';

-- name: ListPersonaProposals :many
SELECT * FROM persona_proposals WHERE chat_id = $1 ORDER BY created_at, id;

-- name: LockPersonaProposal :one
SELECT * FROM persona_proposals WHERE id = $1 AND chat_id = $2 FOR UPDATE;

-- name: DecidePersonaProposal :one
UPDATE persona_proposals SET state = sqlc.arg(state), persona_id = sqlc.narg(persona_id), decided_at = now()
WHERE id = sqlc.arg(id) RETURNING *;

-- name: BumpChatRevision :one
-- A change only the chat's payload shows (participants, proposals): clients
-- refetch on chat.updated.
UPDATE chat_sessions SET revision = revision + 1, updated_at = now() WHERE id = $1 RETURNING *;
