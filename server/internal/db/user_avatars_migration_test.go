package db_test

import (
	"context"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

// TestUserAvatarsMigration: 00040 adds nullable avatar columns that must be set
// together, and rolls back cleanly.
func TestUserAvatarsMigration(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	gooseTo(t, pool, false, 39)
	var ws, user string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'Ann','ann@example.com','A','x') RETURNING id`, ws).Scan(&user); err != nil {
		t.Fatal(err)
	}
	gooseTo(t, pool, true, 40)

	var key *string
	if err := pool.QueryRow(ctx, `SELECT avatar_key FROM users WHERE id=$1`, user).Scan(&key); err != nil || key != nil {
		t.Fatalf("existing user avatar_key %v, %v", key, err)
	}
	for name, sql := range map[string]string{
		"key without type": `UPDATE users SET avatar_key='k' WHERE id=$1`,
		"type without key": `UPDATE users SET avatar_content_type='image/png' WHERE id=$1`,
	} {
		if _, err := pool.Exec(ctx, sql, user); err == nil {
			t.Errorf("%s: succeeded, want a constraint violation", name)
		}
	}
	mustExec(t, pool, `UPDATE users SET avatar_key='k', avatar_content_type='image/png' WHERE id=$1`, user)

	gooseTo(t, pool, false, 39)
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name='users' AND column_name LIKE 'avatar_%' AND column_name<>'avatar_color'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("columns survived the down migration: %d, %v", n, err)
	}
	gooseTo(t, pool, true, 40)
}
