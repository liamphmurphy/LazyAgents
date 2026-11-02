package broker

import (
	"context"
	"os"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

func TestSQLMigration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool := testdb.New(t) // a throwaway database stands in for staging
	dsn := pool.Config().ConnString()
	ctx := context.Background()
	m := SQLMigration{}

	if err := m.Validate(map[string]any{}); err == nil {
		t.Fatal("empty sql accepted")
	}
	r := m.Run(ctx, dsn, map[string]any{"sql": "CREATE TABLE ledger (id int); INSERT INTO ledger VALUES (1), (2);"})
	if !r.OK {
		t.Fatalf("migration failed: %+v", r)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM ledger").Scan(&n); err != nil || n != 2 {
		t.Fatalf("rows = %d %v", n, err)
	}
	// A failing statement rolls back the whole migration.
	r = m.Run(ctx, dsn, map[string]any{"sql": "INSERT INTO ledger VALUES (3); SELECT nope FROM missing;"})
	if r.OK || r.Error == "" {
		t.Fatalf("expected failure: %+v", r)
	}
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM ledger").Scan(&n)
	if n != 2 {
		t.Fatalf("partial migration committed: %d rows", n)
	}
	if r := m.Run(ctx, "postgres://nobody@127.0.0.1:1/x?connect_timeout=1", map[string]any{"sql": "SELECT 1"}); r.OK {
		t.Fatal("bad DSN succeeded")
	}
}
