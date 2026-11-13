package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The map is read from the commit itself: a file changed in the working tree
// afterwards doesn't show, and the map names its commit.
func TestRepoMapIsBuiltFromTheCommit(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=T", "-c", "user.email=t@example.com"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write("README.md", "# Thing\n\n<img src=x>\n\nThing tracks widgets. It has more to say.\n")
	write("Makefile", "VAR := 1\n.PHONY: test\nbuild: deps\n\tgo build\ntest:\n\tgo test ./...\n")
	write("server/go.mod", "module example.com/thing\n")
	write("server/cmd/thing/main.go", "package main\n")
	write("server/internal/store/doc.go", "// Package store reads and writes widgets. Postgres only.\npackage store\n")
	write("web/package.json", `{"description": "The web app", "scripts": {"dev": "next dev", "test": "vitest"}}`)
	write("web/node_modules/x/README.md", "vendored\n")
	write("AGENTS.md", "rules\n")
	git("add", ".")
	git("commit", "-qm", "init")
	commit := git("rev-parse", "HEAD")
	write("README.md", "# Changed after the commit\n\nNot this.\n")

	cmd := exec.Command("sh", "-c", repoMapScript(commit))
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	in := parseRepoMapOutput(string(out))
	m := buildRepoMap("github.com/acme/thing", "main", commit, time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC), in)
	for _, want := range []string{
		"# Repo map: github.com/acme/thing",
		"commit `" + commit + "` (`main`) on 2026-11-13 09:00 UTC",
		"README: Thing tracks widgets.",
		"`web/` (1 files): The web app",
		"`server/internal/store/` (1): Package store reads and writes widgets.",
		"Go commands: `server/cmd/thing`",
		"`make` targets (Makefile): `build`, `test`",
		"npm scripts (web/package.json): `dev`, `test`",
		"Go module in `server/`",
		"`AGENTS.md`",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("map lacks %q:\n%s", want, m)
		}
	}
	for _, not := range []string{"Not this", "node_modules", "VAR", "`.PHONY`"} {
		if strings.Contains(m, not) {
			t.Errorf("map has %q:\n%s", not, m)
		}
	}
	if len(m) > repoMapBudget+100 {
		t.Errorf("map is %d bytes", len(m))
	}
}

// A file can't forge a section marker: content lines are prefixed.
func TestRepoMapOutputCantBeForged(t *testing.T) {
	in := parseRepoMapOutput(mapTree + "\n|a.go\n" + mapFile + "README.md\n|" + mapFile + "evil\n|" + mapTree + "\n|hi\n")
	if len(in.tree) != 1 || len(in.files) != 1 || in.files["README.md"] != mapFile+"evil\n"+mapTree+"\nhi\n" {
		t.Fatalf("%+v", in)
	}
}

func TestTakeBaseCommit(t *testing.T) {
	sha := strings.Repeat("ab", 20)
	log, commit, ref := takeBaseCommit("→ cloning\n" + baseCommitPrefix + sha + " origin/epic/x\n→ ready")
	if log != "→ cloning\n→ ready" || commit != sha || ref != "epic/x" {
		t.Fatalf("%q %q %q", log, commit, ref)
	}
	if _, commit, _ := takeBaseCommit(baseCommitPrefix + "$(rm -rf /) origin/main"); commit != "" {
		t.Fatalf("accepted %q", commit)
	}
}

// The base is the epic branch when it's on origin, then the base branch.
func TestBaseCommitScriptPrefersTheEpicBranch(t *testing.T) {
	s := baseCommitScript("main", "epic/x")
	if !strings.Contains(s, "for ref in 'origin/epic/x' 'origin/main' 'origin/HEAD';") {
		t.Fatal(s)
	}
	if s := baseCommitScript("", ""); !strings.Contains(s, "for ref in 'origin/HEAD';") {
		t.Fatal(s)
	}
}

func TestRepoMapCacheEvictsOldest(t *testing.T) {
	var c repoMapCache
	for i := 0; i <= repoMapCacheSize; i++ {
		c.put(string(rune('a'+i%26))+strings.Repeat("x", i), "m")
	}
	if _, ok := c.get("a"); ok || len(c.maps) != repoMapCacheSize {
		t.Fatalf("%d entries", len(c.maps))
	}
}
