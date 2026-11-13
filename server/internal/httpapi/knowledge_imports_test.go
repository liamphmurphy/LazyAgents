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
