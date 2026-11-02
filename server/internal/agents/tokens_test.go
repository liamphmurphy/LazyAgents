package agents_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

func TestTokens(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	atlas, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})

	tok, row, err := agents.IssueToken(ctx, q, atlas.ID, "unmanaged", "Maya's laptop", nil, &me.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "la_") || len(tok) < 40 || !strings.HasPrefix(tok, row.Prefix) {
		t.Fatalf("token %q prefix %q", tok, row.Prefix)
	}
	c, err := agents.ResolveToken(ctx, q, tok)
	if err != nil || c.Agent.Handle != "atlas" || c.Kind != "unmanaged" {
		t.Fatalf("resolve: %+v %v", c, err)
	}
	for _, bad := range []string{"", "la_nope", tok + "x", strings.TrimPrefix(tok, "la_")} {
		if _, err := agents.ResolveToken(ctx, q, bad); !errors.Is(err, agents.ErrBadToken) {
			t.Errorf("token %q: %v", bad, err)
		}
	}
	past := time.Now().Add(-time.Minute)
	expired, _, _ := agents.IssueToken(ctx, q, atlas.ID, "run", "", nil, nil, &past)
	if _, err := agents.ResolveToken(ctx, q, expired); !errors.Is(err, agents.ErrBadToken) {
		t.Fatalf("expired token accepted: %v", err)
	}
	if err := agents.RevokeToken(ctx, q, atlas.ID, row.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := agents.ResolveToken(ctx, q, tok); !errors.Is(err, agents.ErrBadToken) {
		t.Fatal("revoked token still works")
	}
	if err := agents.RevokeToken(ctx, q, atlas.ID, row.ID); err == nil {
		t.Fatal("double revoke should be not found")
	}
}
