package runner

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func outboxFixture(t *testing.T, dir string) {
	t.Helper()
	outside := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	ob := filepath.Join(dir, ".lazyagents", "outbox")
	if err := os.MkdirAll(filepath.Join(ob, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"a.png": "A", "b.png": "B", ".hidden.png": "H"} {
		if err := os.WriteFile(filepath.Join(ob, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(ob, "escape.png")); err != nil {
		t.Fatal(err)
	}
}

func names(files []OutboxFile) string {
	var n []string
	for _, f := range files {
		n = append(n, f.Name+"="+string(f.Data))
	}
	return strings.Join(n, ",")
}

func TestLocalReadOutbox(t *testing.T) {
	l := &Local{Dir: t.TempDir()}
	ctx := context.Background()
	if err := l.EnsureWorkspace(ctx, "run"); err != nil {
		t.Fatal(err)
	}
	if files, skipped, err := l.ReadOutbox(ctx, "run"); err != nil || len(files)+len(skipped) != 0 {
		t.Fatalf("no outbox must be empty: %v %v %v", files, skipped, err)
	}
	outboxFixture(t, l.WorkspaceDir("run"))
	files, skipped, err := l.ReadOutbox(ctx, "run")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(files); got != "a.png=A,b.png=B" {
		t.Fatalf("files %s", got)
	}
	if s := strings.Join(skipped, "|"); !strings.Contains(s, "adir: not a regular file") || !strings.Contains(s, "escape.png: not a regular file") {
		t.Fatalf("skipped %q", s)
	}
	if err := l.RemoveOutbox(ctx, "run", []string{"a.png", "../../escape", "missing.png"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(l.WorkspaceDir("run"), ".lazyagents", "outbox", "a.png")); !os.IsNotExist(err) {
		t.Fatal("a.png not removed")
	}
}

// An outbox that is itself a link out of the workspace is not followed.
func TestLocalReadOutboxLinkedDirectory(t *testing.T) {
	l := &Local{Dir: t.TempDir()}
	if err := l.EnsureWorkspace(context.Background(), "run"); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "x.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws := l.WorkspaceDir("run")
	if err := os.MkdirAll(filepath.Join(ws, ".lazyagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, ".lazyagents", "outbox")); err != nil {
		t.Fatal(err)
	}
	if files, _, err := l.ReadOutbox(context.Background(), "run"); err == nil || len(files) != 0 {
		t.Fatalf("followed a link out of the workspace: %v %v", files, err)
	}
}

// The script a start-only backend (Docker) runs applies the same rules.
func TestOutboxScriptAndTar(t *testing.T) {
	dir := t.TempDir()
	outboxFixture(t, dir)
	ob := filepath.Join(dir, ".lazyagents", "outbox")
	for _, n := range []string{"c", "d", "e", "f", "g", "h", "i", "j"} { // 10 regular files in all
		if err := os.WriteFile(filepath.Join(ob, n+".png"), []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("sh", "-c", outboxScript)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	files, err := parseOutboxTar(&stdout)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != OutboxMaxFiles || files[0].Name != "a.png" || files[1].Name != "b.png" {
		t.Fatalf("files %s", names(files))
	}
	for _, f := range files {
		if f.Name == "escape.png" || strings.HasPrefix(f.Name, ".") {
			t.Fatalf("took %s", f.Name)
		}
	}
	for _, want := range []string{"skip\tadir\tnot a regular file", "skip\tescape.png\tnot a regular file", "files in one turn"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
	// No outbox is not an error.
	empty := exec.Command("sh", "-c", outboxScript)
	empty.Dir = t.TempDir()
	if out, err := empty.Output(); err != nil || len(out) != 0 {
		t.Fatalf("empty: %q %v", out, err)
	}
}

// A linked .lazyagents or outbox directory is never entered by the script.
func TestOutboxScriptLinkedDirectories(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "x.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws1, ws2 := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(ws1, ".lazyagents")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws2, ".lazyagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws2, ".lazyagents", "outbox")); err != nil {
		t.Fatal(err)
	}
	// Also reachable through a linked .lazyagents holding a real outbox.
	if err := os.Mkdir(filepath.Join(outside, "outbox"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "outbox", "y.png"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{ws1, ws2} {
		cmd := exec.Command("sh", "-c", outboxScript)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil || len(out) != 0 {
			t.Fatalf("%s: read through a link (%v): %q", dir, err, out)
		}
	}
}

// Collecting a Codex thread's images takes only that thread's regular image
// files, never through a link, without overwriting what the outbox holds.
func TestCollectGeneratedImages(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	l := &Local{Dir: t.TempDir()}
	ctx := context.Background()
	if err := l.EnsureWorkspace(ctx, "run"); err != nil {
		t.Fatal(err)
	}
	write := func(p, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gen := filepath.Join(home, "generated_images")
	thread := filepath.Join(gen, "thread-1")
	secret := filepath.Join(t.TempDir(), "secret.png")
	write(secret, "secret")
	write(filepath.Join(thread, "ig_1.png"), "IMG")
	write(filepath.Join(thread, "UPPER.JPG"), "JPG")
	write(filepath.Join(thread, "notes.txt"), "txt")
	write(filepath.Join(thread, ".hidden.png"), "hidden")
	write(filepath.Join(thread, "-flag.png"), "flag")
	write(filepath.Join(thread, "kept.png"), "new")
	write(filepath.Join(gen, "thread-2", "other.png"), "other thread")
	if err := os.Symlink(secret, filepath.Join(thread, "link.png")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(thread, "sub.png"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(l.WorkspaceDir("run"), outboxDir, "kept.png"), "old")

	if err := collectGeneratedImages(ctx, l, "run", "thread-1"); err != nil {
		t.Fatal(err)
	}
	files, _, err := l.ReadOutbox(ctx, "run")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(files); got != "UPPER.JPG=JPG,ig_1.png=IMG,kept.png=old" {
		t.Fatalf("outbox %s", got)
	}

	// A thread folder that is a link, or a missing one, yields nothing.
	if err := os.Symlink(filepath.Join(gen, "thread-2"), filepath.Join(gen, "linked")); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"linked", "missing"} {
		if err := collectGeneratedImages(ctx, l, "run", id); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	if files, _, _ := l.ReadOutbox(ctx, "run"); len(files) != 3 {
		t.Fatalf("collected through %s", names(files))
	}
	// Ids that could name another directory are refused before anything runs.
	for _, id := range []string{"", ".", "..", "../thread-2", "a/b", "-r", "$(id)"} {
		if err := collectGeneratedImages(ctx, l, "run", id); err == nil {
			t.Fatalf("accepted thread id %q", id)
		}
	}
}

// A linked outbox is not written through.
func TestCollectGeneratedImagesLinkedOutbox(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "generated_images", "t1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "generated_images", "t1", "a.png"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := &Local{Dir: t.TempDir()}
	if err := l.EnsureWorkspace(context.Background(), "run"); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	ws := l.WorkspaceDir("run")
	if err := os.MkdirAll(filepath.Join(ws, ".lazyagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, outboxDir)); err != nil {
		t.Fatal(err)
	}
	if err := collectGeneratedImages(context.Background(), l, "run", "t1"); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("wrote through the link: %v", entries)
	}
}

type recordingBlobs struct{ deleted []string }

func (r *recordingBlobs) Put(context.Context, string, string, io.Reader, int64, string) error {
	return nil
}
func (r *recordingBlobs) Delete(_ context.Context, bucket, key string) error {
	r.deleted = append(r.deleted, bucket+"/"+key)
	return nil
}

// Unbound images are removed through the store they were written to, even
// when no separate attachment reader is configured.
func TestDropImageBlobsUsesTheWritingStore(t *testing.T) {
	b := &recordingBlobs{}
	r := &Runner{Blobs: b, Cfg: Config{AttachmentBucket: "artifacts"}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	r.dropImageBlobs([]generatedImage{{key: "k1"}, {key: "k2"}})
	if strings.Join(b.deleted, ",") != "artifacts/k1,artifacts/k2" {
		t.Fatalf("deleted %v", b.deleted)
	}
}
