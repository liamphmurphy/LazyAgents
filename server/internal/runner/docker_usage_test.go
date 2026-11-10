package runner

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/usage"
)

func TestDockerProbeUsageClaudeUsesSandboxAndTokenEnv(t *testing.T) {
	f, d := startModelsEngine(t)
	snap, err := d.ProbeUsage(context.Background(), "claim1", agentdef.HarnessClaudeCode, "sk-ant-oat01-secret", "scope")
	if err != nil || !snap.Available || snap.Plan != "max" || len(snap.Windows) != 2 || snap.Windows[0].UsedPercent != 38 {
		t.Fatalf("snapshot %+v err %v", snap, err)
	}
	c := f.created
	if f.name != "vesper-usage-claim1" {
		t.Fatalf("name %q", f.name)
	}
	if labels := c["Labels"].(map[string]any); labels[labelKey] != "usage-claude-code" {
		t.Fatalf("labels %v", labels)
	}
	if env := c["Env"].([]any); len(env) != 1 || env[0] != ClaudeOAuthEnv+"=sk-ant-oat01-secret" {
		t.Fatalf("env %v", env)
	}
	host := c["HostConfig"].(map[string]any)
	if m, _ := host["Mounts"].([]any); len(m) != 0 {
		t.Fatalf("mounts %v", host["Mounts"])
	}
	// The check must not be able to act: no tools, MCP servers or hooks.
	cmd := []string{}
	for _, a := range c["Cmd"].([]any) {
		cmd = append(cmd, a.(string))
	}
	if cmd[0] != "claude" || !slices.Contains(cmd, "--tools") || !slices.Contains(cmd, "--strict-mcp-config") || !strings.Contains(strings.Join(cmd, " "), "disableAllHooks") {
		t.Fatalf("cmd %v", cmd)
	}
	if !f.hasCall("POST /containers/m1/kill") || !f.hasCall("DELETE /containers/m1") {
		t.Fatalf("container not cleaned up: %v", f.calls)
	}
}

func TestDockerProbeUsageCodexKeepsRefreshedLogin(t *testing.T) {
	f, d := startModelsEngine(t)
	f.authBack = codexLoginNew
	var saved string
	d.SaveCodexAuth = func(_ context.Context, _, auth string) error { saved = auth; return nil }
	snap, err := d.ProbeUsage(context.Background(), "claim1", agentdef.HarnessCodex, codexLoginOld, "scope")
	if err != nil || snap.Plan != "plus" || len(snap.Windows) != 2 || snap.Windows[0].Kind != usage.Session || snap.Windows[1].UsedPercent != 70 {
		t.Fatalf("snapshot %+v err %v", snap, err)
	}
	if got := f.archive[homePath+"|.codex/auth.json"]; got != codexLoginOld {
		t.Fatalf("uploaded %v", f.archive)
	}
	if saved != codexLoginNew {
		t.Fatalf("refreshed login not kept: %q", saved)
	}
}

func TestDockerProbeUsageSignedOutIsTheCLIsAnswer(t *testing.T) {
	f, d := startModelsEngine(t)
	f.signedOut = true
	if _, err := d.ProbeUsage(context.Background(), "claim1", agentdef.HarnessClaudeCode, "sk-ant-oat01-secret", "scope"); !errors.Is(err, usage.ErrSignedOut) {
		t.Fatalf("err %v, want ErrSignedOut", err)
	}
}

func TestDockerProbeUsageHungCLIStillCleansUp(t *testing.T) {
	f, d := startModelsEngine(t)
	f.hang = true
	d.UsageTimeout = 200 * time.Millisecond
	if _, err := d.ProbeUsage(context.Background(), "claim1", agentdef.HarnessCodex, codexLoginOld, "scope"); err == nil {
		t.Fatal("expected an error")
	}
	if !f.hasCall("POST /containers/m1/kill") || !f.hasCall("DELETE /containers/m1") {
		t.Fatalf("container not cleaned up: %v", f.calls)
	}
}

// fakeProber stands in for the Docker backend's usage checks.
type fakeProber struct {
	*Local
	snap  usage.Snapshot
	err   error
	mu    sync.Mutex
	calls []string // harness|login|scope
}

func (f *fakeProber) ProbeUsage(_ context.Context, _, harness, login, scope string) (usage.Snapshot, error) {
	f.mu.Lock()
	f.calls = append(f.calls, harness+"|"+login+"|"+scope)
	f.mu.Unlock()
	return f.snap, f.err
}

// check queues a usage check for the harness and lets the runner answer it.
func (e listingEnv) check(t *testing.T, harness string) (store.ProviderUsage, store.HarnessUsageCheck) {
	t.Helper()
	svc := &usage.Service{Q: e.q, Queued: true}
	svc.Refresh(e.ctx, e.ws, nil)
	e.r.tickUsageChecks(e.ctx)
	e.r.wg.Wait()
	reading, err := e.q.ProviderUsageFor(e.ctx, store.ProviderUsageForParams{WorkspaceID: e.ws, Harness: harness})
	if err != nil {
		t.Fatal(err)
	}
	var c store.HarnessUsageCheck
	row := e.r.Pool.QueryRow(e.ctx, `SELECT status, retry_after FROM harness_usage_checks WHERE workspace_id = $1 AND harness = $2`, e.ws, harness)
	if err := row.Scan(&c.Status, &c.RetryAfter); err != nil {
		t.Fatal(err)
	}
	return reading, c
}

func newUsageEnv(t *testing.T, p *fakeProber) listingEnv {
	t.Helper()
	e := newListingEnv(t, &fakeLister{})
	p.Local = e.f.Local
	e.r.Backend = p
	return e
}

func TestUsageCheckStoresReading(t *testing.T) {
	p := &fakeProber{snap: usage.Snapshot{Harness: agentdef.HarnessClaudeCode, Plan: "max", Available: true, ObservedAt: time.Now(),
		Windows: []usage.Window{{ID: "five_hour", Kind: usage.Session, Label: "Session", UsedPercent: 38}}}}
	e := newUsageEnv(t, p)
	e.set("harness:anthropic", "sk-ant-oat01-abcdefghijklmnopqrstuvwxyz")
	reading, c := e.check(t, agentdef.HarnessClaudeCode)
	var ws []usage.Window
	_ = json.Unmarshal(reading.Windows, &ws)
	if !reading.Available || reading.Plan != "max" || len(ws) != 1 || ws[0].UsedPercent != 38 {
		t.Fatalf("reading %+v windows %s", reading, reading.Windows)
	}
	if c.Status != "done" {
		t.Fatalf("check %+v", c)
	}
	if want := "claude-code|sk-ant-oat01-abcdefghijklmnopqrstuvwxyz|" + e.ws.String(); len(p.calls) != 1 || p.calls[0] != want {
		t.Fatalf("calls %v", p.calls)
	}
}

func TestUsageCheckAPIKeyNeverReachesTheCLI(t *testing.T) {
	p := &fakeProber{}
	e := newUsageEnv(t, p)
	e.set("harness:anthropic", "sk-ant-api03-abcdefghijklmnopqrstuvwxyz")
	reading, c := e.check(t, agentdef.HarnessClaudeCode)
	if reading.Available {
		t.Fatalf("an API key reads as plan limits: %+v", reading)
	}
	if time.Until(c.RetryAfter) < 20*time.Minute {
		t.Fatalf("no back-off for an API key: retry after %v", c.RetryAfter)
	}
	if len(p.calls) != 0 {
		t.Fatalf("an API key reached the CLI: %v", p.calls)
	}
}

func TestUsageCheckFailureKeepsLastReadingAndBacksOff(t *testing.T) {
	p := &fakeProber{err: errors.New("container said: boom")}
	e := newUsageEnv(t, p)
	e.set("harness:openai", codexLoginOld)
	old := usage.Snapshot{Harness: agentdef.HarnessCodex, Plan: "plus", Available: true, ObservedAt: time.Now().Add(-time.Hour),
		Windows: []usage.Window{{ID: "codex_300", Kind: usage.Session, Label: "Session", UsedPercent: 12}}}
	if err := (&usage.Service{Q: e.q}).Save(e.ctx, e.ws, old); err != nil {
		t.Fatal(err)
	}
	reading, c := e.check(t, agentdef.HarnessCodex)
	if !reading.Available || reading.Plan != "plus" || !reading.ObservedAt.Before(time.Now().Add(-50*time.Minute)) {
		t.Fatalf("last reading lost: %+v", reading)
	}
	if c.Status != "done" || time.Until(c.RetryAfter) < 5*time.Minute {
		t.Fatalf("check %+v", c)
	}
}
