package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/usage"
)

func TestBuildHarnessActivity(t *testing.T) {
	at := time.Date(2026, 11, 9, 18, 0, 0, 0, time.UTC)
	resets := at.Add(3 * time.Hour)
	running := []store.ActiveHarnessRunsRow{{Harness: "claude-code", Runs: 2}}
	limits := []store.ActiveProviderLimitsRow{{Harness: "codex", Until: resets}}
	readings := map[string]usage.Snapshot{
		"claude-code": {Harness: "claude-code", Plan: "max", Available: true, ObservedAt: at,
			Windows: []usage.Window{{ID: "five_hour", Kind: usage.Session, Label: "Session", UsedPercent: 38.44, ResetsAt: &resets}}},
		"codex": {Harness: "codex", Available: false, ObservedAt: at, Windows: []usage.Window{}},
	}
	got := buildHarnessActivity(map[string]bool{"claude-code": true}, running, limits, readings, map[string]bool{"codex": true})
	if len(got) != 3 || got[0].Harness != "claude-code" || got[1].Harness != "codex" || got[2].Harness != "antigravity" {
		t.Fatalf("order = %+v", got)
	}
	c := got[0]
	if c.Label != "Claude" || !c.Connected || c.Running != 2 || c.LimitedUntil != nil || !c.UsageAvailable || c.Plan == nil || *c.Plan != "max" || c.UsageChecking {
		t.Fatalf("claude = %+v", c)
	}
	if len(c.Windows) != 1 || c.Windows[0].UsedPercent != 38.4 || string(c.Windows[0].Kind) != "session" || !c.Windows[0].ResetsAt.Equal(resets) {
		t.Fatalf("claude windows = %+v", c.Windows)
	}
	x := got[1]
	// Antigravity is listed with no allowance to read: never "checking", never available.
	if a := got[2]; a.Label != "Antigravity" || a.UsageAvailable || a.UsageChecking || a.UsageCheckedAt != nil || len(a.Windows) != 0 {
		t.Fatalf("antigravity = %+v", a)
	}
	if x.Connected || x.Running != 0 || x.LimitedUntil == nil || !x.LimitedUntil.Equal(resets) || x.UsageAvailable || x.UsageCheckedAt == nil || len(x.Windows) != 0 || !x.UsageChecking {
		t.Fatalf("codex = %+v", x)
	}
}

func TestBuildHarnessActivityNoReadings(t *testing.T) {
	got := buildHarnessActivity(map[string]bool{}, nil, nil, map[string]usage.Snapshot{}, map[string]bool{})
	for _, a := range got {
		if a.UsageAvailable || a.UsageCheckedAt != nil || a.Windows == nil {
			t.Fatalf("%s = %+v", a.Harness, a)
		}
	}
}

func TestPersonaDockReportsConnectedHarnesses(t *testing.T) {
	e := newChatEnv(t)
	connected := func() map[string]any {
		out := map[string]any{}
		for _, h := range e.want("GET", "/persona-dock", nil, 200)["harnesses"].([]any) {
			m := h.(map[string]any)
			out[m["harness"].(string)] = m["connected"]
		}
		return out
	}
	// A runner that needs no keys can run every harness.
	if got := connected(); got["claude-code"] != true || got["codex"] != true || got["antigravity"] != true {
		t.Fatalf("without required keys = %v", got)
	}
	// With keys required, only a harness whose key secret exists is connected.
	e.runs.RequireHarnessKey = true
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO secrets (workspace_id, name, nonce, ciphertext) VALUES ($1, 'harness:anthropic', '\x00', '\x00')`, e.me.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if got := connected(); got["claude-code"] != true || got["codex"] != false || got["antigravity"] != false {
		t.Fatalf("with required keys = %v", got)
	}
}

func TestPersonaDockQueuesUsageChecksOnDocker(t *testing.T) {
	e := newChatEnvServer(t, nil, func(s *Server) { s.Usage = &usage.Service{Q: s.Q, Queued: true} })
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `INSERT INTO secrets (workspace_id, name, nonce, ciphertext) VALUES ($1, 'harness:anthropic', '\x00', '\x00')`, e.me.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	byHarness := func() map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, h := range e.want("GET", "/persona-dock", nil, 200)["harnesses"].([]any) {
			m := h.(map[string]any)
			out[m["harness"].(string)] = m
		}
		return out
	}
	// The poll queues Claude's check and shows it in flight.
	if c := byHarness()["claude-code"]; c["usage_checking"] != true {
		t.Fatalf("claude = %v", c)
	}
	// Codex has no login: by the next poll it reads as having no plan limits.
	if x := byHarness()["codex"]; x["usage_checking"] != false || x["usage_available"] != false || x["usage_checked_at"] == nil {
		t.Fatalf("codex = %v", x)
	}
}
