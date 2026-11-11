package runs

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

type chatFixture struct {
	ctx  context.Context
	q    *store.Queries
	rs   *Service
	ws   uuid.UUID
	user uuid.UUID
}

// newChatFixture seeds a workspace whose default persona can code in a repository.
func newChatFixture(t *testing.T) chatFixture {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	yes, repos := true, []string{"https://github.com/acme/app"}
	if _, err := (&personas.Service{Pool: pool}).Create(ctx, personas.Owner{WorkspaceID: me.WorkspaceID, UserID: me.ID}, personas.NewInput{Name: "Pair", IsDefault: true, Code: &yes, Repos: &repos}); err != nil {
		t.Fatal(err)
	}
	return chatFixture{ctx: ctx, q: q, rs: &Service{Pool: pool, Tracker: &tracker.Service{Pool: pool}, Artifacts: arts}, ws: me.WorkspaceID, user: me.ID}
}

func (f chatFixture) chat(t *testing.T, id uuid.UUID) store.ChatSession {
	t.Helper()
	c, err := f.q.GetChatSession(f.ctx, store.GetChatSessionParams{ID: id, WorkspaceID: f.ws})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// endTurn stands in for the runner finishing the chat's turn: the segment
// waits for the person and the chat takes the given state.
func (f chatFixture) endTurn(t *testing.T, id uuid.UUID, state string) {
	t.Helper()
	c := f.chat(t, id)
	run := *c.CurrentRunID
	if _, err := f.q.SetRunState(f.ctx, store.SetRunStateParams{ID: run, State: "waiting"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.SetChatRunState(f.ctx, store.SetChatRunStateParams{ID: id, CurrentRunID: &run, State: state}); err != nil {
		t.Fatal(err)
	}
}

func (f chatFixture) userMessages(t *testing.T, id uuid.UUID) []string {
	t.Helper()
	rows, err := f.q.ListChatMessages(f.ctx, store.ListChatMessagesParams{SessionID: id, Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range rows {
		if m.Role == "user" {
			out = append(out, m.BodyMd)
		}
	}
	return out
}

func isConflict(err error) bool {
	var c *apperr.ConflictError
	return errors.As(err, &c)
}

// Follow-ups wait for the running reply, go out one per finished turn in the
// order they were queued, and stay held after a turn that failed or was stopped.
func TestChatFollowupQueue(t *testing.T) {
	f := newChatFixture(t)
	chat, err := f.rs.StartChat(f.ctx, f.ws, f.user, ChatInput{Message: &ChatMessageInput{Body: "fix the flaky test"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"then open a PR", "and update the docs"} {
		if _, err := f.rs.QueueChatFollowup(f.ctx, f.ws, f.user, chat.ID, body); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.userMessages(t, chat.ID); len(got) != 1 {
		t.Fatalf("queueing interrupted the reply: %q", got)
	}
	if c := f.chat(t, chat.ID); c.State != "running" || c.InterruptMessageID != nil {
		t.Fatalf("chat after queueing: state %s, interrupt %v", c.State, c.InterruptMessageID)
	}

	// The reply ends: the oldest follow-up is sent as the next message.
	f.endTurn(t, chat.ID, "idle")
	if err := SendNextFollowup(f.ctx, f.q, chat.ID, 0); err != nil {
		t.Fatal(err)
	}
	if got := f.userMessages(t, chat.ID); len(got) != 2 || got[1] != "then open a PR" {
		t.Fatalf("after the first reply: %q", got)
	}
	if c := f.chat(t, chat.ID); c.State != "running" {
		t.Fatalf("the follow-up didn't start a turn: %s", c.State)
	}
	left, _ := f.rs.ListChatFollowups(f.ctx, f.ws, f.user, chat.ID)
	if len(left) != 1 || left[0].Body != "and update the docs" {
		t.Fatalf("queue after one send: %+v", left)
	}

	// A busy chat keeps its queue for the turn that is running.
	if err := SendNextFollowup(f.ctx, f.q, chat.ID, 0); err != nil {
		t.Fatal(err)
	}
	if got := f.userMessages(t, chat.ID); len(got) != 2 {
		t.Fatalf("sent while a turn was running: %q", got)
	}

	// A failed turn holds the queue.
	f.endTurn(t, chat.ID, "failed")
	if err := SendNextFollowup(f.ctx, f.q, chat.ID, 0); err != nil {
		t.Fatal(err)
	}
	if got := f.userMessages(t, chat.ID); len(got) != 2 {
		t.Fatalf("sent after a failure: %q", got)
	}

	if err := f.rs.DeleteChatFollowup(f.ctx, f.ws, f.user, chat.ID, left[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := f.rs.DeleteChatFollowup(f.ctx, f.ws, f.user, chat.ID, left[0].ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("deleting a sent or removed follow-up: %v", err)
	}

	// Queued to an idle chat with nothing waiting: the reply already ended, so it goes now.
	f.endTurn(t, chat.ID, "idle")
	if _, err := f.rs.QueueChatFollowup(f.ctx, f.ws, f.user, chat.ID, "one more thing"); err != nil {
		t.Fatal(err)
	}
	if got := f.userMessages(t, chat.ID); len(got) != 3 || got[2] != "one more thing" {
		t.Fatalf("an idle chat held its only follow-up: %q", got)
	}
	if left, _ := f.rs.ListChatFollowups(f.ctx, f.ws, f.user, chat.ID); len(left) != 0 {
		t.Fatalf("sent follow-up still queued: %+v", left)
	}

	// Someone else's chat is not found.
	if _, err := f.rs.QueueChatFollowup(f.ctx, f.ws, uuid.New(), chat.ID, "hi"); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("queueing on another person's chat: %v", err)
	}
}
