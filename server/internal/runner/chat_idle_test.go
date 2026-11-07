package runner_test

import (
	"strings"
	"testing"
	"time"
)

// A chat segment that waits on the person past ChatIdleTTL is ended and its
// workspace freed; the chat's next message starts a new segment with the
// conversation so far as a handoff.
func TestIdleChatSegmentEndsAndFreesWorkspace(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Message one")
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	first := e.chatRun(chat.ID)
	if first.State != "waiting" || first.WorkspaceVolume == nil {
		t.Fatalf("first segment %+v", first)
	}

	// Within the TTL nothing changes.
	e.runner.Sweep(e.ctx)
	if r := e.runByID(first.ID); r.State != "waiting" || r.WorkspaceVolume == nil {
		t.Fatalf("a fresh idle segment was ended: %+v", r)
	}

	e.runner.Cfg.ChatIdleTTL = time.Millisecond
	time.Sleep(20 * time.Millisecond)
	e.runner.Sweep(e.ctx) // ends the segment
	e.runner.Sweep(e.ctx) // removes its workspace
	old := e.runByID(first.ID)
	if old.State != "completed" || old.WorkspaceVolume != nil || old.EndedAt == nil {
		t.Fatalf("idle segment %+v", old)
	}
	if c := e.chat(chat.ID); c.CurrentRunID != nil || c.State != "idle" {
		t.Fatalf("chat after idle end %+v", c)
	}
	if !strings.Contains(e.eventTypes(first.ID), "lifecycle") {
		t.Fatal("no lifecycle event for the ended segment")
	}

	e.runner.Cfg.ChatIdleTTL = 0
	reply := e.say(chat.ID, "Message two")
	second := e.chatRun(chat.ID)
	if second.ID == first.ID || *reply.RunID != second.ID {
		t.Fatalf("second segment %+v reply %+v", second, reply)
	}
	for _, want := range []string{"resumed=false", "Message one", "Message two"} {
		if !strings.Contains(reply.BodyMd, want) {
			t.Errorf("handoff prompt missing %q in %s", want, reply.BodyMd)
		}
	}
}

// Deleting a chat cancels its segment; the sweep still removes the scratch
// workspace (unlike a cancelled ticket run, a chat has no unpushed work).
func TestDeletedChatWorkspaceIsSwept(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Hello")
	e.until("reply", func() bool { return e.chat(chat.ID).State == "idle" })
	run := e.chatRun(chat.ID)
	if err := e.runs.DeleteChat(e.ctx, e.ws, e.me.ID, chat.ID); err != nil {
		t.Fatal(err)
	}
	e.runner.Sweep(e.ctx)
	if r := e.runByID(run.ID); r.State != "cancelled" || r.WorkspaceVolume != nil {
		t.Fatalf("deleted chat's run %+v", r)
	}
}
