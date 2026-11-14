package runner_test

import (
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// An aside is answered while the reply it asks about keeps running, from a
// session that saw what the reply had written; it then ends its segment.
func TestChatAsideAnswersDuringReply(t *testing.T) {
	e := setup(t)
	chat, release := e.holdChat()
	defer release()
	e.tickUntil("the reply's opening text", func() bool {
		var n int
		_ = e.pool.QueryRow(e.ctx, `SELECT count(*) FROM run_events WHERE run_id=$1 AND type='message'`, e.chatRun(chat.ID).ID).Scan(&n)
		return n > 0
	})
	aside, err := e.runs.StartChatAside(e.ctx, e.ws, e.me.ID, chat.ID, "how far along are you?", nil)
	if err != nil {
		t.Fatal(err)
	}
	e.tickUntil("the aside's answer", func() bool { return e.chat(aside.ID).State == "idle" })
	msgs := e.messages(aside.ID)
	if got := roles(msgs); got != "user,assistant" {
		t.Fatalf("aside history %s", got)
	}
	for _, want := range []string{"this is an aside", "Working on it.", "how far along are you?"} {
		if !strings.Contains(msgs[1].BodyMd, want) {
			t.Fatalf("aside turn didn't see %q:\n%s", want, msgs[1].BodyMd)
		}
	}
	if run, err := e.q.GetRun(e.ctx, store.GetRunParams{ID: *msgs[1].RunID, WorkspaceID: e.ws}); err != nil || run.State != "completed" {
		t.Fatalf("aside run %s, %v", run.State, err)
	}
	if c := e.chat(chat.ID); c.State != "running" {
		t.Fatalf("the aside stopped the reply: %s", c.State)
	}
	if got := roles(e.messages(chat.ID)); got != "user" {
		t.Fatalf("the aside reached the conversation: %s", got)
	}
}

// A thread on a finished message answers while nothing runs, waits for the
// person's next reply instead of ending, and answers that too.
func TestChatThreadKeepsAnswering(t *testing.T) {
	e := setup(t)
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Message: &runs.ChatMessageInput{Body: "compare kokoro and piper"}})
	if err != nil {
		t.Fatal(err)
	}
	e.tickUntil("the chat's reply", func() bool { return e.chat(chat.ID).State == "idle" })
	reply := e.messages(chat.ID)[1]

	thread, err := e.runs.StartChatAside(e.ctx, e.ws, e.me.ID, chat.ID, "why these two?", &reply.ID)
	if err != nil {
		t.Fatal(err)
	}
	e.tickUntil("the thread's answer", func() bool { return e.chat(thread.ID).State == "idle" })
	if run, err := e.q.GetRun(e.ctx, store.GetRunParams{ID: *e.chat(thread.ID).CurrentRunID, WorkspaceID: e.ws}); err != nil || run.State == "completed" {
		t.Fatalf("thread run %s, %v: it should wait for the next reply", run.State, err)
	}
	if _, err := e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, thread.ID, runs.ChatMessageInput{Body: "and for long texts?"}); err != nil {
		t.Fatal(err)
	}
	e.tickUntil("the thread's second answer", func() bool {
		return strings.HasPrefix(roles(e.messages(thread.ID)), "user,assistant,user,assistant") && e.chat(thread.ID).State == "idle"
	})
	if got := roles(e.messages(chat.ID)); got != "user,assistant" {
		t.Fatalf("the thread reached the conversation: %s", got)
	}
}
