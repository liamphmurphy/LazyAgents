package runner_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/chatmodels"
	"github.com/liamphmurphy/lazyagents/server/internal/harness"
	"github.com/liamphmurphy/lazyagents/server/internal/runner"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// OpenCode chats against the fake CLI (synthetic envelope, see opencode_test.go).

const (
	ocSonnet = "anthropic/claude-sonnet-5-5"
	ocOpus   = "anthropic/claude-opus-5-5"
	ocGPT    = "openai/gpt-5"
)

// openCodeChatEnv attests isolation to the catalog, discovers a fixed model
// list per provider, and keeps the selection flowing through the real catalog.
func openCodeChatEnv(t *testing.T, secrets ...string) env {
	t.Helper()
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	e.putSecrets(secrets...)
	e.runs.OpenCodeIsolated = true
	e.runs.ModelDiscovery = func(_ context.Context, h, _ string) ([]chatmodels.Model, error) {
		switch h {
		case agentdef.HarnessCodex:
			return []chatmodels.Model{{ID: "gpt-5", Default: true}, {ID: "gpt-5-mini"}}, nil
		default:
			return []chatmodels.Model{{ID: "claude-sonnet-5-5", Default: true}, {ID: "claude-opus-5-5"}}, nil
		}
	}
	return e
}

// sayAs posts with a harness/model selection through the validating catalog.
func (e env) sayAs(id uuid.UUID, body, h, model string) store.ChatMessage {
	e.t.Helper()
	if _, err := e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, id, runs.ChatMessageInput{Body: body, Harness: h, Model: model}); err != nil {
		e.t.Fatal(err)
	}
	e.until("reply to "+body, func() bool { return e.chat(id).State == "idle" })
	msgs, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: id, Limit: 100})
	if err != nil || len(msgs) == 0 || msgs[len(msgs)-1].Role != "assistant" {
		e.t.Fatalf("messages %+v %v", msgs, err)
	}
	return store.ChatMessage(msgs[len(msgs)-1])
}

func (e env) envLog(run store.Run) string {
	b, _ := os.ReadFile(filepath.Join(e.runner.Backend.WorkspaceDir(run.ID.String()), ".lazyagents", "fake-opencode-env.txt"))
	return string(b)
}

func (e env) chatSystemNotice(id uuid.UUID) string {
	msgs, _ := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: id, Limit: 100})
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "system" {
			return msgs[i].BodyMd
		}
	}
	return ""
}

// Two turns resume one session via --session, with attributed metadata, and
// the run waits for a human again after each. Spec and workspace keep the
// chat's read-only shape.
func TestOpenCodeChatTurnsResume(t *testing.T) {
	e := openCodeChatEnv(t, "harness:anthropic")
	e.useOpenCode(ocSonnet)
	chat := e.startChat("First question")
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	first := e.chatRun(chat.ID)
	if first.Harness != "opencode" || first.Purpose != "chat" || first.TicketID != nil || first.State != "waiting" || deref(first.WaitReason) != "human_reply" ||
		first.HarnessSessionID == nil || !strings.HasPrefix(*first.HarnessSessionID, "ses_") {
		t.Fatalf("first run %+v", first)
	}
	spec, err := runs.Spec(first)
	if err != nil || len(spec.Setup)+len(spec.MCPServers)+len(spec.Fallbacks) != 0 || spec.HarnessOptions["readonly"] != "true" || len(spec.HarnessOptions) != 1 {
		t.Fatalf("unsafe frozen spec %+v %v", spec, err)
	}
	for _, s := range spec.Scopes {
		if s.Mode != agentdef.Denied {
			t.Fatalf("chat scope %s is %v", s.Scope, s.Mode)
		}
	}
	root := e.runner.Backend.WorkspaceDir(first.ID.String())
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatal("chat scratch workspace contains a git repository")
	}
	cfg, err := os.ReadFile(filepath.Join(filepath.Dir(root), ".lazyagents", "opencode", "main", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Read-only: no edit/write/bash grants; only the LazyAgents MCP server.
	for _, bad := range []string{`"edit"`, `"write"`, `"bash"`} {
		if strings.Contains(string(cfg), bad) {
			t.Errorf("chat opencode.json grants %s:\n%s", bad, cfg)
		}
	}
	if strings.Count(string(cfg), `"type": "remote"`) != 1 {
		t.Errorf("chat opencode.json must hold only the lazyagents MCP server:\n%s", cfg)
	}

	second := e.sayAs(chat.ID, "Second question", "", "")
	run := e.chatRun(chat.ID)
	if run.ID != first.ID || deref(run.HarnessSessionID) != deref(first.HarnessSessionID) || run.State != "waiting" || deref(run.WaitReason) != "human_reply" || e.chat(chat.ID).State != "idle" {
		t.Fatalf("second run %+v", run)
	}
	if !strings.Contains(second.BodyMd, "resumed=true") || !strings.Contains(second.BodyMd, "session="+*first.HarnessSessionID) || !strings.Contains(second.BodyMd, "model="+ocSonnet) {
		t.Fatalf("second turn did not resume with --session: %s", second.BodyMd)
	}
	msgs, _ := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 20})
	turns, _ := e.q.ListTurns(e.ctx, first.ID)
	if len(msgs) != 4 || len(turns) != 2 {
		t.Fatalf("messages=%d turns=%d", len(msgs), len(turns))
	}
	for i, idx := range []int{1, 3} {
		m := msgs[idx]
		var u harness.Usage
		if m.Role != "assistant" || deref(m.Harness) != "opencode" || deref(m.Model) != ocSonnet || m.RunID == nil || *m.RunID != first.ID || m.RunTurnID == nil || *m.RunTurnID != turns[i].ID ||
			json.Unmarshal(m.Usage, &u) != nil || u.InputTokens != 1000 || u.OutputTokens != 200 {
			t.Fatalf("assistant metadata %+v", m)
		}
	}
	if run.InputTokens != 2000 || run.OutputTokens != 400 || deref(run.CostSource) != "unknown" {
		t.Fatalf("usage %d/%d %v", run.InputTokens, run.OutputTokens, run.CostSource)
	}
}

// A chat reads project context through the restricted chat MCP token, which
// is harness-agnostic: the opencode chat's read is cited like any other.
func TestOpenCodeChatReadsThroughChatToken(t *testing.T) {
	e := openCodeChatEnv(t, "harness:anthropic")
	t.Setenv("FAKE_HARNESS_MODE", "chat-read")
	e.useOpenCode(ocSonnet)
	var art uuid.UUID
	if err := e.pool.QueryRow(e.ctx, `SELECT a.id FROM artifacts a JOIN artifact_versions v ON v.id = a.current_version_id
		WHERE a.type = 'prd' AND a.project_id = (SELECT id FROM projects WHERE key = 'PAY') LIMIT 1`).Scan(&art); err != nil {
		t.Fatal(err)
	}
	chat := e.startChat("read " + art.String())
	e.until("reply", func() bool { return e.chat(chat.ID).State == "idle" })
	msgs, _ := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 10})
	if len(msgs) != 2 || msgs[1].Role != "assistant" || !strings.Contains(string(msgs[1].Citations), art.String()) {
		t.Fatalf("opencode chat read was not cited: %+v", msgs)
	}
	if !strings.Contains(e.eventTypes(e.chatRun(chat.ID).ID), "tool_call") {
		t.Fatal("expected an MCP tool call")
	}
}

// Claude Code and Codex chats switch to OpenCode and back. Every harness change
// ends the old segment and starts a fresh run (no session, bounded handoff);
// switching back never resumes the stale native session.
func TestOpenCodeChatHarnessSwitches(t *testing.T) {
	for _, tc := range []struct{ harness, model string }{
		{agentdef.HarnessClaudeCode, "claude-sonnet-5-5"},
		{agentdef.HarnessCodex, "gpt-5"},
	} {
		t.Run(tc.harness, func(t *testing.T) {
			e := openCodeChatEnv(t, "harness:anthropic")
			e.runs.HandoffChars = 700
			if err := e.setOverride(agentdef.Override{Harness: tc.harness, Model: tc.model}); err != nil {
				t.Fatal(err)
			}
			big := strings.Repeat("x", 3000)
			chat := e.startChat("Message one " + big)
			e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
			first := e.chatRun(chat.ID)

			reply := e.sayAs(chat.ID, "Message two", "opencode", "")
			oc := e.chatRun(chat.ID)
			if oc.ID == first.ID || oc.Harness != "opencode" || oc.Model != ocSonnet || deref(reply.Harness) != "opencode" || deref(reply.Model) != ocSonnet {
				t.Fatalf("opencode segment %+v reply %+v", oc, reply)
			}
			if oc.HarnessSessionID == nil || deref(oc.HarnessSessionID) == deref(first.HarnessSessionID) {
				t.Fatalf("new segment must own a fresh session: %v vs %v", oc.HarnessSessionID, first.HarnessSessionID)
			}
			if !strings.Contains(reply.BodyMd, "resumed=false") || !strings.Contains(reply.BodyMd, "Message one") || !strings.Contains(reply.BodyMd, "Message two") || !strings.Contains(reply.BodyMd, "different harness or model") {
				t.Fatalf("handoff prompt: %s", reply.BodyMd)
			}
			if strings.Contains(reply.BodyMd, big) || len(reply.BodyMd) > 2500 {
				t.Fatalf("handoff is not bounded (%d chars)", len(reply.BodyMd))
			}
			if old := e.runByID(first.ID); old.State != "completed" {
				t.Fatalf("old segment %+v", old)
			}

			// Back again: another fresh run, not the first one's session.
			back := e.sayAs(chat.ID, "Message three", tc.harness, tc.model)
			third := e.chatRun(chat.ID)
			if third.ID == oc.ID || third.ID == first.ID || third.Harness != tc.harness || deref(back.Harness) != tc.harness || !strings.Contains(back.BodyMd, "resumed=false") ||
				deref(third.HarnessSessionID) == deref(first.HarnessSessionID) || !strings.Contains(back.BodyMd, "(opencode/anthropic/claude-sonnet-5-5)") {
				t.Fatalf("switch back %+v reply %s", third, back.BodyMd)
			}
			if e.runByID(oc.ID).State != "completed" {
				t.Fatal("opencode segment not completed on switch back")
			}
		})
	}
}

func (e env) setOverride(o agentdef.Override) error {
	return (&agents.Service{Pool: e.pool}).SetOverride(e.ctx, e.ws, e.me.ID, "atlas", "PAY", o)
}

// Within OpenCode a model change keeps the run and resumes the session.
// Cross-provider decision: OpenCode sessions are not tied to a provider and
// --model is chosen per invocation, so anthropic -> openai also keeps the run
// and session. The new turn gets only the new provider's key (the runner
// derives it from the run's updated spec); the old provider's key is dropped.
func TestOpenCodeChatModelChangeResumesSession(t *testing.T) {
	for _, tc := range []struct{ name, to, key, absent string }{
		{"same provider", ocOpus, "ANTHROPIC_API_KEY", "OPENAI_API_KEY"},
		{"cross provider", ocGPT, "OPENAI_API_KEY", "ANTHROPIC_API_KEY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := openCodeChatEnv(t, "harness:anthropic", "harness:openai")
			e.useOpenCode(ocSonnet)
			chat := e.startChat("Message one")
			e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
			first := e.chatRun(chat.ID)
			reply := e.sayAs(chat.ID, "Message two", "opencode", tc.to)
			run := e.chatRun(chat.ID)
			if run.ID != first.ID || deref(run.HarnessSessionID) != deref(first.HarnessSessionID) || run.Model != tc.to || run.WorkspaceVolume == nil || deref(reply.Model) != tc.to {
				t.Fatalf("run %+v reply %+v", run, reply)
			}
			if !strings.Contains(reply.BodyMd, "resumed=true") || !strings.Contains(reply.BodyMd, "session="+*first.HarnessSessionID) || !strings.Contains(reply.BodyMd, "model="+tc.to) || strings.Contains(reply.BodyMd, "different harness or model") {
				t.Fatalf("reply %s", reply.BodyMd)
			}
			spec, err := runs.Spec(run)
			if err != nil || spec.Model != tc.to {
				t.Fatalf("spec model %q %v", spec.Model, err)
			}
			lines := strings.Split(strings.TrimSpace(e.envLog(run)), "\n")
			last := lines[len(lines)-1]
			if !strings.HasPrefix(last, tc.to+" ") || !strings.Contains(last, tc.key) || strings.Contains(last, tc.absent) {
				t.Fatalf("env of the changed-model turn %q (want %s only)", last, tc.key)
			}
		})
	}
}

// A post or selection switch while a turn runs is a 409; cancelling mid-flight
// leaves the chat idle and the next post answers once, with no late reply.
func TestOpenCodeChatCancelMidTurn(t *testing.T) {
	e := openCodeChatEnv(t, "harness:anthropic")
	e.useOpenCode(ocSonnet)
	chat := e.startChat("slow")
	deadline := time.Now().Add(15 * time.Second)
	for e.chatRun(chat.ID).HarnessSessionID == nil {
		if time.Now().After(deadline) {
			t.Fatal("fake opencode did not start")
		}
		e.runner.Tick(e.ctx)
		time.Sleep(20 * time.Millisecond)
	}
	first := e.chatRun(chat.ID)
	_, err := e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "Busy"})
	requireConflict(t, err)
	_, err = e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "Switch", Harness: "codex"})
	requireConflict(t, err)
	if c := e.chat(chat.ID); c.Harness != "opencode" || c.Model != ocSonnet {
		t.Fatalf("rejected switch changed the selection: %s %s", c.Harness, c.Model)
	}
	if _, err := e.runs.SetRunControl(e.ctx, e.ws, e.me.ID, first.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	e.runner.Wait()
	if e.chat(chat.ID).State != "idle" || e.chatRun(chat.ID).State != "waiting" {
		t.Fatalf("cancel left chat %s / run %s", e.chat(chat.ID).State, e.chatRun(chat.ID).State)
	}
	reply := e.sayAs(chat.ID, "After cancellation", "", "")
	if e.chatRun(chat.ID).ID != first.ID || !strings.Contains(reply.BodyMd, "After cancellation") {
		t.Fatalf("reply %s", reply.BodyMd)
	}
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	time.Sleep(200 * time.Millisecond)
	msgs, _ := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 20})
	if len(msgs) != 3 || msgs[2].Role != "assistant" {
		t.Fatalf("duplicate or late reply: %+v", msgs)
	}
	turns, _ := e.q.ListTurns(e.ctx, first.ID)
	for _, tn := range turns {
		if tn.Status != "done" {
			t.Fatalf("stranded turn %+v", tn)
		}
	}
}

// Unavailable startup fails the turn with an actionable notice; the failChat
// path leaves the chat failed (not busy) and the segment usable, so the next
// post is accepted once the operator fixes the cause.
func TestOpenCodeChatStartupFailures(t *testing.T) {
	old := filepath.Join(t.TempDir(), "opencode-old")
	if err := os.WriteFile(old, []byte("#!/bin/sh\necho 'opencode v9.9.9'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldTimeout := runner.OpenCodePreflightTimeout
	t.Cleanup(func() { runner.OpenCodePreflightTimeout = oldTimeout })
	local := func(e env) *runner.Local { return e.runner.Backend.(*runner.Local) }
	for _, tc := range []struct {
		name, want string
		secrets    []string
		break_     func(e env)
		fix        func(e env)
	}{
		{"no isolation", "isolated config", []string{"harness:anthropic"},
			func(e env) { local(e).OpenCodeIsolated = false }, func(e env) { local(e).OpenCodeIsolated = true }},
		{"no provider key", "harness:anthropic", nil,
			func(e env) {}, func(e env) { e.putSecrets("harness:anthropic") }},
		{"only the other provider's key", "harness:anthropic", []string{"harness:openai"},
			func(e env) {}, func(e env) { e.putSecrets("harness:anthropic") }},
		{"missing binary", "OpenCode is not available", []string{"harness:anthropic"},
			func(e env) { local(e).Bins["opencode"] = "/nonexistent/opencode" }, nil},
		{"wrong binary", "2.2.1 is required", []string{"harness:anthropic"},
			func(e env) { local(e).Bins["opencode"] = old }, nil},
		{"preflight timeout", "did not answer `opencode --version` within 300ms", []string{"harness:anthropic"},
			func(e env) {
				runner.OpenCodePreflightTimeout = 300 * time.Millisecond
				e.t.Setenv("FAKE_HARNESS_MODE", "version-hang")
			}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := openCodeChatEnv(t, tc.secrets...)
			e.useOpenCode(ocSonnet)
			goodBin := local(e).Bins["opencode"]
			tc.break_(e)
			chat := e.startChat("hello")
			e.until("chat failure", func() bool { return e.chat(chat.ID).State == "failed" })
			notice := e.chatSystemNotice(chat.ID)
			if !strings.Contains(notice, tc.want) || strings.Contains(notice, anthropicSecret) || strings.Contains(notice, openaiSecret) {
				t.Fatalf("notice %q, want mention of %q", notice, tc.want)
			}
			run := e.chatRun(chat.ID)
			if run.State != "waiting" || deref(run.WaitReason) != "human_reply" {
				t.Fatalf("setup failure stranded the run: %s %s", run.State, deref(run.WaitReason))
			}
			if turns, _ := e.q.ListTurns(e.ctx, run.ID); len(turns) != 1 || turns[0].Status == "running" || turns[0].Status == "pending" {
				t.Fatalf("setup failure left the turn open: %+v", turns)
			}
			// Fix the cause: the next post is accepted and answered.
			runner.OpenCodePreflightTimeout = oldTimeout
			e.t.Setenv("FAKE_HARNESS_MODE", "chat")
			local(e).Bins["opencode"] = goodBin
			if tc.fix != nil {
				tc.fix(e)
			} else if tc.name != "missing binary" && tc.name != "wrong binary" && tc.name != "preflight timeout" {
				t.Fatal("no fix")
			}
			e.sayAs(chat.ID, "Try again", "", "")
			if e.chatRun(chat.ID).ID != run.ID {
				t.Fatal("retry switched segment")
			}
		})
	}
}
