package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The wrapper must git-exclude skill files before the harness runs, so a
// turn's `git add -A` can't commit them.
func TestTurnWrapperExcludesSkills(t *testing.T) {
	dir := t.TempDir()
	run := func(name string, args ...string) string {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
		return string(out)
	}
	write := func(p, s string) {
		p = filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("git", "init", "-q", ".")
	write(".lazyagents/agents-section.md", "ours\n")
	write(".lazyagents/skills-exclude.txt", "/.claude/skills/go-errors/\n/.agents/skills/go-errors/\n")
	write(".claude/skills/go-errors/SKILL.md", "x")
	write(".agents/skills/go-errors/SKILL.md", "x")
	write("code.go", "package x")
	argv := append(append([]string{}, TurnWrapper...), "true") // "true" stands in for the harness
	run(argv[0], argv[1:]...)
	run(argv[0], argv[1:]...) // idempotent
	ex, _ := os.ReadFile(filepath.Join(dir, ".git/info/exclude"))
	if strings.Count(string(ex), "/.claude/skills/go-errors/") != 1 {
		t.Fatalf("exclude file:\n%s", ex)
	}
	run("git", "add", "-A")
	if staged := run("git", "diff", "--cached", "--name-only"); strings.Contains(staged, ".claude") || strings.Contains(staged, ".agents/skills") || !strings.Contains(staged, "code.go") {
		t.Fatalf("staged:\n%s", staged)
	}
}
