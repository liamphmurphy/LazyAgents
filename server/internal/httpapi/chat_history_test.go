package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func (e *personaChatEnv) hits(token string, args map[string]any) []any {
	e.t.Helper()
	code, out := e.call(token, "search_chats", args)
	if code != 200 {
		e.t.Fatalf("search_chats %v: %d %v", args, code, out)
	}
	return out["chats"].([]any)
}

// A persona finds what was said in its own earlier chats with the person, but
// not in this chat, another persona's chats (unless any_persona) or a
// temporary chat; read_chat opens a hit under the same rules.
func TestSearchChatsStaysInThePersonasOwnChats(t *testing.T) {
	e := newPersonaChatEnv(t)
	gwenPast, _ := e.chat(e.gwen, false, "We compared Kokoro and Cartesia for the voice feature.")
	coachPast, _ := e.chat(e.coach, false, "Kokoro has a cheaper plan than Cartesia.")
	e.chat(e.gwen, true, "Kokoro came up in a temporary chat too.")
	_, now := e.chat(e.gwen, false, "Kokoro, again, in this very chat.")

	hits := e.hits(now, map[string]any{"query": "kokoro"})
	if len(hits) != 1 {
		t.Fatalf("hits = %v, want only Gwen's earlier chat", hits)
	}
	h := hits[0].(map[string]any)
	if h["chat_id"] != gwenPast.ID.String() || h["persona"] != "Gwen" || !strings.Contains(h["snippet"].(string), "**Kokoro**") {
		t.Fatalf("hit %v", h)
	}
	if n := len(e.hits(now, map[string]any{"query": "kokoro", "any_persona": true})); n != 2 {
		t.Errorf("any_persona hits = %d, want Gwen's and Coach's", n)
	}

	read := func(id uuid.UUID, anyPersona bool) (int, map[string]any) {
		return e.call(now, "read_chat", map[string]any{"chat_id": id.String(), "any_persona": anyPersona})
	}
	if code, out := read(gwenPast.ID, false); code != 200 || len(out["messages"].([]any)) != 2 {
		t.Errorf("read own chat: %d %v", code, out)
	}
	if code, _ := read(coachPast.ID, false); code != http.StatusNotFound {
		t.Errorf("read another persona's chat without any_persona: %d", code)
	}
	if code, out := read(coachPast.ID, true); code != 200 || out["messages"].([]any)[1].(map[string]any)["persona"] != "Coach" {
		t.Errorf("read another persona's chat with any_persona: %d %v", code, out)
	}
	if code, _ := e.call(now, "search_chats", map[string]any{"query": " "}); code != 422 {
		t.Errorf("empty query: %d", code)
	}
}
