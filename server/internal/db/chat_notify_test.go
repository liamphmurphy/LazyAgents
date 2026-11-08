package db_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
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
