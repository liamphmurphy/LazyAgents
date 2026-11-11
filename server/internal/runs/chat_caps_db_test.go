package runs

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// Segments end only for what a persona's capabilities change, a repository
// the persona no longer lists revokes coding, and a new chat takes the default
// persona resolved when it was created.
func TestChatSegmentsAndCapabilities(t *testing.T) {
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
	yes, repo := true, "https://github.com/acme/app"
	repos := []string{repo}
	p, err := ps.Create(ctx, owner, personas.NewInput{Name: "Pair", IsDefault: true, Code: &yes, Repos: &repos})
	if err != nil {
		t.Fatal(err)
	}

	// No persona given: the default's id is stored on the chat.
	chat, err := rs.StartChat(ctx, me.WorkspaceID, me.ID, ChatInput{Project: "PAY"})
	if err != nil || chat.PersonaID == nil || *chat.PersonaID != p.ID || chat.RepoUrl == nil || *chat.RepoUrl != repo {
		t.Fatalf("default persona chat %+v, %v", chat, err)
	}

	// release hands the turn back so the next message can arrive.
	post := func(body string) uuid.UUID {
		t.Helper()
		if _, err := rs.PostChatMessage(ctx, me.WorkspaceID, me.ID, chat.ID, ChatMessageInput{Body: body}); err != nil {
			t.Fatal(err)
		}
		c, _ := q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: me.WorkspaceID})
		run := *c.CurrentRunID
		if _, err := q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: chat.ID, CurrentRunID: &run, State: "idle"}); err != nil {
			t.Fatal(err)
		}
		if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: run, State: "waiting"}); err != nil {
			t.Fatal(err)
		}
		return run
	}
	first := post("one")
	run, _ := q.GetRun(ctx, store.GetRunParams{ID: first, WorkspaceID: me.WorkspaceID})
	if spec, _ := Spec(run); spec.Mode("repo:write") != agentdef.Allowed {
		t.Fatalf("a coding chat must start coding: %+v", spec.Scopes)
	}

	// Editing the assistant's definition keeps the segment.
	assistant, err := agents.EnsureAssistant(ctx, q, me.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := (&agents.Service{Pool: pool}).SetOverride(ctx, me.WorkspaceID, me.ID, assistant.Handle, "PAY", agentdef.Override{InstructionsAppend: "Be brief."}); err != nil {
		t.Fatal(err)
	}
	if got := post("two"); got != first {
		t.Fatal("an unrelated change to the assistant's definition ended the segment")
	}

	// Removing a discovery shortcut does not revoke coding or end the segment.
	none := []string{}
	if p, err = ps.Update(ctx, owner, p.ID, personas.Patch{Revision: p.Revision, Repos: &none}); err != nil {
		t.Fatal(err)
	}
	chatRow, _ := q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: me.WorkspaceID})
	caps, err := chatCaps(ctx, q, chatRow)
	if err != nil || !caps.Coding() {
		t.Fatalf("removing a shortcut revoked coding: %+v, %v", caps, err)
	}
	if got := post("three"); got != first {
		t.Fatal("removing a shortcut ended the segment")
	}
	off := false
	if p, err = ps.Update(ctx, owner, p.ID, personas.Patch{Revision: p.Revision, Code: &off}); err != nil {
		t.Fatal(err)
	}
	second := post("three")
	if second == first {
		t.Fatal("revoking coding must end the segment")
	}
	run, _ = q.GetRun(ctx, store.GetRunParams{ID: second, WorkspaceID: me.WorkspaceID})
	if spec, _ := Spec(run); spec.Mode("repo:write") != agentdef.Denied || spec.Mode("tests:run") != agentdef.Denied {
		t.Fatalf("new segment still codes: %+v", spec.Scopes)
	}
}
