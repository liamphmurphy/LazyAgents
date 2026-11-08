package httpapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/linkpreview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type fakePreviews struct {
	calls atomic.Int32
	empty atomic.Bool
}

func (f *fakePreviews) Fetch(_ context.Context, url string) linkpreview.Preview {
	f.calls.Add(1)
	if f.empty.Load() {
		return linkpreview.Preview{Kind: linkpreview.KindSite}
	}
	if strings.Contains(url, "youtu") {
		return linkpreview.Preview{Kind: linkpreview.KindVideo, Title: "A talk", Site: "Lab", VideoID: "dQw4w9WgXcQ"}
	}
	return linkpreview.Preview{Kind: linkpreview.KindSite, Title: "Routes", Description: "Quiet", Image: []byte("\x89PNG\r\n\x1a\nbytes"), ImageType: "image/png"}
}

func usePreviews(t *testing.T) *fakePreviews {
	t.Helper()
	old := linkPreviews
	f := &fakePreviews{}
	linkPreviews = f
	t.Cleanup(func() { linkPreviews = old })
	return f
}

func (e *chatEnv) previewMessage(t *testing.T, role, body string) string {
	t.Helper()
	chat := uuid.MustParse(e.newChat(nil)["id"].(string))
	m, err := e.q.AppendChatMessage(t.Context(), store.AppendChatMessageParams{SessionID: chat, Role: role, BodyMd: body, ContextRefs: []byte("[]"), Citations: []byte("[]")})
	if err != nil {
		t.Fatal(err)
	}
	return m.ID.String()
}

const recommendReply = "Try:\n\n```recommend\nhttps://a.example/x\nWorth it.\n```\n\n```recommend\nhttps://youtu.be/dQw4w9WgXcQ\n```\n"

func TestLinkPreviewsFetchOnceAndServeTheImage(t *testing.T) {
	e, _ := newAvatarEnv(t)
	f := usePreviews(t)
	id := e.previewMessage(t, "assistant", recommendReply)

	got := e.want("GET", "/chat-messages/"+id+"/link-previews", nil, 200)["previews"].([]any)
	if len(got) != 2 {
		t.Fatalf("previews %v", got)
	}
	site, video := got[0].(map[string]any), got[1].(map[string]any)
	if site["url"] != "https://a.example/x" || site["kind"] != "site" || site["title"] != "Routes" || site["image_url"] == nil {
		t.Fatalf("site %v", site)
	}
	if video["kind"] != "video" || video["video_id"] != "dQw4w9WgXcQ" || video["image_url"] != nil {
		t.Fatalf("video %v", video)
	}

	e.want("GET", "/chat-messages/"+id+"/link-previews", nil, 200)
	if n := f.calls.Load(); n != 2 {
		t.Fatalf("fetched %d times", n)
	}

	res, err := http.Get(e.srv.URL + site["image_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.HasPrefix(string(body), "\x89PNG") || res.Header.Get("Content-Type") != "image/png" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("image %d %v", res.StatusCode, res.Header)
	}
}

func TestLinkPreviewFailureIsCachedThenRetried(t *testing.T) {
	e, _ := newAvatarEnv(t)
	f := usePreviews(t)
	f.empty.Store(true)
	id := e.previewMessage(t, "assistant", "```recommend\nhttps://a.example/x\n```")
	path := "/chat-messages/" + id + "/link-previews"

	got := e.want("GET", path, nil, 200)["previews"].([]any)
	if got[0].(map[string]any)["title"] != "" {
		t.Fatalf("%v", got)
	}
	e.want("GET", path, nil, 200)
	if n := f.calls.Load(); n != 1 {
		t.Fatalf("fetched %d times", n)
	}
	f.empty.Store(false)
	if _, err := e.pool.Exec(t.Context(), `UPDATE link_previews SET fetched_at = now() - interval '2 days'`); err != nil {
		t.Fatal(err)
	}
	got = e.want("GET", path, nil, 200)["previews"].([]any)
	if got[0].(map[string]any)["title"] != "Routes" {
		t.Fatalf("%v", got)
	}
}

func TestLinkPreviewsOnlyReadAssistantFencesInYourChats(t *testing.T) {
	e, _ := newAvatarEnv(t)
	f := usePreviews(t)
	mine := e.previewMessage(t, "user", recommendReply)
	if got := e.want("GET", "/chat-messages/"+mine+"/link-previews", nil, 200)["previews"].([]any); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	e.want("GET", "/chat-messages/"+uuid.NewString()+"/link-previews", nil, http.StatusNotFound)
	e.want("GET", "/link-previews/"+uuid.NewString()+"/image", nil, http.StatusNotFound)
	if n := f.calls.Load(); n != 0 {
		t.Fatalf("fetched %d times", n)
	}
}
