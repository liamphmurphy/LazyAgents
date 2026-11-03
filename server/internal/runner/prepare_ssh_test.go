package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupScriptSSH(t *testing.T) {
	script := SetupScriptForBranch("", "", "feat", "A", "a@x", nil)
	for _, want := range []string{"StrictHostKeyChecking=yes", "known_hosts", "core.sshCommand"} {
		if !strings.Contains(script, want) {
			t.Fatalf("setup script missing %q", want)
		}
	}
	dir, err := filepath.EvalSymlinks(t.TempDir()) // $PWD in the script is the resolved path
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), SSHKeyEnv+"=KEYDATA")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if st, err := os.Stat(dir + "/.lazyagents/ssh/id"); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("key file: %v %v", st, err)
	}
	cfg, _ := exec.Command("git", "-C", dir, "config", "core.sshCommand").Output()
	if !strings.Contains(string(cfg), "-i "+dir+"/.lazyagents/ssh/id") || !strings.Contains(string(cfg), "IdentitiesOnly=yes") {
		t.Fatalf("core.sshCommand = %q", cfg)
	}
}

func TestSetupScriptClonesIntoWorkspaceWithLazyagentsDir(t *testing.T) {
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@x", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@x")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	src := t.TempDir()
	git(src, "init", "-q", "-b", "trunk")
	if err := os.WriteFile(src+"/hello.txt", []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(src, "add", ".")
	git(src, "commit", "-q", "-m", "init")

	for _, name := range []string{"empty", "with-lazyagents"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if name == "with-lazyagents" {
				if err := os.MkdirAll(dir+"/.lazyagents/context", 0o755); err != nil {
					t.Fatal(err)
				}
			}
			script := SetupScriptForBranch(src, "trunk", "me/feat", "A", "a@x", nil)
			cmd := exec.Command("sh", "-c", script)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			if b, err := os.ReadFile(dir + "/hello.txt"); err != nil || string(b) != "hi" {
				t.Fatalf("checkout missing: %v %q", err, b)
			}
			br, _ := exec.Command("git", "-C", dir, "branch", "--show-current").Output()
			if strings.TrimSpace(string(br)) != "me/feat" {
				t.Fatalf("branch = %q", br)
			}
			if _, err := os.Stat(dir + "/.lazyagents/clone"); err == nil {
				t.Fatal("temporary clone dir left behind")
			}
		})
	}
}
