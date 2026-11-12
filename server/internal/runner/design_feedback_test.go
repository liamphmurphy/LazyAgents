package runner_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/canvas"
	"github.com/liamphmurphy/vesper/server/internal/design"
	"github.com/liamphmurphy/vesper/server/internal/designs"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// What the person decides on a persona's design suggestions is posted into the
// design's chat when they resolve it, and reaches the persona in the prompt of
// the next turn (once). A selection ref delivers a short context file.
func TestDesignReviewFeedbackAndSelectionReachTheNextTurn(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	svc := &designs.Service{Pool: e.pool, Runs: e.runs}
	cv, chat, err := svc.Create(e.ctx, e.ws, e.me.ID, "Checkout", "mobile-flow", "")
	if err != nil {
		t.Fatal(err)
	}
	cur, err := e.q.LatestChatCanvasVersion(e.ctx, cv.ID)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := design.Parse(cur.Body)
	frame := d["pages"].([]any)[0].(map[string]any)["children"].([]any)[0].(map[string]any)
	kids := frame["children"].([]any)
	title, copy := kids[0].(map[string]any)["id"].(string), kids[1].(map[string]any)["id"].(string)

	p, err := svc.Propose(e.ctx, designs.Patch{CanvasID: cv.ID, ChatID: chat.ID, AgentID: chat.AgentID, Base: cur.Seq, Summary: "Tidy the Start screen", Ops: []design.Op{
		{"op": "set_node", "id": title, "fields": map[string]any{"name": "Heading"}, "why": "Remove the placeholder copy"},
		{"op": "set_node", "id": copy, "fields": map[string]any{"name": "Body"}, "why": "Name the body"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	// Meanwhile the person deletes the second line themselves.
	gone, err := design.Apply(cur.Body, []design.Op{{"op": "remove_node", "id": copy}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&canvas.Service{Pool: e.pool}).Save(e.ctx, chat.ID, cv.ID, cur.Seq, gone, "Removed a line", canvas.Actor{Type: "user", ID: e.me.ID}); err != nil {
		t.Fatal(err)
	}
	mine, err := e.q.GetMyCanvas(e.ctx, store.GetMyCanvasParams{ID: cv.ID, WorkspaceID: e.ws, UserID: e.me.ID})
	if err != nil {
		t.Fatal(err)
	}
	// Op 0 is rejected with a reason; op 1 is accepted but renames a node the
	// person has removed since, so it comes out stale.
	if _, err := svc.Resolve(e.ctx, mine, e.ws, e.me.ID, p.Proposal.ID, []designs.Decision{
		{I: 0, State: designs.Rejected, Reason: "keep it for now"}, {I: 1, State: designs.Accepted}}); err != nil {
		t.Fatal(err)
	}

	msgs, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 20})
	if err != nil || len(msgs) != 1 || msgs[0].Role != "system" {
		t.Fatalf("messages after resolve: %+v %v", msgs, err)
	}
	want := "rejected 'Remove the placeholder copy': keep it for now"
	if !strings.Contains(msgs[0].BodyMd, want) || !strings.Contains(msgs[0].BodyMd, "Tidy the Start screen") || !strings.Contains(msgs[0].BodyMd, "'Name the body' was accepted but") {
		t.Fatalf("feedback: %q", msgs[0].BodyMd)
	}
	if got, _ := e.q.GetChatSession(e.ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: e.ws}); got.State == "running" {
		t.Fatal("posting feedback started a turn")
	}

	// The next message's prompt carries it, with the selection's context file.
	if _, err := e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "Make this bigger", ContextRefs: []runs.ChatContextRef{{Kind: "canvas", ID: &cv.ID,
		Selection: &canvas.Selection{Page: d["pages"].([]any)[0].(map[string]any)["id"].(string), Nodes: []string{title}, Viewport: canvas.Viewport{W: 800, H: 600, Zoom: 1}}}}}); err != nil {
		t.Fatal(err)
	}
	e.until("reply", func() bool { return e.chat(chat.ID).State == "idle" })
	msgs, _ = e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 20})
	reply := msgs[len(msgs)-1].BodyMd // the fake harness echoes the prompt it received
	if !strings.Contains(reply, want) || !strings.Contains(reply, "reviewed your design suggestions") {
		t.Fatalf("the persona's prompt lacks the review feedback:\n%s", reply)
	}
	root := e.runner.Backend.WorkspaceDir(e.chatRun(chat.ID).ID.String())
	got, err := os.ReadFile(filepath.Join(root, ".lazyagents/context/chat-turn-1-ref-1.md"))
	if err != nil || !strings.Contains(string(got), "- "+title+":") || !strings.Contains(string(got), "When the person says this/these") || strings.Contains(string(got), `"pages"`) {
		t.Fatalf("selection doc: %v\n%s", err, got)
	}

	// Told once: the following message doesn't repeat it.
	if _, err := e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "Thanks"}); err != nil {
		t.Fatal(err)
	}
	e.until("second reply", func() bool { return e.chat(chat.ID).State == "idle" })
	msgs, _ = e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 20})
	if again := msgs[len(msgs)-1].BodyMd; strings.Contains(again, want) {
		t.Fatalf("feedback repeated:\n%s", again)
	}
}
