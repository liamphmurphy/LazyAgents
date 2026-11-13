-- name: NewKnowledgeImport :one
INSERT INTO knowledge_imports(workspace_id,user_id,blob_key,folder)VALUES($1,$2,$3,$4) RETURNING *;

-- name: GetKnowledgeImport :one
SELECT * FROM knowledge_imports WHERE id=$1 AND workspace_id=$2 AND user_id=$3;

-- name: ListKnowledgeImports :many
SELECT * FROM knowledge_imports WHERE workspace_id=$1 AND user_id=$2 ORDER BY created_at DESC LIMIT 50;

-- name: ClaimKnowledgeImport :one
UPDATE knowledge_imports SET state=CASE WHEN state IN('queued','scanning') THEN 'scanning' ELSE 'importing' END,
 lease_id=sqlc.arg(lease_id),lease_until=now()+interval '10 minutes',updated_at=now()
WHERE id=(SELECT id FROM knowledge_imports WHERE state IN('queued','queued_commit') OR (state IN('scanning','importing') AND lease_until<now()) ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING *;

-- name: UpdateKnowledgeImport :execrows
UPDATE knowledge_imports SET state=sqlc.arg(state),plan=sqlc.arg(plan),report=sqlc.arg(report),error=sqlc.arg(error),files_done=sqlc.arg(files_done),
 lease_until=now()+interval '10 minutes',updated_at=now()
WHERE id=sqlc.arg(id) AND lease_id=sqlc.arg(lease_id);

-- name: LockKnowledgeImportLease :one
SELECT * FROM knowledge_imports WHERE id=$1 AND lease_id=$2 AND state='importing' AND lease_until>now() FOR UPDATE;

-- name: ConfirmKnowledgeImport :one
UPDATE knowledge_imports SET state='queued_commit',policy=sqlc.arg(policy),updated_at=now()
WHERE id=sqlc.arg(id) AND workspace_id=sqlc.arg(workspace_id) AND user_id=sqlc.arg(user_id) AND state='preview' RETURNING *;

-- name: RecordImportFile :exec
INSERT INTO knowledge_import_files(import_id,source_path,artifact_id,before_id,applied_id,asset_id,created,status,before_title,before_path,applied_title,applied_path)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,sqlc.narg(before_title),sqlc.narg(before_path),sqlc.narg(applied_title),sqlc.narg(applied_path)) ON CONFLICT(import_id,source_path) DO NOTHING;

-- name: ListImportFiles :many
SELECT * FROM knowledge_import_files WHERE import_id=$1 ORDER BY source_path;

-- name: GetImportSource :one
SELECT s.* FROM knowledge_import_sources s JOIN artifacts a ON a.id=s.artifact_id
WHERE s.workspace_id=$1 AND s.user_id=$2 AND s.folder=$3 AND s.source_path=$4
AND artifact_visible(a.audience,a.owner_user_id,sqlc.narg(viewer)::uuid);

-- name: SetImportSource :exec
INSERT INTO knowledge_import_sources(workspace_id,user_id,folder,source_path,artifact_id,version_id)
SELECT sqlc.arg(workspace_id),sqlc.arg(user_id),sqlc.arg(folder),sqlc.arg(source_path),a.id,sqlc.narg(version_id)
FROM artifacts a WHERE a.id=sqlc.arg(artifact_id) AND a.workspace_id=sqlc.arg(workspace_id)
AND artifact_visible(a.audience,a.owner_user_id,sqlc.narg(viewer)::uuid)
ON CONFLICT(workspace_id,user_id,folder,source_path) DO UPDATE SET artifact_id=excluded.artifact_id,version_id=excluded.version_id;

-- name: LockKnowledgeImportForUndo :one
SELECT * FROM knowledge_imports WHERE id=$1 AND workspace_id=$2 AND user_id=$3 AND state IN ('done','failed') FOR UPDATE;

-- name: MarkKnowledgeImportUndone :exec
UPDATE knowledge_imports SET state='undone',updated_at=now() WHERE id=$1;

-- name: SetImportedPageTimes :exec
UPDATE artifacts SET created_at=$2,updated_at=$3 WHERE id=$1
AND artifact_visible(audience,owner_user_id,sqlc.narg(viewer)::uuid);

-- name: CancelKnowledgeImport :one
-- Cancel before confirming: nothing was written, so the job ends as undone and
-- its archive is collected like any finished one. A scan in progress holds a
-- lease and is left to finish into preview first.
UPDATE knowledge_imports SET state='undone',updated_at=now()
WHERE id=sqlc.arg(id) AND workspace_id=sqlc.arg(workspace_id) AND user_id=sqlc.arg(user_id) AND state IN('queued','preview') RETURNING *;
