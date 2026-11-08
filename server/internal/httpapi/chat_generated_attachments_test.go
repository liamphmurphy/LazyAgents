package httpapi

import (
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Images the assistant generated are attachments on its message, served to
// the chat's owner like uploads and marked with their source.
func TestGeneratedAttachmentsOnAssistantMessages(t *testing.T) {
	e, s3 := newAvatarEnv(t)
	chat := e.newChat(nil)
	chatID := uuid.MustParse(chat["id"].(string))
	ctx := t.Context()
	user, err := e.q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chatID, Role: "user", BodyMd: "draw", ContextRefs: []byte("[]"), Citations: []byte("[]")})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := e.q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chatID, Role: "assistant", BodyMd: "here", ContextRefs: []byte("[]"), Citations: []byte("[]")})
	if err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG\r\n\x1a\nbytes")
	key := "chat-attachments/" + uuid.NewString()
	s3.put(avatarBucket+"/"+key, png, "image/png")
	if _, err := e.q.CreateGeneratedChatAttachment(ctx, store.CreateGeneratedChatAttachmentParams{WorkspaceID: e.me.WorkspaceID, UserID: e.me.ID, MessageID: &reply.ID,
		Filename: "cat.png", ContentType: "image/png", SizeBytes: int64(len(png)), BlobKey: key}); err != nil {
		t.Fatal(err)
	}

	list := e.want("GET", "/chats/"+chatID.String()+"/messages", nil, 200)["messages"].([]any)
	if len(list) != 2 {
		t.Fatalf("messages %v", list)
	}
	if _, ok := list[0].(map[string]any)["attachments"].([]any); !ok || len(list[0].(map[string]any)["attachments"].([]any)) != 0 || list[0].(map[string]any)["id"] != user.ID.String() {
		t.Fatalf("a user message always lists attachments: %v", list[0])
	}
	atts, _ := list[1].(map[string]any)["attachments"].([]any)
	if len(atts) != 1 {
		t.Fatalf("assistant attachments %v", list[1])
	}
	a := atts[0].(map[string]any)
	if a["source"] != "generated" || a["filename"] != "cat.png" || a["content_type"] != "image/png" {
		t.Fatalf("attachment %v", a)
	}
	res, err := http.Get(e.srv.URL + "/api/v1/chat-attachments/" + a["id"].(string) + "/content")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || string(body) != string(png) || res.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("content %d %q %s", res.StatusCode, body, res.Header.Get("Content-Type"))
	}
	// Someone else's request can't read it.
	other, err := e.q.CreateUser(ctx, store.CreateUserParams{WorkspaceID: e.me.WorkspaceID, Name: "Other", Email: "o@example.com", Initials: "OT"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.q.GetChatAttachment(ctx, store.GetChatAttachmentParams{ID: uuid.MustParse(a["id"].(string)), WorkspaceID: e.me.WorkspaceID, UserID: other.ID}); err == nil {
		t.Fatal("another user can load the generated image")
	}
}
