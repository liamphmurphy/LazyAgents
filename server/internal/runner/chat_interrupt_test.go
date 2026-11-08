package runner_test

import (
	"strings"
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A message sent while a reply runs stops it; once the process has exited, the
// same session answers the new message together with the previous one and
// what the stopped reply had written and called.
func TestChatInterruptMergesStoppedReply(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Warm up")
	e.until("warmup", func() bool { return e.chat(chat.ID).State == "idle" })
	first := e.chatRun(chat.ID)
	if _, err := e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "slow partial"}); err != nil {
		t.Fatal(err)
	}
	// Tick without waiting: the reply hangs once it has written and called a tool.
	deadline := time.Now().Add(15 * time.Second)
	for {
		var n int
		if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM run_events WHERE run_id=$1 AND type='tool_call'`, first.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("reply never got going")
		}
		e.runner.Tick(e.ctx)
		time.Sleep(20 * time.Millisecond)
	}
	msg, err := e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "Also check the payroll"})
	if err != nil {
		t.Fatal(err)
	}
	if c := e.chat(chat.ID); c.State != "running" || c.InterruptMessageID == nil || *c.InterruptMessageID != msg.ID {
		t.Fatalf("interruption not waiting: %+v", c)
	}
	e.until("interrupted reply", func() bool { c := e.chat(chat.ID); return c.State == "idle" && c.InterruptMessageID == nil })
	if run := e.chatRun(chat.ID); run.ID != first.ID || deref(run.HarnessSessionID) != deref(first.HarnessSessionID) {
		t.Fatal("interruption lost the chat session")
	}
	msgs, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	// Warm up, its reply, the stopped message (no reply), the interruption, and one answer.
	if len(msgs) != 5 || msgs[3].BodyMd != "Also check the payroll" || msgs[4].Role != "assistant" {
		t.Fatalf("history %+v", msgs)
	}
	if msgs[3].RunTurnID == nil {
		t.Fatal("interruption not bound to the turn that answered it")
	}
	reply := msgs[4].BodyMd
	for _, want := range []string{"interrupted your reply", "> slow partial", "mcp__lazyagents__read_page", "> Halfway there: the ledger balances.", "Also check the payroll", "resumed=true"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply lacks %q:\n%s", want, reply)
		}
	}
}

// Stop drops an interruption that is still waiting: nothing answers it.
func TestChatStopDropsWaitingInterruption(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("slow")
	deadline := time.Now().Add(15 * time.Second)
	for e.chatRun(chat.ID).HarnessSessionID == nil {
		if time.Now().After(deadline) {
			t.Fatal("harness did not start")
		}
		e.runner.Tick(e.ctx)
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "Never mind"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.runs.CancelChat(e.ctx, e.ws, e.me.ID, chat.ID); err != nil {
		t.Fatal(err)
	}
	e.runner.Wait()
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	if c := e.chat(chat.ID); c.State != "idle" || c.InterruptMessageID != nil {
		t.Fatalf("chat %+v", c)
	}
	msgs, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 20})
	if err != nil || len(msgs) != 2 || msgs[1].Role != "user" {
		t.Fatalf("history %+v %v", msgs, err)
	}
}
