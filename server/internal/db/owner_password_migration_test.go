package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

// 00102 opens first-run setup only on installs no browser has signed in to:
// an install already in use must not become claimable by whoever arrives next.
func TestOwnerPasswordMigrationClaimsInstallsInUse(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	sqldb := stdlib.OpenDBFromPool(pool)
	defer sqldb.Close()
	if err := goose.DownToContext(ctx, sqldb, ".", 101); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		WITH ws AS (INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id)
		INSERT INTO users(workspace_id,name,email,initials,password_hash) SELECT id,'U','u@example.com','U','x' FROM ws`); err != nil {
		t.Fatal(err)
	}
	claimed := func() bool {
		t.Helper()
		var c bool
		if err := pool.QueryRow(ctx, `SELECT claimed_at IS NOT NULL FROM users`).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c
	}

	if err := goose.UpToContext(ctx, sqldb, ".", 102); err != nil {
		t.Fatal(err)
	}
	if claimed() {
		t.Error("an install no browser signed in to was marked claimed; first-run setup would never show")
	}

	if err := goose.DownToContext(ctx, sqldb, ".", 101); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO browser_sessions(token_hash, expires_at) VALUES ('\x01', now() - interval '1 day')`); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, sqldb, ".", 102); err != nil {
		t.Fatal(err)
	}
	if !claimed() {
		t.Error("an install a browser had signed in to (even with the session since expired) was left open to first-run setup")
	}
}
