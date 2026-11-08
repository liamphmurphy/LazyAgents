package db_test

import (
	"context"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

// TestPersonaWebMigration: 00042 turns web access on for existing personas
// except those that can code (their chats keep having no web), and rolls back.
func TestPersonaWebMigration(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	gooseTo(t, pool, false, 41)
	var ws, user string
	must(pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id`).Scan(&ws))
	must(pool.QueryRow(ctx, `INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'Ann','ann@example.com','A','x') RETURNING id`, ws).Scan(&user))
	mustExec(t, pool, `INSERT INTO personas(workspace_id,user_id,name,is_default,can_code) VALUES($1,$2,'Plain',true,false)`, ws, user)
	mustExec(t, pool, `INSERT INTO personas(workspace_id,user_id,name,can_code) VALUES($1,$2,'Coder',true)`, ws, user)

	gooseTo(t, pool, true, 42)
	for name, want := range map[string]bool{"Plain": true, "Coder": false} {
		var web bool
		must(pool.QueryRow(ctx, `SELECT can_use_web FROM personas WHERE name=$1`, name).Scan(&web))
		if web != want {
			t.Errorf("%s: can_use_web %v, want %v", name, web, want)
		}
	}
	mustExec(t, pool, `INSERT INTO personas(workspace_id,user_id,name) VALUES($1,$2,'New')`, ws, user)
	var web bool
	must(pool.QueryRow(ctx, `SELECT can_use_web FROM personas WHERE name='New'`).Scan(&web))
	if !web {
		t.Error("new personas must default to web on")
	}

	gooseTo(t, pool, false, 41)
	var exists bool
	must(pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='personas' AND column_name='can_use_web')`).Scan(&exists))
	if exists {
		t.Fatal("column survived the down migration")
	}
	gooseTo(t, pool, true, 42)
}
