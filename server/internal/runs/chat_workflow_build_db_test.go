package runs

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// A build chat stays out of every list of the person's chats, and only a
// persona with the capability may propose workflows.
func TestHiddenBuildChatAndWorkflowCapability(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	rs := &Service{Pool: pool, Tracker: &tracker.Service{Pool: pool}, Artifacts: arts}
	ps := &personas.Service{Pool: pool}
	owner := personas.Owner{WorkspaceID: me.WorkspaceID, UserID: me.ID}
	yes := true
	builder, err := ps.Create(ctx, owner, personas.NewInput{Name: "Builder", Workflows: &yes})
	if err != nil {
		t.Fatal(err)
	}
	other, err := ps.Create(ctx, owner, personas.NewInput{Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}

	can := func(c store.ChatSession) bool {
		t.Helper()
		ok, err := SpeakerCanCreateWorkflows(ctx, q, c)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	body := "uniquebuildprompt add a step"
	hidden, err := rs.StartChat(ctx, me.WorkspaceID, me.ID, ChatInput{Project: "PAY", Title: "Workflow: x", PersonaID: &builder.ID, Hidden: true, Message: &ChatMessageInput{Body: body}})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := rs.StartChat(ctx, me.WorkspaceID, me.ID, ChatInput{Project: "PAY", PersonaID: &other.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !can(hidden) || can(plain) {
		t.Fatalf("capability: builder %v, other %v", can(hidden), can(plain))
	}

	listed, err := q.ListChatsForUser(ctx, store.ListChatsForUserParams{WorkspaceID: me.WorkspaceID, CreatedBy: me.ID, PersonaFilter: "all", RowLimit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != plain.ID {
		t.Fatalf("listed %+v", listed)
	}
	dock, err := q.PersonaDockChats(ctx, store.PersonaDockChatsParams{WorkspaceID: me.WorkspaceID, CreatedBy: me.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(dock) != 0 {
		t.Fatalf("dock %+v", dock)
	}
	found, err := q.SearchChatMessages(ctx, store.SearchChatMessagesParams{WorkspaceID: me.WorkspaceID, UserID: me.ID, Query: "uniquebuildprompt", RowLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("search %+v", found)
	}
	counts, err := q.PersonaChatCounts(ctx, []uuid.UUID{builder.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 0 {
		t.Fatalf("counts %+v", counts)
	}
}
