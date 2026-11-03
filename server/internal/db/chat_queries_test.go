package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

func TestChatQueries(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	var ws, user, agent, project uuid.UUID
	for sql, dst := range map[string]*uuid.UUID{
		`INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id`: &ws,
	} {
		if err := pool.QueryRow(ctx, sql).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []struct {
		sql string
		dst *uuid.UUID
	}{
		{`INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'U','u@example.com','U','x') RETURNING id`, &user},
		{`INSERT INTO projects(workspace_id,key,name,color) VALUES($1,'PRJ','P','#000') RETURNING id`, &project},
		{`INSERT INTO agents(workspace_id,name,handle) VALUES($1,'A','atlas') RETURNING id`, &agent},
	} {
		if err := pool.QueryRow(ctx, s.sql, ws).Scan(s.dst); err != nil {
			t.Fatal(err)
		}
	}
	mk := func(title string) store.ChatSession {
		c, err := q.CreateChatSession(ctx, store.CreateChatSessionParams{WorkspaceID: ws, ProjectID: project, Title: title, CreatedBy: user, AgentID: agent, Harness: "codex", Model: "m"})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	a, b := mk("a"), mk("b")
	if a.State != "idle" || a.Revision != 1 || a.LastMessageAt != nil {
		t.Fatalf("defaults: %+v", a)
	}

	var seqs []int64
	for _, role := range []string{"user", "assistant", "user"} {
		m, err := q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: a.ID, Role: role, BodyMd: "x", ContextRefs: []byte("[]"), Citations: []byte("[]")})
		if err != nil {
			t.Fatal(err)
		}
		seqs = append(seqs, m.Seq)
	}
	if seqs[0] != 1 || seqs[1] != 2 || seqs[2] != 3 {
		t.Fatalf("seqs = %v", seqs)
	}
	got, err := q.GetChatSession(ctx, store.GetChatSessionParams{ID: a.ID, WorkspaceID: ws})
	if err != nil || got.LastMessageAt == nil {
		t.Fatalf("last_message_at not bumped: %+v %v", got, err)
	}

	// a has activity, so it lists before b even though b was created later.
	list, err := q.ListChatSessions(ctx, store.ListChatSessionsParams{WorkspaceID: ws, ProjectID: project, CreatedBy: user, Limit: 10})
	if err != nil || len(list) != 2 || list[0].ID != a.ID {
		t.Fatalf("list = %+v %v", list, err)
	}

	msgs, err := q.ListChatMessages(ctx, store.ListChatMessagesParams{SessionID: a.ID, Seq: 1, Limit: 10})
	if err != nil || len(msgs) != 2 || msgs[0].Seq != 2 {
		t.Fatalf("messages after 1 = %+v %v", msgs, err)
	}
	if msgs, _ = q.ListChatMessages(ctx, store.ListChatMessagesParams{SessionID: a.ID, Seq: 0, Limit: 2}); len(msgs) != 2 {
		t.Fatalf("limit not applied: %d", len(msgs))
	}

	// Revision-checked rename and archive.
	r, err := q.RenameChatSession(ctx, store.RenameChatSessionParams{ID: b.ID, WorkspaceID: ws, Title: "renamed", Revision: 1})
	if err != nil || r.Title != "renamed" || r.Revision != 2 {
		t.Fatalf("rename = %+v %v", r, err)
	}
	if _, err = q.RenameChatSession(ctx, store.RenameChatSessionParams{ID: b.ID, WorkspaceID: ws, Title: "stale", Revision: 1}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale rename err = %v, want ErrNoRows", err)
	}
	if _, err = q.ArchiveChatSession(ctx, store.ArchiveChatSessionParams{ID: b.ID, WorkspaceID: ws, Revision: 1}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale archive err = %v, want ErrNoRows", err)
	}
	if _, err = q.ArchiveChatSession(ctx, store.ArchiveChatSessionParams{ID: b.ID, WorkspaceID: ws, Revision: 2}); err != nil {
		t.Fatal(err)
	}
	if list, _ = q.ListChatSessions(ctx, store.ListChatSessionsParams{WorkspaceID: ws, ProjectID: project, CreatedBy: user, Limit: 10}); len(list) != 1 {
		t.Fatalf("archived session still listed: %d", len(list))
	}
}
