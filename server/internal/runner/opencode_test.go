package runner_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/harness"
	"github.com/liamphmurphy/lazyagents/server/internal/runner"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/secrets"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

// These run the real runner against the fake CLI speaking the SYNTHETIC
// OpenCode envelope (cmd/fake-harness), so they prove Vesper's dispatch,
// credentials, isolation, completion and usage rules, not the real wire.

const (
	anthropicSecret = "anthropic-secret-value-0001"
	openaiSecret    = "openai-secret-value-0002"
)

func (e env) putSecrets(names ...string) {
	e.t.Helper()
	vals := map[string]string{"harness:anthropic": anthropicSecret, "harness:openai": openaiSecret}
	sec := e.runner.Secrets.(*secrets.Service)
	for _, n := range names {
		if _, err := sec.Set(e.ctx, e.ws, e.me.ID, secrets.SetInput{Name: n, Value: vals[n]}); err != nil {
			e.t.Fatal(err)
		}
	}
}

// useOpenCode points Atlas at OpenCode in project PAY.
func (e env) useOpenCode(model string) {
	e.t.Helper()
	svc := &agents.Service{Pool: e.pool}
	p, _ := e.q.GetProjectByKey(e.ctx, store.GetProjectByKeyParams{WorkspaceID: e.ws, Key: "PAY"})
	row, err := e.q.GetOverride(e.ctx, store.GetOverrideParams{AgentID: e.atlas.ID, ProjectID: p.ID})
	if err != nil {
		e.t.Fatal(err)
	}
	ov, err := agents.DecodeOverride(row.Spec)
	if err != nil {
		e.t.Fatal(err)
	}
	ov.Harness, ov.Model = agentdef.HarnessOpenCode, model
	if err := svc.SetOverride(e.ctx, e.ws, e.me.ID, "atlas", "PAY", ov); err != nil {
		e.t.Fatal(err)
	}
}

func (e env) workspace(run store.Run) string {
	return filepath.Join(e.dir, "work", "runs", run.ID.String(), "workspace")
}

func (e env) failedWith(key, want string) store.Run {
	e.t.Helper()
	e.until("run failed", e.state(key+"=failed"))
	r := e.run(key)
	if !strings.Contains(deref(r.Outcome), want) {
		e.t.Fatalf("outcome %q does not mention %q", deref(r.Outcome), want)
	}
	for _, v := range []string{anthropicSecret, openaiSecret} {
		if strings.Contains(deref(r.Outcome), v) {
			e.t.Fatalf("a secret value leaked into the outcome: %q", deref(r.Outcome))
		}
	}
	return r
}

// A ticket run through to approval and a resumed turn: completion semantics
// are the ticket's (complete_run), the session is resumed, only the selected
// provider key reaches the process, and an unpriced model is unknown, not free.
func TestOpenCodeTicketRunWithResume(t *testing.T) {
	e := setup(t)
	e.putSecrets("harness:anthropic", "harness:openai")
	e.useOpenCode("anthropic/claude-sonnet-5-5")
	t.Setenv("ANTHROPIC_API_KEY", "host-anthropic-must-not-leak")
	t.Setenv("OPENAI_API_KEY", "host-openai-must-not-leak")
	e.dispatch("PAY-214")
	if r := e.run("PAY-214"); r.Harness != "opencode" || r.Model != "anthropic/claude-sonnet-5-5" {
		t.Fatalf("frozen target %s %s", r.Harness, r.Model)
	}
	e.until("waiting for approval", e.state("PAY-214=waiting"))
	run := e.run("PAY-214")
	evs := e.eventTypes(run.ID)
	for _, want := range []string{"lifecycle:harness_started", "tool_call:lazyagents_claim_ticket", "tool_call:lazyagents_request_approval", "usage"} {
		if !strings.Contains(evs, want) {
			t.Errorf("missing %s in %s", want, evs)
		}
	}
	if run.HarnessSessionID == nil || !strings.HasPrefix(*run.HarnessSessionID, "ses_") {
		t.Fatalf("session %v", run.HarnessSessionID)
	}
	first := *run.HarnessSessionID
	// Unpriced: unknown cost, never a free run.
	if run.CostSource == nil || *run.CostSource != "unknown" || run.CostUsdMicros != 0 {
		t.Fatalf("cost %v / %d", run.CostSource, run.CostUsdMicros)
	}
	if run.InputTokens != 1000 || run.OutputTokens != 200 {
		t.Fatalf("tokens %d/%d", run.InputTokens, run.OutputTokens)
	}
	got, err := os.ReadFile(filepath.Join(e.workspace(run), ".lazyagents", "fake-opencode-env.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "ANTHROPIC_API_KEY") || strings.Contains(string(got), "OPENAI_API_KEY") {
		t.Fatalf("credential env names seen by opencode: %q", got)
	}
	cfg, err := os.ReadFile(filepath.Join(filepath.Dir(e.workspace(run)), ".lazyagents", "opencode", "main", "opencode.json"))
	if err != nil || strings.Contains(string(cfg), "la_") || !strings.Contains(string(cfg), "{env:LAZYAGENTS_RUN_TOKEN}") {
		t.Fatalf("generated config: %v\n%s", err, cfg)
	}

	tk, _ := tracker.Get(e.ctx, e.q, e.ws, "PAY-214")
	aps, _ := e.q.PendingApprovalsForTicket(e.ctx, tk.ID)
	if _, err := e.runs.Decide(e.ctx, e.ws, e.me.ID, aps[0].ID, true, "go"); err != nil {
		t.Fatal(err)
	}
	e.until("completed", e.state("PAY-214=completed"))
	run = e.run("PAY-214")
	turns, _ := e.q.ListTurns(e.ctx, run.ID)
	if len(turns) != 2 || turns[1].Trigger != "approval" || *turns[1].ExitCode != 0 {
		t.Fatalf("turns %+v", turns)
	}
	if run.HarnessSessionID == nil || *run.HarnessSessionID != first {
		t.Fatalf("the resumed turn must continue session %s, got %v", first, run.HarnessSessionID)
	}
	if run.CostSource == nil || *run.CostSource != "unknown" || run.InputTokens != 2000 {
		t.Fatalf("after resume: cost %v tokens %d", run.CostSource, run.InputTokens)
	}
	if tk, _ = tracker.Get(e.ctx, e.q, e.ws, "PAY-214"); tk.Status != "in_review" {
		t.Fatalf("ticket %s", tk.Status)
	}
	if _, err := os.Stat(e.workspace(run)); !os.IsNotExist(err) {
		t.Fatal("workspace not removed")
	}
}

// An exact operator price turns the same tokens into a computed cost; the
// price for another provider's model of the same name is ignored.
func TestOpenCodeComputedCost(t *testing.T) {
	e := setup(t)
	e.putSecrets("harness:openai")
	e.useOpenCode("openai/gpt-5#high")
	e.runner.Cfg.ModelPrices = map[string]harness.Price{
		"openai/gpt-5#high": {Input: 2, CachedInput: 1, Output: 10},
		"anthropic/gpt-5":   {Input: 99, CachedInput: 99, Output: 99},
	}
	t.Setenv("FAKE_HARNESS_MODE", "complete")
	e.dispatch("PAY-214")
	e.until("completed", e.state("PAY-214=completed"))
	run := e.run("PAY-214")
	// 500 uncached*2 + 500 cached*1 + 200 output*10 = 3500 per million.
	if run.CostSource == nil || *run.CostSource != "computed" || run.CostUsdMicros != 3500 {
		t.Fatalf("cost %v / %d", run.CostSource, run.CostUsdMicros)
	}
}

// Planning keeps its own completion rule (propose_tickets ends the run), gets
// only the read-only option OpenCode understands, and its nudge turn resumes
// the same OpenCode session.
func TestOpenCodePlanningRunWithResume(t *testing.T) {
	e := setup(t)
	e.putSecrets("harness:openai")
	e.useOpenCode("openai/gpt-5")
	t.Setenv("FAKE_HARNESS_MODE", "idle")
	plan, err := e.runs.StartPlanning(e.ctx, e.ws, e.me.ID, runs.PlanningInput{Project: "PAY", AgentID: e.atlas.ID, Goal: "Plan safely"})
	if err != nil {
		t.Fatal(err)
	}
	frozen, _ := e.q.GetRun(e.ctx, store.GetRunParams{ID: plan.RunID, WorkspaceID: e.ws})
	spec, _ := runs.Spec(frozen)
	if frozen.Harness != "opencode" || len(spec.HarnessOptions) != 1 || spec.HarnessOptions["readonly"] != "true" {
		t.Fatalf("planning spec %s %v", frozen.Harness, spec.HarnessOptions)
	}
	var session string
	e.until("planning submitted after the resumed nudge turn", func() bool {
		turns, _ := e.q.ListTurns(e.ctx, plan.RunID)
		if len(turns) >= 1 && turns[0].Status == "done" {
			t.Setenv("FAKE_HARNESS_MODE", "planning") // the nudged turn proposes
			if r, _ := e.q.GetRun(e.ctx, store.GetRunParams{ID: plan.RunID, WorkspaceID: e.ws}); r.HarnessSessionID != nil {
				session = *r.HarnessSessionID
			}
		}
		p, err := e.q.GetPlanningSession(e.ctx, store.GetPlanningSessionParams{ID: plan.ID, WorkspaceID: e.ws})
		if err != nil {
			t.Fatal(err)
		}
		if r, _ := e.q.GetRun(e.ctx, store.GetRunParams{ID: plan.RunID, WorkspaceID: e.ws}); r.State == "failed" {
			t.Fatalf("planning failed: %s", deref(r.Outcome))
		}
		return p.State == "review"
	})
	run, _ := e.q.GetRun(e.ctx, store.GetRunParams{ID: plan.RunID, WorkspaceID: e.ws})
	turns, _ := e.q.ListTurns(e.ctx, run.ID)
	if run.State != "completed" || run.TicketID != nil || len(turns) != 2 || turns[1].Trigger != "resume" {
		t.Fatalf("planning run %s turns=%d", run.State, len(turns))
	}
	if session == "" || run.HarnessSessionID == nil || *run.HarnessSessionID != session {
		t.Fatalf("nudge turn did not resume session %q: %v", session, run.HarnessSessionID)
	}
	if run.CostSource == nil || *run.CostSource != "unknown" {
		t.Fatalf("cost %v", run.CostSource)
	}
	p, _ := e.q.GetPlanningSession(e.ctx, store.GetPlanningSessionParams{ID: plan.ID, WorkspaceID: e.ws})
	var proposals []runs.Proposal
	if err := json.Unmarshal(p.Proposals, &proposals); err != nil || len(proposals) != 1 {
		t.Fatalf("proposals %s: %v", p.Proposals, err)
	}
}

func TestOpenCodeInvalidSelections(t *testing.T) {
	e := setup(t)
	svc := &agents.Service{Pool: e.pool}
	for name, ov := range map[string]agentdef.Override{
		"no provider":          {Harness: "opencode", Model: "gpt-5"},
		"keeps claude model":   {Harness: "opencode"},
		"unsupported provider": {Harness: "opencode", Model: "google/gemini-3"},
	} {
		err := svc.SetOverride(e.ctx, e.ws, e.me.ID, "atlas", "PAY", ov)
		if err == nil || !strings.Contains(err.Error(), "opencode") {
			t.Errorf("%s: want an actionable opencode error, got %v", name, err)
		}
	}
	spec := agents.DefaultSpec()
	spec.Fallbacks = []agentdef.Fallback{{Harness: "opencode", Model: "sonnet-5.5"}}
	if _, err := svc.Create(e.ctx, e.ws, e.me.ID, "badfallback", agents.Identity{Name: "Bad"}, &spec); err == nil || !strings.Contains(err.Error(), "provider/model") {
		t.Errorf("fallback: %v", err)
	}
}

// The frozen spec decides the harness even if the definition changes later.
func TestOpenCodeFrozenSnapshot(t *testing.T) {
	e := setup(t)
	e.putSecrets("harness:anthropic")
	e.useOpenCode("anthropic/claude-sonnet-5-5#high")
	e.dispatch("PAY-214")
	queued := e.run("PAY-214")
	e.useOpenCode("anthropic/claude-opus-5-5") // changed after dispatch
	t.Setenv("FAKE_HARNESS_MODE", "complete")
	e.until("completed", e.state("PAY-214=completed"))
	run := e.run("PAY-214")
	if run.ID != queued.ID || run.Model != "anthropic/claude-sonnet-5-5#high" || run.Harness != "opencode" {
		t.Fatalf("run drifted from its snapshot: %s %s", run.Harness, run.Model)
	}
}

func TestOpenCodeFailClosedWithoutIsolation(t *testing.T) {
	e := setup(t)
	e.putSecrets("harness:anthropic")
	e.useOpenCode("anthropic/claude-sonnet-5-5")
	e.runner.Backend.(*runner.Local).OpenCodeIsolated = false
	e.dispatch("PAY-214")
	r := e.failedWith("PAY-214", "isolated config")
	if turns, _ := e.q.ListTurns(e.ctx, r.ID); len(turns) > 0 && turns[0].ExitCode != nil {
		t.Fatalf("a process ran without isolation: %+v", turns[0])
	}
}

func TestOpenCodeMissingAuth(t *testing.T) {
	t.Run("no secret", func(t *testing.T) {
		e := setup(t)
		e.useOpenCode("anthropic/claude-sonnet-5-5")
		e.dispatch("PAY-214")
		e.failedWith("PAY-214", "harness:anthropic")
	})
	t.Run("only another provider's key exists", func(t *testing.T) {
		e := setup(t)
		e.putSecrets("harness:openai") // must never stand in for anthropic
		e.useOpenCode("anthropic/claude-sonnet-5-5")
		e.dispatch("PAY-214")
		e.failedWith("PAY-214", "harness:anthropic")
	})
}

func TestOpenCodeMissingOrWrongBinary(t *testing.T) {
	old := filepath.Join(t.TempDir(), "opencode-old")
	if err := os.WriteFile(old, []byte("#!/bin/sh\necho 'opencode v9.9.9'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct{ bin, want string }{
		"missing": {"/nonexistent/opencode", "OpenCode is not available"},
		"wrong":   {old, "2.2.1 is required"},
	} {
		t.Run(name, func(t *testing.T) {
			e := setup(t)
			e.putSecrets("harness:anthropic")
			e.useOpenCode("anthropic/claude-sonnet-5-5")
			e.runner.Backend.(*runner.Local).Bins["opencode"] = c.bin
			e.dispatch("PAY-214")
			e.failedWith("PAY-214", c.want)
		})
	}
}

// A binary that never answers --version is killed at the timeout: the turn
// fails with an actionable message instead of blocking. The fake sleeps for an
// hour, so returning at all proves the process was killed and Wait unblocked.
func TestOpenCodePreflightTimeout(t *testing.T) {
	old := runner.OpenCodePreflightTimeout
	runner.OpenCodePreflightTimeout = 500 * time.Millisecond
	t.Cleanup(func() { runner.OpenCodePreflightTimeout = old })
	e := setup(t)
	e.putSecrets("harness:anthropic")
	e.useOpenCode("anthropic/claude-sonnet-5-5")
	t.Setenv("FAKE_HARNESS_MODE", "version-hang")
	start := time.Now()
	e.dispatch("PAY-214")
	e.failedWith("PAY-214", "did not answer `opencode --version` within 500ms")
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("preflight took %s", d)
	}
	e.failedWith("PAY-214", "RUNNER_OPENCODE_BIN")
}

func TestOpenCodeProviderFailure(t *testing.T) {
	e := setup(t)
	e.putSecrets("harness:anthropic")
	e.useOpenCode("anthropic/claude-sonnet-5-5")
	t.Setenv("FAKE_HARNESS_MODE", "crash")
	e.dispatch("PAY-214")
	e.failedWith("PAY-214", "overloaded")
}

// Stop while a turn runs: the run stays cancelled, the turn ends non-zero and
// nothing completes it afterwards.
func TestOpenCodeCancellation(t *testing.T) {
	e := setup(t)
	e.putSecrets("harness:anthropic")
	e.useOpenCode("anthropic/claude-sonnet-5-5")
	t.Setenv("FAKE_HARNESS_MODE", "slow")
	e.dispatch("PAY-224")
	run := e.run("PAY-224")
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				e.runner.Tick(e.ctx)
				time.Sleep(30 * time.Millisecond)
			}
		}
	}()
	deadline := time.Now().Add(20 * time.Second)
	for !strings.Contains(e.eventTypes(run.ID), "tool_result:lazyagents_claim_ticket") {
		if time.Now().After(deadline) {
			t.Fatal("slow turn never started")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := e.runs.SetRunControl(e.ctx, e.ws, e.me.ID, run.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	for {
		turns, _ := e.q.ListTurns(e.ctx, run.ID)
		if turns[0].Status == "done" {
			if *turns[0].ExitCode == 0 {
				t.Fatalf("cancelled turn exited 0")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cancel did not stop the turn")
		}
		time.Sleep(50 * time.Millisecond)
	}
	close(stop)
	<-done
	e.runner.Wait()
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	r := e.run("PAY-224")
	if r.State != "cancelled" {
		t.Fatalf("state %s", r.State)
	}
	if turns, _ := e.q.ListTurns(e.ctx, run.ID); len(turns) != 1 {
		t.Fatalf("a turn started after cancel: %d", len(turns))
	}
}

// After a configured fallback the run, session and final output belong to the
// harness that was actually invoked, in both directions.
func TestOpenCodeFallbackAttribution(t *testing.T) {
	for name, c := range map[string]struct {
		from, fromModel string
		to, toModel     string
		session         string
	}{
		"opencode to codex":  {agentdef.HarnessOpenCode, "anthropic/claude-sonnet-5-5", agentdef.HarnessCodex, "test-fallback", ""},
		"claude to opencode": {agentdef.HarnessClaudeCode, "sonnet-5.5", agentdef.HarnessOpenCode, "openai/gpt-5", "ses_"},
	} {
		t.Run(name, func(t *testing.T) {
			e := setup(t)
			e.putSecrets("harness:anthropic", "harness:openai")
			if c.from == agentdef.HarnessOpenCode {
				e.useOpenCode(c.fromModel)
			}
			t.Setenv("FAKE_HARNESS_MODE", "quota")
			e.dispatch("PAY-214")
			run := e.run("PAY-214")
			var spec agentdef.Resolved
			if err := json.Unmarshal(run.ResolvedSpec, &spec); err != nil {
				t.Fatal(err)
			}
			spec.Fallbacks = []agentdef.Fallback{{Harness: c.to, Model: c.toModel}}
			raw, _ := json.Marshal(spec)
			if _, err := e.pool.Exec(e.ctx, "UPDATE runs SET resolved_spec=$2 WHERE id=$1", run.ID, raw); err != nil {
				t.Fatal(err)
			}
			e.until("fallback completed", e.state("PAY-214=completed"))
			run = e.run("PAY-214")
			if run.Harness != c.to || run.Model != c.toModel {
				t.Fatalf("attributed to %s %s", run.Harness, run.Model)
			}
			if c.session != "" && (run.HarnessSessionID == nil || !strings.HasPrefix(*run.HarnessSessionID, c.session)) {
				t.Fatalf("session %v belongs to the wrong harness", run.HarnessSessionID)
			}
			if !strings.Contains(deref(run.Outcome), "preserved checkout") || !strings.Contains(e.eventTypes(run.ID), "lifecycle:harness_fallback") {
				t.Fatalf("outcome %v events %s", run.Outcome, e.eventTypes(run.ID))
			}
		})
	}
}

// A fallback to OpenCode with no isolation or no key fails the run cleanly.
func TestOpenCodeFallbackTargetRejected(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "quota")
	e.dispatch("PAY-214")
	run := e.run("PAY-214")
	var spec agentdef.Resolved
	_ = json.Unmarshal(run.ResolvedSpec, &spec)
	spec.Fallbacks = []agentdef.Fallback{{Harness: agentdef.HarnessOpenCode, Model: "openai/gpt-5"}}
	raw, _ := json.Marshal(spec)
	if _, err := e.pool.Exec(e.ctx, "UPDATE runs SET resolved_spec=$2 WHERE id=$1", run.ID, raw); err != nil {
		t.Fatal(err)
	}
	// No harness:openai secret exists.
	r := e.failedWith("PAY-214", "fallback")
	if !strings.Contains(deref(r.Outcome), "harness:openai") {
		t.Fatalf("outcome %q", deref(r.Outcome))
	}
}

// Locally OpenCode inherits no provider keys, HOME or XDG roots from the host.
func TestLocalOpenCodeEnvironmentIsAllowlisted(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "host-anthropic")
	t.Setenv("OPENAI_API_KEY", "host-openai")
	t.Setenv("GEMINI_API_KEY", "host-gemini")
	t.Setenv("XDG_CONFIG_HOME", "/host/xdg")
	t.Setenv("HOME", "/host/home")
	l := &runner.Local{Dir: t.TempDir(), Bins: map[string]string{"opencode": "/usr/bin/env"}, OnlyBins: true, OpenCodeIsolated: true}
	if err := l.EnsureWorkspace(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	p, err := l.Start(context.Background(), "r1", runner.ExecSpec{Argv: []string{"opencode"}, Env: map[string]string{"OPENAI_API_KEY": "injected", "HOME": "/iso/home"}})
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := p.Stdout().Read(buf)
		out.Write(buf[:n])
		if err != nil {
			break
		}
	}
	_, _ = p.Wait()
	env := out.String()
	for _, bad := range []string{"host-anthropic", "host-openai", "host-gemini", "/host/xdg", "/host/home"} {
		if strings.Contains(env, bad) {
			t.Errorf("host value %q leaked to opencode:\n%s", bad, env)
		}
	}
	if !strings.Contains(env, "OPENAI_API_KEY=injected") || !strings.Contains(env, "HOME="+filepath.Join(l.OpenCodeStateDir("r1", ""), "home")) {
		t.Errorf("injected env missing:\n%s", env)
	}
}

// A clean turn that ends without saying what's next is nudged, then handed to
// a person. This only works when the runner reports the process exit status to
// the parser (Parser.Exited): without it every real turn "ended without
// completion" and failed the run.
func TestOpenCodeIdleNudgeNeedsExitStatus(t *testing.T) {
	e := setup(t)
	e.putSecrets("harness:anthropic")
	e.useOpenCode("anthropic/claude-sonnet-5-5")
	t.Setenv("FAKE_HARNESS_MODE", "idle")
	t.Setenv("FAKE_HARNESS_RESUME", "idle")
	e.dispatch("PAY-224")
	e.until("handed to a person", func() bool {
		r := e.run("PAY-224")
		return r.State == "waiting" && r.WaitReason != nil && *r.WaitReason == "human_reply"
	})
	run := e.run("PAY-224")
	turns, _ := e.q.ListTurns(e.ctx, run.ID)
	if len(turns) != 2 || turns[1].InputMessage != runner.IdleNudge || *turns[0].ExitCode != 0 {
		t.Fatalf("turns %+v", turns)
	}
	if run.HarnessSessionID == nil {
		t.Fatal("no session to resume")
	}
}

// Credentials resolve by the model's provider and never fall back.
func TestHarnessKeyMapping(t *testing.T) {
	for model, want := range map[string]struct{ secret, env string }{
		"anthropic/claude-sonnet-5-5": {"harness:anthropic", "ANTHROPIC_API_KEY"},
		"openai/gpt-5#high":           {"harness:openai", "OPENAI_API_KEY"},
	} {
		k, err := runner.HarnessKey(agentdef.Resolved{Harness: agentdef.HarnessOpenCode, Model: model})
		if err != nil || k.Secret != want.secret || len(k.Env) != 1 || k.Env[0] != want.env {
			t.Errorf("%s: %+v %v", model, k, err)
		}
	}
	for _, spec := range []agentdef.Resolved{
		{Harness: agentdef.HarnessOpenCode, Model: "google/gemini-3"},
		{Harness: agentdef.HarnessOpenCode, Model: "gpt-5"},
		{Harness: "mystery", Model: "x"},
	} {
		if k, err := runner.HarnessKey(spec); err == nil {
			t.Errorf("%+v resolved to %+v instead of failing", spec, k)
		}
	}
	if k, _ := runner.HarnessKey(agentdef.Resolved{Harness: agentdef.HarnessCodex}); len(k.Env) != 2 {
		t.Errorf("codex mapping changed: %+v", k)
	}
	if runner.NeedsHarnessKey(false, agentdef.Resolved{Harness: agentdef.HarnessClaudeCode}) || !runner.NeedsHarnessKey(false, agentdef.Resolved{Harness: agentdef.HarnessOpenCode}) {
		t.Error("only opencode requires a key without RequireHarnessKey")
	}
}
