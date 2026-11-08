package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/personas"
)

func TestProfileUpdateAndValidation(t *testing.T) {
	e := newChatEnv(t)
	got := e.want("GET", "/me", nil, 200)
	if got["id"] != e.me.ID.String() {
		t.Fatalf("GET /me = %v", got)
	}
	got = e.want("PATCH", "/me", map[string]any{"name": "  ada king lovelace  "}, 200)
	if got["name"] != "ada king lovelace" || got["initials"] != "AL" {
		t.Fatalf("patch name = %v", got)
	}
	got = e.want("PATCH", "/me", map[string]any{"role_title": " Engineer "}, 200)
	if got["role_title"] != "Engineer" || got["name"] != "ada king lovelace" {
		t.Fatalf("patch role = %v", got)
	}
	got = e.want("PATCH", "/me", map[string]any{"name": "Prince"}, 200)
	if got["initials"] != "P" {
		t.Fatalf("one word initials = %v", got)
	}
	if got = e.want("PATCH", "/me", map[string]any{"role_title": ""}, 200); got["role_title"] != "" {
		t.Fatalf("clear role = %v", got)
	}
	boot := e.want("GET", "/bootstrap", nil, 200)
	if boot["me"].(map[string]any)["name"] != "Prince" {
		t.Fatalf("bootstrap me %v", boot["me"])
	}

	long := strings.Repeat("é", 81)
	e.invalid("PATCH", "/me", map[string]any{"name": "   "}, "name", "required")
	e.invalid("PATCH", "/me", map[string]any{"name": long}, "name", "at most")
	e.invalid("PATCH", "/me", map[string]any{"name": "a\x07b"}, "name", "control")
	e.invalid("PATCH", "/me", map[string]any{"role_title": long}, "role_title", "at most")
	e.invalid("PATCH", "/me", map[string]any{"role_title": "x\ny"}, "role_title", "control")
	e.invalid("PATCH", "/me", map[string]any{}, "body", "name or role_title")
	// 80 multibyte runes is fine.
	e.want("PATCH", "/me", map[string]any{"name": strings.Repeat("é", 80)}, 200)
}

func TestProfileAvatar(t *testing.T) {
	e, fake := newAvatarEnv(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("%s/users/%s/%s/avatar/", avatarBucket, e.me.WorkspaceID, e.me.ID)
	getAvatar := func(id string) (*http.Response, []byte) {
		e.t.Helper()
		res, err := http.Get(e.srv.URL + "/api/v1/users/" + id + "/avatar")
		if err != nil {
			e.t.Fatal(err)
		}
		defer res.Body.Close()
		var b bytes.Buffer
		b.ReadFrom(res.Body)
		return res, b.Bytes()
	}
	if res, _ := getAvatar(e.me.ID.String()); res.StatusCode != 404 {
		t.Fatalf("avatar before upload = %d", res.StatusCode)
	}
	if got := e.want("GET", "/me", nil, 200); got["avatar_url"] != nil {
		t.Fatalf("avatar_url before upload: %v", got)
	}

	img := avatarPNG(t)
	code, got := e.uploadAvatarTo("/me/avatar", "x.html", "text/html", img)
	url, _ := got["avatar_url"].(string)
	if code != 200 || !strings.HasPrefix(url, "/api/v1/users/"+e.me.ID.String()+"/avatar?v=") {
		t.Fatalf("upload = %d %v", code, got)
	}
	first := fake.keys(prefix)
	if len(first) != 1 {
		t.Fatalf("keys %v", first)
	}
	res, body := getAvatar(e.me.ID.String())
	if res.StatusCode != 200 || !bytes.Equal(body, img) || res.Header.Get("X-Content-Type-Options") != "nosniff" ||
		res.Header.Get("Content-Type") != "image/png" || res.Header.Get("Content-Security-Policy") != "default-src 'none'; sandbox" {
		t.Fatalf("serve = %d %v", res.StatusCode, res.Header)
	}

	// avatar_url shows up in bootstrap me and users, and in chat message authors.
	boot := e.want("GET", "/bootstrap", nil, 200)
	if boot["me"].(map[string]any)["avatar_url"] != url {
		t.Fatalf("bootstrap me %v", boot["me"])
	}
	found := false
	for _, u := range boot["users"].([]any) {
		if m := u.(map[string]any); m["id"] == e.me.ID.String() {
			found = m["avatar_url"] == url
		}
	}
	if !found {
		t.Fatalf("bootstrap users lack avatar_url")
	}

	// Replacing deletes the old blob.
	jpg := avatarJPEG(t)
	if code, got := e.uploadAvatarTo("/me/avatar", "a.jpg", "image/jpeg", jpg); code != 200 {
		t.Fatalf("replace = %d %v", code, got)
	}
	second := fake.keys(prefix)
	if len(second) != 1 || second[0] == first[0] {
		t.Fatalf("keys after replace %v (was %v)", second, first)
	}
	if res, body := getAvatar(e.me.ID.String()); res.Header.Get("Content-Type") != "image/jpeg" || !bytes.Equal(body, jpg) {
		t.Fatalf("replaced avatar served as %q", res.Header.Get("Content-Type"))
	}

	// Refused uploads change nothing.
	oversize := append(append([]byte{}, img...), make([]byte, personas.MaxAvatarBytes+1-len(img))...)
	for _, c := range []struct {
		name string
		data []byte
		code int
	}{
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`), 415},
		{"oversize", oversize, 413},
		{"empty", nil, 422},
	} {
		if code, got := e.uploadAvatarTo("/me/avatar", "a.png", "image/png", c.data); code != c.code || got["field"] != "file" {
			t.Fatalf("%s = %d %v", c.name, code, got)
		}
	}
	if keys := fake.keys(prefix); len(keys) != 1 || keys[0] != second[0] {
		t.Fatalf("keys after refusals %v", keys)
	}

	// Delete removes the blob and is idempotent.
	for range 2 {
		if del := e.want("DELETE", "/me/avatar", nil, 200); del["avatar_url"] != nil {
			t.Fatalf("delete = %v", del)
		}
	}
	if keys := fake.keys(prefix); len(keys) != 0 {
		t.Fatalf("keys after delete %v", keys)
	}
	if res, _ := getAvatar(e.me.ID.String()); res.StatusCode != 404 {
		t.Fatalf("avatar after delete = %d", res.StatusCode)
	}

	// Another workspace's user is a 404, even with a picture; so is an unknown id.
	var ws, other uuid.UUID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(e.pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('Other','other') RETURNING id`).Scan(&ws))
	must(e.pool.QueryRow(ctx, `INSERT INTO users(workspace_id,name,email,initials,password_hash,avatar_key,avatar_content_type) VALUES($1,'Bo','bo@example.com','B','x','k','image/png') RETURNING id`, ws).Scan(&other))
	fake.put(avatarBucket+"/k", img, "image/png")
	for _, id := range []string{other.String(), uuid.NewString()} {
		if res, _ := getAvatar(id); res.StatusCode != 404 {
			t.Fatalf("avatar of %s = %d", id, res.StatusCode)
		}
	}
}
