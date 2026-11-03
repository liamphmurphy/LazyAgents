package runner

import (
	"os"
	"os/exec"
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
	dir := t.TempDir()
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
	if !strings.Contains(string(cfg), "-i "+dir+"/.lazyagents/ssh/id") && !strings.Contains(string(cfg), "IdentitiesOnly=yes") {
		t.Fatalf("core.sshCommand = %q", cfg)
	}
}

func TestSaveScriptQuotesBranchAndNeverForces(t *testing.T) {
	s := SaveScript("feat/it's")
	if !strings.Contains(s, `branch='feat/it'\''s'`) {
		t.Fatalf("branch not quoted: %s", s)
	}
	for _, bad := range []string{"--force", " -f ", "--mirror", "--all", "+refs"} {
		if strings.Contains(s, bad) {
			t.Fatalf("script contains %q", bad)
		}
	}
	if !strings.Contains(s, `refs/heads/$branch:refs/heads/$branch`) || !strings.Contains(s, "--untracked-files=no") {
		t.Fatalf("script %s", s)
	}
}
