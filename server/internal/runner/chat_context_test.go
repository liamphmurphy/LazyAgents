package runner_test

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Context refs attached to a message are delivered as generated-name files
// under .lazyagents/context/ for that turn and listed in the turn's prompt.
func TestChatContextRefsAreDeliveredPerTurn(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	var art uuid.UUID
	var title, body, key string
	if err := e.pool.QueryRow(e.ctx, `SELECT a.id, a.title, v.body_md FROM artifacts a JOIN artifact_versions v ON v.id=a.current_version_id
		JOIN projects p ON p.id=a.project_id WHERE p.key='PAY' AND v.body_md IS NOT NULL AND a.type<>'note' LIMIT 1`).Scan(&art, &title, &body); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(e.ctx, `SELECT t.key FROM tickets t JOIN projects p ON p.id=t.project_id WHERE p.key='PAY' ORDER BY t.number LIMIT 1`).Scan(&key); err != nil {
		t.Fatal(err)
	}

	// An unknown ref is a 422 naming it, and nothing is queued.
	missing := uuid.New()
	_, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY",
		Message: &runs.ChatMessageInput{Body: "hi", ContextRefs: []runs.ChatContextRef{{Kind: "artifact", ID: &missing}}}})
	var ve *apperr.ValidationError
	if !errors.As(err, &ve) || ve.Field != "context_refs" || !strings.Contains(ve.Msg, missing.String()) {
		t.Fatalf("unknown ref: %v", err)
	}
	var n int
	_ = e.pool.QueryRow(e.ctx, "SELECT count(*) FROM chat_sessions").Scan(&n)
	if n != 0 {
		t.Fatalf("%d chats were created by a rejected start", n)
	}

	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY",
		Message: &runs.ChatMessageInput{Body: "Summarise these", ContextRefs: []runs.ChatContextRef{{Kind: "artifact", ID: &art}, {Kind: "ticket", Key: key}}}})
	if err != nil {
		t.Fatal(err)
	}
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	run := e.chatRun(chat.ID)
	root := e.runner.Backend.WorkspaceDir(run.ID.String())
	msgs, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 20})
	if err != nil || len(msgs) != 2 {
		t.Fatalf("messages %+v %v", msgs, err)
	}
	reply := msgs[1].BodyMd // the fake harness echoes the prompt it received
	name := regexp.MustCompile(`^chat-turn-\d+-ref-\d+\.md$`)
	for i, want := range []string{body, key} {
		file := ".lazyagents/context/chat-turn-1-ref-" + string(rune('1'+i)) + ".md"
		if !strings.Contains(reply, file) || !name.MatchString(filepath.Base(file)) {
			t.Fatalf("prompt does not list %s:\n%s", file, reply)
		}
		got, err := os.ReadFile(filepath.Join(root, file))
		if err != nil || !strings.Contains(string(got), want) {
			t.Fatalf("%s: %v\n%s", file, err, got)
		}
	}
	if !strings.Contains(reply, "Summarise these") || strings.Contains(reply, "no longer available") {
		t.Fatalf("prompt %s", reply)
	}

	// The next message has no refs: no ref list, and the earlier turn's files stay.
	if _, err := e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "And now?"}); err != nil {
		t.Fatal(err)
	}
	e.until("second reply", func() bool { return e.chat(chat.ID).State == "idle" })
	msgs, _ = e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 20})
	if second := msgs[3].BodyMd; strings.Contains(second, "attached these references") || strings.Contains(second, "chat-turn-2-ref") {
		t.Fatalf("second prompt lists refs: %s", second)
	}
	// A ticket in another project can't be attached.
	if _, err := e.pool.Exec(e.ctx, "UPDATE tickets SET project_id=(SELECT id FROM projects WHERE key<>'PAY' ORDER BY key LIMIT 1) WHERE key=$1", key); err != nil {
		t.Fatal(err)
	}
	_, err = e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "again", ContextRefs: []runs.ChatContextRef{{Kind: "ticket", Key: key}}})
	if !errors.As(err, &ve) || !strings.Contains(ve.Msg, key) {
		t.Fatalf("cross-project ref: %v", err)
	}
}
