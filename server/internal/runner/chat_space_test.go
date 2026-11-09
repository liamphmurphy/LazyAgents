package runner_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/memory"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A chat in a space gets the space's instructions below the persona, its
// sources as files, and its memory in the index, refreshed every turn; a chat
// outside the space sees none of it.
func TestChatSpaceContextPerTurn(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	q := store.New(e.pool)
	instructions := "Act as my training coach."
	sp, err := e.runs.CreateSpace(e.ctx, e.ws, e.me.ID, runs.SpaceInput{Name: new("Health"), Instructions: &instructions})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateChatSpaceSource(e.ctx, store.CreateChatSpaceSourceParams{SpaceID: sp.ID, Kind: "text", Title: "Training plan", Body: "Lift Mon, Wed, Fri.", ContentType: "text/markdown", SizeBytes: 19, Tokens: 5}); err != nil {
		t.Fatal(err)
	}
	mem := memory.New(q)
	person := memory.Owner{WorkspaceID: e.ws, UserID: e.me.ID}
	if _, err := mem.Save(e.ctx, person.InSpace(&sp.ID), memory.Input{Title: "Protein target", Summary: "About 140 g a day"}, memory.Source{Label: "manual"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Save(e.ctx, person, memory.Input{Title: "Prefers metric units"}, memory.Source{Label: "manual"}); err != nil {
		t.Fatal(err)
	}

	read := func(chat uuid.UUID, name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(e.runner.Backend.WorkspaceDir(e.chatRun(chat).ID.String()), name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{SpaceID: &sp.ID, Message: &runs.ChatMessageInput{Body: "How much protein?"}})
	if err != nil {
		t.Fatal(err)
	}
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	md := read(chat.ID, "AGENTS.md")
	for _, want := range []string{"## Space: Health", "Act as my training coach.", "`.lazyagents/context/space-1-training-plan.md`: pasted text \"Training plan\"",
		"### Space memory", "[other, this space] Protein target", "Prefers metric units"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if got := read(chat.ID, ".lazyagents/context/space-1-training-plan.md"); got != "Lift Mon, Wed, Fri." {
		t.Fatalf("source file %q", got)
	}

	// An edit shows on the next turn.
	edited := "Keep answers under 100 words."
	if _, err := e.runs.UpdateSpace(e.ctx, e.ws, e.me.ID, sp.ID, runs.SpaceInput{Instructions: &edited, SpaceMemory: new(false)}); err != nil {
		t.Fatal(err)
	}
	e.say(chat.ID, "And on rest days?")
	md = read(chat.ID, "AGENTS.md")
	if !strings.Contains(md, edited) || strings.Contains(md, "Act as my training coach.") || strings.Contains(md, "Protein target") || strings.Contains(md, "### Space memory") {
		t.Fatalf("second turn:\n%s", md)
	}

	// A chat outside the space has none of it.
	plain, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Message: &runs.ChatMessageInput{Body: "Hi"}})
	if err != nil {
		t.Fatal(err)
	}
	e.until("plain reply", func() bool { return e.chat(plain.ID).State == "idle" })
	if md := read(plain.ID, "AGENTS.md"); strings.Contains(md, "## Space") || strings.Contains(md, "Protein target") {
		t.Fatalf("outside the space:\n%s", md)
	}
}
