package db_test

import (
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Agents become personas with the agent's own id, so everything keyed on the agent (runs,
// tickets, approvals) needs no rewrite; the assistant and archived agents don't.
func TestPersonasDoTheWorkMigration(t *testing.T) {
	f := newConstraintFixture(t)
	ctx := t.Context()
	sqlDB := stdlib.OpenDBFromPool(f.pool)
	defer sqlDB.Close()
	if err := goose.DownToContext(ctx, sqlDB, ".", 132); err != nil {
		t.Fatal(err)
	}
	// A later user doesn't own them: the workspace's first person does.
	mustExec(t, f.pool, `INSERT INTO users(workspace_id,name,email,initials,password_hash,created_at) VALUES($1,'Bo','bo@example.com','B','x',now()+interval '1 day')`, f.ws)
	// An existing persona already uses the name Builder.
	mustExec(t, f.pool, `INSERT INTO personas(workspace_id,user_id,name) VALUES($1,$2,'builder')`, f.ws, f.user)
	var builder, reviewer, assistant, archived string
	for _, a := range []struct {
		dst                         *string
		name, handle, kind, archive string
		color                       int
	}{
		{&builder, "Builder", "builder", "main", "", 9},
		{&reviewer, "Code Reviewer", "code-reviewer", "sub", "", 2},
		{&assistant, "Assistant", "assistant", "assistant", "", 0},
		{&archived, "Old", "old", "main", "now()", 0},
	} {
		archivedAt := "NULL"
		if a.archive != "" {
			archivedAt = a.archive
		}
		if err := f.pool.QueryRow(ctx, `INSERT INTO agents(workspace_id,name,handle,kind,color_index,archived_at) VALUES($1,$2,$3,$4,$5,`+archivedAt+`) RETURNING id`,
			f.ws, a.name, a.handle, a.kind, a.color).Scan(a.dst); err != nil {
			t.Fatal(err)
		}
	}
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		t.Fatal(err)
	}

	type row struct{ name, color, owner string }
	got := map[string]row{}
	rows, err := f.pool.Query(ctx, `SELECT id::text,name,color,user_id::text FROM personas WHERE id IN (SELECT id FROM agents)`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		var r row
		if err := rows.Scan(&id, &r.name, &r.color, &r.owner); err != nil {
			t.Fatal(err)
		}
		got[id] = r
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	if len(got) != 3 { // atlas (the fixture's), builder, code-reviewer
		t.Fatalf("migrated personas = %v, want atlas, builder and code-reviewer", got)
	}
	if r := got[builder]; r.name != "Builder 2" || r.owner != f.user || r.color != "#0b7a6e" {
		t.Errorf("builder = %+v, want \"Builder 2\" owned by the first person in hue 9%%8 (#0b7a6e)", r)
	}
	if r := got[reviewer]; r.name != "Code Reviewer" || r.color != "#b42f48" {
		t.Errorf("code-reviewer = %+v", r)
	}
	for _, id := range []string{assistant, archived} {
		if _, ok := got[id]; ok {
			t.Errorf("agent %s became a persona; the assistant and archived agents stay work records only", id)
		}
	}
	var listed bool
	if err := f.pool.QueryRow(ctx, `SELECT in_chat_list FROM personas WHERE id=$1`, builder).Scan(&listed); err != nil || !listed {
		t.Fatalf("in_chat_list = %v, %v; migrated personas start in the chat list", listed, err)
	}

	// Down keeps the personas (they're ordinary chat personas to older code) and drops the flag.
	if err := goose.DownToContext(ctx, sqlDB, ".", 132); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM personas WHERE id=$1`, builder).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		t.Fatal(err)
	}
}
