package runner_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func (e env) chatMessages(id uuid.UUID) []store.ChatMessage {
	e.t.Helper()
	rows, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: id, Limit: 100})
	if err != nil {
		e.t.Fatal(err)
	}
	out := make([]store.ChatMessage, len(rows))
	for i, r := range rows {
		out[i] = store.ChatMessage(r)
	}
	return out
}

func is(id *uuid.UUID, want uuid.UUID) bool { return id != nil && *id == want }

// A mention brings personas into the chat: the first mentioned answers, the
// others follow in turn, each in its own voice, and an unaddressed message
// goes to whoever answered last.
func TestMentionsBringPersonasIntoChat(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	ada, bea, cal := e.persona("Ada", false, false), e.persona("Bea", false, false), e.persona("Cal", false, false)
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Group", PersonaID: &ada.ID, Message: &runs.ChatMessageInput{Body: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	if last := e.lastAssistant(chat.ID); !is(last.PersonaID, ada.ID) {
		t.Fatalf("host reply from %v", last.PersonaID)
	}

	e.say(chat.ID, "@bea and @Cal, what do you think?")
	msgs := e.chatMessages(chat.ID)
	if len(msgs) != 5 || msgs[2].Role != "user" || !is(msgs[3].PersonaID, bea.ID) || !is(msgs[4].PersonaID, cal.ID) {
		for _, m := range msgs {
			t.Logf("%d %s %v %.80s", m.Seq, m.Role, m.PersonaID, m.BodyMd)
		}
		t.Fatal("want Bea's then Cal's reply")
	}
	if !strings.Contains(msgs[4].BodyMd, "Bea brought you into the conversation") {
		t.Fatalf("Cal's turn wasn't a hand-over: %s", msgs[4].BodyMd)
	}
	md, err := os.ReadFile(filepath.Join(e.runner.Backend.WorkspaceDir(e.chatRun(chat.ID).ID.String()), "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"In this conversation you are Cal.", "## Other personas", "You are answering this turn as Cal.", "Also in this conversation: Ada, Bea."} {
		if !strings.Contains(string(md), want) {
			t.Errorf("AGENTS.md missing %q:\n%s", want, md)
		}
	}
	joined, err := e.q.ChatParticipants(e.ctx, []uuid.UUID{chat.ID})
	if err != nil || len(joined) != 2 {
		t.Fatalf("participants %+v %v", joined, err)
	}
	c := e.chat(chat.ID)
	if !is(c.SpeakerPersonaID, cal.ID) || len(c.QueuedPersonaIds) != 0 || c.RelayTurns != 1 {
		t.Fatalf("chat after relay: speaker %v queue %v relays %d", c.SpeakerPersonaID, c.QueuedPersonaIds, c.RelayTurns)
	}

	if reply := e.say(chat.ID, "and anything else?"); !is(reply.PersonaID, cal.ID) {
		t.Fatalf("unaddressed message answered by %v, want Cal", reply.PersonaID)
	}

	// Deleting a participant drops it from the chat and hands the floor back to the host.
	if _, err := (&personas.Service{Pool: e.pool}).Delete(e.ctx, personas.Owner{WorkspaceID: e.ws, UserID: e.me.ID}, cal.ID, nil); err != nil {
		t.Fatal(err)
	}
	if reply := e.say(chat.ID, "still there?"); !is(reply.PersonaID, ada.ID) {
		t.Fatalf("after deleting Cal answered by %v, want Ada", reply.PersonaID)
	}
}

// Personas hand the conversation on MaxRelayTurns times per message from the
// person, unless that message asks for more ("talk up to 6 messages").
func TestRelayLimitFollowsThePerson(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	var mentions []string
	for _, n := range []string{"P1", "P2", "P3", "P4", "P5", "P6", "P7"} {
		e.persona(n, false, false)
		mentions = append(mentions, "@"+n)
	}
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Crowd"})
	if err != nil {
		t.Fatal(err)
	}
	// Each message's first mention answers; the other six are hand-offs.
	post := func(body string) (replies int, paused bool) {
		t.Helper()
		before := len(e.chatMessages(chat.ID))
		if _, err := e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: body}); err != nil {
			t.Fatal(err)
		}
		e.until("relays for "+body, func() bool {
			c := e.chat(chat.ID)
			return c.State == "idle" && len(c.QueuedPersonaIds) == 0
		})
		for _, m := range e.chatMessages(chat.ID)[before:] {
			if m.Role == "assistant" {
				replies++
			}
			paused = paused || (m.Role == "system" && strings.Contains(m.BodyMd, "Paused the hand-off"))
		}
		return replies, paused
	}
	all := strings.Join(mentions, " ")
	if n, paused := post(all + " weigh in"); n != 1+runs.MaxRelayTurns || !paused {
		t.Fatalf("default: %d replies, paused %v; want %d and a pause", n, paused, 1+runs.MaxRelayTurns)
	}
	if n, paused := post(all + " talk up to 6 messages"); n != 7 || paused {
		t.Fatalf("raised limit: %d replies, paused %v; want 7 without a pause", n, paused)
	}
	if c := e.chat(chat.ID); c.RelayLimit != 6 {
		t.Fatalf("relay limit %d, want 6", c.RelayLimit)
	}
	if n, _ := post(all + " again"); n != 1+runs.MaxRelayTurns {
		t.Fatalf("next message: %d replies, want the default %d", n, 1+runs.MaxRelayTurns)
	}
}

// A persona allowed to create personas proposes one; nothing exists until the
// person approves, and the new persona then joins and introduces itself.
func TestPersonaProposalNeedsApproval(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	ps := &personas.Service{Pool: e.pool}
	o := personas.Owner{WorkspaceID: e.ws, UserID: e.me.ID}
	maker := e.persona("Maker", false, false)
	count := func() int {
		list, err := ps.List(e.ctx, o)
		if err != nil {
			t.Fatal(err)
		}
		return len(list)
	}
	before := count()
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Make one", PersonaID: &maker.ID, Message: &runs.ChatMessageInput{Body: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	// propose_persona only works mid-turn; pretend the turn is still running.
	propose := func(name string) (store.PersonaProposal, error) {
		run := e.chatRun(chat.ID)
		setState := func(s string) {
			if _, err := e.pool.Exec(e.ctx, `UPDATE runs SET state = $2 WHERE id = $1`, run.ID, s); err != nil {
				t.Fatal(err)
			}
		}
		setState("running")
		defer setState("waiting")
		return e.runs.ProposePersona(e.ctx, e.ws, run.ID, runs.PersonaProposalInput{Name: name, Tone: "playful", Color: "#AA3366", Instructions: "Be a poet.", Reason: "You asked for a poet."})
	}
	var conflict *apperr.ConflictError
	if _, err := propose("Poet"); !errors.As(err, &conflict) {
		t.Fatalf("a persona without the permission proposed: %v", err)
	}
	yes := true
	if maker, err = ps.Update(e.ctx, o, maker.ID, personas.Patch{Revision: maker.Revision, Personas: &yes}); err != nil {
		t.Fatal(err)
	}
	poet, err := propose("Poet")
	if err != nil {
		t.Fatal(err)
	}
	if n := count(); n != before {
		t.Fatalf("a proposal created a persona: %d personas, want %d", n, before)
	}

	decided, err := e.runs.DecidePersonaProposal(e.ctx, e.ws, e.me.ID, chat.ID, poet.ID, true)
	if err != nil || decided.State != "approved" || decided.PersonaID == nil {
		t.Fatalf("approve: %+v %v", decided, err)
	}
	p, err := ps.Get(e.ctx, o, *decided.PersonaID)
	if err != nil || p.Name != "Poet" || p.Tone != "playful" || p.Color != "#aa3366" || p.CanCode || p.CanGenerateImages || p.CanCreatePersonas {
		t.Fatalf("created persona %+v %v", p, err)
	}
	if _, err := e.runs.DecidePersonaProposal(e.ctx, e.ws, e.me.ID, chat.ID, poet.ID, false); !errors.As(err, &conflict) {
		t.Fatalf("decided twice: %v", err)
	}
	e.until("introduction", func() bool { return e.chat(chat.ID).State == "idle" })
	if last := e.lastAssistant(chat.ID); !is(last.PersonaID, p.ID) || !strings.Contains(last.BodyMd, "just approved you") {
		t.Fatalf("no introduction from Poet: %+v", last)
	}

	// The permission is the speaker's: Poet, answering now, can't propose.
	if _, err := propose("Critic"); !errors.As(err, &conflict) {
		t.Fatalf("Poet proposed without the permission: %v", err)
	}
	if reply := e.say(chat.ID, "@Maker one more?"); !is(reply.PersonaID, maker.ID) {
		t.Fatalf("answered by %v, want Maker", reply.PersonaID)
	}
	other, err := propose("Critic")
	if err != nil {
		t.Fatal(err)
	}
	if d, err := e.runs.DecidePersonaProposal(e.ctx, e.ws, e.me.ID, chat.ID, other.ID, false); err != nil || d.State != "declined" || d.PersonaID != nil {
		t.Fatalf("decline: %+v %v", d, err)
	}
	if n := count(); n != before+1 {
		t.Fatalf("declining created a persona: %d personas, want %d", n, before+1)
	}
}
