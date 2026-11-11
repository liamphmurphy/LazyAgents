package runner_test

import (
	"strings"
	"testing"

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
	aside, err := e.runs.StartChatAside(e.ctx, e.ws, e.me.ID, chat.ID, "how far along are you?")
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
