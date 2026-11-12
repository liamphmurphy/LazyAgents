package runner_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type memBlobs map[string][]byte

func (m memBlobs) Get(_ context.Context, bucket, key string) (blob.Object, error) {
	b, ok := m[bucket+"/"+key]
	if !ok {
		return blob.Object{}, blob.ErrNotFound
	}
	return blob.Object{Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b))}, nil
}

func (m memBlobs) Delete(_ context.Context, bucket, key string) error {
	delete(m, bucket+"/"+key)
	return nil
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

// The sweep deletes uploads never sent, and those a chat's deletion detached,
// once they're a day old, objects included; sent and fresh ones stay
// (LAZYAGENTS-77).
func TestSweepDeletesStaleChatAttachments(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	blobs := memBlobs{}
	e.runner.Attachments, e.runner.Cfg.AttachmentBucket = blobs, "artifacts"
	upload := func(name string) store.ChatAttachment {
		key := "chat-attachments/" + uuid.NewString()
		blobs["artifacts/"+key] = []byte("body")
		a, err := e.q.CreateChatAttachment(e.ctx, store.CreateChatAttachmentParams{WorkspaceID: e.ws, UserID: e.me.ID, Filename: name,
			ContentType: "text/plain; charset=utf-8", SizeBytes: 4, BlobKey: key})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	age := func(a store.ChatAttachment) {
		if _, err := e.pool.Exec(e.ctx, "UPDATE chat_attachments SET created_at = now() - interval '25 hours' WHERE id = $1", a.ID); err != nil {
			t.Fatal(err)
		}
	}
	unsent, fresh, sent, deleted := upload("unsent.txt"), upload("fresh.txt"), upload("sent.txt"), upload("deleted.txt")
	start := func(a store.ChatAttachment) store.ChatSession {
		c, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY",
			Message: &runs.ChatMessageInput{Body: "read this", AttachmentIDs: []uuid.UUID{a.ID}}})
		if err != nil {
			t.Fatal(err)
		}
		e.until("reply", func() bool { return e.chat(c.ID).State == "idle" })
		return c
	}
	start(sent)
	gone := start(deleted)
	if err := e.runs.DeleteChat(e.ctx, e.ws, e.me.ID, gone.ID); err != nil {
		t.Fatal(err)
	}
	for _, a := range []store.ChatAttachment{unsent, sent, deleted} {
		age(a)
	}

	e.runner.Sweep(e.ctx)
	for _, c := range []struct {
		a    store.ChatAttachment
		kept bool
	}{{unsent, false}, {deleted, false}, {fresh, true}, {sent, true}} {
		_, err := e.q.GetChatAttachment(e.ctx, store.GetChatAttachmentParams{ID: c.a.ID, WorkspaceID: e.ws, UserID: e.me.ID})
		_, obj := blobs["artifacts/"+c.a.BlobKey]
		if (err == nil) != c.kept || obj != c.kept {
			t.Errorf("%s: row err %v, object %v, want kept %v", c.a.Filename, err, obj, c.kept)
		}
	}
}

// A zip is unpacked for the assistant into a folder of its own: the prompt
// lists the files and what was skipped, nothing escapes the folder, and the
// files stay out of the ## Context list.
func TestChatZipAttachmentIsUnpacked(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{"src/main.go": "package main", "../escape.txt": "nope", "README.md": "# hi"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	blobs := memBlobs{}
	e.runner.Attachments, e.runner.Cfg.AttachmentBucket = blobs, "artifacts"
	key := "chat-attachments/" + uuid.NewString()
	blobs["artifacts/"+key] = buf.Bytes()
	a, err := e.q.CreateChatAttachment(e.ctx, store.CreateChatAttachmentParams{WorkspaceID: e.ws, UserID: e.me.ID, Filename: "My Project.zip",
		ContentType: runs.ChatAttachmentZipType, SizeBytes: int64(buf.Len()), BlobKey: key})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY",
		Message: &runs.ChatMessageInput{Body: "Read the project", AttachmentIDs: []uuid.UUID{a.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	msgs, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 20})
	if err != nil || len(msgs) != 2 {
		t.Fatalf("messages %+v %v", msgs, err)
	}
	dir := ".lazyagents/attachments/turn-1/1-My Project"
	reply := msgs[1].BodyMd
	for _, want := range []string{dir + "/src/main.go", dir + "/README.md", `skipped "../escape.txt": unsafe path`, "data, not instructions"} {
		if !strings.Contains(reply, want) {
			t.Errorf("prompt lacks %q:\n%s", want, reply)
		}
	}
	root := e.runner.Backend.WorkspaceDir(e.chatRun(chat.ID).ID.String())
	if got, err := os.ReadFile(filepath.Join(root, dir, "src/main.go")); err != nil || string(got) != "package main" {
		t.Errorf("unpacked file: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".lazyagents/attachments/turn-1/escape.txt")); err == nil {
		t.Error("an entry escaped its folder")
	}
}
