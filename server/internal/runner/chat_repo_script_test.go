package runner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/runner"
)

func TestChatRepoSetupScript(t *testing.T) {
	s := runner.ChatRepoSetupScript("https://github.com/acme/app.git", []string{"vesper/pair-1a2b3c4d", "feat/add-search"}, "Ada Lovelace", "ada@example.com")
	for _, want := range []string{
		`clone --quiet --no-checkout -- 'https://github.com/acme/app.git'`,
		// The hook lets the chat push the branches it made, and new ones it names.
		`.git/hooks/pre-push`, `known='\''refs/heads/vesper/pair-1a2b3c4d refs/heads/feat/add-search'\''`,
		`pattern='\''^(feat|fix|chore|docs|refactor|test|perf|build|ci|style)/`, `chmod +x .git/hooks/pre-push`,
		`git config user.name 'Ada Lovelace'`, `git config user.email 'ada@example.com'`,
		`.lazyagents/' >> .git/info/exclude`, `mkdir -p .lazyagents/outbox`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script missing %q:\n%s", want, s)
		}
	}
	// No branch is made up front: the clone stays on the default branch.
	if strings.Contains(s, "git checkout") || strings.Contains(s, "git switch") {
		t.Errorf("setup checks out a branch:\n%s", s)
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
		if s := runner.ChatRepoSetupScript(url, nil, "X (Vesper)", "x@personas.lazyagents.local"); !strings.Contains(s, `token=""`+"\n") {
			t.Errorf("%s may receive the git token", url)
		}
	}
	// Everything is quoted: a hostile repo, branch or name stays inside its quotes.
	evil := runner.ChatRepoSetupScript("https://github.com/a/b'; touch /tmp/x; '", []string{"vesper/x'; rm -rf /; '"}, "N'; id; '", "e@x")
	for _, want := range []string{
		`-- 'https://github.com/a/b'\''; touch /tmp/x; '\'''`,
		`user.name 'N'\''; id; '\'''`,
	} {
		if !strings.Contains(evil, want) {
			t.Errorf("input not shell-quoted, want %q", want)
		}
	}
	dir := gitInit(t)
	runScript(t, dir, evil, "")
	hook, err := os.ReadFile(filepath.Join(dir, ".git", "hooks", "pre-push"))
	if err != nil || !strings.Contains(string(hook), "\nknown='refs/heads/vesper/x'\\''; rm -rf /; '\\'''\n") {
		t.Fatalf("hook doesn't keep the branch quoted (%v):\n%s", err, hook)
	}
	if out, err := exec.Command("sh", "-n", filepath.Join(dir, ".git", "hooks", "pre-push")).CombinedOutput(); err != nil {
		t.Fatalf("hook isn't valid sh: %v\n%s", err, out)
	}
}

// A token added after the workspace was cloned still reaches git: the
// credential helper is set on every turn, not only by the first clone.
func TestChatRepoSetupScriptAddsLaterToken(t *testing.T) {
	dir := gitInit(t)
	run := func(token string) string {
		t.Helper()
		runScript(t, dir, runner.ChatRepoSetupScript("https://github.com/acme/app.git", nil, "Pair (Vesper)", "pair@x"), token)
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

// A workspace cloned under an earlier author identity commits as the current
// one: the identity is set on every turn, not only by the first clone.
func TestChatRepoSetupScriptUpdatesAuthor(t *testing.T) {
	dir := gitInit(t)
	for _, c := range [][]string{{"user.name", "Pair (Vesper)"}, {"user.email", "pair@personas.lazyagents.local"}} {
		if out, err := exec.Command("git", "-C", dir, "config", c[0], c[1]).CombinedOutput(); err != nil {
			t.Fatalf("git config: %v\n%s", err, out)
		}
	}
	runScript(t, dir, runner.ChatRepoSetupScript("https://github.com/acme/app.git", nil, "Ada Lovelace", "ada@example.com"), "")
	for key, want := range map[string]string{"user.name": "Ada Lovelace", "user.email": "ada@example.com"} {
		out, _ := exec.Command("git", "-C", dir, "config", "--get", key).Output()
		if got := strings.TrimSpace(string(out)); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func gitInit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func runScript(t *testing.T, dir, script, token string) {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GH_TOKEN="+token, "HOME="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("setup script: %v\n%s", err, out)
	}
}

// The pre-push hook, run for real against a local origin: new TYPE/... branches
// and the chat's earlier ones go through and new ones are recorded; the
// default branch, misnamed branches, tags, someone else's branch and deletes
// are refused.
func TestChatPrePushHook(t *testing.T) {
	seed := gitInit(t)
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@x"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(seed, "add", ".")
	git(seed, "commit", "-qm", "init")
	git(seed, "branch", "vesper/pair-1234abcd")
	git(seed, "branch", "feat/theirs")
	bare := filepath.Join(t.TempDir(), "origin.git")
	git(seed, "clone", "-q", "--bare", seed, bare)

	ws := t.TempDir()
	runScript(t, ws, runner.ChatRepoSetupScript(bare, []string{"vesper/pair-1234abcd"}, "Pair (Vesper)", "pair@x"), "")
	if got := strings.TrimSpace(git(ws, "rev-parse", "--abbrev-ref", "HEAD")); got != "main" {
		t.Fatalf("workspace starts on %q, want the default branch", got)
	}
	push := func(args ...string) error {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", ws, "push", "-q", "origin"}, args...)...).CombinedOutput()
		if err != nil {
			return &pushError{string(out)}
		}
		return nil
	}
	git(ws, "commit", "-q", "--allow-empty", "-m", "work")
	for _, ok := range [][]string{{"HEAD:refs/heads/feat/add-search"}, {"HEAD:refs/heads/fix/typo"}, {"HEAD:refs/heads/vesper/pair-1234abcd"}} {
		if err := push(ok...); err != nil {
			t.Fatalf("push %v refused: %v", ok, err)
		}
	}
	git(ws, "commit", "-q", "--allow-empty", "-m", "more")
	if err := push("HEAD:refs/heads/feat/add-search"); err != nil {
		t.Fatalf("a branch the chat made refused later: %v", err)
	}
	if got := strings.Fields(git(ws, "config", "--get-all", runner.ChatBranchConfig)); strings.Join(got, " ") != "feat/add-search fix/typo" {
		t.Fatalf("recorded %v", got)
	}
	for refspec, why := range map[string]string{
		"HEAD:refs/heads/main":        "isn't one",
		"HEAD:refs/heads/other":       "isn't one",
		"HEAD:refs/heads/feat/Bad":    "isn't one",
		"HEAD:refs/tags/v1":           "isn't one",
		"HEAD:refs/heads/feat/theirs": "wasn't made in this chat",
		":refs/heads/fix/typo":        "may not delete",
	} {
		if err := push(refspec); err == nil || !strings.Contains(err.Error(), why) {
			t.Errorf("push %s: %v, want refused (%s)", refspec, err, why)
		}
	}
	if got := git(bare, "for-each-ref", "--format=%(refname) %(subject)"); strings.Contains(got, "other") || strings.Contains(got, "v1") ||
		!strings.Contains(got, "refs/heads/main init") || !strings.Contains(got, "refs/heads/feat/theirs init") || !strings.Contains(got, "refs/heads/fix/typo") {
		t.Fatalf("a refused push landed on origin:\n%s", got)
	}
}

type pushError struct{ out string }

func (e *pushError) Error() string { return e.out }
