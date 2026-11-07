package runner

import (
	"archive/tar"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type engineTransport func(*http.Request) (*http.Response, error)

func (f engineTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDockerOpenCodeExternalVolumeAndCleanup(t *testing.T) {
	var created map[string]any
	var deleted []string
	var payload []byte
	d := &Docker{Image: "test", OpenCodeIsolated: true}
	d.client = &http.Client{Transport: engineTransport(func(r *http.Request) (*http.Response, error) {
		body := "{}"
		switch {
		case strings.Contains(r.URL.Path, "containers/create"):
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatal(err)
			}
			body = `{"Id":"fake-container"}`
		case strings.HasSuffix(r.URL.Path, "/wait"):
			body = `{"StatusCode":0}`
		case strings.Contains(r.URL.Path, "/archive"):
			if r.URL.Query().Get("path") != "/run/lazyagents-opencode" {
				t.Fatal("archive touches persistent agent files")
			}
			tr := tar.NewReader(r.Body)
			if h, err := tr.Next(); err != nil || h.Name != "payload.json" {
				t.Fatalf("payload tar %v %v", h, err)
			}
			payload, _ = io.ReadAll(tr)
		case r.Method == "DELETE":
			deleted = append(deleted, r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	p, err := d.Start(context.Background(), "one", ExecSpec{Argv: []string{"opencode", "run"}, Env: map[string]string{"OPENCODE_CONFIG_CONTENT": "poison"}, Files: map[string][]byte{"AGENTS.md": []byte(strings.Repeat("instructions", 32768))}, StateFiles: map[string][]byte{"opencode.json": []byte("{}")}})
	if err != nil {
		t.Fatal(err)
	}
	if code, err := p.Wait(); code != 0 || err != nil {
		t.Fatalf("%d %v", code, err)
	}
	cmd := created["Cmd"].([]any)
	if cmd[0] != "python3" || cmd[2] != openCodeStage {
		t.Fatal(cmd)
	}
	mounts := created["HostConfig"].(map[string]any)["Mounts"].([]any)
	if len(mounts) != 3 {
		t.Fatal(mounts)
	}
	cache := mounts[1].(map[string]any)
	if cache["Source"] != "lazyagents-dependency-cache" || cache["Target"] != "/cache" {
		t.Fatal(cache)
	}
	state := mounts[2].(map[string]any)
	if state["Source"] != stateVolumeName("one") || state["Target"] != "/.lazyagents/opencode" {
		t.Fatal(state)
	}
	for _, v := range created["Env"].([]any) {
		if strings.HasPrefix(v.(string), "OPENCODE_CONFIG_CONTENT=") {
			t.Fatal(v)
		}
	}
	if cmd[3] != "/run/lazyagents-opencode/payload.json" {
		t.Fatal(cmd[3])
	}
	if !strings.Contains(string(payload), d.OpenCodeStateDir("one", "")) {
		t.Fatal(string(payload))
	}
	if err := d.RemoveWorkspace(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	all := strings.Join(deleted, "\n")
	if strings.Contains(all, "lazyagents-dependency-cache") {
		t.Fatal("shared cache deleted")
	}
	if !strings.Contains(all, volumeName("one")) || !strings.Contains(all, stateVolumeName("one")) {
		t.Fatal(all)
	}
}

func TestDockerOpenCodeNoFollowStaging(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	for _, poison := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "symlink"}[poison], func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			ws := filepath.Join(root, "workspace")
			state := filepath.Join(root, ".lazyagents", "opencode", "main")
			if err := os.Mkdir(ws, 0700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(root, "untouched")
			if poison {
				if err := os.MkdirAll(state, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(state, "opencode.json")); err != nil {
					t.Fatal(err)
				}
			}
			payload, _ := json.Marshal(map[string]any{"files": map[string][]byte{"AGENTS.md": []byte("instructions")}, "state_files": map[string][]byte{"opencode.json": []byte("{}")}, "state": state, "resume": false})
			payloadPath := filepath.Join(root, "payload.json")
			if err := os.WriteFile(payloadPath, payload, 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(python, "-c", openCodeStage, payloadPath, "/usr/bin/env")
			cmd.Dir = ws
			cmd.Env = append(os.Environ(), "OPENCODE_CONFIG_CONTENT=poison", "OPENCODE_CONFIG=/bad")
			b, err := cmd.CombinedOutput()
			if poison {
				if err == nil {
					t.Fatal("symlink config accepted")
				}
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatal("symlink target modified")
				}
			} else {
				if err != nil {
					t.Fatalf("%v %s", err, b)
				}
				if !strings.Contains(string(b), "OPENCODE_CONFIG="+state+"/opencode.json") || strings.Contains(string(b), "OPENCODE_CONFIG_CONTENT=") {
					t.Fatal(string(b))
				}
				if b, err := os.ReadFile(filepath.Join(state, "opencode.json")); err != nil || string(b) != "{}" {
					t.Fatalf("%s %v", b, err)
				}
			}
		})
	}
}
