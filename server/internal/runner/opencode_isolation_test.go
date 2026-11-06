package runner

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolatedLocal(t *testing.T) *Local {
	t.Helper()
	l := &Local{Dir: t.TempDir(), Bins: map[string]string{"opencode": "/usr/bin/env"}, OnlyBins: true, OpenCodeIsolated: true}
	if err := l.EnsureWorkspace(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	return l
}
func runIsolated(t *testing.T, l *Local, s ExecSpec) (string, int, error) {
	t.Helper()
	if len(s.Argv) == 0 {
		s.Argv = []string{"opencode"}
	}
	p, err := l.Start(context.Background(), "one", s)
	if err != nil {
		return "", -1, err
	}
	defer p.(io.Closer).Close()
	done := make(chan struct{})
	go func() { defer close(done); _, _ = io.Copy(io.Discard, p.Stderr()) }()
	b, _ := io.ReadAll(p.Stdout())
	code, err := p.Wait()
	<-done
	return string(b), code, err
}
func TestOpenCodeExternalStateAndResume(t *testing.T) {
	l := isolatedLocal(t)
	spec := ExecSpec{StateFiles: map[string][]byte{"opencode.json": []byte("{}")}, Env: map[string]string{"OPENCODE_CONFIG": "/bad", "OPENCODE_CONFIG_CONTENT": "bad", "HOME": "/bad"}}
	got, code, err := runIsolated(t, l, spec)
	if err != nil || code != 0 {
		t.Fatalf("%d %v", code, err)
	}
	state := l.OpenCodeStateDir("one", "")
	if !strings.Contains(got, "OPENCODE_CONFIG="+filepath.Join(state, "opencode.json")) || strings.Contains(got, "/bad") || strings.Contains(got, "OPENCODE_CONFIG_CONTENT=") {
		t.Fatal(got)
	}
	if _, err := os.Stat(filepath.Join(l.WorkspaceDir("one"), "opencode.json")); !os.IsNotExist(err) {
		t.Fatal("config in checkout")
	}
	marker := filepath.Join(state, "data", "history")
	if err := os.WriteFile(marker, []byte("history"), 0600); err != nil {
		t.Fatal(err)
	}
	_, code, err = runIsolated(t, l, spec)
	if err != nil || code != 0 {
		t.Fatalf("%d %v", code, err)
	}
	if b, err := os.ReadFile(marker); err != nil || string(b) != "history" {
		t.Fatalf("lost state: %s %v", b, err)
	}
	if state == l.OpenCodeStateDir("two", "") || state == l.OpenCodeStateDir("one", ".lazyagents/worktrees/child") {
		t.Fatal("shared state")
	}
	if err := l.RemoveWorkspace(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("state not cleaned")
	}
}

func TestOpenCodeRejectsConfigLayersEveryTurn(t *testing.T) {
	for _, name := range []string{"opencode.json", "opencode.jsonc", ".opencode/opencode.json", ".opencode/opencode.jsonc", ".lazyagents/opencode/config/opencode/opencode.json"} {
		t.Run(name, func(t *testing.T) {
			l := isolatedLocal(t)
			if _, c, e := runIsolated(t, l, ExecSpec{}); c != 0 || e != nil {
				t.Fatalf("fresh %d %v", c, e)
			}
			p := filepath.Join(l.WorkspaceDir("one"), name)
			if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, c, e := runIsolated(t, l, ExecSpec{}); c == 0 && e == nil {
				t.Fatal("agent config accepted on next turn")
			}
		})
	}
	for _, layer := range []string{"ancestor", "xdg", "home", "dangling"} {
		t.Run(layer, func(t *testing.T) {
			l := isolatedLocal(t)
			if _, c, e := runIsolated(t, l, ExecSpec{}); c != 0 || e != nil {
				t.Fatalf("fresh %d %v", c, e)
			}
			p := filepath.Join(l.Dir, "opencode.json")
			if layer == "xdg" {
				p = filepath.Join(l.OpenCodeStateDir("one", ""), "config", "opencode", "opencode.json")
			}
			if layer == "home" {
				p = filepath.Join(l.OpenCodeStateDir("one", ""), "home", ".config", "opencode", "opencode.json")
			}
			if layer == "dangling" {
				p = filepath.Join(l.WorkspaceDir("one"), "opencode.json")
			}
			if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
				t.Fatal(err)
			}
			var err error
			if layer == "dangling" {
				err = os.Symlink("/no-such-target", p)
			} else {
				err = os.WriteFile(p, []byte("{}"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, c, e := runIsolated(t, l, ExecSpec{}); c == 0 && e == nil {
				t.Fatal("ambient config accepted")
			}
		})
	}
}
func TestOpenCodeStagingRejectsSymlinks(t *testing.T) {
	for _, which := range []string{"workspace", "metadata", "state", "config"} {
		t.Run(which, func(t *testing.T) {
			l := isolatedLocal(t)
			target := t.TempDir()
			p := l.WorkspaceDir("one")
			files := map[string][]byte{}
			switch which {
			case "workspace":
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
			case "metadata":
				p = filepath.Join(p, ".lazyagents")
				files[".lazyagents/context/a"] = []byte("no")
			case "state":
				p = l.OpenCodeStateDir("one", "")
			case "config":
				p = filepath.Join(l.OpenCodeStateDir("one", ""), "opencode.json")
				target = filepath.Join(target, "config")
			}
			if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, p); err != nil {
				t.Fatal(err)
			}
			if _, _, err := runIsolated(t, l, ExecSpec{Files: files, StateFiles: map[string][]byte{"opencode.json": []byte("no")}}); err == nil {
				t.Fatal("symlink accepted")
			}
			if which == "config" {
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatal("symlink target modified")
				}
			}
		})
	}
}

func TestOpenCodeMissingResumeStateFails(t *testing.T) {
	l := isolatedLocal(t)
	if _, _, err := runIsolated(t, l, ExecSpec{Argv: []string{"opencode", "--session", "ses_lost"}}); err == nil || !strings.Contains(err.Error(), "resume state") {
		t.Fatalf("missing session state accepted: %v", err)
	}
}
