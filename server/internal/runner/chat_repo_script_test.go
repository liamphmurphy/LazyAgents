package runner_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/runner"
)

func TestChatRepoSetupScript(t *testing.T) {
	s := runner.ChatRepoSetupScript("https://github.com/acme/app.git", "vesper/pair-1a2b3c4d", "Pair (Vesper)", "pair@personas.lazyagents.local")
	for _, want := range []string{
		`clone --quiet --no-checkout -- 'https://github.com/acme/app.git'`,
		// The earlier segment's pushed work is resumed, else a fresh branch.
		`fetch --quiet origin '+refs/heads/vesper/pair-1a2b3c4d:refs/remotes/origin/vesper/pair-1a2b3c4d'`,
		`checkout --quiet -B 'vesper/pair-1a2b3c4d' 'origin/vesper/pair-1a2b3c4d'`,
		`git checkout --quiet -B 'vesper/pair-1a2b3c4d'` + "\n",
		// The hook refuses every other ref.
		`.git/hooks/pre-push`, `want='\''refs/heads/vesper/pair-1a2b3c4d'\'''`, `chmod +x .git/hooks/pre-push`,
		`git config user.name 'Pair (Vesper)'`, `git config user.email 'pair@personas.lazyagents.local'`,
		`.lazyagents/' >> .git/info/exclude`, `mkdir -p .lazyagents/outbox`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script missing %q:\n%s", want, s)
		}
	}
	// The persistent helper answers for github.com only, never repo-wide.
	if !strings.Contains(s, `git config credential.https://github.com.helper "$helper"`) || strings.Contains(s, "git config credential.helper") {
		t.Errorf("the persistent credential helper must be scoped to github.com:\n%s", s)
	}
	if !strings.Contains(s, `if [ -n "$token" ]; then git -c credential.helper=`) || strings.Contains(s, `token=""`+"\n") {
		t.Errorf("github https must use the git token:\n%s", s)
	}
	// Hosts the persona's owner picked never receive the workspace's token.
	for _, url := range []string{"https://git.example.com/a/b", "git@github.com:a/b", "/tmp/repo"} {
		if s := runner.ChatRepoSetupScript(url, "vesper/x-1", "X (Vesper)", "x@personas.lazyagents.local"); !strings.Contains(s, `token=""`+"\n") {
			t.Errorf("%s may receive the git token", url)
		}
	}
	// Everything is quoted: a hostile branch or name stays inside its quotes.
	evil := runner.ChatRepoSetupScript("https://github.com/a/b'; touch /tmp/x; '", "vesper/x'; rm -rf /; '", "N'; id; '", "e@x")
	for _, want := range []string{
		`-- 'https://github.com/a/b'\''; touch /tmp/x; '\'''`,
		`-B 'vesper/x'\''; rm -rf /; '\'''`,
		`user.name 'N'\''; id; '\'''`,
	} {
		if !strings.Contains(evil, want) {
			t.Errorf("input not shell-quoted, want %q", want)
		}
	}
}

// A token added after the workspace was cloned still reaches git: the
// credential helper is set on every turn, not only by the first clone.
func TestChatRepoSetupScriptAddsLaterToken(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	run := func(token string) string {
		t.Helper()
		cmd := exec.Command("sh", "-c", runner.ChatRepoSetupScript("https://github.com/acme/app.git", "vesper/pair-1", "Pair (Vesper)", "pair@x"))
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GH_TOKEN="+token, "HOME="+dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup script: %v\n%s", err, out)
		}
		out, _ := exec.Command("git", "-C", dir, "config", "--get", "credential.https://github.com.helper").Output()
		return strings.TrimSpace(string(out))
	}
	if got := run(""); got != "" {
		t.Fatalf("helper set without a token: %q", got)
	}
	if got := run("tok"); !strings.Contains(got, "$GH_TOKEN") {
		t.Fatalf("existing workspace didn't get the helper once a token was added: %q", got)
	}
}
