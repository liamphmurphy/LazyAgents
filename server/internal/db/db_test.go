package db_test

import (
	"context"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/db"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

func TestMigrateIsIdempotent(t *testing.T) {
	pool := testdb.New(t)
	if err := db.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestUUIDv7(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	var version, variant int
	var ordered bool
	err := pool.QueryRow(ctx, `
		WITH ids AS (SELECT uuid_v7() AS a, pg_sleep(0.002), uuid_v7() AS b)
		SELECT get_byte(uuid_send(a), 6) >> 4,
		       get_byte(uuid_send(a), 8) >> 6,
		       a < (SELECT uuid_v7())
		FROM ids`).Scan(&version, &variant, &ordered)
	if err != nil {
		t.Fatal(err)
	}
	if version != 7 {
		t.Errorf("version nibble = %d, want 7", version)
	}
	if variant != 2 {
		t.Errorf("variant bits = %b, want 10", variant)
	}
	if !ordered {
		t.Error("later uuid_v7() should sort after an earlier one")
	}
}
