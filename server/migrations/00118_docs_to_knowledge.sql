-- +goose Up
-- Copy, never delete, the source Docs. A later, separately reviewed migration
-- may retire them after Knowledge has been used with real data.
ALTER TABLE artifact_versions ADD COLUMN source_canvas_version_id uuid;
CREATE TABLE doc_page_redirects (
 canvas_id uuid PRIMARY KEY REFERENCES chat_canvases(id) ON DELETE CASCADE,
 artifact_id uuid NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
 merged boolean NOT NULL, seq_shift integer NOT NULL DEFAULT 0,
 migration_seq integer NOT NULL, original_head_id uuid,
 divergent_version_id uuid, cleanup_version_id uuid, warning text NOT NULL DEFAULT '', indexed boolean NOT NULL DEFAULT false
);
CREATE TABLE page_comments (
 id uuid PRIMARY KEY DEFAULT uuid_v7(), artifact_id uuid NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
 parent_id uuid REFERENCES page_comments(id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
 seq integer NOT NULL, author_type text NOT NULL CHECK(author_type IN ('user','agent')), author_id uuid NOT NULL,
 body text NOT NULL CHECK(length(body) BETWEEN 1 AND 4000),anchor jsonb NOT NULL DEFAULT '{}',
 target_lost boolean NOT NULL DEFAULT false,status text NOT NULL CHECK(status IN ('draft','open','addressed','resolved')),
 suggestion text,sent_in_message_id uuid REFERENCES chat_messages(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX page_comments_page ON page_comments(artifact_id,created_at);
-- +goose StatementBegin
DO $$
DECLARE d record; aid uuid; head uuid; vid uuid; source_head uuid; match_count integer; n integer; maxseq integer; counter integer; p text; base text; merged boolean; warning text; cleanup uuid; original_head uuid; old_body text; cleaned text; marker text; at_pos integer;
BEGIN
 FOR d IN SELECT c.*,s.workspace_id,s.created_by FROM chat_canvases c JOIN chat_sessions s ON s.id=c.session_id JOIN chat_spaces sp ON sp.id=s.space_id WHERE c.kind='text' AND sp.app='docs' ORDER BY c.created_at,c.id LOOP
  aid:=NULL;head:=NULL;cleanup:=NULL;original_head:=NULL;warning:='';merged:=false;
  SELECT count(DISTINCT a.id),min(a.id::text)::uuid INTO match_count,aid
  FROM artifacts a JOIN artifact_versions v ON v.artifact_id=a.id
  WHERE a.workspace_id=d.workspace_id AND a.type='note' AND v.created_by_type='user' AND v.created_by_id=d.created_by AND v.note='Promoted from the Doc “'||d.title||'”';
  IF d.promoted_artifact_id IS NOT NULL THEN
   SELECT id INTO aid FROM artifacts WHERE id=d.promoted_artifact_id AND workspace_id=d.workspace_id AND type='note';
   IF aid IS NOT NULL THEN match_count:=1; END IF;
  END IF;
  SELECT count(*),max(id::text)::uuid INTO n,source_head FROM chat_canvas_versions WHERE canvas_id=d.id;
  IF n=0 THEN CONTINUE; END IF;
  SELECT id INTO source_head FROM chat_canvas_versions WHERE canvas_id=d.id ORDER BY seq DESC LIMIT 1;
  IF match_count=1 AND aid IS NOT NULL AND NOT EXISTS(SELECT 1 FROM doc_page_redirects WHERE artifact_id=aid) THEN
   merged:=true;
   SELECT current_version_id INTO head FROM artifacts WHERE id=aid;original_head:=head;
   UPDATE artifact_versions SET seq=seq+1000000000 WHERE artifact_id=aid;
   UPDATE artifact_versions SET seq=seq-1000000000+n WHERE artifact_id=aid;
   UPDATE artifacts SET chat_session_id=d.session_id WHERE id=aid;
  ELSE
   aid:=uuid_v7();
   IF match_count>1 THEN warning:='Ambiguous promotions: retained as a private page for review';END IF;
   base:='Docs/'||trim(regexp_replace(regexp_replace(d.title,'[\[\]#|^/\\[:cntrl:]]',' ','g'),'\s+',' ','g'));
   IF base='Docs/' OR base IN ('Docs/.','Docs/..') THEN base:='Docs/Untitled'; END IF;
   p:=base;counter:=1;
   WHILE EXISTS(SELECT 1 FROM artifacts WHERE workspace_id=d.workspace_id AND audience='private' AND owner_user_id=d.created_by AND lower(path)=lower(p)) LOOP counter:=counter+1;p:=base||' '||counter;END LOOP;
   INSERT INTO artifacts(id,workspace_id,type,title,path,author_type,author_id,audience,owner_user_id,persona_direct,chat_session_id,created_at,updated_at)
   VALUES(aid,d.workspace_id,'note',d.title,p,'user',d.created_by,'private',d.created_by,true,d.session_id,d.created_at,d.updated_at);
  END IF;
  INSERT INTO artifact_versions(artifact_id,seq,label,body_md,content_type,size_bytes,note,created_by_type,created_by_id,created_at,state,properties,source_canvas_version_id,review_state)
  SELECT aid,seq,'v'||seq,body,'text/markdown; charset=utf-8',octet_length(body),note,author_type,author_id,created_at,'published','{}',id,
    CASE WHEN author_type='agent' AND seq>d.reviewed_seq THEN 'pending' END
  FROM chat_canvas_versions WHERE canvas_id=d.id ORDER BY seq;
  IF NOT merged THEN
   SELECT id INTO vid FROM artifact_versions WHERE artifact_id=aid ORDER BY seq DESC LIMIT 1;
   UPDATE artifacts SET current_version_id=vid WHERE id=aid;
  END IF;
  SELECT max(seq) INTO maxseq FROM artifact_versions WHERE artifact_id=aid;
  vid:=NULL;
  IF merged AND head IS NOT NULL THEN
   SELECT body_md INTO old_body FROM artifact_versions WHERE id=head;
   marker:=E'\n\n---\n\n_Promoted from the Doc “'||d.title||'” on ';
   at_pos:=CASE WHEN strpos(reverse(old_body),reverse(marker))>0 THEN length(old_body)-strpos(reverse(old_body),reverse(marker))-length(marker)+2 ELSE 0 END;
   IF at_pos>0 AND substring(old_body FROM at_pos+length(marker)) ~ E'^\\d{4}-\\d{2}-\\d{2}\\._\\s*$' THEN
    cleaned:=left(old_body,at_pos-1)||E'\n';
    INSERT INTO artifact_versions(artifact_id,seq,label,body_md,content_type,size_bytes,note,created_by_type,created_by_id,state,properties)
    SELECT aid,maxseq+1,'v'||(maxseq+1),cleaned,'text/markdown; charset=utf-8',octet_length(cleaned),'Removed legacy Doc promotion footer','user',d.created_by,'published',properties FROM artifact_versions WHERE id=head RETURNING id INTO cleanup;
    head:=cleanup;maxseq:=maxseq+1;
    UPDATE artifacts SET current_version_id=head WHERE id=aid;
   END IF;
  END IF;
  IF merged AND (SELECT body FROM chat_canvas_versions WHERE id=source_head) IS DISTINCT FROM (SELECT regexp_replace(body_md,E'\n\n---\n\n_Promoted from the Doc .*_$','','s') FROM artifact_versions WHERE id=head) THEN
   INSERT INTO artifact_versions(artifact_id,seq,label,body_md,content_type,size_bytes,note,created_by_type,created_by_id,state,based_on_version_id,properties)
   SELECT aid,maxseq+1,'suggestion',body,'text/markdown; charset=utf-8',octet_length(body),'Doc differed from promoted wiki page: choose which text to keep',author_type,author_id,'suggested',head,'{}' FROM chat_canvas_versions WHERE id=source_head RETURNING id INTO vid;
  END IF;
  INSERT INTO doc_page_redirects(canvas_id,artifact_id,merged,seq_shift,migration_seq,original_head_id,divergent_version_id,cleanup_version_id,warning)
  VALUES(d.id,aid,merged,CASE WHEN merged THEN n ELSE 0 END,maxseq,original_head,vid,cleanup,warning);
  INSERT INTO page_comments SELECT id,aid,parent_id,seq,author_type,author_id,body,anchor,target_lost,status,suggestion,sent_in_message_id,created_at,updated_at FROM chat_canvas_comments WHERE canvas_id=d.id;
 END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Restore post-migration edits and comments before removing copied pages.
-- +goose StatementBegin
DO $$
DECLARE m record; offset_seq integer;
BEGIN
 FOR m IN SELECT * FROM doc_page_redirects LOOP
  SELECT coalesce(max(seq),0) INTO offset_seq FROM chat_canvas_versions WHERE canvas_id=m.canvas_id;
  INSERT INTO chat_canvas_versions(canvas_id,seq,body,note,author_type,author_id,created_at)
  SELECT m.canvas_id,offset_seq+row_number()OVER(ORDER BY seq),body_md,note,created_by_type,created_by_id,created_at
  FROM artifact_versions WHERE artifact_id=m.artifact_id AND source_canvas_version_id IS NULL AND seq>m.migration_seq AND id IS DISTINCT FROM m.divergent_version_id AND state='published' AND body_md IS NOT NULL;
  INSERT INTO chat_canvas_comments(id,canvas_id,parent_id,seq,author_type,author_id,body,anchor,target_lost,status,suggestion,sent_in_message_id,created_at,updated_at)
  SELECT id,m.canvas_id,parent_id,seq,author_type,author_id,body,anchor,target_lost,status,suggestion,sent_in_message_id,created_at,updated_at FROM page_comments WHERE artifact_id=m.artifact_id
  ON CONFLICT(id) DO UPDATE SET body=excluded.body,anchor=excluded.anchor,target_lost=excluded.target_lost,status=excluded.status,suggestion=excluded.suggestion,updated_at=excluded.updated_at;
  IF m.merged THEN
   UPDATE artifacts SET current_version_id=m.original_head_id WHERE id=m.artifact_id AND current_version_id=m.cleanup_version_id;
   DELETE FROM artifact_versions WHERE artifact_id=m.artifact_id AND (source_canvas_version_id IS NOT NULL OR id=m.divergent_version_id OR id=m.cleanup_version_id);
   UPDATE artifact_versions SET seq=seq+1000000000 WHERE artifact_id=m.artifact_id;
   UPDATE artifact_versions SET seq=seq-1000000000-m.seq_shift WHERE artifact_id=m.artifact_id;
   UPDATE artifacts SET chat_session_id=NULL WHERE id=m.artifact_id;
  ELSE
   DELETE FROM artifacts WHERE id=m.artifact_id;
  END IF;
 END LOOP;
END $$;
-- +goose StatementEnd
DROP TABLE page_comments;
DROP TABLE doc_page_redirects;
ALTER TABLE artifact_versions DROP COLUMN source_canvas_version_id;
