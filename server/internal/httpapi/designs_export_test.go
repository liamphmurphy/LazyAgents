package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/runs"
)

func (e *designsEnv) saveExport(id, filename string, data []byte) (int, map[string]any) {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	pw, _ := mw.CreateFormFile("file", filename)
	pw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/designs/"+id+"/library", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	raw, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

// Each export format lands in the Library as an export, typed by its content,
// linked to its design and the design's chat and counted against the Library cap; a
// refused file (wrong type, a full Library) stores neither a row nor an
// object.
func TestSaveDesignExportToLibrary(t *testing.T) {
	e := newDesignsEnv(t)
	id, chat := e.design(map[string]any{"title": "Checkout"})
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><text x="1" y="8">Pay</text></svg>`
	cases := []struct {
		name  string
		data  []byte
		kind  string
		ctype string
		file  string
	}{
		{"Checkout – Payment.png", pngSized(t, 30, 20, 2), "export", "image/png", "Checkout – Payment.png"},
		{"Checkout – Payment.svg", []byte(svg), "export", "image/svg+xml", "Checkout – Payment.svg"},
		{"Checkout – all frames.pdf", []byte("%PDF-1.3\n%\xba\xdf\xac\xe0\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF\n"), "export", "application/pdf", "Checkout – all frames.pdf"},
		// The content decides the type, and the name gets its extension.
		{"export.bin", []byte(strings.Replace(svg, "Pay", "Pay now", 1)), "export", "image/svg+xml", "export.svg"},
	}
	for _, c := range cases {
		code, out := e.saveExport(id, c.name, c.data)
		if src, _ := out["export_source"].(map[string]any); src["kind"] != "designs" || src["id"] != id || src["status"] != "current" || src["title"] != "Checkout" {
			t.Fatalf("%s: export source %v", c.name, src)
		}
		if code != http.StatusCreated || out["kind"] != c.kind || out["content_type"] != c.ctype || out["filename"] != c.file || out["chat_id"] != chat || out["size_bytes"] != float64(len(c.data)) {
			t.Fatalf("%s: %d %v", c.name, code, out)
		}
		res, err := http.Get(e.srv.URL + "/api/v1/library/" + out["id"].(string) + "/content")
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK || !bytes.Equal(got, c.data) || res.Header.Get("Content-Type") != c.ctype {
			t.Fatalf("%s content: %d %q", c.name, res.StatusCode, res.Header.Get("Content-Type"))
		}
	}
	if keys := e.s3.keys(avatarBucket + "/library-exports/"); len(keys) != len(cases) {
		t.Fatalf("stored %v", keys)
	}
	items := func() int {
		return e.count(`SELECT count(*) FROM library_items WHERE user_id = $1`, e.me.ID)
	}
	before := items()

	for name, data := range map[string][]byte{
		"notes.txt":  []byte("hello"),
		"fake.svg":   []byte(`<svg viewBox="0 0 1 1"></svg>`), // no SVG namespace
		"photo.jpg":  {0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F'},
		"empty.png":  {},
		"broken.pdf": []byte("PDF but not really"),
	} {
		if code, out := e.saveExport(id, name, data); code != http.StatusUnprocessableEntity || out["field"] != "file" {
			t.Fatalf("%s: %d %v", name, code, out)
		}
	}

	// A full Library refuses the save and keeps nothing of it.
	if _, err := e.pool.Exec(t.Context(), `INSERT INTO library_items (workspace_id, user_id, kind, title, filename, content_type, size_bytes, blob_key)
		VALUES ($1, $2, 'image', 'Big', 'big.png', 'image/png', $3, 'big-object')`, e.me.WorkspaceID, e.me.ID, runs.MaxGeneratedImageStorageBytes-100); err != nil {
		t.Fatal(err)
	}
	// New content: re-saving bytes already kept stores nothing, so it isn't refused.
	if code, out := e.saveExport(id, "late.png", pngSized(t, 31, 20, 2)); code != http.StatusUnprocessableEntity || !strings.Contains(out["message"].(string), "Library is full") {
		t.Fatalf("full Library: %d %v", code, out)
	}
	if n := items(); n != before+1 {
		t.Fatalf("items after refusals = %d, want %d", n, before+1)
	}
	if keys := e.s3.keys(avatarBucket + "/library-exports/"); len(keys) != len(cases) {
		t.Fatalf("a refused save stored an object: %v", keys)
	}

	if code, _ := e.saveExport(uuid.NewString(), "x.png", pngSized(t, 30, 20, 2)); code != http.StatusNotFound {
		t.Fatalf("unknown design = %d", code)
	}

	other := e.otherUser()
	theirs, _, err := e.svc().Create(t.Context(), e.me.WorkspaceID, other.ID, "Theirs", "diagram", "")
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := e.saveExport(theirs.ID.String(), "x.png", pngSized(t, 30, 20, 2)); code != http.StatusNotFound {
		t.Fatalf("another user's design = %d", code)
	}
	if n := items(); n != before+1 {
		t.Fatalf("unauthorized save changed Library items: %d", n)
	}
	if keys := e.s3.keys(avatarBucket + "/library-exports/"); len(keys) != len(cases) {
		t.Fatalf("unauthorized save stored an object: %v", keys)
	}
	doc := e.want("POST", "/docs", map[string]any{"title": "Notes"}, http.StatusCreated)
	if code, _ := e.saveExport(doc["id"].(string), "x.png", pngSized(t, 30, 20, 2)); code != http.StatusNotFound {
		t.Fatalf("a document's id = %d", code)
	}
}
