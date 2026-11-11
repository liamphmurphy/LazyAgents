package runner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func TestChatSwitchesRepositoriesAndRecoversBothCheckouts(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	// Rewrite GitHub URLs to independent local origins, exercising real clones,
	// pushes and branch hooks without using network or real credentials.
	a, b := bareRepo(t), bareRepo(t)
	config := filepath.Join(t.TempDir(), "gitconfig")
	for url, bare := range map[string]string{"https://github.com/acme/alpha.git": a, "https://github.com/acme/beta.git": b} {
		out, err := exec.Command("git", "config", "--file", config, "url."+bare+".insteadOf", url).CombinedOutput()
		if err != nil {
			t.Fatalf("rewrite: %v %s", err, out)
		}
	}
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	p := e.persona("Multi repo", true, false)
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{PersonaID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	selectRepo := func(name string) {
		t.Helper()
		if _, err := e.runs.SetChatRepository(e.ctx, e.ws, e.me.ID, chat.ID, name, nil); err != nil {
			t.Fatal(err)
		}
	}
	selectRepo("acme/alpha")
	alpha := ".lazyagents/repos/acme/alpha"
	beta := ".lazyagents/repos/acme/beta"
	reply := e.postSh(chat.ID, "cd "+alpha+" && git checkout -q -b feat/alpha-work && echo alpha > same.txt && git add same.txt && git commit -qm alpha && git push -q -u origin feat/alpha-work && echo alpha-unsaved > pending.txt")
	if !strings.Contains(reply, "err=<nil>") {
		t.Fatal(reply)
	}
	first := e.chatRun(chat.ID)
	selectRepo("acme/beta")
	reply = e.postSh(chat.ID, "cd "+beta+" && git checkout -q -b fix/beta-work && echo beta > same.txt && git add same.txt && git commit -qm beta && git push -q -u origin fix/beta-work && echo beta-unsaved > pending.txt")
	if !strings.Contains(reply, "err=<nil>") {
		t.Fatal(reply)
	}
	if e.chatRun(chat.ID).ID != first.ID {
		t.Fatal("switching replaced the chat segment")
	}
	rows, err := e.q.ChatRepositories(e.ctx, chat.ID)
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	for _, repo := range rows {
		if len(repo.WorkBranches) != 1 {
			t.Fatalf("repository branches: %+v", repo)
		}
	}
	selectRepo("acme/alpha")
	reply = e.postSh(chat.ID, "cat "+alpha+"/pending.txt "+beta+"/pending.txt; cd "+alpha+" && git push origin HEAD:main 2>&1")
	if !strings.Contains(reply, "alpha-unsaved") || !strings.Contains(reply, "beta-unsaved") || !strings.Contains(reply, "isn't one") {
		t.Fatal(reply)
	}
	// Every reported file names its repository, including files with equal paths.
	msg := e.lastAssistant(chat.ID)
	if !strings.Contains(string(msg.CodeChanges), "https://github.com/acme/alpha.git") || !strings.Contains(string(msg.CodeChanges), "https://github.com/acme/beta.git") {
		t.Fatalf("repository diffs: %s", msg.CodeChanges)
	}
	e.endIdleChat()
	rows, err = e.q.ChatRepositories(e.ctx, chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, repo := range rows {
		if repo.CheckpointBranch == nil {
			t.Fatalf("missing recovery snapshot: %+v", repo)
		}
	}
	if _, err := os.Stat(e.runner.Backend.WorkspaceDir(first.ID.String())); !os.IsNotExist(err) {
		t.Fatal("checkpointed workspace was not removed", err)
	}
	reply = e.postSh(chat.ID, "cat "+alpha+"/pending.txt "+beta+"/pending.txt")
	if !strings.Contains(reply, "alpha-unsaved") || !strings.Contains(reply, "beta-unsaved") {
		t.Fatalf("recovery lost work: %s", reply)
	}
	if e.chatRun(chat.ID).ID == first.ID {
		t.Fatal("idle recovery reused the deleted workspace")
	}
	// Conversation history stays attached to the original chat.
	messages, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 100})
	if err != nil || len(messages) < 8 {
		t.Fatal(len(messages), err)
	}
}
