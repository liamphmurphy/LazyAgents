-- name: ListInbox :many
-- Every human gate in one list. agent_id, run_id and artifact_id let the
-- Inbox act on an item (reply, retry, cancel, review) without another lookup.
WITH items AS (
 SELECT ('planning:'||pl.id)::text AS id,'planning'::text AS kind,pl.goal AS title,
 CASE WHEN pl.state='review' THEN 'Draft tickets are ready for your review' ELSE coalesce(r.outcome,'Planning run needs attention') END::text AS summary,
 ('/plans?id='||pl.id)::text AS href,p.key AS project,''::text AS ticket_key,greatest(pl.updated_at,r.updated_at)::timestamptz AS created_at,
 (pl.state IN ('accepted','dismissed')) AS resolved, r.agent_id::uuid AS agent_id, r.id AS run_id, NULL::uuid AS artifact_id
 FROM planning_sessions pl LEFT JOIN runs r ON r.id=pl.run_id JOIN projects p ON p.id=pl.project_id
 WHERE pl.workspace_id=sqlc.arg(workspace_id) AND (pl.state<>'planning' OR r.state IN ('failed','cancelled','blocked'))
 UNION ALL
 SELECT ('approval:'||a.id)::text AS id, 'approval'::text AS kind, t.title, a.action_summary AS summary,
 ('/tickets/'||t.key)::text AS href, p.key AS project, t.key AS ticket_key, a.created_at,
 a.state <> 'pending' AS resolved, a.agent_id, a.run_id, NULL
 FROM approval_requests a JOIN tickets t ON t.id=a.ticket_id JOIN projects p ON p.id=t.project_id
 WHERE a.workspace_id=sqlc.arg(workspace_id)
 UNION ALL
 SELECT 'dispatch:'||t.id,'dispatch',t.title,ag.name||' is waiting for dispatch','/tickets/'||t.key,p.key,t.key,greatest(t.updated_at,(SELECT max(b.updated_at) FROM ticket_dependencies d JOIN tickets b ON b.id=d.blocker_id WHERE d.ticket_id=t.id))::timestamptz,false,
 ag.id,NULL,NULL
 FROM tickets t JOIN projects p ON p.id=t.project_id JOIN agents ag ON ag.id=t.assignee_agent_id
 WHERE t.workspace_id=sqlc.arg(workspace_id) AND NOT p.archived AND t.status NOT IN ('done', 'canceled')
 AND NOT EXISTS(SELECT 1 FROM ticket_dependencies d JOIN tickets b ON b.id=d.blocker_id WHERE d.ticket_id=t.id AND b.status<>'done')
 AND NOT EXISTS (SELECT 1 FROM runs r WHERE r.ticket_id=t.id AND r.state NOT IN ('completed','failed','cancelled'))
 UNION ALL
 SELECT 'review:'||t.id,'review',t.title,'Ready for your review','/tickets/'||t.key,p.key,t.key,t.updated_at,false,t.assignee_agent_id,NULL,NULL
 FROM tickets t JOIN projects p ON p.id=t.project_id
 WHERE t.workspace_id=sqlc.arg(workspace_id) AND NOT p.archived AND t.status='in_review' AND t.reviewer_user_id=sqlc.arg(user_id)
 UNION ALL
 SELECT 'blocked:'||r.id,'blocked',t.title,coalesce(r.outcome,'Run needs a person'),'/runs/'||r.id,p.key,t.key,r.updated_at,false,r.agent_id,r.id,NULL
 FROM runs r JOIN tickets t ON t.id=r.ticket_id JOIN projects p ON p.id=t.project_id
 -- Ticket runs only: a delegated child's result goes to its parent, and Retry refuses children.
 WHERE r.workspace_id=sqlc.arg(workspace_id) AND t.status NOT IN ('done', 'canceled') AND r.state IN ('blocked','failed') AND r.parent_run_id IS NULL
 AND NOT EXISTS (SELECT 1 FROM runs newer WHERE newer.ticket_id=r.ticket_id AND newer.parent_run_id IS NULL AND newer.started_at>r.started_at)
 UNION ALL
 -- await_human: the summary is the agent's question.
 SELECT 'question:'||r.id,'question',t.title,
 coalesce((SELECT nullif(trim(e.payload->>'reason'),'') FROM activity_events e WHERE e.ticket_id=t.id AND e.verb='waiting_for_human' AND e.payload->>'run_id'=r.id::text ORDER BY e.created_at DESC LIMIT 1),'The agent is waiting for a reply'),
 '/runs/'||r.id,p.key,t.key,r.updated_at,false,r.agent_id,r.id,NULL
 FROM runs r JOIN tickets t ON t.id=r.ticket_id JOIN projects p ON p.id=t.project_id
 WHERE r.workspace_id=sqlc.arg(workspace_id) AND t.status NOT IN ('done', 'canceled') AND r.state='waiting' AND r.wait_reason='human_reply'
 UNION ALL
 -- Stalled: running with no event for 30 minutes. created_at is the last activity.
 SELECT 'stalled:'||r.id,'stalled',t.title,'Running, no activity since the last event','/runs/'||r.id,p.key,t.key,
 coalesce((SELECT e.at FROM run_events e WHERE e.run_id=r.id ORDER BY e.seq DESC LIMIT 1),r.started_at),false,r.agent_id,r.id,NULL
 FROM runs r JOIN tickets t ON t.id=r.ticket_id JOIN projects p ON p.id=t.project_id
 WHERE r.workspace_id=sqlc.arg(workspace_id) AND r.state='running'
 AND coalesce((SELECT e.at FROM run_events e WHERE e.run_id=r.id ORDER BY e.seq DESC LIMIT 1),r.started_at) < now()-interval '30 minutes'
 UNION ALL
 SELECT 'mention:'||c.id,'mention',t.title,left(c.body_md,240),'/tickets/'||t.key,p.key,t.key,c.created_at,false,
 CASE WHEN c.author_type='agent' THEN c.author_id END,NULL,NULL
 FROM comments c JOIN tickets t ON t.id=c.ticket_id JOIN projects p ON p.id=t.project_id JOIN users u ON u.id=sqlc.arg(user_id)
 WHERE c.workspace_id=sqlc.arg(workspace_id) AND NOT (c.author_type='user' AND c.author_id=u.id)
 AND (lower(c.body_md) ~ ('@'||regexp_replace(lower(split_part(u.email,'@',1)), '([^a-z0-9_])', '\\\1','g')||'\y')
 OR strpos(lower(c.body_md),'@'||lower(u.name))>0)
 UNION ALL
 SELECT 'suggestion:'||v.id,'suggestion',ar.title,v.note,'/wiki/'||ar.path,coalesce(p.key,''),'',v.created_at,v.state<>'suggested',
 CASE WHEN v.created_by_type='agent' THEN v.created_by_id END,NULL,ar.id
 FROM artifact_versions v JOIN artifacts ar ON ar.id=v.artifact_id LEFT JOIN projects p ON p.id=ar.project_id
 WHERE ar.workspace_id=sqlc.arg(workspace_id) AND v.state IN ('suggested','published','rejected') AND v.based_on_version_id IS NOT NULL
 UNION ALL
 SELECT 'mockup:'||v.id,'mockup',ar.title,
 CASE v.review_state WHEN 'pending' THEN 'Mockup '||v.label||' is waiting for your approval'
 WHEN 'approved' THEN 'Mockup '||v.label||' approved' ELSE 'Changes requested on mockup '||v.label END,
 '/artifacts/'||ar.id||'/review',coalesce(p.key,''),
 coalesce((SELECT t.key FROM ticket_artifacts ta JOIN tickets t ON t.id=ta.ticket_id WHERE ta.artifact_id=ar.id ORDER BY ta.created_at,t.key LIMIT 1),''),
 v.created_at,(v.review_state<>'pending' OR v.id<>ar.current_version_id),
 CASE WHEN v.created_by_type='agent' THEN v.created_by_id END,NULL,ar.id
 FROM artifact_versions v JOIN artifacts ar ON ar.id=v.artifact_id LEFT JOIN projects p ON p.id=ar.project_id
 WHERE ar.workspace_id=sqlc.arg(workspace_id) AND ar.type='mockup' AND v.review_state IS NOT NULL
)
SELECT items.id, items.kind, items.title, items.summary, items.href, items.project, items.ticket_key, items.created_at, items.resolved,
 items.run_id, items.artifact_id, ag.id AS agent_id, coalesce(ag.handle,'')::text AS agent_handle, coalesce(ag.name,'')::text AS agent_name, coalesce(ag.color_index,0)::int AS agent_color_index,
 (rd.read_at IS NOT NULL AND rd.read_at>=items.created_at)::boolean AS read,
 (count(*) FILTER (WHERE NOT items.resolved AND NOT coalesce(rd.read_at>=items.created_at,false)) OVER ())::int AS unread_count
FROM items LEFT JOIN inbox_reads rd ON rd.user_id=sqlc.arg(user_id) AND rd.item_id=items.id
LEFT JOIN agents ag ON ag.id=items.agent_id
WHERE (sqlc.narg(project)::text IS NULL OR items.project=sqlc.narg(project))
ORDER BY items.resolved, (rd.read_at IS NOT NULL AND rd.read_at>=items.created_at), items.created_at DESC,items.id LIMIT 500;

-- name: ReadInboxItem :exec
INSERT INTO inbox_reads (user_id,item_id) VALUES ($1,$2)
ON CONFLICT (user_id,item_id) DO UPDATE SET read_at=clock_timestamp();

-- name: SearchAttentionTargets :many
-- At most 8 per kind, so many chats or agents can't crowd epics out.
SELECT kind,id,title,href FROM (
SELECT *, row_number() OVER (PARTITION BY kind ORDER BY title,id) AS n FROM (
 SELECT 'epic'::text AS kind,e.id,e.name AS title,('/epics?epic='||e.id)::text AS href
 FROM epics e JOIN projects p ON p.id=e.project_id
 WHERE e.workspace_id=sqlc.arg(workspace_id) AND e.name ILIKE '%'||sqlc.arg(q)::text||'%'
 AND (sqlc.narg(project)::text IS NULL OR p.key=sqlc.narg(project))
 UNION ALL
 SELECT 'agent',a.id,a.name, '/agents/'||a.handle
 FROM agents a WHERE a.workspace_id=sqlc.arg(workspace_id) AND a.archived_at IS NULL
 AND (a.name ILIKE '%'||sqlc.arg(q)::text||'%' OR a.handle ILIKE replace(sqlc.arg(q),'@','')||'%')
 AND (sqlc.narg(project)::text IS NULL OR EXISTS (
 SELECT 1 FROM tickets t JOIN projects p ON p.id=t.project_id WHERE t.assignee_agent_id=a.id AND p.key=sqlc.narg(project)))
 UNION ALL
 -- Chats are private: only the asking user's, unarchived, matched by title.
 SELECT 'chat',c.id,c.title,'/chat/'||c.id
 FROM chat_sessions c JOIN projects p ON p.id=c.project_id
 WHERE c.workspace_id=sqlc.arg(workspace_id) AND c.created_by=sqlc.arg(user_id) AND NOT c.archived
 AND c.title ILIKE '%'||sqlc.arg(q)::text||'%'
 AND (sqlc.narg(project)::text IS NULL OR p.key=sqlc.narg(project))
) matches
) results WHERE n<=8 ORDER BY kind,title,id LIMIT 24;
