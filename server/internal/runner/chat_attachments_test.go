package runner_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/blob"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

type memBlobs map[string][]byte

func (m memBlobs) Get(_ context.Context, bucket, key string) (blob.Object, error) {
	b, ok := m[bucket+"/"+key]
	if !ok {
		return blob.Object{}, blob.ErrNotFound
	}
	return blob.Object{Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b))}, nil
}

// Uploads are bound only to their owner's next message and delivered to that
// turn's workspace under .lazyagents/attachments/ (LAZYAGENTS-72).
func TestChatAttachmentsAreBoundAndDelivered(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	blobs := memBlobs{}
	e.runner.Attachments, e.runner.Cfg.AttachmentBucket = blobs, "artifacts"
	upload := func(user uuid.UUID, name, body string) store.ChatAttachment {
		key := "chat-attachments/" + uuid.NewString()
		blobs["artifacts/"+key] = []byte(body)
		a, err := e.q.CreateChatAttachment(e.ctx, store.CreateChatAttachmentParams{WorkspaceID: e.ws, UserID: user, Filename: name,
			ContentType: "text/plain; charset=utf-8", SizeBytes: int64(len(body)), BlobKey: key})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	other, err := e.q.CreateUser(e.ctx, store.CreateUserParams{WorkspaceID: e.ws, Name: "Other", Email: "other@example.com", Initials: "OT"})
	if err != nil {
		t.Fatal(err)
	}
	theirs := upload(other.ID, "theirs.txt", "not yours")
	mine := upload(e.me.ID, "../notes.txt", "the secret plan")

	// Someone else's upload is refused and nothing is created.
	_, err = e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY",
		Message: &runs.ChatMessageInput{Body: "read this", AttachmentIDs: []uuid.UUID{theirs.ID}}})
	var ve *apperr.ValidationError
	if !errors.As(err, &ve) || ve.Field != "attachment_ids" {
		t.Fatalf("another person's attachment: %v", err)
	}
	var n int
	_ = e.pool.QueryRow(e.ctx, "SELECT count(*) FROM chat_sessions").Scan(&n)
	if n != 0 {
		t.Fatalf("%d chats created by a rejected start", n)
	}

	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY",
		Message: &runs.ChatMessageInput{Body: "Summarise the file", AttachmentIDs: []uuid.UUID{mine.ID, mine.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	msgs, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 20})
	if err != nil || len(msgs) != 2 {
		t.Fatalf("messages %+v %v", msgs, err)
	}
	file := ".lazyagents/attachments/turn-1/1-notes.txt"
	if reply := msgs[1].BodyMd; !strings.Contains(reply, file) || !strings.Contains(reply, "attached these files") {
		t.Fatalf("prompt does not list %s:\n%s", file, reply)
	}
	root := e.runner.Backend.WorkspaceDir(e.chatRun(chat.ID).ID.String())
	if got, err := os.ReadFile(filepath.Join(root, file)); err != nil || string(got) != "the secret plan" {
		t.Fatalf("%s: %q %v", file, got, err)
	}
	bound, _ := e.q.ListChatAttachmentsForMessages(e.ctx, []uuid.UUID{msgs[0].ID})
	if len(bound) != 1 || bound[0].ID != mine.ID {
		t.Fatalf("bound %+v", bound)
	}

	// An upload goes with one message only.
	_, err = e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "again", AttachmentIDs: []uuid.UUID{mine.ID}})
	if !errors.As(err, &ve) || ve.Field != "attachment_ids" {
		t.Fatalf("reused attachment: %v", err)
	}
}
