package runner_test

import (
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
