package db_test

import (
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestKnowledgeChatVisibilityBackfill(t *testing.T) {
	f := newConstraintFixture(t)
	ctx := t.Context()
	sqlDB := stdlib.OpenDBFromPool(f.pool)
	defer sqlDB.Close()
	if err := goose.DownToContext(ctx, sqlDB, ".", 121); err != nil {
		t.Fatal(err)
	}
	general := "I'm in Knowledge, my notes and documents. Help me find, connect and write pages: search_knowledge before answering."
	page := "I'm working on the Knowledge page [[Notes/Reading list]]. Read it with read_page and help me with it."
	type sample struct {
		body   string
		hidden bool
		want   bool
		id     string
	}
	cases := []sample{
		{body: general, want: true},
		{body: page, want: true},
		{body: general, hidden: true, want: true},
		{body: "Help me plan the Knowledge app"},
	}
	for i := range cases {
		c := &cases[i]
		if err := f.pool.QueryRow(ctx, `INSERT INTO chat_sessions(workspace_id,created_by,agent_id,harness,model,title,hidden)
			VALUES($1,$2,$3,'codex','m','Renamed conversation',$4) RETURNING id`, f.ws, f.user, f.agent, c.hidden).Scan(&c.id); err != nil {
			t.Fatal(err)
		}
		mustExec(t, f.pool, `INSERT INTO chat_messages(session_id,seq,role,body_md) VALUES($1,1,'user',$2),($1,2,'assistant','Kept reply')`, c.id, c.body)
	}
	// Mentioning the starter later in an ordinary conversation doesn't move it.
	mustExec(t, f.pool, `INSERT INTO chat_messages(session_id,seq,role,body_md) VALUES($1,3,'user',$2)`, cases[3].id, general)
	check := func(migrated bool) {
		t.Helper()
		for _, c := range cases {
			var hidden bool
			var reply string
			if err := f.pool.QueryRow(ctx, `SELECT c.hidden,m.body_md FROM chat_sessions c JOIN chat_messages m ON m.session_id=c.id AND m.seq=2 WHERE c.id=$1`, c.id).Scan(&hidden, &reply); err != nil {
				t.Fatal(err)
			}
			want := c.hidden
			if migrated {
				want = c.want
			}
			if hidden != want || reply != "Kept reply" {
				t.Fatalf("chat %s: hidden=%v want=%v reply=%q", c.id, hidden, want, reply)
			}
		}
	}
	for range 2 {
		if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
			t.Fatal(err)
		}
		check(true)
		if err := goose.DownToContext(ctx, sqlDB, ".", 121); err != nil {
			t.Fatal(err)
		}
		check(false)
	}
}
