package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/db"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/pressly/goose/v3"
)

func TestKnowledgeMigrationPreservesHistoryAndRollsBackLaterEdits(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	user, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: user.WorkspaceID, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	if err = goose.DownToContext(ctx, sqlDB, ".", 117); err != nil {
		t.Fatal(err)
	}
	var space, chat, cv uuid.UUID
	if err = pool.QueryRow(ctx, `INSERT INTO chat_spaces(workspace_id,user_id,name,color,app)VALUES($1,$2,'Docs','blue','docs')RETURNING id`, user.WorkspaceID, user.ID).Scan(&space); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO chat_sessions(workspace_id,title,created_by,agent_id,harness,model,effort,space_id)VALUES($1,'Old Doc',$2,$3,'codex','gpt-6.1-sol','high',$4)RETURNING id`, user.WorkspaceID, user.ID, agent.ID, space).Scan(&chat); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO chat_canvases(session_id,kind,title)VALUES($1,'text','Title [free]')RETURNING id`, chat).Scan(&cv); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO chat_canvas_versions(canvas_id,seq,body,author_type,author_id)VALUES($1,1,'# Old Doc','user',$2),($1,2,'# Old Doc' || E'\n\nPersona edit','agent',$3)`, cv, user.ID, agent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO chat_canvas_comments(canvas_id,seq,author_type,author_id,body,status)VALUES($1,2,'user',$2,'Keep this comment','open')`, cv, user.ID); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	var audience string
	var mappedChat uuid.UUID
	if err = pool.QueryRow(ctx, `SELECT a.id,a.audience,a.chat_session_id FROM doc_page_redirects d JOIN artifacts a ON a.id=d.artifact_id WHERE d.canvas_id=$1`, cv).Scan(&id, &audience, &mappedChat); err != nil {
		t.Fatal(err)
	}
	if audience != "private" || mappedChat != chat {
		t.Fatalf("audience %s chat %s", audience, mappedChat)
	}
	var n int
	if err = pool.QueryRow(ctx, `SELECT count(*)FROM artifact_versions WHERE artifact_id=$1`, id).Scan(&n); err != nil || n != 2 {
		t.Fatalf("versions %d %v", n, err)
	}
	var preserved bool
	if err = pool.QueryRow(ctx, `SELECT bool_and(v.body_md=c.body AND v.created_by_type=c.author_type AND v.created_by_id=c.author_id AND v.created_at=c.created_at)FROM artifact_versions v JOIN chat_canvas_versions c ON c.id=v.source_canvas_version_id WHERE v.artifact_id=$1`, id).Scan(&preserved); err != nil || !preserved {
		t.Fatalf("history not preserved: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO artifact_versions(artifact_id,seq,label,body_md,content_type,size_bytes,created_by_type,created_by_id,state)VALUES($1,3,'v3','Written after migration','text/markdown',23,'user',$2,'published')`, id, user.ID); err != nil {
		t.Fatal(err)
	}
	if err = goose.DownToContext(ctx, sqlDB, ".", 117); err != nil {
		t.Fatal(err)
	}
	var last string
	if err = pool.QueryRow(ctx, `SELECT body FROM chat_canvas_versions WHERE canvas_id=$1 ORDER BY seq DESC LIMIT 1`, cv).Scan(&last); err != nil || last != "Written after migration" {
		t.Fatalf("lost post-migration edit: %q %v", last, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM chat_canvas_comments WHERE canvas_id=$1`, cv).Scan(&n); err != nil || n != 1 {
		t.Fatalf("comments %d %v", n, err)
	}
	if err = db.Migrate(ctx, pool); err != nil {
		t.Fatalf("up again: %v", err)
	}
}

func TestKnowledgeMigrationMergesPromotedDocWithoutDroppingSource(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	user, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: user.WorkspaceID, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	if err = goose.DownToContext(ctx, sqlDB, ".", 117); err != nil {
		t.Fatal(err)
	}
	var space, chat, cv, page, head uuid.UUID
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	check(pool.QueryRow(ctx, `INSERT INTO chat_spaces(workspace_id,user_id,name,color,app)VALUES($1,$2,'Docs','blue','docs')RETURNING id`, user.WorkspaceID, user.ID).Scan(&space))
	check(pool.QueryRow(ctx, `INSERT INTO chat_sessions(workspace_id,title,created_by,agent_id,harness,model,effort,space_id)VALUES($1,'Promoted',$2,$3,'codex','test','high',$4)RETURNING id`, user.WorkspaceID, user.ID, agent.ID, space).Scan(&chat))
	check(pool.QueryRow(ctx, `INSERT INTO chat_canvases(session_id,kind,title)VALUES($1,'text','Promoted')RETURNING id`, chat).Scan(&cv))
	_, err = pool.Exec(ctx, `INSERT INTO chat_canvas_versions(canvas_id,seq,body,author_type,author_id)VALUES($1,1,'Doc diverged later','user',$2)`, cv, user.ID)
	check(err)
	check(pool.QueryRow(ctx, `INSERT INTO artifacts(workspace_id,type,title,path,author_type,author_id)VALUES($1,'note','Promoted','Promoted','user',$2)RETURNING id`, user.WorkspaceID, user.ID).Scan(&page))
	check(pool.QueryRow(ctx, `INSERT INTO artifact_versions(artifact_id,seq,label,body_md,content_type,size_bytes,note,created_by_type,created_by_id,state)VALUES($1,1,'v1',$2,'text/markdown',100,'Promoted from the Doc “Promoted”','user',$3,'published')RETURNING id`, page, "Wiki text\n\n---\n\n_Promoted from the Doc “Promoted” on 2026-11-12._\n", user.ID).Scan(&head))
	_, err = pool.Exec(ctx, `UPDATE artifacts SET current_version_id=$2 WHERE id=$1`, page, head)
	check(err)
	check(db.Migrate(ctx, pool))
	var mapped uuid.UUID
	var merged bool
	check(pool.QueryRow(ctx, `SELECT artifact_id,merged FROM doc_page_redirects WHERE canvas_id=$1`, cv).Scan(&mapped, &merged))
	if mapped != page || !merged {
		t.Fatalf("not merged: %s %v", mapped, merged)
	}
	var current string
	check(pool.QueryRow(ctx, `SELECT v.body_md FROM artifacts a JOIN artifact_versions v ON v.id=a.current_version_id WHERE a.id=$1`, page).Scan(&current))
	if current != "Wiki text\n" {
		t.Fatalf("footer cleanup/current text: %q", current)
	}
	var proposed string
	check(pool.QueryRow(ctx, `SELECT body_md FROM artifact_versions WHERE artifact_id=$1 AND state='suggested'`, page).Scan(&proposed))
	if proposed != "Doc diverged later" {
		t.Fatal("lost divergent Doc")
	}
	var source string
	check(pool.QueryRow(ctx, `SELECT body FROM chat_canvas_versions WHERE canvas_id=$1`, cv).Scan(&source))
	if source != proposed {
		t.Fatal("source was mutated")
	}
	check(goose.DownToContext(ctx, sqlDB, ".", 117))
	var restoredHead uuid.UUID
	check(pool.QueryRow(ctx, `SELECT current_version_id FROM artifacts WHERE id=$1`, page).Scan(&restoredHead))
	if restoredHead != head {
		t.Fatal("original wiki head not restored")
	}
	check(db.Migrate(ctx, pool))
}
