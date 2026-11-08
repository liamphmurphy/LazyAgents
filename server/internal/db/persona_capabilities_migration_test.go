package db_test

import (
	"context"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

// TestPersonaCapabilitiesMigration: 00038 adds capability columns that default
// off, keeps repo_url and work_branch together, bounds the repo list, marks
// attachments' source, and rolls back cleanly.
func TestPersonaCapabilitiesMigration(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	gooseTo(t, pool, false, 37)
	var ws, user, agent, project, chat, msg string
	must(pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id`).Scan(&ws))
	must(pool.QueryRow(ctx, `INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'Ann','ann@example.com','A','x') RETURNING id`, ws).Scan(&user))
	mustExec(t, pool, `INSERT INTO personas(workspace_id,user_id,name,is_default) VALUES($1,$2,'Assistant',true)`, ws, user)
	must(pool.QueryRow(ctx, `INSERT INTO projects(workspace_id,key,name,color) VALUES($1,'PRJ','P','#000') RETURNING id`, ws).Scan(&project))
	must(pool.QueryRow(ctx, `INSERT INTO agents(workspace_id,name,handle) VALUES($1,'A','atlas') RETURNING id`, ws).Scan(&agent))
	must(pool.QueryRow(ctx, `INSERT INTO chat_sessions(workspace_id,project_id,created_by,agent_id,harness,model) VALUES($1,$2,$3,$4,'codex','m') RETURNING id`, ws, project, user, agent).Scan(&chat))
	must(pool.QueryRow(ctx, `INSERT INTO chat_messages(session_id,role,body_md,seq) VALUES($1,'assistant','hi',1) RETURNING id`, chat).Scan(&msg))
	var att string
	must(pool.QueryRow(ctx, `INSERT INTO chat_attachments(workspace_id,user_id,filename,content_type,size_bytes,blob_key) VALUES($1,$2,'a.png','image/png',1,'k') RETURNING id`, ws, user).Scan(&att))

	gooseTo(t, pool, true, 38)

	var code, images bool
	var repos int
	must(pool.QueryRow(ctx, `SELECT can_code, can_generate_images, cardinality(repos) FROM personas WHERE user_id=$1`, user).Scan(&code, &images, &repos))
	if code || images || repos != 0 {
		t.Fatalf("existing personas got capabilities: %v %v %d", code, images, repos)
	}
	var source string
	var repo *string
	must(pool.QueryRow(ctx, `SELECT source FROM chat_attachments WHERE id=$1`, att).Scan(&source))
	must(pool.QueryRow(ctx, `SELECT repo_url FROM chat_sessions WHERE id=$1`, chat).Scan(&repo))
	if source != "upload" || repo != nil {
		t.Fatalf("existing rows: source %q repo %v", source, repo)
	}

	for name, sql := range map[string]string{
		"repo without branch": `UPDATE chat_sessions SET repo_url='https://github.com/a/b' WHERE id=$1`,
		"branch without repo": `UPDATE chat_sessions SET work_branch='vesper/x-1' WHERE id=$1`,
		"unknown source":      `UPDATE chat_attachments SET source='other' WHERE id=$2`,
		"eleven repos":        `UPDATE personas SET repos=ARRAY['a','b','c','d','e','f','g','h','i','j','k'] WHERE user_id=$3`,
	} {
		if _, err := pool.Exec(ctx, sql, chat, att, user); err == nil {
			t.Errorf("%s: succeeded, want a constraint violation", name)
		}
	}
	mustExec(t, pool, `UPDATE chat_sessions SET repo_url='https://github.com/a/b', work_branch='vesper/x-1' WHERE id=$1`, chat)
	mustExec(t, pool, `UPDATE chat_attachments SET source='generated', message_id=$2 WHERE id=$1`, att, msg)
	mustExec(t, pool, `UPDATE personas SET can_code=true, repos=ARRAY['a','b','c','d','e','f','g','h','i','j'] WHERE user_id=$1`, user)

	gooseTo(t, pool, false, 37)
	var exists bool
	must(pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE (table_name,column_name) IN (('personas','can_code'),('personas','repos'),('chat_sessions','repo_url'),('chat_attachments','source')))`).Scan(&exists))
	if exists {
		t.Fatal("columns survived the down migration")
	}
	gooseTo(t, pool, true, 38)
}
