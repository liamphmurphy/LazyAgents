package runs

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// An aside is a hidden, read-only chat answered by a fresh session that sees
// the parent's running reply; it never touches the parent's conversation.
func TestChatAside(t *testing.T) {
	f := newChatFixture(t)
	parent, err := f.rs.StartChat(f.ctx, f.ws, f.user, ChatInput{Message: &ChatMessageInput{Body: "fix the flaky retry test"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rs.StartChatAside(f.ctx, f.ws, f.user, parent.ID, "   "); err == nil {
		t.Fatal("an empty aside was accepted")
	}

	// What the running reply has done so far.
	run := *f.chat(t, parent.ID).CurrentRunID
	for _, e := range []struct {
		typ     string
		payload map[string]any
	}{
		{"message", map[string]any{"role": "assistant", "text": "Reproducing it first."}},
		{"tool_call", map[string]any{"id": "t1", "tool": "Bash", "input": map[string]any{"command": "go test ./runner -count=50"}}},
		{"tool_result", map[string]any{"id": "t1", "tool": "Bash", "output": "--- FAIL: TestRetry (2.00s)\nFAIL 3 of 50"}},
	} {
		raw, _ := json.Marshal(e.payload)
		if err := f.q.AddRunEvent(f.ctx, store.AddRunEventParams{RunID: run, Type: e.typ, Payload: raw}); err != nil {
			t.Fatal(err)
		}
	}

	aside, err := f.rs.StartChatAside(f.ctx, f.ws, f.user, parent.ID, "how many runs failed so far?")
	if err != nil {
		t.Fatal(err)
	}
	if !aside.Hidden || aside.ParentChatID == nil || *aside.ParentChatID != parent.ID || aside.RepoUrl != nil || aside.State != "running" {
		t.Fatalf("aside chat %+v", aside)
	}
	if aside.Harness != parent.Harness || aside.Model != parent.Model {
		t.Fatalf("aside runs %s/%s, parent %s/%s", aside.Harness, aside.Model, parent.Harness, parent.Model)
	}
	if got := f.userMessages(t, aside.ID); len(got) != 1 || got[0] != "how many runs failed so far?" {
		t.Fatalf("aside stored %q, want only the question", got)
	}
	if got := f.userMessages(t, parent.ID); len(got) != 1 {
		t.Fatalf("the aside reached the parent's conversation: %q", got)
	}
	if c := f.chat(t, parent.ID); c.State != "running" || *c.CurrentRunID != run {
		t.Fatalf("the aside disturbed the running reply: %+v", c)
	}

	// Its turn sees the conversation, the live reply and the question, and isn't asked to retitle.
	turns, err := f.q.ListTurns(f.ctx, *aside.CurrentRunID)
	if err != nil || len(turns) != 1 {
		t.Fatalf("aside turns %d, %v", len(turns), err)
	}
	prompt := turns[0].InputMessage
	for _, want := range []string{"fix the flaky retry test", "Reproducing it first.", "Called Bash", "go test ./runner -count=50", "FAIL 3 of 50", "this is an aside", "how many runs failed so far?"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("aside prompt lacks %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "different harness or model") {
		t.Error("an aside was told other models answered the conversation")
	}
	if strings.Contains(prompt, "set_chat_title") {
		t.Error("an aside was asked to title itself")
	}

	// Read-only, whatever the persona answering the parent may do.
	asideRun, err := f.q.GetRun(f.ctx, store.GetRunParams{ID: *aside.CurrentRunID, WorkspaceID: f.ws})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := Spec(asideRun)
	if err != nil {
		t.Fatal(err)
	}
	if spec.HarnessOptions[agentdef.OptionAside] != "true" || spec.HarnessOptions["sandbox"] != "read-only" || spec.Mode("repo:write") == agentdef.Allowed || len(spec.MCPServers) != 0 {
		t.Fatalf("aside spec isn't read-only: %+v %+v", spec.HarnessOptions, spec.Scopes)
	}

	// Hidden from the chat list; listed under its parent.
	list, err := f.q.ListChatsForUser(f.ctx, store.ListChatsForUserParams{WorkspaceID: f.ws, CreatedBy: f.user, PersonaFilter: "all", SpaceFilter: "all", RowLimit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if c.ID == aside.ID {
			t.Fatal("the aside is in the chat list")
		}
	}
	if got, _ := f.rs.ListChatAsides(f.ctx, f.ws, f.user, parent.ID); len(got) != 1 || got[0].ID != aside.ID {
		t.Fatalf("parent's asides %+v", got)
	}
	if _, err := f.rs.QueueChatFollowup(f.ctx, f.ws, f.user, aside.ID, "and?"); !isConflict(err) {
		t.Fatalf("queueing on an aside: %v", err)
	}

	// At most MaxLiveAsides answer at once.
	for i := 1; i < MaxLiveAsides; i++ {
		if _, err := f.rs.StartChatAside(f.ctx, f.ws, f.user, parent.ID, "still there?"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.rs.StartChatAside(f.ctx, f.ws, f.user, parent.ID, "one too many"); !isConflict(err) {
		t.Fatalf("aside past the limit: %v", err)
	}

	// Once it has answered, its segment ends.
	f.endTurn(t, aside.ID, "idle")
	if err := EndAside(f.ctx, f.q, aside.ID); err != nil {
		t.Fatal(err)
	}
	if r, _ := f.q.GetRun(f.ctx, store.GetRunParams{ID: asideRun.ID, WorkspaceID: f.ws}); r.State != "completed" {
		t.Fatalf("answered aside's run is %s", r.State)
	}

	// Nothing running: there's nothing to ask about.
	f.endTurn(t, parent.ID, "idle")
	if _, err := f.rs.StartChatAside(f.ctx, f.ws, f.user, parent.ID, "and now?"); !isConflict(err) {
		t.Fatalf("aside on an idle chat: %v", err)
	}

	// Deleting the chat deletes its asides.
	if err := f.rs.DeleteChat(f.ctx, f.ws, f.user, parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.GetChatSession(f.ctx, store.GetChatSessionParams{ID: aside.ID, WorkspaceID: f.ws}); err == nil {
		t.Fatal("the aside outlived its chat")
	}
}
