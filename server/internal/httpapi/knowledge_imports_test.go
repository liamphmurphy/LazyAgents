package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// memObjects is an in-memory object store for vault archives.
type memObjects struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (m *memObjects) Put(_ context.Context, bucket, key string, r io.Reader, _ int64, _ string) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = map[string][]byte{}
	}
	m.data[bucket+"/"+key] = b
	return nil
}

func (m *memObjects) Open(_ context.Context, bucket, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.data[bucket+"/"+key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (m *memObjects) Delete(_ context.Context, bucket, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, bucket+"/"+key)
	return nil
}

// Cancel on the dry-run screen ends a previewed import without writing
// anything, and the canceled job can no longer be confirmed.
func TestCancelPreviewedVaultImport(t *testing.T) {
	var srv *Server
	e := newChatEnvServer(t, nil, func(s *Server) { s.Wiki.Objects = &memObjects{}; srv = s })
	ctx := t.Context()

	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	f, _ := z.Create("Vault/Harbour.md")
	f.Write([]byte("# Harbour\n\nSee [[Ferry]].\n"))
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/knowledge/imports?folder=Imported", bytes.NewReader(buf.Bytes()))
	req.Header.Set("Content-Type", "application/zip")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var job map[string]any
	json.NewDecoder(res.Body).Decode(&job)
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("upload %d %v", res.StatusCode, job)
	}
	id := job["id"].(string)
	if worked, err := srv.Wiki.ProcessNextVault(ctx); err != nil || !worked {
		t.Fatalf("scan %v %v", worked, err)
	}
	if got := e.want("GET", "/knowledge/imports/"+id, nil, 200); got["state"] != "preview" {
		t.Fatalf("not previewed: %v", got)
	}

	if got := e.want("POST", "/knowledge/imports/"+id+"/cancel", nil, 200); got["state"] != "undone" {
		t.Fatalf("cancel %v", got)
	}
	e.want("POST", "/knowledge/imports/"+id+"/confirm", map[string]any{"policy": "skip"}, 404)
	e.want("POST", "/knowledge/imports/"+id+"/cancel", nil, 409)
	e.want("POST", "/knowledge/imports/00000000-0000-0000-0000-000000000000/cancel", nil, 404)
	if pages := e.want("GET", "/knowledge/pages?folder=Imported", nil, 200); len(pages["rows"].([]any)) != 0 {
		t.Fatalf("canceled import wrote pages: %v", pages)
	}
}

// import_vault starts a dry run from a zip the person sent in chat, and only
// a dry run: nothing is imported until they confirm.
func TestImportVaultToolStartsDryRunFromChatAttachment(t *testing.T) {
	var srv *Server
	objects := &memObjects{}
	e := newChatEnvServer(t, nil, func(s *Server) { s.Wiki.Objects = objects; srv = s })
	ctx := t.Context()
	chat := e.newChat(nil)
	chatID := uuid.MustParse(chat["id"].(string))
	token := e.canvasToken(chatID)

	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	f, _ := z.Create("Vault/Harbour.md")
	f.Write([]byte("# Harbour\n"))
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	key := "chat-attachments/" + uuid.NewString()
	if err := objects.Put(ctx, srv.Wiki.Artifacts.Bucket, key, bytes.NewReader(buf.Bytes()), int64(buf.Len()), "application/zip"); err != nil {
		t.Fatal(err)
	}
	att, err := e.q.CreateChatAttachment(ctx, store.CreateChatAttachmentParams{WorkspaceID: e.me.WorkspaceID, UserID: e.me.ID, Filename: "vault.zip", ContentType: "application/zip", SizeBytes: int64(buf.Len()), BlobKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if code, out := e.tool(token, "import_vault", map[string]any{"attachment_id": att.ID}); code == 200 {
		t.Fatalf("an unsent upload started an import: %v", out)
	}
	if _, err = e.pool.Exec(ctx, `UPDATE chat_attachments SET message_id=(SELECT id FROM chat_messages WHERE session_id=$2 ORDER BY seq LIMIT 1) WHERE id=$1`, att.ID, chatID); err != nil {
		t.Fatal(err)
	}

	code, out := e.tool(token, "import_vault", map[string]any{"attachment_id": att.ID, "folder": "Imported"})
	if code != 200 || out["state"] != "queued" {
		t.Fatalf("import_vault %d %v", code, out)
	}
	if worked, err := srv.Wiki.ProcessNextVault(ctx); err != nil || !worked {
		t.Fatalf("scan %v %v", worked, err)
	}
	job := e.want("GET", "/knowledge/imports/"+out["import_id"].(string), nil, 200)
	if job["state"] != "preview" || job["plan"].(map[string]any)["notes"] != float64(1) {
		t.Fatalf("dry run %v", job)
	}
	if pages := e.want("GET", "/knowledge/pages?folder=Imported", nil, 200); len(pages["rows"].([]any)) != 0 {
		t.Fatalf("dry run wrote pages: %v", pages)
	}
}
