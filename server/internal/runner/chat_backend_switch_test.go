package runner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func (e env) hasSystemMessage(id uuid.UUID, want string) bool {
	e.t.Helper()
	msgs, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: id, Limit: 100})
	if err != nil {
		e.t.Fatal(err)
	}
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.BodyMd, want) {
			return true
		}
	}
	return false
}

func (e env) checkContinuedFresh(chat store.ChatSession, first store.Run, reply store.ChatMessage) {
	e.t.Helper()
	second := e.chatRun(chat.ID)
	if second.ID == first.ID || *reply.RunID != second.ID {
		e.t.Fatalf("second segment %+v reply %+v", second, reply)
	}
	for _, want := range []string{"resumed=false", "Message one", "Message two"} {
		if !strings.Contains(reply.BodyMd, want) {
			e.t.Errorf("reply missing %q in %s", want, reply.BodyMd)
		}
	}
	if !e.hasSystemMessage(chat.ID, "continued in a new session") {
		e.t.Error("the chat doesn't say it moved to a new session")
	}
	if old := e.runByID(first.ID); old.State != "completed" {
		e.t.Errorf("old segment %+v", old)
	}
}

// A chat whose workspace is on the other backend carries on from the
// conversation so far, without trying to run where it can't.
func TestChatWithWorkspaceElsewhereContinues(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Message one")
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	first := e.chatRun(chat.ID)
	if err := os.RemoveAll(e.runner.Backend.WorkspaceDir(first.ID.String())); err != nil {
		t.Fatal(err)
	}
	e.checkContinuedFresh(chat, first, e.say(chat.ID, "Message two"))
}

// A session the CLI can't resume, reported only on stderr, does the same.
func TestChatWithLostSessionContinues(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Message one")
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	first := e.chatRun(chat.ID)
	t.Setenv("FAKE_HARNESS_RESUME", "gone")
	e.checkContinuedFresh(chat, first, e.say(chat.ID, "Message two"))
}

// A runner leaves alone a workspace that isn't on its backend, so the runner
// that holds it can still save it.
func TestSweepSkipsWorkspaceOnOtherBackend(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Hello")
	e.until("reply", func() bool { return e.chat(chat.ID).State == "idle" })
	run := e.chatRun(chat.ID)
	if err := os.RemoveAll(e.runner.Backend.WorkspaceDir(run.ID.String())); err != nil {
		t.Fatal(err)
	}
	e.runner.Cfg.ChatIdleTTL = time.Millisecond
	time.Sleep(20 * time.Millisecond)
	e.runner.Sweep(e.ctx)
	e.runner.Sweep(e.ctx)
	if r := e.runByID(run.ID); r.State != "completed" || r.WorkspaceVolume == nil {
		t.Fatalf("run %+v", r)
	}
}

func (e env) endIdleChat() {
	e.t.Helper()
	e.runner.Cfg.ChatIdleTTL = time.Millisecond
	time.Sleep(20 * time.Millisecond)
	e.runner.Sweep(e.ctx)
	e.runner.Sweep(e.ctx)
	e.runner.Cfg.ChatIdleTTL = 0
}

// Files a chat made with no repository to push to stay on disk.
func TestIdleChatKeepsFilesItCannotSave(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Hello")
	e.until("reply", func() bool { return e.chat(chat.ID).State == "idle" })
	run := e.chatRun(chat.ID)
	file := filepath.Join(e.runner.Backend.WorkspaceDir(run.ID.String()), "notes.txt")
	if err := os.WriteFile(file, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.endIdleChat()
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("the file was deleted: %v", err)
	}
	if r := e.runByID(run.ID); r.WorkspaceVolume != nil {
		t.Fatalf("the sweep would retry this forever: %+v", r)
	}
}

func (e env) codingChat(bare string) store.ChatSession {
	e.t.Helper()
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		e.t.Fatal(err)
	}
	p := e.persona("Pair", true, false, "https://github.com/acme/app")
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Code", PersonaID: &p.ID})
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE chat_sessions SET repo_url=$2 WHERE id=$1`, chat.ID, bare); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE personas SET repos=ARRAY[$2]::text[] WHERE id=$1`, p.ID, bare); err != nil {
		e.t.Fatal(err)
	}
	return chat
}

// A coding chat's unpushed work reaches a checkpoint branch before its
// workspace goes.
func TestIdleCodingChatCheckpointsUnpushedWork(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	bare := bareRepo(t)
	chat := e.codingChat(bare)
	e.postSh(chat.ID, "echo unsaved > work.txt")
	run := e.chatRun(chat.ID)
	e.endIdleChat()
	branch := "vesper-checkpoint/chat/" + run.ID.String()
	out, err := exec.Command("git", "-C", bare, "show", branch+":work.txt").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "unsaved" {
		t.Fatalf("checkpoint: %v: %s", err, out)
	}
	if _, err := os.Stat(e.runner.Backend.WorkspaceDir(run.ID.String())); !os.IsNotExist(err) {
		t.Fatalf("workspace still there after a saved checkpoint: %v", err)
	}
}

// If the work can't be pushed, the workspace stays.
func TestIdleCodingChatKeepsWorkspaceWhenPushFails(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	bare := bareRepo(t)
	chat := e.codingChat(bare)
	e.postSh(chat.ID, "echo unsaved > work.txt")
	run := e.chatRun(chat.ID)
	if err := os.RemoveAll(bare); err != nil {
		t.Fatal(err)
	}
	e.endIdleChat()
	if _, err := os.Stat(filepath.Join(e.runner.Backend.WorkspaceDir(run.ID.String()), "work.txt")); err != nil {
		t.Fatalf("the work was deleted: %v", err)
	}
	if r := e.runByID(run.ID); r.WorkspaceVolume == nil || r.CheckpointError == nil {
		t.Fatalf("run %+v", r)
	}
}
