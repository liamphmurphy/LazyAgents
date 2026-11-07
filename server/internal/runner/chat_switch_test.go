package runner_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/harness"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// selectChat stands in for LAZYAGENTS-11's validated selection update.
func (e env) selectChat(id uuid.UUID, h, model string) {
	e.t.Helper()
	if _, err := e.q.SetChatSelection(e.ctx, store.SetChatSelectionParams{ID: id, Harness: h, Model: model}); err != nil {
		e.t.Fatal(err)
	}
}

// say posts a message and ticks the runner until the turn finishes.
func (e env) say(id uuid.UUID, body string) store.ChatMessage {
	e.t.Helper()
	if _, err := e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, id, runs.ChatMessageInput{Body: body}); err != nil {
		e.t.Fatal(err)
	}
	e.until("reply to "+body, func() bool { return e.chat(id).State == "idle" })
	msgs, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: id, Limit: 100})
	if err != nil || len(msgs) == 0 {
		e.t.Fatalf("messages %v %v", msgs, err)
	}
	last := msgs[len(msgs)-1]
	if last.Role != "assistant" {
		e.t.Fatalf("last message %+v", last)
	}
	return store.ChatMessage(last)
}

func (e env) switchSetup(h, model string) store.ChatSession {
	e.t.Helper()
	e.t.Setenv("FAKE_HARNESS_MODE", "chat")
	if err := (&agents.Service{Pool: e.pool}).SetOverride(e.ctx, e.ws, e.me.ID, e.assistant().Handle, "PAY", agentdef.Override{Harness: h, Model: model}); err != nil {
		e.t.Fatal(err)
	}
	chat := e.startChat("Message one")
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	return chat
}

func TestChatHarnessSwitchStartsFreshSegmentWithHandoff(t *testing.T) {
	e := setup(t)
	chat := e.switchSetup(agentdef.HarnessClaudeCode, "opus-5.5")
	first := e.chatRun(chat.ID)
	e.say(chat.ID, "Message two")

	e.selectChat(chat.ID, agentdef.HarnessCodex, "gpt-5.5")
	reply := e.say(chat.ID, "Message three")
	second := e.chatRun(chat.ID)
	if second.ID == first.ID || second.Harness != agentdef.HarnessCodex || deref(reply.Harness) != agentdef.HarnessCodex || deref(reply.Model) != "gpt-5.5" || *reply.RunID != second.ID {
		t.Fatalf("second run %+v reply %+v", second, reply)
	}
	body := reply.BodyMd
	for _, want := range []string{"resumed=false", "Message one", "Message two", "Chat reply", "New message from the user", "Message three", "different harness or model"} {
		if !strings.Contains(body, want) {
			t.Errorf("handoff prompt missing %q in %s", want, body)
		}
	}
	if strings.Index(body, "Message one") > strings.Index(body, "Message two") || strings.Index(body, "Message two") > strings.Index(body, "Message three") {
		t.Errorf("handoff order: %s", body)
	}
	old := e.runByID(first.ID)
	if old.State != "completed" {
		t.Fatalf("old segment %+v", old)
	}
	e.runner.Sweep(e.ctx)
	if e.runByID(first.ID).WorkspaceVolume != nil {
		t.Fatal("old segment workspace was not swept")
	}

	// Switching back also starts fresh: the native session is stale.
	e.selectChat(chat.ID, agentdef.HarnessClaudeCode, "opus-5.5")
	back := e.say(chat.ID, "Message four")
	third := e.chatRun(chat.ID)
	if third.ID == second.ID || third.ID == first.ID || deref(third.HarnessSessionID) == deref(first.HarnessSessionID) || deref(back.Harness) != agentdef.HarnessClaudeCode {
		t.Fatalf("switch back %+v reply %+v", third, back)
	}
	for _, want := range []string{"resumed=false", "Message one", "Message three", "codex/gpt-5.5", "Message four"} {
		if !strings.Contains(back.BodyMd, want) {
			t.Errorf("switch-back prompt missing %q in %s", want, back.BodyMd)
		}
	}
	// A plain follow-up on the same segment resumes and carries no handoff.
	again := e.say(chat.ID, "Message five")
	if !strings.Contains(again.BodyMd, "resumed=true") || strings.Contains(again.BodyMd, "different harness or model") || e.chatRun(chat.ID).ID != third.ID {
		t.Fatalf("follow-up %s", again.BodyMd)
	}
}

func TestChatModelSwitchKeepsSession(t *testing.T) {
	for _, tc := range []struct{ harness, from, to string }{
		{agentdef.HarnessClaudeCode, "opus-5.5", "sonnet-5.5"},
		{agentdef.HarnessCodex, "gpt-5.5", "gpt-5.4"},
	} {
		t.Run(tc.harness, func(t *testing.T) {
			e := setup(t)
			chat := e.switchSetup(tc.harness, tc.from)
			first := e.chatRun(chat.ID)
			e.selectChat(chat.ID, tc.harness, tc.to)
			reply := e.say(chat.ID, "Message two")
			run := e.chatRun(chat.ID)
			want := tc.to
			if tc.harness == agentdef.HarnessClaudeCode {
				want = harness.AnthropicModel(tc.to)
			}
			if run.ID != first.ID || deref(run.HarnessSessionID) != deref(first.HarnessSessionID) || run.Model != tc.to || run.WorkspaceVolume == nil {
				t.Fatalf("run %+v", run)
			}
			spec, err := runs.Spec(run)
			if err != nil || spec.Model != tc.to {
				t.Fatalf("spec model %q %v", spec.Model, err)
			}
			if !strings.Contains(reply.BodyMd, "resumed=true") || !strings.Contains(reply.BodyMd, "model="+want+")") || strings.Contains(reply.BodyMd, "different harness or model") {
				t.Fatalf("reply %s want model %s", reply.BodyMd, want)
			}
			if deref(reply.Model) != tc.to || deref(reply.Harness) != tc.harness {
				t.Fatalf("attribution %+v", reply)
			}
			msgs, _ := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 10})
			if deref(msgs[1].Model) != tc.from {
				t.Fatalf("earlier reply must keep its model: %+v", msgs[1])
			}
		})
	}
}

func (e env) runByID(id uuid.UUID) store.Run {
	e.t.Helper()
	r, err := e.q.GetRun(e.ctx, store.GetRunParams{ID: id, WorkspaceID: e.ws})
	if err != nil {
		e.t.Fatal(err)
	}
	return r
}
