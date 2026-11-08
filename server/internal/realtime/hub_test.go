package realtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

func TestCommitNotifications(t *testing.T) {
	pool := testdb.New(t)
	// Ticket triggers send nothing for statements that touch no rows, so seed one.
	if _, err := pool.Exec(context.Background(), `WITH w AS (INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id),
		p AS (INSERT INTO projects(workspace_id,key,name,color) SELECT id,'PRJ','P','#000' FROM w RETURNING id, workspace_id)
		INSERT INTO tickets(workspace_id,project_id,number,key,title) SELECT workspace_id,id,1,'PRJ-1','T' FROM p`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := New()
	ch := make(chan string, 32)
	h.clients[ch] = struct{}{}
	done := make(chan struct{})
	go func() { defer close(done); h.Run(ctx, pool) }()
	defer func() { cancel(); <-done }()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("listener not ready")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "UPDATE tickets SET title=title"); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-ch:
		t.Fatalf("notification before commit: %s", msg)
	case <-time.After(80 * time.Millisecond):
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-ch:
		t.Fatalf("notification after rollback: %s", msg)
	case <-time.After(80 * time.Millisecond):
	}
	if _, err = pool.Exec(ctx, "UPDATE tickets SET title=title"); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-ch:
		if !strings.Contains(msg, "ticket.updated") {
			t.Fatal(msg)
		}
	case <-time.After(time.Second):
		t.Fatal("no committed notification")
	}
}

func TestSlowSubscriberDisconnects(t *testing.T) {
	h := New()
	ch := make(chan string, 1)
	h.clients[ch] = struct{}{}
	h.publish("one")
	h.publish("two")
	if _, ok := h.clients[ch]; ok {
		t.Fatal("slow subscriber retained")
	}
	if <-ch != "one" {
		t.Fatal("first message missing")
	}
	if _, ok := <-ch; ok {
		t.Fatal("slow subscriber must reconnect")
	}
}
