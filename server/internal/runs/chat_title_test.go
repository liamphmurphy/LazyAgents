package runs

import (
	"context"
	"strings"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

func TestChatFirstMessageTitleAndNote(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	rs := &Service{Pool: pool, Tracker: &tracker.Service{Pool: pool}, Artifacts: arts}

	chat, err := rs.StartChat(ctx, me.WorkspaceID, me.ID, ChatInput{Project: "PAY"})
	if err != nil || chat.Title != "" {
		t.Fatalf("empty chat = %+v, %v", chat, err)
	}
	if _, err = rs.PostChatMessage(ctx, me.WorkspaceID, me.ID, chat.ID, ChatMessageInput{Body: "Plan the refund flow\nwith details"}); err != nil {
		t.Fatal(err)
	}
	chat, _ = q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: me.WorkspaceID})
	if chat.Title != "Plan the refund flow" {
		t.Fatalf("fallback title = %q", chat.Title)
	}
	turnInput := func() string {
		t.Helper()
		var in string
		if err := pool.QueryRow(ctx, "SELECT input_message FROM run_turns WHERE run_id=$1 ORDER BY seq DESC LIMIT 1", *chat.CurrentRunID).Scan(&in); err != nil {
			t.Fatal(err)
		}
		return in
	}
	if in := turnInput(); !strings.Contains(in, "set_chat_title") || !strings.HasPrefix(in, "Plan the refund flow\nwith details") {
		t.Fatalf("first turn input = %q", in)
	}
	msgs, _ := q.ListChatMessages(ctx, store.ListChatMessagesParams{SessionID: chat.ID, Seq: 0, Limit: 10})
	if len(msgs) != 1 || strings.Contains(msgs[0].BodyMd, "set_chat_title") {
		t.Fatalf("stored message = %+v", msgs)
	}

	// Release the turn, then a second message gets no note.
	if _, err = q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: chat.ID, CurrentRunID: chat.CurrentRunID, State: "idle"}); err != nil {
		t.Fatal(err)
	}
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: *chat.CurrentRunID, State: "waiting"}); err != nil {
		t.Fatal(err)
	}
	if _, err = rs.PostChatMessage(ctx, me.WorkspaceID, me.ID, chat.ID, ChatMessageInput{Body: "And then?"}); err != nil {
		t.Fatal(err)
	}
	if in := turnInput(); in != "And then?" {
		t.Fatalf("second turn input = %q", in)
	}
}
