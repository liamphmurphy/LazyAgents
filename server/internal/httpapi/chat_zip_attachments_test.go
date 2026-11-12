package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
)

func (e *chatEnv) uploadChatFile(name string, data []byte) (int, map[string]any) {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	pw, err := mw.CreateFormFile("file", name)
	if err != nil {
		e.t.Fatal(err)
	}
	pw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/chat-attachments", &body)
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

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A chat takes a zip, but only a real one with files in it: a renamed file or
// an empty archive is refused when attached, not after the message is sent.
func TestChatAttachmentTakesZipFiles(t *testing.T) {
	e, _ := newAvatarEnv(t)

	code, out := e.uploadChatFile("project.zip", zipOf(t, map[string]string{"a.txt": "hello"}))
	if code != http.StatusCreated || out["content_type"] != "application/zip" || out["filename"] != "project.zip" {
		t.Fatalf("zip upload: %d %v", code, out)
	}
	for name, data := range map[string][]byte{
		"fake.zip":  []byte("%PDF-1.7 not a zip"),
		"cut.zip":   zipOf(t, map[string]string{"a.txt": "hello"})[:20],
		"empty.zip": zipOf(t, nil),
	} {
		if code, out := e.uploadChatFile(name, data); code != http.StatusUnprocessableEntity || out["field"] != "file" {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
}
