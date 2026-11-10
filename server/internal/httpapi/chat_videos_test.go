package httpapi

import (
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

func fetch(t *testing.T, url string, header map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(b)
}

// A chat's videos are listed with their frames; frames are cacheable by
// version, the finished video seeks with byte ranges, and its Library copy
// shows its length and poster. Deleting the Library copy keeps the video the
// chat still shows; deleting the chat lets the sweep take everything.
func TestChatVideosAPI(t *testing.T) {
	e, s3 := newAvatarEnv(t)
	ctx := t.Context()
	chat := e.newChat(nil)
	chatID := uuid.MustParse(chat["id"].(string))
	reply, err := e.q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chatID, Role: "assistant", BodyMd: "here", ContextRefs: []byte("[]"), Citations: []byte("[]")})
	if err != nil {
		t.Fatal(err)
	}
	job, err := e.q.CreateChatVideoJob(ctx, store.CreateChatVideoJobParams{WorkspaceID: e.me.WorkspaceID, UserID: e.me.ID, ChatID: &chatID, Title: "Black cat",
		Width: 1920, Height: 1080, Fps: 30, DurationMs: 15000, ScriptPath: "cat.py"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.q.SetChatVideoStage(ctx, store.SetChatVideoStageParams{ID: job.ID, Stage: "stills", Checks: []byte(`[{"label":"Paws planted","passed":true},{"label":"Framing","passed":null}]`)}); err != nil {
		t.Fatal(err)
	}
	jpeg := []byte("\xff\xd8\xff\xe0frame")
	frameKey := "chat-videos/" + uuid.NewString()
	s3.put(avatarBucket+"/"+frameKey, jpeg, "image/jpeg")
	if _, err := e.q.UpsertChatVideoFrame(ctx, store.UpsertChatVideoFrameParams{JobID: job.ID, Kind: "still", Idx: 120, BlobKey: frameKey, ContentType: "image/jpeg", SizeBytes: int64(len(jpeg)), SourceStamp: "1:1"}); err != nil {
		t.Fatal(err)
	}

	list := e.want("GET", "/chats/"+chatID.String()+"/videos", nil, 200)["videos"].([]any)
	if len(list) != 1 {
		t.Fatalf("videos %v", list)
	}
	v := list[0].(map[string]any)
	checks := v["checks"].([]any)
	if v["stage"] != "stills" || v["state"] != "running" || v["output"] != nil || v["script_path"] != "cat.py" || len(checks) != 2 ||
		checks[0].(map[string]any)["passed"] != true || checks[1].(map[string]any)["passed"] != nil {
		t.Fatalf("video %v", v)
	}
	frames := v["frames"].([]any)
	f := frames[0].(map[string]any)
	if len(frames) != 1 || f["kind"] != "still" || f["idx"] != float64(120) {
		t.Fatalf("frames %v", frames)
	}
	base := e.srv.URL + "/api/v1/chats/" + chatID.String() + "/videos/" + job.ID.String()
	version := strconv.FormatInt(int64(f["version"].(float64)), 10)
	res, body := fetch(t, base+"/frames/still/120?v="+version, nil)
	if res.StatusCode != 200 || body != string(jpeg) || res.Header.Get("Cache-Control") != "private, max-age=31536000, immutable" {
		t.Fatalf("frame %d %q %s", res.StatusCode, body, res.Header.Get("Cache-Control"))
	}
	if res, _ := fetch(t, base+"/frames/still/120?v=1", nil); res.Header.Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("stale version cached: %s", res.Header.Get("Cache-Control"))
	}
	if res, _ := fetch(t, base+"/frames/strip/3", nil); res.StatusCode != 404 {
		t.Fatalf("missing frame %d", res.StatusCode)
	}
	// Not finished: nothing to play.
	if res, _ := fetch(t, base+"/content", nil); res.StatusCode != 404 {
		t.Fatalf("unfinished content %d", res.StatusCode)
	}

	// Finished, with a Library copy.
	mp4 := []byte("0123456789abcdef")
	videoKey, posterKey := "chat-videos/"+uuid.NewString()+".mp4", "library-posters/"+uuid.NewString()
	s3.put(avatarBucket+"/"+videoKey, mp4, "video/mp4")
	s3.put(avatarBucket+"/"+posterKey, jpeg, "image/jpeg")
	dur, w, h := int32(15000), int32(1920), int32(1080)
	item, err := e.q.CreateLibraryVideo(ctx, store.CreateLibraryVideoParams{WorkspaceID: e.me.WorkspaceID, UserID: e.me.ID, Title: "Black cat", Filename: "black-cat.mp4",
		ContentType: "video/mp4", SizeBytes: int64(len(mp4)), BlobKey: videoKey, ChatID: &chatID, MessageID: &reply.ID, DurationMs: &dur, Width: &w, Height: &h, PosterKey: &posterKey})
	if err != nil {
		t.Fatal(err)
	}
	size, name, ct := int64(len(mp4)), "black-cat.mp4", "video/mp4"
	if _, err := e.q.FinishChatVideoJob(ctx, store.FinishChatVideoJobParams{ID: job.ID, OutputBlobKey: &videoKey, OutputFilename: &name, OutputContentType: &ct,
		OutputSizeBytes: &size, OutputCodec: "H.264", OutputDurationMs: &dur, LibraryItemID: &item.ID}); err != nil {
		t.Fatal(err)
	}
	v = e.want("GET", "/chats/"+chatID.String()+"/videos", nil, 200)["videos"].([]any)[0].(map[string]any)
	out, _ := v["output"].(map[string]any)
	if v["state"] != "done" || out["codec"] != "H.264" || out["size_bytes"] != float64(16) || v["library_item_id"] != item.ID.String() {
		t.Fatalf("finished %v", v)
	}
	res, body = fetch(t, base+"/content", nil)
	if res.StatusCode != 200 || body != string(mp4) || res.Header.Get("Accept-Ranges") != "bytes" || res.Header.Get("Content-Type") != "video/mp4" {
		t.Fatalf("content %d %q %v", res.StatusCode, body, res.Header)
	}
	for rng, want := range map[string][3]string{
		"bytes=2-5":     {"206", "2345", "bytes 2-5/16"},
		"bytes=10-":     {"206", "abcdef", "bytes 10-15/16"},
		"bytes=-3":      {"206", "def", "bytes 13-15/16"},
		"bytes=4-999":   {"206", "456789abcdef", "bytes 4-15/16"},
		"bytes=16-":     {"416", "", "bytes */16"},
		"bytes=0-1,4-5": {"200", string(mp4), ""},
	} {
		res, body := fetch(t, base+"/content", map[string]string{"Range": rng})
		if strconv.Itoa(res.StatusCode) != want[0] || (want[0] != "416" && body != want[1]) || res.Header.Get("Content-Range") != want[2] {
			t.Errorf("%s: %d %q %q", rng, res.StatusCode, body, res.Header.Get("Content-Range"))
		}
	}
	// The Library copy seeks too, and has its length and poster.
	items := e.want("GET", "/library?kind=video", nil, 200)["items"].([]any)
	it := items[0].(map[string]any)
	if len(items) != 1 || it["duration_ms"] != float64(15000) || it["width"] != float64(1920) || it["has_poster"] != true {
		t.Fatalf("library %v", items)
	}
	lib := e.srv.URL + "/api/v1/library/" + item.ID.String()
	if res, body := fetch(t, lib+"/content", map[string]string{"Range": "bytes=0-3"}); res.StatusCode != 206 || body != "0123" {
		t.Fatalf("library range %d %q", res.StatusCode, body)
	}
	if res, body := fetch(t, lib+"/poster", nil); res.StatusCode != 200 || body != string(jpeg) {
		t.Fatalf("poster %d %q", res.StatusCode, body)
	}

	// Removing it from the Library takes its poster but not the video the chat shows.
	e.want("DELETE", "/library/"+item.ID.String(), nil, http.StatusNoContent)
	if len(s3.keys(avatarBucket+"/"+posterKey)) != 0 || len(s3.keys(avatarBucket+"/"+videoKey)) != 1 {
		t.Fatalf("after library delete: poster %v video %v", s3.keys(avatarBucket+"/"+posterKey), s3.keys(avatarBucket+"/"+videoKey))
	}
	if res, _ := fetch(t, base+"/content", nil); res.StatusCode != 200 {
		t.Fatalf("chat video gone with its Library copy: %d", res.StatusCode)
	}

	// Deleting the chat detaches the job; the sweep returns its frame and video.
	e.want("DELETE", "/chats/"+chatID.String(), nil, http.StatusNoContent)
	keys, err := e.q.DeleteDetachedChatVideoJobs(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, k := range keys {
		got[k] = true
	}
	if len(keys) != 2 || !got[frameKey] || !got[videoKey] {
		t.Fatalf("swept %v", keys)
	}
	if n, err := e.q.ListChatVideoJobs(ctx, &chatID); err != nil || len(n) != 0 {
		t.Fatalf("jobs left %v %v", n, err)
	}
}

// Another person's chat video is a 404 everywhere.
func TestChatVideosArePrivate(t *testing.T) {
	e, _ := newAvatarEnv(t)
	ctx := t.Context()
	other, err := e.q.CreateUser(ctx, store.CreateUserParams{WorkspaceID: e.me.WorkspaceID, Name: "Other", Email: "o@example.com", Initials: "OT"})
	if err != nil {
		t.Fatal(err)
	}
	theirs := e.newChat(nil)
	chatID := uuid.MustParse(theirs["id"].(string))
	if _, err := e.pool.Exec(ctx, `UPDATE chat_sessions SET created_by=$2 WHERE id=$1`, chatID, other.ID); err != nil {
		t.Fatal(err)
	}
	chat := store.ChatSession{ID: chatID}
	job, err := e.q.CreateChatVideoJob(ctx, store.CreateChatVideoJobParams{WorkspaceID: e.me.WorkspaceID, UserID: other.ID, ChatID: &chat.ID, Title: "Theirs", Width: 64, Height: 64, Fps: 1, DurationMs: 1000})
	if err != nil {
		t.Fatal(err)
	}
	e.want("GET", "/chats/"+chat.ID.String()+"/videos", nil, 404)
	e.want("GET", "/chats/"+chat.ID.String()+"/videos/"+job.ID.String()+"/content", nil, 404)
}
