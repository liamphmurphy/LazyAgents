package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

// Rolling back 00068 keeps project-less chats by moving them to an active project.
func TestChatOptionalProjectRollbackKeepsChats(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	var ws, user, agent, archived, active, chat uuid.UUID
	for _, s := range []struct {
		sql string
		dst *uuid.UUID
	}{
		{`INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id`, &ws},
	} {
		if err := pool.QueryRow(ctx, s.sql).Scan(s.dst); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []struct {
		sql string
		dst *uuid.UUID
	}{
		{`INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'U','u@example.com','U','x') RETURNING id`, &user},
		{`INSERT INTO agents(workspace_id,name,handle) VALUES($1,'A','atlas') RETURNING id`, &agent},
		{`INSERT INTO projects(workspace_id,key,name,color,archived) VALUES($1,'OLD','Old','#000',true) RETURNING id`, &archived},
		{`INSERT INTO projects(workspace_id,key,name,color) VALUES($1,'NEW','New','#000') RETURNING id`, &active},
	} {
		if err := pool.QueryRow(ctx, s.sql, ws).Scan(s.dst); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO chat_sessions(workspace_id,title,created_by,agent_id,harness,model) VALUES($1,'Kept',$2,$3,'codex','m') RETURNING id`, ws, user, agent).Scan(&chat); err != nil {
		t.Fatal(err)
	}

	sqldb := stdlib.OpenDBFromPool(pool)
	defer sqldb.Close()
	if err := goose.DownToContext(ctx, sqldb, ".", 67); err != nil {
		t.Fatal(err)
	}

	var got uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT project_id FROM chat_sessions WHERE id = $1`, chat).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != active {
		t.Fatalf("project = %v, want the active project %v", got, active)
	}
}
