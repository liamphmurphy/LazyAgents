package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/config"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// fakeS3 is an in-memory, path-style S3 gateway with just PutObject,
// GetObject and DeleteObject, so the avatar handlers run against the real
// blob client.
type fakeS3 struct {
	mu   sync.Mutex
	objs map[string]fakeObject // "bucket/key"
}

type fakeObject struct {
	body []byte
	ct   string
}

func newFakeS3(t *testing.T) (*fakeS3, *httptest.Server) {
	f := &fakeS3{objs: map[string]fakeObject{}}
	ts := httptest.NewServer(f)
	t.Cleanup(ts.Close)
	return f, ts
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/")
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case http.MethodPut:
		b, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if strings.Contains(r.Header.Get("Content-Encoding"), "aws-chunked") || r.Header.Get("X-Amz-Decoded-Content-Length") != "" {
			b = decodeAWSChunked(b)
		}
		f.objs[key] = fakeObject{body: b, ct: r.Header.Get("Content-Type")}
		w.Header().Set("ETag", `"x"`)
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		o, ok := f.objs[key]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
			return
		}
		w.Header().Set("Content-Type", o.ct)
		// ServeContent answers a Range with 206, as S3 does.
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(o.body))
	case http.MethodDelete:
		delete(f.objs, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// decodeAWSChunked unwraps an aws-chunked body: <hex size>[;ext]\r\n<data>\r\n ... 0\r\n<trailers>.
func decodeAWSChunked(b []byte) []byte {
	var out []byte
	for {
		i := bytes.Index(b, []byte("\r\n"))
		if i < 0 {
			return out
		}
		size, _, _ := strings.Cut(string(b[:i]), ";")
		n, err := strconv.ParseInt(strings.TrimSpace(size), 16, 64)
		if err != nil || n == 0 {
			return out
		}
		b = b[i+2:]
		out = append(out, b[:n]...)
		b = b[n+2:]
	}
}

// keys lists the stored keys under prefix.
func (f *fakeS3) keys(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for k := range f.objs {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out
}

func (f *fakeS3) put(key string, body []byte, ct string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objs[key] = fakeObject{body: body, ct: ct}
}

const avatarBucket = "test-artifacts"

func newAvatarEnv(t *testing.T) (*chatEnv, *fakeS3) {
	t.Helper()
	fake, s3 := newFakeS3(t)
	e := newChatEnvServer(t, nil, func(s *Server) {
		s.Blobs = blob.New(config.S3{Endpoint: s3.URL, Region: "us-east-1", AccessKey: "test", SecretKey: "test-secret", UsePathStyle: true})
		s.ArtifactBucket = avatarBucket
	})
	return e, fake
}

func avatarImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		for y := range h {
			img.Set(x, y, color.RGBA{uint8(x * 9), uint8(y * 5), 140, 255})
		}
	}
	return img
}

func avatarPNG(t *testing.T) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, avatarImage(12, 12)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func avatarJPEG(t *testing.T) []byte {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, avatarImage(12, 12), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// uploadAvatar PUTs data as the multipart file part with a declared name and type.
func (e *chatEnv) uploadAvatar(id, filename, declared string, data []byte) (int, map[string]any) {
	e.t.Helper()
	return e.uploadAvatarTo("/personas/"+id+"/avatar", filename, declared, data)
}

func (e *chatEnv) uploadAvatarTo(path, filename, declared string, data []byte) (int, map[string]any) {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	h.Set("Content-Type", declared)
	pw, err := mw.CreatePart(h)
	if err != nil {
		e.t.Fatal(err)
	}
	pw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("PUT", e.srv.URL+"/api/v1"+path, &body)
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

func (e *chatEnv) getAvatar(id string) (*http.Response, []byte) {
	e.t.Helper()
	res, err := http.Get(e.srv.URL + "/api/v1/personas/" + id + "/avatar?v=1")
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, b
}

func TestPersonaAvatarUploadServeAndReplace(t *testing.T) {
	e, fake := newAvatarEnv(t)
	ctx := context.Background()
	p := e.want("POST", "/personas", map[string]any{"name": "Work"}, http.StatusCreated)
	id := p["id"].(string)
	prefix := fmt.Sprintf("%s/personas/%s/%s/%s/", avatarBucket, e.me.WorkspaceID, e.me.ID, id)
	chat := e.newChat(map[string]any{"persona_id": id})

	// No avatar yet.
	if res, _ := e.getAvatar(id); res.StatusCode != http.StatusNotFound {
		t.Fatalf("avatar before upload: %d", res.StatusCode)
	}

	// A PNG declared as HTML is stored and served as the sniffed PNG.
	pngImg := avatarPNG(t)
	code, got := e.uploadAvatar(id, "evil.html", "text/html", pngImg)
	if code != 200 || got["avatar_url"] != fmt.Sprintf("/api/v1/personas/%s/avatar?v=2", id) || got["revision"].(float64) != 2 {
		t.Fatalf("upload png = %d %v", code, got)
	}
	first := fake.keys(prefix)
	if len(first) != 1 {
		t.Fatalf("stored keys %v", first)
	}
	res, body := e.getAvatar(id)
	if res.StatusCode != 200 || !bytes.Equal(body, pngImg) {
		t.Fatalf("get avatar = %d (%d bytes)", res.StatusCode, len(body))
	}
	for h, want := range map[string]string{
		"Content-Type": "image/png", "X-Content-Type-Options": "nosniff", "Content-Disposition": "inline",
		"Content-Security-Policy": "default-src 'none'; sandbox", "Cache-Control": "private, max-age=86400",
	} {
		if res.Header.Get(h) != want {
			t.Fatalf("%s = %q, want %q", h, res.Header.Get(h), want)
		}
	}
	// The chat's embedded summary carries the new URL.
	if s := e.want("GET", "/chats/"+chat["id"].(string), nil, 200)["persona"].(map[string]any); s["avatar_url"] != got["avatar_url"] {
		t.Fatalf("chat summary %v", s)
	}

	// Replacing it with a JPEG deletes the old blob.
	jpegImg := avatarJPEG(t)
	code, got = e.uploadAvatar(id, "photo.png", "image/png", jpegImg)
	if code != 200 || got["revision"].(float64) != 3 {
		t.Fatalf("replace = %d %v", code, got)
	}
	second := fake.keys(prefix)
	if len(second) != 1 || second[0] == first[0] {
		t.Fatalf("keys after replace %v (before %v)", second, first)
	}
	if res, body := e.getAvatar(id); res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/jpeg" || !bytes.Equal(body, jpegImg) {
		t.Fatalf("get replaced avatar = %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}

	// WebP is accepted too.
	webp, _ := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	if code, got := e.uploadAvatar(id, "a.webp", "application/octet-stream", webp); code != 200 {
		t.Fatalf("webp = %d %v", code, got)
	}
	if res, _ := e.getAvatar(id); res.Header.Get("Content-Type") != "image/webp" {
		t.Fatalf("webp served as %q", res.Header.Get("Content-Type"))
	}
	if keys := fake.keys(prefix); len(keys) != 1 {
		t.Fatalf("keys after webp %v", keys)
	}

	// The unsent-attachment sweep only reads chat_attachments: even with a
	// cutoff in the future it never returns or removes an avatar.
	swept, err := e.q.DeleteStaleChatAttachments(ctx, store.DeleteStaleChatAttachmentsParams{Cutoff: time.Now().Add(time.Hour), RowLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range swept {
		if strings.HasPrefix(k, "personas/") {
			t.Fatalf("sweep returned avatar key %s", k)
		}
	}
	if res, _ := e.getAvatar(id); res.StatusCode != 200 || len(fake.keys(prefix)) != 1 {
		t.Fatalf("avatar after sweep = %d", res.StatusCode)
	}

	// Removing it deletes the blob; removing again is a no-op.
	rev := e.want("GET", "/personas/"+id, nil, 200)["revision"].(float64)
	del := e.want("DELETE", "/personas/"+id+"/avatar", nil, 200)
	if _, ok := del["avatar_url"]; ok || del["revision"].(float64) != rev+1 {
		t.Fatalf("delete avatar %v", del)
	}
	if keys := fake.keys(prefix); len(keys) != 0 {
		t.Fatalf("keys after delete %v", keys)
	}
	if res, _ := e.getAvatar(id); res.StatusCode != http.StatusNotFound {
		t.Fatalf("avatar after delete: %d", res.StatusCode)
	}
	if again := e.want("DELETE", "/personas/"+id+"/avatar", nil, 200); again["revision"].(float64) != rev+1 {
		t.Fatalf("second delete bumped the revision: %v", again)
	}

	// Deleting the persona deletes its avatar.
	if code, got := e.uploadAvatar(id, "a.png", "image/png", pngImg); code != 200 {
		t.Fatalf("re-upload = %d %v", code, got)
	}
	e.want("DELETE", "/personas/"+id, nil, http.StatusNoContent)
	if keys := fake.keys(prefix); len(keys) != 0 {
		t.Fatalf("keys after persona delete %v", keys)
	}
}

func TestPersonaAvatarRejectsUnsafeUploads(t *testing.T) {
	e, fake := newAvatarEnv(t)
	id := e.want("POST", "/personas", map[string]any{"name": "Work"}, http.StatusCreated)["id"].(string)
	pngImg, jpegImg := avatarPNG(t), avatarJPEG(t)
	var gifBuf bytes.Buffer
	if err := gif.Encode(&gifBuf, avatarImage(4, 4), nil); err != nil {
		t.Fatal(err)
	}
	const html = "<html><body><script>alert(document.cookie)</script></body></html>"
	oversize := append(append([]byte{}, pngImg...), make([]byte, personas.MaxAvatarBytes+1-len(pngImg))...)

	cases := []struct {
		name, filename, declared string
		data                     []byte
		code                     int
	}{
		{"svg", "a.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`), 415},
		{"svg spoofed as png", "a.png", "image/png", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`), 415},
		{"html spoofed as jpeg", "a.jpg", "image/jpeg", []byte(html), 415},
		{"gif", "a.gif", "image/gif", gifBuf.Bytes(), 415},
		{"png+html polyglot", "a.png", "image/png", append(append([]byte{}, pngImg...), html...), 415},
		{"jpeg+html polyglot", "a.jpg", "image/jpeg", append(append([]byte{}, jpegImg...), html...), 415},
		{"over 1 MB", "big.png", "image/png", oversize, 413},
		{"empty", "a.png", "image/png", nil, 422},
	}
	for _, c := range cases {
		code, got := e.uploadAvatar(id, c.filename, c.declared, c.data)
		if code != c.code || got["field"] != "file" {
			t.Fatalf("%s: %d %v, want %d on file", c.name, code, got, c.code)
		}
	}
	// Nothing was stored and the persona is unchanged.
	if keys := fake.keys(avatarBucket + "/"); len(keys) != 0 {
		t.Fatalf("refused uploads stored %v", keys)
	}
	if p := e.want("GET", "/personas/"+id, nil, 200); p["revision"].(float64) != 1 || p["avatar_url"] != nil {
		t.Fatalf("persona after refused uploads %v", p)
	}

	// Only the file field is accepted.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("note", "hi")
	mw.Close()
	req, _ := http.NewRequest("PUT", e.srv.URL+"/api/v1/personas/"+id+"/avatar", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("form without file = %d", res.StatusCode)
	}
}

func TestPersonaAvatarIsOwnerOnly(t *testing.T) {
	e, fake := newAvatarEnv(t)
	ctx := context.Background()
	stranger := e.strangerPersona()
	// Give the other person's persona a real avatar.
	var ws, user uuid.UUID
	if err := e.pool.QueryRow(ctx, `SELECT workspace_id, user_id FROM personas WHERE id=$1`, stranger).Scan(&ws, &user); err != nil {
		t.Fatal(err)
	}
	key := personas.AvatarKey(personas.Owner{WorkspaceID: ws, UserID: user}, uuid.MustParse(stranger))
	fake.put(avatarBucket+"/"+key, avatarPNG(t), "image/png")
	if _, err := e.pool.Exec(ctx, `UPDATE personas SET avatar_key=$2, avatar_content_type='image/png' WHERE id=$1`, stranger, key); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{stranger, uuid.NewString()} {
		if res, _ := e.getAvatar(id); res.StatusCode != http.StatusNotFound {
			t.Fatalf("GET avatar of %s = %d", id, res.StatusCode)
		}
		if code, got := e.uploadAvatar(id, "a.png", "image/png", avatarPNG(t)); code != http.StatusNotFound {
			t.Fatalf("PUT avatar of %s = %d %v", id, code, got)
		}
		e.want("DELETE", "/personas/"+id+"/avatar", nil, http.StatusNotFound)
	}
	// The other person's avatar is untouched.
	var still *string
	if err := e.pool.QueryRow(ctx, `SELECT avatar_key FROM personas WHERE id=$1`, stranger).Scan(&still); err != nil || still == nil || *still != key {
		t.Fatalf("stranger avatar key %v, %v", still, err)
	}
	if keys := fake.keys(avatarBucket + "/" + key); len(keys) != 1 {
		t.Fatalf("stranger blob %v", keys)
	}
}
