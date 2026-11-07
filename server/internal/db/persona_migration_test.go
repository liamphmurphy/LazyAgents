package db_test

import (
	"context"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

// TestPersonaMigration: 00037 seeds one default "Assistant" per person, links
// existing chats to it, enforces one default per person, and rolls back cleanly.
func TestPersonaMigration(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	gooseTo(t, pool, false, 36)
	var ws, ann, bob, agent, project string
	must(pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id`).Scan(&ws))
	must(pool.QueryRow(ctx, `INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'Ann','ann@example.com','A','x') RETURNING id`, ws).Scan(&ann))
	must(pool.QueryRow(ctx, `INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'Bob','bob@example.com','B','x') RETURNING id`, ws).Scan(&bob))
	must(pool.QueryRow(ctx, `INSERT INTO projects(workspace_id,key,name,color) VALUES($1,'PRJ','P','#000') RETURNING id`, ws).Scan(&project))
	must(pool.QueryRow(ctx, `INSERT INTO agents(workspace_id,name,handle) VALUES($1,'A','atlas') RETURNING id`, ws).Scan(&agent))
	insChat := `INSERT INTO chat_sessions(workspace_id,project_id,created_by,agent_id,harness,model) VALUES($1,$2,$3,$4,'codex','m') RETURNING id`
	var annChat1, annChat2, bobChat string
	must(pool.QueryRow(ctx, insChat, ws, project, ann, agent).Scan(&annChat1))
	must(pool.QueryRow(ctx, insChat, ws, project, ann, agent).Scan(&annChat2))
	must(pool.QueryRow(ctx, insChat, ws, project, bob, agent).Scan(&bobChat))

	gooseTo(t, pool, true, 37)

	personaOf := func(user string) (id, name, tone, color string) {
		t.Helper()
		var n int
		must(pool.QueryRow(ctx, `SELECT count(*) FROM personas WHERE user_id=$1`, user).Scan(&n))
		if n != 1 {
			t.Fatalf("user %s has %d personas after the migration, want 1", user, n)
		}
		must(pool.QueryRow(ctx, `SELECT id, name, tone, color FROM personas WHERE user_id=$1 AND workspace_id=$2 AND is_default`, user, ws).Scan(&id, &name, &tone, &color))
		return
	}
	annP, name, tone, color := personaOf(ann)
	if name != "Assistant" || tone != "neutral" || color != "#64748b" {
		t.Fatalf("seeded persona = %q %q %q", name, tone, color)
	}
	bobP, _, _, _ := personaOf(bob)
	chatPersona := func(chat string) string {
		var p *string
		must(pool.QueryRow(ctx, `SELECT persona_id::text FROM chat_sessions WHERE id=$1`, chat).Scan(&p))
		if p == nil {
			return ""
		}
		return *p
	}
	for chat, want := range map[string]string{annChat1: annP, annChat2: annP, bobChat: bobP} {
		if got := chatPersona(chat); got != want {
			t.Errorf("chat %s persona = %q, want %q", chat, got, want)
		}
	}

	// One default per person, case-insensitive unique names, and the column checks.
	for name, sql := range map[string]string{
		"second default":      `INSERT INTO personas(workspace_id,user_id,name,is_default) VALUES($1,$2,'Work',true)`,
		"duplicate name":      `INSERT INTO personas(workspace_id,user_id,name) VALUES($1,$2,' assistant ')`,
		"unknown tone":        `INSERT INTO personas(workspace_id,user_id,name,tone) VALUES($1,$2,'T','angry')`,
		"upper-case colour":   `INSERT INTO personas(workspace_id,user_id,name,color) VALUES($1,$2,'C','#ABCDEF')`,
		"blank name":          `INSERT INTO personas(workspace_id,user_id,name) VALUES($1,$2,'   ')`,
		"avatar without type": `INSERT INTO personas(workspace_id,user_id,name,avatar_key) VALUES($1,$2,'V','k')`,
	} {
		if _, err := pool.Exec(ctx, sql, ws, ann); err == nil {
			t.Errorf("%s: insert succeeded, want a constraint violation", name)
		}
	}
	mustExec(t, pool, `INSERT INTO personas(workspace_id,user_id,name) VALUES($1,$2,'Work')`, ws, ann)

	// Deleting a persona detaches its chats instead of deleting them.
	mustExec(t, pool, `DELETE FROM personas WHERE id=$1`, bobP)
	if got := chatPersona(bobChat); got != "" {
		t.Fatalf("bob's chat still points at a deleted persona: %q", got)
	}

	// Down drops the table and the column; chats survive. Up again re-seeds.
	gooseTo(t, pool, false, 36)
	var exists bool
	must(pool.QueryRow(ctx, `SELECT to_regclass('personas') IS NOT NULL`).Scan(&exists))
	if exists {
		t.Fatal("personas survived the down migration")
	}
	must(pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='chat_sessions' AND column_name='persona_id')`).Scan(&exists))
	if exists {
		t.Fatal("chat_sessions.persona_id survived the down migration")
	}
	var chats int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM chat_sessions`).Scan(&chats))
	if chats != 3 {
		t.Fatalf("%d chats after down, want 3", chats)
	}
	gooseTo(t, pool, true, 37)
	personaOf(ann)
	personaOf(bob)
}
