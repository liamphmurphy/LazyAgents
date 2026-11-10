package httpapi

import (
	"bytes"
	"context"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A proposal's picture shows on the card once its turn has linked it, and
// approving stores it as the new persona's avatar: cropped, scaled down and
// re-encoded, however large the generated image was.
func TestApprovingAProposalSetsItsAvatar(t *testing.T) {
	e, fake := newAvatarEnv(t)
	ctx := context.Background()
	ws := e.me.WorkspaceID
	yes := true
	maker, err := (&personas.Service{Pool: e.pool}).Create(ctx, personas.Owner{WorkspaceID: ws, UserID: e.me.ID}, personas.NewInput{Name: "Maker", Images: &yes, Personas: &yes})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := e.runs.StartChat(ctx, ws, e.me.ID, runs.ChatInput{Project: "PAY", PersonaID: &maker.ID, Message: &runs.ChatMessageInput{Body: "make a poet"}})
	if err != nil {
		t.Fatal(err)
	}
	chat, _ = e.q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: ws})
	runID := *chat.CurrentRunID
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Mid-turn, as when the persona calls propose_persona.
	exec("UPDATE runs SET state='running', harness='claude' WHERE id=$1", runID)
	exec("UPDATE run_turns SET status='running', ended_at=NULL WHERE run_id=$1", runID)
	prop, err := e.runs.ProposePersona(ctx, ws, runID, runs.PersonaProposalInput{Name: "Poet", AvatarPrompt: "a poet with a quill"})
	if err != nil || prop.AvatarJobID == nil {
		t.Fatalf("propose: %+v %v", prop, err)
	}
	list := func() map[string]any {
		t.Helper()
		out := e.want("GET", "/chats/"+chat.ID.String()+"/persona-proposals", nil, 200)
		return out["proposals"].([]any)[0].(map[string]any)
	}
	if p := list(); p["avatar_state"] != "pending" || p["avatar_attachment_id"] != nil {
		t.Fatalf("while making it: %v", p)
	}
	if code, out := e.do("POST", "/chats/"+chat.ID.String()+"/persona-proposals/"+prop.ID.String(), map[string]any{"decision": "approve"}); code != 409 {
		t.Fatalf("approve while pending = %d %v", code, out)
	}

	// What the runner does when the turn ends: store the image, attach it
	// to the reply and the Library, link it to the job and end the turn.
	var big bytes.Buffer
	if err := png.Encode(&big, avatarImage(1024, 1536)); err != nil {
		t.Fatal(err)
	}
	key := runs.ChatAttachmentKey(ws, e.me.ID)
	fake.put(avatarBucket+"/"+key, big.Bytes(), "image/png")
	msg, err := e.q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chat.ID, Role: "assistant", BodyMd: "Proposed Poet.", ContextRefs: []byte("[]"), Citations: []byte("[]")})
	if err != nil {
		t.Fatal(err)
	}
	att, err := e.q.CreateGeneratedChatAttachment(ctx, store.CreateGeneratedChatAttachmentParams{WorkspaceID: ws, UserID: e.me.ID, MessageID: &msg.ID, Filename: "poet.png", ContentType: "image/png", SizeBytes: int64(big.Len()), BlobKey: key})
	if err != nil {
		t.Fatal(err)
	}
	item, err := e.q.CreateLibraryItem(ctx, store.CreateLibraryItemParams{WorkspaceID: ws, UserID: e.me.ID, Kind: "image", Title: "Poet", Filename: "poet.png", ContentType: "image/png", SizeBytes: int64(big.Len()), BlobKey: key, ChatID: &chat.ID, MessageID: &msg.ID, PersonaID: &maker.ID})
	if err != nil {
		t.Fatal(err)
	}
	// The person already uses this image as their own likeness: approving
	// mustn't take that away.
	exec("UPDATE library_items SET reference='self' WHERE id=$1", item.ID)
	exec("UPDATE chat_image_jobs SET state='done', attachment_id=$2 WHERE id=$1", *prop.AvatarJobID, att.ID)
	exec("UPDATE run_turns SET status='done', ended_at=now() WHERE run_id=$1", runID)
	if p := list(); p["avatar_state"] != "ready" || p["avatar_attachment_id"] != att.ID.String() {
		t.Fatalf("after the turn: %v", p)
	}

	out := e.want("POST", "/chats/"+chat.ID.String()+"/persona-proposals/"+prop.ID.String(), map[string]any{"decision": "approve"}, 200)
	persona, _ := out["persona"].(map[string]any)
	if persona == nil || persona["avatar_url"] == nil {
		t.Fatalf("approved without an avatar: %v", out)
	}
	res, body := e.getAvatar(persona["id"].(string))
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("avatar: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	img, err := jpeg.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != personas.AvatarSide || b.Dy() != personas.AvatarSide || len(body) > personas.MaxAvatarBytes {
		t.Fatalf("avatar %v, %d bytes", b.Size(), len(body))
	}
	var ref string
	if err := e.pool.QueryRow(ctx, "SELECT coalesce(reference, '') FROM library_items WHERE id=$1", item.ID).Scan(&ref); err != nil || ref != "self" {
		t.Fatalf("the person's reference became %q (%v)", ref, err)
	}
	// The generated image stays where it was: the chat and the Library keep it.
	if keys := fake.keys(avatarBucket + "/" + key); len(keys) != 1 {
		t.Fatalf("generated image gone: %v", keys)
	}
}

// A picture that can't be made into an avatar doesn't undo the approval: the
// persona is created without one.
func TestApprovingAProposalWithABrokenPicture(t *testing.T) {
	e, fake := newAvatarEnv(t)
	ctx := context.Background()
	ws := e.me.WorkspaceID
	yes := true
	maker, err := (&personas.Service{Pool: e.pool}).Create(ctx, personas.Owner{WorkspaceID: ws, UserID: e.me.ID}, personas.NewInput{Name: "Maker", Images: &yes, Personas: &yes})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := e.runs.StartChat(ctx, ws, e.me.ID, runs.ChatInput{Project: "PAY", PersonaID: &maker.ID, Message: &runs.ChatMessageInput{Body: "make a poet"}})
	if err != nil {
		t.Fatal(err)
	}
	chat, _ = e.q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: ws})
	runID := *chat.CurrentRunID
	if _, err := e.pool.Exec(ctx, "UPDATE runs SET state='running', harness='claude' WHERE id=$1", runID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, "UPDATE run_turns SET status='running', ended_at=NULL WHERE run_id=$1", runID); err != nil {
		t.Fatal(err)
	}
	prop, err := e.runs.ProposePersona(ctx, ws, runID, runs.PersonaProposalInput{Name: "Poet", AvatarPrompt: "a poet"})
	if err != nil {
		t.Fatal(err)
	}
	key := runs.ChatAttachmentKey(ws, e.me.ID)
	fake.put(avatarBucket+"/"+key, []byte("\x89PNG\r\n\x1a\nnot really"), "image/png")
	att, err := e.q.CreateGeneratedChatAttachment(ctx, store.CreateGeneratedChatAttachmentParams{WorkspaceID: ws, UserID: e.me.ID, Filename: "poet.png", ContentType: "image/png", SizeBytes: 10, BlobKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, "UPDATE chat_image_jobs SET state='done', attachment_id=$2 WHERE id=$1", *prop.AvatarJobID, att.ID); err != nil {
		t.Fatal(err)
	}
	out := e.want("POST", "/chats/"+chat.ID.String()+"/persona-proposals/"+prop.ID.String(), map[string]any{"decision": "approve"}, 200)
	persona, _ := out["persona"].(map[string]any)
	if persona == nil || persona["avatar_url"] != nil || out["state"] != "approved" {
		t.Fatalf("approve: %v", out)
	}
	if keys := fake.keys(avatarBucket + "/personas/"); len(keys) != 0 {
		t.Fatalf("left avatar blobs %v", keys)
	}
}
