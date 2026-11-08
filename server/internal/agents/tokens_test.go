package agents_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
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
	if !strings.HasPrefix(tok, agents.TokenPrefix) || len(tok) < 40 || !strings.HasPrefix(tok, row.Prefix) {
		t.Fatalf("token %q prefix %q", tok, row.Prefix)
	}
	c, err := agents.ResolveToken(ctx, q, tok)
	if err != nil || c.Agent.Handle != "atlas" || c.Kind != "unmanaged" {
		t.Fatalf("resolve: %+v %v", c, err)
	}
	for _, bad := range []string{"", "vsp_nope", "la_nope", tok + "x", strings.TrimPrefix(tok, agents.TokenPrefix), "la_" + strings.TrimPrefix(tok, agents.TokenPrefix)} {
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

// Tokens minted before the rename (la_…) keep working; garbage with either
// prefix still fails.
func TestLegacyTokenPrefix(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	atlas, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})

	legacy := agents.LegacyTokenPrefix + "Zm9vYmFyYmF6cXV4Zm9vYmFyYmF6cXV4Zm9vYmFy"
	h := sha256.Sum256([]byte(legacy))
	if _, err := q.CreateAgentToken(ctx, store.CreateAgentTokenParams{
		AgentID: atlas.ID, TokenHash: h[:], Prefix: legacy[:7], Kind: "unmanaged", Label: "old laptop", CreatedByUserID: &me.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if c, err := agents.ResolveToken(ctx, q, legacy); err != nil || c.Agent.Handle != "atlas" {
		t.Fatalf("legacy token: %+v %v", c, err)
	}
	if _, err := agents.ResolveToken(ctx, q, agents.TokenPrefix+legacy[len(agents.LegacyTokenPrefix):]); !errors.Is(err, agents.ErrBadToken) {
		t.Fatalf("re-prefixed legacy token accepted: %v", err)
	}
}
