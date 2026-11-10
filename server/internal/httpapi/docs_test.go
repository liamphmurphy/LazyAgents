package httpapi

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// A document started in the Docs app is a text canvas in its own chat, kept
// out of the chat list and the Spaces menu, and found again by its id.
func TestCreateDoc(t *testing.T) {
	e := newChatEnv(t)

	doc := e.want("POST", "/docs", map[string]any{"title": "  Trip   plan "}, http.StatusCreated)
	id, chat := doc["id"].(string), doc["chat_id"].(string)
	if doc["title"] != "Trip plan" || doc["kind"] != "text" || doc["chat_title"] != "Trip plan" {
		t.Fatalf("created %v", doc)
	}
	cv := e.want("GET", "/chats/"+chat+"/canvases/"+id, nil, http.StatusOK)
	if cv["body"] != "# Trip plan\n" || cv["seq"].(float64) != 1 {
		t.Fatalf("first version %v", cv)
	}
	if got := e.want("GET", "/canvases/"+id, nil, http.StatusOK); got["chat_id"] != chat || got["title"] != "Trip plan" {
		t.Fatalf("by id %v", got)
	}
	e.want("GET", "/canvases/"+uuid.NewString(), nil, http.StatusNotFound)

	untitled := e.want("POST", "/docs", map[string]any{}, http.StatusCreated)
	if untitled["title"] != "Untitled document" {
		t.Fatalf("untitled %v", untitled)
	}
	e.invalid("POST", "/docs", map[string]any{"title": strings.Repeat("x", 201)}, "title", "200")

	// Both documents' chats share one Docs space, which stays out of the menu.
	if a, b := e.want("GET", "/chats/"+chat, nil, http.StatusOK)["space"], e.want("GET", "/chats/"+untitled["chat_id"].(string), nil, http.StatusOK)["space"]; a == nil || a.(map[string]any)["id"] != b.(map[string]any)["id"] {
		t.Fatalf("spaces %v %v", a, b)
	}
	if spaces := e.want("GET", "/chat-spaces", nil, http.StatusOK)["spaces"].([]any); len(spaces) != 0 {
		t.Fatalf("Spaces list %v", spaces)
	}
	for _, c := range e.want("GET", "/chats", nil, http.StatusOK)["chats"].([]any) {
		if c.(map[string]any)["id"] == chat {
			t.Fatal("a document's chat is in the chat list")
		}
	}
	listed := 0
	for _, c := range e.want("GET", "/canvases", nil, http.StatusOK)["canvases"].([]any) {
		if m := c.(map[string]any); m["id"] == id || m["id"] == untitled["id"] {
			listed++
		}
	}
	if listed != 2 {
		t.Fatalf("listed %d of 2 documents", listed)
	}
}

// The review mark only moves forward, and only to a version the document has,
// so a stale tab can't reopen a review the person already finished.
func TestMarkDocReviewed(t *testing.T) {
	e := newChatEnv(t)
	doc := e.want("POST", "/docs", map[string]any{"title": "Plan"}, http.StatusCreated)
	id, chat := doc["id"].(string), doc["chat_id"].(string)
	if got := e.want("GET", "/canvases/"+id, nil, http.StatusOK)["reviewed_seq"]; got != float64(0) {
		t.Fatalf("new document reviewed_seq %v", got)
	}
	e.want("POST", "/chats/"+chat+"/canvases/"+id+"/versions", map[string]any{"base_seq": 1, "body": "# Plan\n\nMore.\n"}, http.StatusCreated)

	if got := e.want("POST", "/docs/"+id+"/reviewed", map[string]any{"seq": 2}, http.StatusOK)["reviewed_seq"]; got != float64(2) {
		t.Fatalf("reviewed_seq %v, want 2", got)
	}
	if got := e.want("POST", "/docs/"+id+"/reviewed", map[string]any{"seq": 1}, http.StatusOK)["reviewed_seq"]; got != float64(2) {
		t.Fatalf("moved back to %v", got)
	}
	if got := e.want("GET", "/canvases/"+id, nil, http.StatusOK)["reviewed_seq"]; got != float64(2) {
		t.Fatalf("GET reviewed_seq %v", got)
	}
	e.invalid("POST", "/docs/"+id+"/reviewed", map[string]any{"seq": 3}, "seq", "no version 3")
	e.want("POST", "/docs/"+uuid.NewString()+"/reviewed", map[string]any{"seq": 1}, http.StatusNotFound)
}

// Saving to the Library copies the text as it is now; saving the same text
// again returns that copy, and an edit makes a new one.
func TestSaveDocToLibrary(t *testing.T) {
	e, fake := newAvatarEnv(t)
	doc := e.want("POST", "/docs", map[string]any{"title": "Trip / plan"}, http.StatusCreated)
	id, chat := doc["id"].(string), doc["chat_id"].(string)

	first := e.want("POST", "/docs/"+id+"/library", nil, http.StatusCreated)
	if first["kind"] != "document" || first["title"] != "Trip / plan" || first["filename"] != "Trip plan.md" || first["chat_id"] != chat {
		t.Fatalf("saved %v", first)
	}
	if again := e.want("POST", "/docs/"+id+"/library", nil, http.StatusCreated); again["id"] != first["id"] {
		t.Fatalf("saving unchanged text made %v, want %v", again["id"], first["id"])
	}
	e.want("POST", "/chats/"+chat+"/canvases/"+id+"/versions", map[string]any{"base_seq": 1, "body": "# Trip / plan\n\nDay one.\n"}, http.StatusCreated)
	if edited := e.want("POST", "/docs/"+id+"/library", nil, http.StatusCreated); edited["id"] == first["id"] {
		t.Fatal("an edited document reused the old copy")
	}
	var bodies []string
	for _, k := range fake.keys(avatarBucket + "/library-docs/") {
		bodies = append(bodies, string(fake.objs[k].body))
	}
	if len(bodies) != 2 || !slices.Contains(bodies, "# Trip / plan\n") || !slices.Contains(bodies, "# Trip / plan\n\nDay one.\n") {
		t.Fatalf("stored %q", bodies)
	}

	e.want("POST", "/docs/"+uuid.NewString()+"/library", nil, http.StatusNotFound)
}
