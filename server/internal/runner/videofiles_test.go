package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

const testVideoDir = ".lazyagents/videos/0193c3d4-0000-7000-8000-000000000001"

// videoFixture is a job directory with the files a renderer writes, plus
// what must never be read: a link out of the workspace, a hidden file, an
// oversized frame and a file the runner doesn't copy.
func videoFixture(t *testing.T, ws string) {
	t.Helper()
	outside := filepath.Join(t.TempDir(), "secret.jpg")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(ws, testVideoDir)
	for _, sub := range []string{"stills", "strip"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"progress.json": `{"frames_done":1}`, "preview.jpg": "P", "poster.jpg": "O", "notes.txt": "N",
		"stills/0.png": "S0", "stills/.hidden.png": "H", "strip/3.jpg": "C3",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "strip", "4.jpg"), make([]byte, videoMaxFrameBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "strip", "5.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "render.py"), []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func videoPaths(files []VideoFile) []string {
	var out []string
	for _, f := range files {
		out = append(out, strings.TrimPrefix(f.Path, testVideoDir+"/"))
	}
	sort.Strings(out)
	return out
}

var wantVideoPaths = []string{"poster.jpg", "preview.jpg", "progress.json", "render.py", "stills/0.png", "strip/3.jpg"}

// The host reader takes only the files a renderer reports through, never a
// link, hidden or oversized file, and skips what it already copied.
func TestLocalReadVideoFiles(t *testing.T) {
	l := &Fake{Dir: t.TempDir()}
	ws := l.WorkspaceDir("run")
	videoFixture(t, ws)
	req := VideoFilesRequest{Dir: testVideoDir, Script: "render.py", Known: map[string]string{}}
	files, err := l.ReadVideoFiles(context.Background(), "run", req)
	if err != nil {
		t.Fatal(err)
	}
	if got := videoPaths(files); !slices.Equal(got, wantVideoPaths) {
		t.Fatalf("files %v", got)
	}
	for _, f := range files {
		req.Known[f.Path] = f.Stamp
	}
	if again, _ := l.ReadVideoFiles(context.Background(), "run", req); len(again) != 0 {
		t.Fatalf("re-read unchanged files %v", videoPaths(again))
	}
	// A script outside the workspace is refused by the root.
	req.Script = "../../outside.py"
	if files, err := l.ReadVideoFiles(context.Background(), "run", req); err == nil && len(files) != 0 {
		t.Fatalf("read outside the workspace: %v", videoPaths(files))
	}

	if err := os.WriteFile(filepath.Join(ws, testVideoDir, "video.mp4"), []byte("MP4"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, size, err := l.OpenWorkspaceFile(context.Background(), "run", testVideoDir+"/video.mp4", 1<<20)
	if err != nil || size != 3 {
		t.Fatalf("open %d %v", size, err)
	}
	b, _ := io.ReadAll(f)
	f.Close()
	if string(b) != "MP4" {
		t.Fatalf("read %q", b)
	}
	if _, _, err := l.OpenWorkspaceFile(context.Background(), "run", testVideoDir+"/missing.mp4", 1<<20); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
	if _, _, err := l.OpenWorkspaceFile(context.Background(), "run", testVideoDir+"/video.mp4", 2); err == nil {
		t.Fatal("over the limit")
	}
}

// The script a start-only backend (Docker) runs applies the same rules.
func TestVideoFilesScriptAndTar(t *testing.T) {
	ws := t.TempDir()
	videoFixture(t, ws)
	run := func(known map[string]string) ([]VideoFile, string) {
		req := VideoFilesRequest{Dir: testVideoDir, Script: "render.py", Known: known}
		args := []string{"-c", videoFilesScript, "video-files", req.Dir, req.Script}
		for p, s := range known {
			args = append(args, p+"="+s)
		}
		cmd := exec.Command("sh", args...)
		cmd.Dir = ws
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%v\n%s", err, stderr.String())
		}
		stamps := map[string]string{}
		for _, line := range strings.Split(stderr.String(), "\n") {
			if parts := strings.SplitN(line, "\t", 3); len(parts) == 3 && parts[0] == "stamp" {
				stamps[parts[1]] = parts[2]
			}
		}
		files, err := parseVideoFilesTar(&stdout, req, stamps)
		if err != nil {
			t.Fatal(err)
		}
		return files, stderr.String()
	}
	files, stderr := run(map[string]string{})
	if got := videoPaths(files); !slices.Equal(got, wantVideoPaths) {
		t.Fatalf("files %v\n%s", got, stderr)
	}
	known := map[string]string{}
	for _, f := range files {
		if f.Stamp == "" {
			t.Fatalf("%s has no stamp", f.Path)
		}
		known[f.Path] = f.Stamp
	}
	if again, _ := run(known); len(again) != 0 {
		t.Fatalf("re-read unchanged files %v", videoPaths(again))
	}
}

// The streamed-file script reports a missing file apart from other failures.
func TestWorkspaceFileScript(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "v.mp4"), []byte("MP4"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", workspaceFileScript, "workspace-file", "v.mp4")
	cmd.Dir = ws
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]) != "3" || !strings.HasSuffix(string(out), "MP4") {
		t.Fatalf("%q %v", out, err)
	}
	cmd = exec.Command("sh", "-c", workspaceFileScript, "workspace-file", "missing.mp4")
	cmd.Dir = ws
	var exit *exec.ExitError
	if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("missing: %v", err)
	}
}
