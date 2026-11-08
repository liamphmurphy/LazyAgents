package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

// constraintFixture is one workspace with a person, a project, an agent and a chat.
type constraintFixture struct {
	pool                           *pgxpool.Pool
	ws, user, project, agent, chat string
}

func newConstraintFixture(t *testing.T) constraintFixture {
	t.Helper()
	ctx := context.Background()
	f := constraintFixture{pool: testdb.New(t)}
	q := func(sql string, dst *string, args ...any) {
		t.Helper()
		if err := f.pool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	q(`INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id`, &f.ws)
	q(`INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'Ann','ann@example.com','A','x') RETURNING id`, &f.user, f.ws)
	q(`INSERT INTO projects(workspace_id,key,name,color) VALUES($1,'PRJ','P','#000') RETURNING id`, &f.project, f.ws)
	q(`INSERT INTO agents(workspace_id,name,handle) VALUES($1,'A','atlas') RETURNING id`, &f.agent, f.ws)
	q(`INSERT INTO chat_sessions(workspace_id,project_id,created_by,agent_id,harness,model) VALUES($1,$2,$3,$4,'codex','m') RETURNING id`, &f.chat, f.ws, f.project, f.user, f.agent)
	return f
}

// rejects fails the test for every statement the schema accepts.
func rejects(t *testing.T, pool *pgxpool.Pool, cases map[string]string, args ...any) {
	t.Helper()
	for name, sql := range cases {
		if _, err := pool.Exec(context.Background(), sql, args...); err == nil {
			t.Errorf("%s: succeeded, want a constraint violation", name)
		}
	}
}

func TestUserAvatarConstraints(t *testing.T) {
	f := newConstraintFixture(t)
	rejects(t, f.pool, map[string]string{
		"key without type": `UPDATE users SET avatar_key='k' WHERE id=$1`,
		"type without key": `UPDATE users SET avatar_content_type='image/png' WHERE id=$1`,
	}, f.user)
	mustExec(t, f.pool, `UPDATE users SET avatar_key='k', avatar_content_type='image/png' WHERE id=$1`, f.user)
}

func TestPersonaConstraints(t *testing.T) {
	f := newConstraintFixture(t)
	ctx := context.Background()
	var persona string
	if err := f.pool.QueryRow(ctx, `INSERT INTO personas(workspace_id,user_id,name,is_default) VALUES($1,$2,'Assistant',true) RETURNING id`, f.ws, f.user).Scan(&persona); err != nil {
		t.Fatal(err)
	}

	// One default per person, case-insensitive unique names, and the column checks.
	rejects(t, f.pool, map[string]string{
		"second default":      `INSERT INTO personas(workspace_id,user_id,name,is_default) VALUES($1,$2,'Work',true)`,
		"duplicate name":      `INSERT INTO personas(workspace_id,user_id,name) VALUES($1,$2,' assistant ')`,
		"unknown tone":        `INSERT INTO personas(workspace_id,user_id,name,tone) VALUES($1,$2,'T','angry')`,
		"upper-case colour":   `INSERT INTO personas(workspace_id,user_id,name,color) VALUES($1,$2,'C','#ABCDEF')`,
		"blank name":          `INSERT INTO personas(workspace_id,user_id,name) VALUES($1,$2,'   ')`,
		"avatar without type": `INSERT INTO personas(workspace_id,user_id,name,avatar_key) VALUES($1,$2,'V','k')`,
		"eleven repos":        `INSERT INTO personas(workspace_id,user_id,name,repos) VALUES($1,$2,'R',ARRAY['a','b','c','d','e','f','g','h','i','j','k'])`,
	}, f.ws, f.user)
	mustExec(t, f.pool, `INSERT INTO personas(workspace_id,user_id,name,repos) VALUES($1,$2,'Work',ARRAY['a','b','c','d','e','f','g','h','i','j'])`, f.ws, f.user)

	// Capabilities default off.
	var code, images bool
	if err := f.pool.QueryRow(ctx, `SELECT can_code, can_generate_images FROM personas WHERE id=$1`, persona).Scan(&code, &images); err != nil {
		t.Fatal(err)
	}
	if code || images {
		t.Fatalf("new persona has capabilities: code %v images %v", code, images)
	}

	// Deleting a persona detaches its chats instead of deleting them.
	mustExec(t, f.pool, `UPDATE chat_sessions SET persona_id=$1 WHERE id=$2`, persona, f.chat)
	mustExec(t, f.pool, `DELETE FROM personas WHERE id=$1`, persona)
	var p *string
	if err := f.pool.QueryRow(ctx, `SELECT persona_id::text FROM chat_sessions WHERE id=$1`, f.chat).Scan(&p); err != nil {
		t.Fatal(err)
	}
	if p != nil {
		t.Fatalf("chat still points at a deleted persona: %q", *p)
	}
}

func TestChatCodeAndAttachmentConstraints(t *testing.T) {
	f := newConstraintFixture(t)
	ctx := context.Background()
	var msg, att, source string
	if err := f.pool.QueryRow(ctx, `INSERT INTO chat_messages(session_id,role,body_md,seq) VALUES($1,'assistant','hi',1) RETURNING id`, f.chat).Scan(&msg); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO chat_attachments(workspace_id,user_id,filename,content_type,size_bytes,blob_key) VALUES($1,$2,'a.png','image/png',1,'k') RETURNING id, source`, f.ws, f.user).Scan(&att, &source); err != nil {
		t.Fatal(err)
	}
	if source != "upload" {
		t.Fatalf("attachment source defaults to %q, want upload", source)
	}
	rejects(t, f.pool, map[string]string{
		"repo without branch": `UPDATE chat_sessions SET repo_url='https://github.com/a/b' WHERE id=$1 AND $2::uuid IS NOT NULL`,
		"branch without repo": `UPDATE chat_sessions SET work_branch='vesper/x-1' WHERE id=$1 AND $2::uuid IS NOT NULL`,
		"unknown source":      `UPDATE chat_attachments SET source='other' WHERE id=$2 AND $1::uuid IS NOT NULL`,
	}, f.chat, att)
	mustExec(t, f.pool, `UPDATE chat_sessions SET repo_url='https://github.com/a/b', work_branch='vesper/x-1' WHERE id=$1`, f.chat)
	mustExec(t, f.pool, `UPDATE chat_attachments SET source='generated', message_id=$2 WHERE id=$1`, att, msg)
}
