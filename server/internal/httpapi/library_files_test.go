package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func zipBytes(t *testing.T, names ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(w, "contents of %s", n)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A file is described when it's stored, previewed on request, and only ever
// downloaded: even inline, text or HTML never renders in the page.
func TestLibraryFileUploadPreviewAndDownload(t *testing.T) {
	e, _ := newAvatarEnv(t)
	code, py := e.uploadLibrary("render_riff.py", []byte("import mido\n\nprint('riff')\n"))
	if code != http.StatusCreated || py["kind"] != "file" || py["detail"] != "3 lines" || py["source"] != "upload" || py["title"] != "render_riff.py" {
		t.Fatalf("py upload %d %v", code, py)
	}
	pv := e.want("GET", "/library/"+py["id"].(string)+"/preview", nil, 200)
	if lines, _ := pv["lines"].([]any); pv["kind"] != "text" || len(lines) != 3 || lines[0] != "import mido" || pv["truncated"] != false {
		t.Fatalf("text preview %v", pv)
	}

	code, zp := e.uploadLibrary("project.zip", zipBytes(t, "a.py", "src/b.py"))
	if code != http.StatusCreated || zp["detail"] != "2 files" || zp["content_type"] != "application/zip" {
		t.Fatalf("zip upload %d %v", code, zp)
	}
	pv = e.want("GET", "/library/"+zp["id"].(string)+"/preview", nil, 200)
	if entries, _ := pv["entries"].([]any); pv["kind"] != "zip" || len(entries) != 2 || entries[1].(map[string]any)["name"] != "src/b.py" {
		t.Fatalf("zip preview %v", pv)
	}

	code, csv := e.uploadLibrary("payouts.csv", []byte("id,amount\n1,5\n2,7\n"))
	if code != http.StatusCreated || csv["detail"] != "3 rows · 2 columns" {
		t.Fatalf("csv upload %d %v", code, csv)
	}
	pv = e.want("GET", "/library/"+csv["id"].(string)+"/preview", nil, 200)
	if rows, _ := pv["rows"].([]any); pv["kind"] != "table" || len(rows) != 3 {
		t.Fatalf("table preview %v", pv)
	}

	// Inline or not, a file comes back as a download and is never rendered, whatever it holds.
	code, html := e.uploadLibrary("page.html", []byte("<script>alert(1)</script>"))
	if code != http.StatusCreated || html["kind"] != "file" {
		t.Fatalf("html upload %d %v", code, html)
	}
	for _, id := range []string{html["id"].(string), py["id"].(string)} {
		res, _ := fetch(t, e.srv.URL+"/api/v1/library/"+id+"/content", nil)
		if d := res.Header.Get("Content-Disposition"); !strings.HasPrefix(d, "attachment") || res.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s served as %q (nosniff %q)", id, d, res.Header.Get("X-Content-Type-Options"))
		}
	}

	// Only a file has a preview.
	code, img := e.uploadLibrary("me.jpg", avatarJPEG(t))
	if code != http.StatusCreated {
		t.Fatalf("jpg %d", code)
	}
	e.want("GET", "/library/"+img["id"].(string)+"/preview", nil, http.StatusNotFound)
	e.want("GET", "/library/"+uuid.NewString()+"/preview", nil, http.StatusNotFound)

	if files := e.want("GET", "/library?kind=file", nil, 200)["items"].([]any); len(files) != 4 {
		t.Fatalf("files tab %v", files)
	}
}

// Executables and disk images are refused by name and by content, and a refusal
// stores nothing.
func TestLibraryFileUploadRefusesExecutables(t *testing.T) {
	e, s3 := newAvatarEnv(t)
	for name, data := range map[string][]byte{
		"setup.exe":  []byte("not really"),
		"disk.iso":   []byte("x"),
		"renamed.py": []byte("MZ\x90\x00\x03\x00\x00\x00"),    // a Windows executable
		"lib.txt":    []byte("\x7fELF\x02\x01\x01\x00"),       // ELF
		"tool.dat":   {0xcf, 0xfa, 0xed, 0xfe, 0x07, 0, 0, 1}, // Mach-O
	} {
		if code, out := e.uploadLibrary(name, data); code != http.StatusUnprocessableEntity || out["field"] != "file" {
			t.Fatalf("%s: %d %v", name, code, out)
		}
	}
	if keys := s3.keys(avatarBucket + "/chat-attachments/"); len(keys) != 0 {
		t.Fatalf("a refused file stored %v", keys)
	}
	if code, out := e.uploadLibrary("empty.txt", nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("empty: %d %v", code, out)
	}
	if items := e.want("GET", "/library", nil, 200)["items"].([]any); len(items) != 0 {
		t.Fatalf("a refused file left %v", items)
	}
}

// A persona hands over a file with save_file: it lands under Files, tied to the
// chat and the persona, and the same checks apply.
func TestSaveFileToolKeepsAPersonasFile(t *testing.T) {
	e, _ := newAvatarEnv(t)
	chat := uuid.MustParse(e.newChat(nil)["id"].(string))
	token := e.canvasToken(chat)

	code, out := e.tool(token, "save_file", map[string]any{"filename": "render_riff.py", "title": "Riff renderer", "text": "import mido\nmido.go()\n"})
	if code != 200 || out["detail"] != "2 lines" || out["library_item_id"] == nil {
		t.Fatalf("save_file text = %d %v", code, out)
	}
	zipData := base64.StdEncoding.EncodeToString(zipBytes(t, "a.txt", "b.txt", "c.txt"))
	if code, out = e.tool(token, "save_file", map[string]any{"filename": "bundle.zip", "data": zipData}); code != 200 || out["detail"] != "3 files" {
		t.Fatalf("save_file zip = %d %v", code, out)
	}

	files := e.want("GET", "/library?kind=file", nil, 200)["items"].([]any)
	if len(files) != 2 {
		t.Fatalf("files %v", files)
	}
	var riff map[string]any
	for _, f := range files {
		if m := f.(map[string]any); m["filename"] == "render_riff.py" {
			riff = m
		}
	}
	if riff == nil || riff["title"] != "Riff renderer" || riff["source"] != "generated" || riff["chat_id"] != chat.String() {
		t.Fatalf("riff %v", riff)
	}

	// The same rules as an upload, and bad input says what's wrong.
	if code, out = e.tool(token, "save_file", map[string]any{"filename": "setup.exe", "text": "x"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("exe = %d %v", code, out)
	}
	if code, _ = e.tool(token, "save_file", map[string]any{"filename": "a.txt"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("no content = %d", code)
	}
	if code, _ = e.tool(token, "save_file", map[string]any{"filename": "a.txt", "text": "x", "data": "eA=="}); code != http.StatusUnprocessableEntity {
		t.Fatalf("both contents = %d", code)
	}
	if code, _ = e.tool(token, "save_file", map[string]any{"filename": "a.bin", "data": "!!not base64!!"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad base64 = %d", code)
	}
	if n := len(e.want("GET", "/library?kind=file", nil, 200)["items"].([]any)); n != 2 {
		t.Fatalf("refusals left files: %d", n)
	}
}

// An export says whether its source changed since, and when it's gone.
func TestExportsTellWhetherTheirSourceChanged(t *testing.T) {
	e, fake := newAvatarEnv(t)
	doc := e.want("POST", "/docs", map[string]any{"title": "Launch notes"}, http.StatusCreated)
	id, chat := doc["id"].(string), doc["chat_id"].(string)
	saved := e.want("POST", "/docs/"+id+"/library", nil, http.StatusCreated)
	if saved["kind"] != "export" || saved["detail"] != "2 words" {
		t.Fatalf("saved %v", saved)
	}

	status := func() (string, any) {
		items := e.want("GET", "/library?kind=export", nil, 200)["items"].([]any)
		if len(items) == 0 {
			t.Fatal("no exports")
		}
		src := items[0].(map[string]any)["export_source"].(map[string]any)
		return src["status"].(string), src["title"]
	}
	if st, title := status(); st != "current" || title != "Launch notes" {
		t.Fatalf("fresh export reads %s %v", st, title)
	}
	e.want("POST", "/chats/"+chat+"/canvases/"+id+"/versions", map[string]any{"base_seq": 1, "body": "# Launch notes\n\nShip it.\n"}, http.StatusCreated)
	if st, _ := status(); st != "changed" {
		t.Fatalf("after an edit the export reads %s", st)
	}
	// Saving the edited text makes a second export; the first still says changed.
	second := e.want("POST", "/docs/"+id+"/library", nil, http.StatusCreated)
	if second["id"] == saved["id"] {
		t.Fatal("an edited document reused the old export")
	}
	if n := len(fake.keys(avatarBucket + "/library-exports/")); n != 2 {
		t.Fatalf("stored %d export objects, want 2", n)
	}

	if _, err := e.pool.Exec(t.Context(), `DELETE FROM chat_canvases WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	items := e.want("GET", "/library?kind=export", nil, 200)["items"].([]any)
	for _, it := range items {
		src := it.(map[string]any)["export_source"].(map[string]any)
		if src["status"] != "deleted" || src["title"] != nil || src["kind"] != "docs" {
			t.Fatalf("after deleting the source: %v", src)
		}
	}
}

// A deck's PDF or PowerPoint export is kept as an export linked to the deck;
// anything else is refused and stores nothing.
func TestSaveDeckExportToLibrary(t *testing.T) {
	e := newSlidesEnv(t)
	id, chat := e.deck("Q4 planning")
	save := func(name string, data []byte) (int, map[string]any) {
		t.Helper()
		return postFile(t, e.srv.URL+"/api/v1/slides/"+id+"/library", name, data)
	}
	pdf := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n%%EOF\n")
	code, out := save("Q4 planning.pdf", pdf)
	src, _ := out["export_source"].(map[string]any)
	if code != http.StatusCreated || out["kind"] != "export" || out["content_type"] != "application/pdf" || out["filename"] != "Q4 planning.pdf" || out["chat_id"] != chat ||
		src["kind"] != "slides" || src["id"] != id || src["status"] != "current" || out["detail"] == nil {
		t.Fatalf("pdf %d %v", code, out)
	}
	if _, again := save("Q4 planning.pdf", pdf); again["id"] != out["id"] {
		t.Fatalf("exporting unchanged work made %v, want %v", again["id"], out["id"])
	}

	pptx := zipBytes(t, "[Content_Types].xml", "ppt/presentation.xml")
	if code, out = save("Q4 planning.pptx", pptx); code != http.StatusCreated || out["content_type"] != "application/vnd.openxmlformats-officedocument.presentationml.presentation" || out["filename"] != "Q4 planning.pptx" {
		t.Fatalf("pptx %d %v", code, out)
	}
	before := len(e.want("GET", "/library?kind=export", nil, 200)["items"].([]any))
	for name, data := range map[string][]byte{"notes.txt": []byte("hi"), "plain.zip": zipBytes(t, "a.txt"), "empty.pdf": nil, "fake.pdf": []byte("not a pdf")} {
		if code, out := save(name, data); code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: %d %v", name, code, out)
		}
	}
	if after := len(e.want("GET", "/library?kind=export", nil, 200)["items"].([]any)); after != before {
		t.Fatalf("refusals left exports: %d -> %d", before, after)
	}
	if code, _ := postFile(t, e.srv.URL+"/api/v1/slides/"+uuid.NewString()+"/library", "x.pdf", pdf); code != http.StatusNotFound {
		t.Fatalf("unknown deck = %d", code)
	}
}

// postFile POSTs one file as the multipart "file" part and decodes the JSON reply.
func postFile(t *testing.T, url, name string, data []byte) (int, map[string]any) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	pw, _ := mw.CreateFormFile("file", name)
	pw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", url, &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	raw, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}
