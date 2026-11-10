package runner

import (
	"archive/tar"
	"bytes"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The setup snippet runs in the checkout and reports the files inference
// reads, which the runner strips from the log.
func TestTestFilesScriptReportsCandidates(t *testing.T) {
	dir := t.TempDir()
	write := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".github/workflows/ci.yml", "jobs: {}\n")
	write("docker-compose.yml", "services: {}\n")
	write("README.md", "not read")
	cmd := exec.Command("sh", "-c", "echo before; "+testFilesScript()+"echo after")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	log, files := takeTestFiles(string(out))
	if strings.Contains(log, testFilesPrefix) || !strings.Contains(log, "before") || !strings.Contains(log, "after") {
		t.Fatalf("log = %q", log)
	}
	if len(files) != 2 || string(files[".github/workflows/ci.yml"]) != "jobs: {}\n" || files["docker-compose.yml"] == nil {
		t.Fatalf("files = %v", files)
	}
}

// After a turn the runner reads the run's Nix profile out of the stopped
// container, as it does the Codex login.
func TestDockerWaitReadsNixManifest(t *testing.T) {
	manifest := `{"version":2,"elements":[]}`
	var tb bytes.Buffer
	tw := tar.NewWriter(&tb)
	_ = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(manifest))})
	_, _ = tw.Write([]byte(manifest))
	_ = tw.Close()
	var asked string
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.43/containers/c1/wait", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"StatusCode":0}`)) })
	mux.HandleFunc("/v1.43/containers/c1/archive", func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query().Get("path")
		_, _ = w.Write(tb.Bytes())
	})
	mux.HandleFunc("/v1.43/containers/c1", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	d := &Docker{Socket: sock, NixVolume: "lazyagents-nix"}
	p := &dockerProc{d: d, id: "c1"}
	if _, err := p.Wait(); err != nil {
		t.Fatal(err)
	}
	if asked != "/home/node/.nix-profile/manifest.json" || string(p.NixManifest()) != manifest {
		t.Fatalf("asked %q, got %q", asked, p.NixManifest())
	}
	var _ NixManifestProcess = p
}
