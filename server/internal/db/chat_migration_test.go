package db_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
	"github.com/liamphmurphy/lazyagents/server/migrations"
)

func gooseTo(t *testing.T, pool *pgxpool.Pool, up bool, version int64) {
	t.Helper()
	sqldb := stdlib.OpenDBFromPool(pool)
	defer sqldb.Close()
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	var err error
	if up {
		err = goose.UpToContext(context.Background(), sqldb, ".", version)
	} else {
		err = goose.DownToContext(context.Background(), sqldb, ".", version)
	}
	if err != nil {
		t.Fatalf("goose to %d: %v", version, err)
	}
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func TestChatMigration(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	// Go back to the schema before chat, seed old-style runs, then migrate forward.
	gooseTo(t, pool, false, 15)
	var ws, user, agent, project, ticket string
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id`).Scan(&ws))
	must(pool.QueryRow(ctx, `INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'U','u@example.com','U','x') RETURNING id`, ws).Scan(&user))
	must(pool.QueryRow(ctx, `INSERT INTO projects(workspace_id,key,name,color) VALUES($1,'PRJ','P','#000') RETURNING id`, ws).Scan(&project))
	must(pool.QueryRow(ctx, `INSERT INTO agents(workspace_id,name,handle) VALUES($1,'A','atlas') RETURNING id`, ws).Scan(&agent))
	must(pool.QueryRow(ctx, `INSERT INTO tickets(workspace_id,project_id,number,key,title) VALUES($1,$2,1,'PRJ-1','T') RETURNING id`, ws, project).Scan(&ticket))

	insRun := `INSERT INTO runs(workspace_id,ticket_id,agent_id,mode,state,resolved_spec,harness,model) VALUES($1,$2,$3,'managed','completed','{}','codex','m') RETURNING id`
	var ticketRun, planRun, orphanRun, childRun string
	must(pool.QueryRow(ctx, insRun, ws, ticket, agent).Scan(&ticketRun))
	must(pool.QueryRow(ctx, insRun, ws, nil, agent).Scan(&planRun))
	must(pool.QueryRow(ctx, insRun, ws, nil, agent).Scan(&orphanRun)) // ticket-less, no planning session
	must(pool.QueryRow(ctx, `INSERT INTO runs(workspace_id,ticket_id,agent_id,mode,state,resolved_spec,harness,model,parent_run_id) VALUES($1,$2,$3,'managed','completed','{}','codex','m',$4) RETURNING id`, ws, ticket, agent, ticketRun).Scan(&childRun))
	mustExec(t, pool, `INSERT INTO planning_sessions(workspace_id,project_id,goal,run_id,created_by) VALUES($1,$2,'g',$3,$4)`, ws, project, planRun, user)

	gooseTo(t, pool, true, 16)

	purpose := func(id string) string {
		var p string
		must(pool.QueryRow(ctx, `SELECT purpose FROM runs WHERE id=$1`, id).Scan(&p))
		return p
	}
	for name, c := range map[string]struct{ id, want string }{
		"ticket run":              {ticketRun, "ticket"},
		"delegated child":         {childRun, "ticket"},
		"planning run":            {planRun, "planning"},
		"ticket-less, no session": {orphanRun, "planning"},
	} {
		if got := purpose(c.id); got != c.want {
			t.Errorf("%s: purpose = %q, want %q", name, got, c.want)
		}
	}

	// purpose/ticket_id consistency and the allowed values.
	for name, args := range map[string][]any{
		"ticket purpose without ticket": {nil, "ticket"},
		"planning purpose with ticket":  {ticket, "planning"},
		"chat purpose with ticket":      {ticket, "chat"},
		"unknown purpose":               {nil, "other"},
	} {
		_, err := pool.Exec(ctx, `INSERT INTO runs(workspace_id,ticket_id,agent_id,mode,state,resolved_spec,harness,model,purpose) VALUES($1,$2,$3,'managed','completed','{}','codex','m',$4)`, ws, args[0], agent, args[1])
		if err == nil {
			t.Errorf("%s: insert succeeded, want constraint violation", name)
		}
	}
	var chatRun string
	must(pool.QueryRow(ctx, `INSERT INTO runs(workspace_id,agent_id,mode,state,resolved_spec,harness,model,purpose) VALUES($1,$2,'managed','waiting','{}','codex','m','chat') RETURNING id`, ws, agent).Scan(&chatRun))

	// Chat tables.
	var chat string
	must(pool.QueryRow(ctx, `INSERT INTO chat_sessions(workspace_id,project_id,created_by,agent_id,harness,model,current_run_id) VALUES($1,$2,$3,$4,'codex','m',$5) RETURNING id`, ws, project, user, agent, chatRun).Scan(&chat))
	addMsg := func(seq int, role string) error {
		_, err := pool.Exec(ctx, `INSERT INTO chat_messages(session_id,seq,role,body_md) VALUES($1,$2,$3,'hi')`, chat, seq, role)
		return err
	}
	if err := addMsg(1, "user"); err != nil {
		t.Fatal(err)
	}
	if err := addMsg(1, "assistant"); err == nil {
		t.Error("duplicate (session_id, seq) accepted")
	}
	if err := addMsg(2, "bot"); err == nil {
		t.Error("role 'bot' accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE chat_sessions SET state='stuck' WHERE id=$1`, chat); err == nil {
		t.Error("state 'stuck' accepted")
	}
	var refs, cites string
	must(pool.QueryRow(ctx, `SELECT context_refs::text, citations::text FROM chat_messages WHERE session_id=$1`, chat).Scan(&refs, &cites))
	if refs != "[]" || cites != "[]" {
		t.Errorf("jsonb defaults = %s, %s", refs, cites)
	}

	// Down removes the chat tables, the purpose column, and chat runs; other runs survive.
	gooseTo(t, pool, false, 15)
	var n int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name IN ('chat_sessions','chat_messages')`).Scan(&n))
	if n != 0 {
		t.Errorf("chat tables remain after down: %d", n)
	}
	must(pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name='runs' AND column_name='purpose'`).Scan(&n))
	if n != 0 {
		t.Error("runs.purpose remains after down")
	}
	must(pool.QueryRow(ctx, `SELECT count(*) FROM runs`).Scan(&n))
	if n != 4 {
		t.Errorf("runs after down = %d, want 4 (chat run removed)", n)
	}
	// And up again works.
	gooseTo(t, pool, true, 16)
}

func TestChatNotifyPayload(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	var ws, user, agent, project string
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id`).Scan(&ws))
	must(pool.QueryRow(ctx, `INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'U','u@example.com','U','x') RETURNING id`, ws).Scan(&user))
	must(pool.QueryRow(ctx, `INSERT INTO projects(workspace_id,key,name,color) VALUES($1,'PRJ','P','#000') RETURNING id`, ws).Scan(&project))
	must(pool.QueryRow(ctx, `INSERT INTO agents(workspace_id,name,handle) VALUES($1,'A','atlas') RETURNING id`, ws).Scan(&agent))

	conn, err := pool.Acquire(ctx)
	must(err)
	defer conn.Release()
	_, err = conn.Exec(ctx, `LISTEN lazyagents_changes`)
	must(err)

	expect := func(what, wantID string) {
		t.Helper()
		wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		for {
			n, err := conn.Conn().WaitForNotification(wctx)
			if err != nil {
				t.Fatalf("%s: no chat.updated notification: %v", what, err)
			}
			var p struct {
				Type string   `json:"type"`
				IDs  []string `json:"ids"`
			}
			must(json.Unmarshal([]byte(n.Payload), &p))
			if p.Type != "chat.updated" {
				continue // other tables' triggers
			}
			if len(p.IDs) != 1 || p.IDs[0] != wantID {
				t.Fatalf("%s: ids = %v, want [%s]", what, p.IDs, wantID)
			}
			return
		}
	}

	var chat string
	must(pool.QueryRow(ctx, `INSERT INTO chat_sessions(workspace_id,project_id,created_by,agent_id,harness,model) VALUES($1,$2,$3,$4,'codex','m') RETURNING id`, ws, project, user, agent).Scan(&chat))
	expect("session insert", chat)
	mustExec(t, pool, `INSERT INTO chat_messages(session_id,seq,role,body_md) VALUES($1,1,'user','hi'),($1,2,'assistant','yo')`, chat)
	expect("message insert (chat id, not message id)", chat)
	mustExec(t, pool, `UPDATE chat_sessions SET title='x' WHERE id=$1`, chat)
	expect("session update", chat)
	mustExec(t, pool, `DELETE FROM chat_sessions WHERE id=$1`, chat) // cascades to messages
	expect("session delete", chat)
}
