package runner_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/runner"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// bareRepo makes a local origin with one commit.
func bareRepo(t *testing.T) string {
	t.Helper()
	bare, seed := filepath.Join(t.TempDir(), "origin.git"), t.TempDir()
	gitIn(t, seed, "init", "-q", "-b", "main")
	for name, body := range map[string]string{"README.md": "hello\n", "old.txt": "one\ntwo\nthree\nfour\nfive\nsix\n", "gone.txt": "bye\n"} {
		if err := os.WriteFile(filepath.Join(seed, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, seed, "add", ".")
	gitIn(t, seed, "commit", "-qm", "init")
	gitIn(t, filepath.Dir(bare), "clone", "-q", "--bare", seed, bare)
	return bare
}

// lastAssistant is the newest assistant message of a chat.
func (e env) lastAssistant(id uuid.UUID) store.ChatMessage {
	e.t.Helper()
	msgs, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: id, Limit: 200})
	if err != nil {
		e.t.Fatal(err)
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			return store.ChatMessage(msgs[i])
		}
	}
	e.t.Fatal("no assistant message")
	return store.ChatMessage{}
}

func changesOf(t *testing.T, m store.ChatMessage) map[string]runner.CodeChange {
	t.Helper()
	var list []runner.CodeChange
	if err := json.Unmarshal(m.CodeChanges, &list); err != nil {
		t.Fatalf("%v: %s", err, m.CodeChanges)
	}
	out := map[string]runner.CodeChange{}
	for _, c := range list {
		out[c.Path] = c
	}
	if len(out) != len(list) {
		t.Fatalf("duplicate paths: %s", m.CodeChanges)
	}
	return out
}

// A coding chat's reply carries what the turn changed: modified, added
// (committed and untracked), deleted and binary files, without our own files,
// and nothing in the repository's config can run a program while it is read.
func TestCodingChatCodeChanges(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	bare := bareRepo(t)
	p := e.persona("Pair", true, false, "https://github.com/acme/app")
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Code", PersonaID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `WITH attached AS (UPDATE chat_repositories SET repo_url=$2 WHERE chat_id=$1) UPDATE chat_sessions SET repo_url=$2 WHERE id=$1`, chat.ID, bare); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE personas SET repos=ARRAY[$2]::text[] WHERE id=$1`, p.ID, bare); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "ran")
	evil := "touch " + marker
	// The agent's own git use comes first; then it leaves booby traps in the
	// repository config for whatever reads the repository next.
	reply := e.postSh(chat.ID,
		"echo changed >> README.md; sed -i.bak 's/three/3/' old.txt; rm old.txt.bak; printf 'new\\n' > added.txt; echo c > committed.txt; git add committed.txt; git commit -qm c; "+
			"git rm -q gone.txt; printf 'x y\\n' > 'with space.txt'; echo hidden > .lazyagents/secret.txt; printf '\\000\\001bin' > blob.bin; "+
			"git config filter.evil.clean '"+evil+"; cat'; git config filter.evil.process '"+evil+"'; git config diff.external '"+evil+"'; git config core.fsmonitor '"+evil+"'; "+
			"git config diff.evil.textconv '"+evil+"'; printf 'README.md filter=evil diff=evil\\n' > .git/info/attributes")
	if !strings.Contains(reply, "err=<nil>") {
		t.Fatal(reply)
	}
	got := changesOf(t, e.lastAssistant(chat.ID))
	if len(got) != 7 {
		var evs string
		_ = e.pool.QueryRow(e.ctx, `SELECT string_agg(payload::text, E'\n') FROM run_events WHERE run_id=$1 AND payload->>'state' LIKE 'code_changes%'`, e.chatRun(chat.ID).ID).Scan(&evs)
		t.Logf("events: %s", evs)
		t.Fatalf("%d changes: %+v", len(got), got)
	}
	check := func(got map[string]runner.CodeChange, path, status string, add, del int, binary bool, patchHas string) {
		t.Helper()
		c, ok := got[path]
		if !ok || c.Status != status || c.Additions != add || c.Deletions != del || c.Binary != binary || c.Truncated {
			t.Errorf("%s = %+v", path, c)
			return
		}
		if patchHas == "" && c.Patch != nil || patchHas != "" && (c.Patch == nil || !strings.Contains(*c.Patch, patchHas) || !strings.HasPrefix(*c.Patch, "@@")) {
			t.Errorf("%s patch = %v, want it to contain %q", path, c.Patch, patchHas)
		}
	}
	check(got, "README.md", "modified", 1, 0, false, "+changed")
	check(got, "old.txt", "modified", 1, 1, false, "+3")
	check(got, "added.txt", "added", 1, 0, false, "+new")
	check(got, "committed.txt", "added", 1, 0, false, "+c")
	check(got, "gone.txt", "deleted", 0, 1, false, "-bye")
	check(got, "with space.txt", "added", 1, 0, false, "+x y")
	check(got, "blob.bin", "added", 0, 0, true, "")
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the repository's config ran a program while reading the changes")
	}

	// The next turn is measured from its own starting commit: the earlier
	// commit isn't reported again, uncommitted leftovers still are.
	e.postSh(chat.ID, "echo more >> committed.txt")
	got = changesOf(t, e.lastAssistant(chat.ID))
	check(got, "committed.txt", "modified", 1, 0, false, "+more")
	check(got, "gone.txt", "deleted", 0, 1, false, "-bye")
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the repository's config ran a program on the second turn")
	}

	// Commit everything; a following turn that changes nothing reports nothing.
	commitReply := e.postSh(chat.ID, "git -c core.fsmonitor=false -c filter.evil.process= -c filter.evil.clean= add -A && git -c core.fsmonitor=false -c filter.evil.process= -c filter.evil.clean= commit -qm all")
	if !strings.Contains(commitReply, "err=<nil>") {
		t.Fatal(commitReply)
	}
	msg := e.say(chat.ID, "sh: true")
	if string(msg.CodeChanges) != "[]" {
		t.Fatalf("an unchanged turn reported %s", msg.CodeChanges)
	}
}

// A turn in a chat without a repository never touches git.
func TestChatWithoutRepoHasNoCodeChanges(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("hi")
	e.until("reply", func() bool { return e.chat(chat.ID).State == "idle" })
	if msg := e.lastAssistant(chat.ID); string(msg.CodeChanges) != "[]" {
		t.Fatalf("code_changes %s", msg.CodeChanges)
	}
}

// A persona that can code but lists no repository still writes and runs
// code, in an empty workspace with nothing to push to. A chat whose
// repository the persona stopped listing gets no shell at all.
func TestCodingChatWithoutRepoUsesScratchWorkspace(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	p := e.persona("Scratch", true, false)
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "scratch", PersonaID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if chat.RepoUrl != nil {
		t.Fatalf("a persona with no repos gave the chat repo %q", *chat.RepoUrl)
	}
	reply := e.postSh(chat.ID, "printf 'echo $((6*7))' > calc.sh && sh calc.sh; test -d .git || echo no-checkout")
	for _, want := range []string{"42", "no-checkout"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("scratch workspace: missing %q in %q", want, reply)
		}
	}
	spec, _ := runs.Spec(e.chatRun(chat.ID))
	for _, scope := range []string{"repo:read", "repo:write", "tests:run"} {
		if spec.Mode(scope) != agentdef.Allowed {
			t.Errorf("scratch chat: %s is %s", scope, spec.Mode(scope))
		}
	}
	if spec.HarnessOptions["sandbox"] != "workspace-write" || spec.HarnessOptions["readonly"] != "" || !strings.Contains(spec.Instructions, "use_repository") {
		t.Fatalf("scratch chat spec: %v", spec.HarnessOptions)
	}

	// List membership is covered by the runs boundary test; disabling code
	// remains the revocation mechanism.
}

func TestCodingChatGitCredentials(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	bare := bareRepo(t)
	// Git is pointed at the local origin for the URLs below (inherited by the fake runs).
	urls := map[string]string{"https://github.com/acme/app": "gh-https", "git@github.com:acme/app": "gh-ssh", "https://git.example.com/acme/app": "other-https"}
	t.Setenv("GIT_CONFIG_COUNT", "3")
	i := 0
	for u := range urls {
		t.Setenv("GIT_CONFIG_KEY_"+string(rune('0'+i)), "url."+bare+".insteadOf")
		t.Setenv("GIT_CONFIG_VALUE_"+string(rune('0'+i)), u)
		i++
	}
	sec := e.runner.Secrets.(*secrets.Service)
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "test")
	if err != nil {
		t.Fatal(err)
	}
	for name, grants := range map[string]string{"git:pat": "", "git:ssh": "", "harness:anthropic": "", "ci:token": "tests:run", "db:staging": "", "deploy:key": "repo:read"} {
		value := "fake-secret-" + name
		if name == "git:ssh" {
			value = string(pem.EncodeToMemory(block))
		}
		if _, err := sec.Set(e.ctx, e.ws, e.me.ID, secrets.SetInput{Name: name, Value: value, GrantsScope: grants}); err != nil {
			t.Fatal(err)
		}
	}
	envNames := func(url string) string {
		p := e.persona("P"+urls[url], true, false, url)
		chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: urls[url], PersonaID: &p.ID})
		if err != nil {
			t.Fatal(err)
		}
		return e.postSh(chat.ID, "env | cut -d= -f1 | sort | tr '\\n' ' '; ls ${LAZYAGENTS_SSH_DIR:-.lazyagents/ssh}")
	}
	has := func(reply, name string) bool {
		return strings.Contains(" "+strings.ReplaceAll(reply, "\n", " ")+" ", " "+name+" ")
	}
	for url, want := range map[string]struct{ pat, ssh bool }{
		"https://github.com/acme/app":      {true, true},
		"git@github.com:acme/app":          {true, true},
		"https://git.example.com/acme/app": {true, true},
	} {
		reply := envNames(url)
		if !has(reply, "ANTHROPIC_API_KEY") {
			t.Errorf("%s: harness key missing: %s", url, reply)
		}
		for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GIT_PAT"} {
			if has(reply, name) != want.pat {
				t.Errorf("%s: %s present=%v, want %v: %s", url, name, has(reply, name), want.pat, reply)
			}
		}
		// The env var and the key file the setup script writes from it.
		if has(reply, runner.SSHKeyEnv) != want.ssh || has(reply, "id") != want.ssh {
			t.Errorf("%s: ssh key present=%v file=%v, want %v: %s", url, has(reply, runner.SSHKeyEnv), has(reply, "id"), want.ssh, reply)
		}
		for _, name := range []string{"CI_TOKEN", "DB_STAGING", "DEPLOY_KEY"} {
			if has(reply, name) {
				t.Errorf("%s: unrelated secret %s reached the chat: %s", url, name, reply)
			}
		}
	}
	// Repository-less coding chats can attach a repository in the same turn.
	p := e.persona("Scratch", true, false)
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "none", PersonaID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	reply := e.postSh(chat.ID, "env | cut -d= -f1 | sort | tr '\\n' ' '")
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GIT_PAT", runner.SSHKeyEnv} {
		if !has(reply, name) {
			t.Errorf("no-repo coding chat lacks %s: %s", name, reply)
		}
	}
}

func TestChatSecretPlanAndScrub(t *testing.T) {
	spec := agentdef.Resolved{Harness: agentdef.HarnessCodex}
	plan := []runner.SecretDecision{
		{Name: "harness:openai", Injected: true}, {Name: "git:pat", Injected: true}, {Name: "git:ssh", Injected: true},
		{Name: "ci:token", Injected: true}, {Name: "db:prod", Injected: true},
	}
	var names []string
	for _, d := range runner.ChatSecretPlan(plan, spec) {
		names = append(names, d.Name)
	}
	if strings.Join(names, ",") != "harness:openai,git:pat,git:ssh" || len(plan) != 5 {
		t.Fatalf("plan %v", names)
	}
	mk := func() map[string]string {
		return map[string]string{"GH_TOKEN": "a", "GITHUB_TOKEN": "a", "GIT_PAT": "a", runner.SSHKeyEnv: "k", "CODEX_API_KEY": "c"}
	}
	for url, want := range map[string]string{
		"https://github.com/a/b":         "CODEX_API_KEY,GH_TOKEN,GITHUB_TOKEN,GIT_PAT",
		"git@github.com:a/b":             "CODEX_API_KEY," + runner.SSHKeyEnv,
		"ssh://git@github.com/a/b":       "CODEX_API_KEY," + runner.SSHKeyEnv,
		"https://gitlab.com/a/b":         "CODEX_API_KEY",
		"https://github.com.evil.io/a/b": "CODEX_API_KEY",
		"":                               "CODEX_API_KEY",
	} {
		env := mk()
		runner.ScrubChatGitEnv(env, url)
		var got []string
		for k := range env {
			got = append(got, k)
		}
		if g := sortedJoin(got); g != want {
			t.Errorf("%q: env %s, want %s", url, g, want)
		}
	}
}

func sortedJoin(s []string) string {
	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
	return strings.Join(s, ",")
}
