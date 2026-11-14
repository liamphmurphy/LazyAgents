package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

func (e *chatEnv) project(key string) store.Project {
	e.t.Helper()
	p, err := e.q.GetProjectByKey(context.Background(), store.GetProjectByKeyParams{WorkspaceID: e.me.WorkspaceID, Key: key})
	if err != nil {
		e.t.Fatalf("project %s: %v", key, err)
	}
	return p
}

func (e *chatEnv) conflict(method, path string, body any, contains string) {
	e.t.Helper()
	out := e.want(method, path, body, http.StatusConflict)
	if msg, _ := out["message"].(string); !strings.Contains(msg, contains) {
		e.t.Fatalf("%s %s: %q, want it to mention %q", method, path, msg, contains)
	}
}

// A space starts tracking work and becomes the work's: one name and color, its new chats file
// tickets there, it can't be deleted from under its work, and it stops tracking only while
// the work has no tickets (LAZYAGENTS-213).
func TestSpaceTracksWork(t *testing.T) {
	e := newChatEnv(t)
	sp := e.want("POST", "/chat-spaces", map[string]any{"name": "Side project", "color": "violet"}, http.StatusCreated)
	id := sp["id"].(string)
	if sp["work"] != nil {
		t.Fatalf("new space tracks work: %v", sp["work"])
	}

	e.invalid("PUT", "/chat-spaces/"+id+"/work", map[string]any{"key": "1x"}, "key", "2–10 letters")
	sp = e.want("PUT", "/chat-spaces/"+id+"/work", map[string]any{"key": "side", "repo_url": "https://github.com/me/side"}, http.StatusOK)
	work, _ := sp["work"].(map[string]any)
	if work == nil || work["key"] != "SIDE" || work["repo_url"] != "https://github.com/me/side.git" || sp["project"] != "SIDE" {
		t.Fatalf("tracking work: %v", sp)
	}
	if p := e.project("SIDE"); p.SpaceID == nil || p.SpaceID.String() != id || p.Name != "Side project" || p.Color != "#A78BFA" {
		t.Fatalf("SIDE = %+v", p)
	}
	e.conflict("PUT", "/chat-spaces/"+id+"/work", map[string]any{"key": "OTHER"}, "already tracks work")
	other := e.want("POST", "/chat-spaces", map[string]any{"name": "Another"}, http.StatusCreated)["id"].(string)
	e.conflict("PUT", "/chat-spaces/"+other+"/work", map[string]any{"key": "SIDE"}, "already in use")

	// Renaming or recoloring either side shows on the other.
	e.want("PATCH", "/chat-spaces/"+id, map[string]any{"name": "Side quest", "color": "teal"}, http.StatusOK)
	p := e.project("SIDE")
	if p.Name != "Side quest" || p.Color != "#2DD4BF" {
		t.Fatalf("after space rename: %s %s", p.Name, p.Color)
	}
	e.want("PUT", "/projects/SIDE", map[string]any{"name": "Quest", "color": "#E05050", "default_branch": "main", "archived": false, "revision": p.Revision}, http.StatusOK)
	if got := e.want("GET", "/chat-spaces/"+id, nil, http.StatusOK); got["name"] != "Quest" || got["color"] != "red" {
		t.Fatalf("after work rename: %v %v", got["name"], got["color"])
	}

	e.invalid("PATCH", "/chat-spaces/"+id, map[string]any{"project": ""}, "project", "its own key, SIDE")
	e.invalid("PATCH", "/chat-spaces/"+id, map[string]any{"project": "PAY"}, "project", "its own key, SIDE")

	// A new chat in it files tickets under its key.
	if chat := e.want("POST", "/chats", map[string]any{"space_id": id}, http.StatusCreated); chat["project"] != "SIDE" {
		t.Fatalf("chat in the space files under %v", chat["project"])
	}

	e.conflict("DELETE", "/chat-spaces/"+id, nil, "tracks work (SIDE)")
	e.want("POST", "/tickets", map[string]any{"project_key": "SIDE", "title": "First"}, http.StatusCreated)
	e.conflict("DELETE", "/chat-spaces/"+id+"/work", nil, "SIDE has tickets")

	// Without tickets it stops: the work is archived, the key stays taken, and the space
	// files tickets nowhere until told.
	e.want("PUT", "/chat-spaces/"+other+"/work", map[string]any{"key": "ANO"}, http.StatusOK)
	stopped := e.want("DELETE", "/chat-spaces/"+other+"/work", nil, http.StatusOK)
	if stopped["work"] != nil || stopped["project"] != nil {
		t.Fatalf("stopped: %v", stopped)
	}
	if p := e.project("ANO"); p.SpaceID != nil || !p.Archived {
		t.Fatalf("ANO after stopping = %+v", p)
	}
	e.conflict("DELETE", "/chat-spaces/"+other+"/work", nil, "doesn't track work")
	e.want("DELETE", "/chat-spaces/"+other, nil, http.StatusNoContent)
}

// A space can track work from the start; a bad key creates nothing. A project made the old way
// comes with its space.
func TestNewSpaceWithWork(t *testing.T) {
	e := newChatEnv(t)
	before := len(e.want("GET", "/chat-spaces", nil, http.StatusOK)["spaces"].([]any))
	e.invalid("POST", "/chat-spaces", map[string]any{"name": "Bad", "work": map[string]any{"key": "1x"}}, "key", "2–10 letters")
	e.invalid("POST", "/chat-spaces", map[string]any{"name": "Both", "project": "PAY", "work": map[string]any{"key": "BOTH"}}, "project", "its own key")
	if after := len(e.want("GET", "/chat-spaces", nil, http.StatusOK)["spaces"].([]any)); after != before {
		t.Fatalf("a refused space was kept: %d spaces, had %d", after, before)
	}
	sp := e.want("POST", "/chat-spaces", map[string]any{"name": "Waterdeep", "color": "amber", "work": map[string]any{"key": "WD"}}, http.StatusCreated)
	if w, _ := sp["work"].(map[string]any); w == nil || w["key"] != "WD" || sp["project"] != "WD" {
		t.Fatalf("created %v", sp)
	}

	made := e.want("POST", "/projects", map[string]any{"key": "APP", "name": "App", "color": "#3366FF"}, http.StatusCreated)
	if made["space_id"] == nil {
		t.Fatalf("project without a space: %v", made)
	}
	got := e.want("GET", "/chat-spaces/"+made["space_id"].(string), nil, http.StatusOK)
	if w, _ := got["work"].(map[string]any); w == nil || w["key"] != "APP" || got["name"] != "App" || got["color"] != "blue" || got["space_memory"] != true {
		t.Fatalf("APP's space = %v", got)
	}
}

// A chat moves between spaces: it then files tickets where its new space does, unless the
// same patch names a project; taking it out of a space leaves its project.
func TestChatMovesBetweenSpaces(t *testing.T) {
	e := newChatEnv(t)
	work := e.want("POST", "/chat-spaces", map[string]any{"name": "Work", "work": map[string]any{"key": "WRK"}}, http.StatusCreated)["id"].(string)
	health := e.want("POST", "/chat-spaces", map[string]any{"name": "Health"}, http.StatusCreated)["id"].(string)
	chat := e.want("POST", "/chats", map[string]any{}, http.StatusCreated)
	id := chat["id"].(string)
	rev := func() float64 { return e.want("GET", "/chats/"+id, nil, http.StatusOK)["revision"].(float64) }

	moved := e.want("PATCH", "/chats/"+id, map[string]any{"revision": rev(), "space_id": work}, http.StatusOK)
	if moved["space"].(map[string]any)["id"] != work || moved["project"] != "WRK" {
		t.Fatalf("moved into Work: space %v project %v", moved["space"], moved["project"])
	}
	moved = e.want("PATCH", "/chats/"+id, map[string]any{"revision": rev(), "space_id": health}, http.StatusOK)
	if moved["space"].(map[string]any)["id"] != health || moved["project"] != "WRK" {
		t.Fatalf("Health files nowhere, so the chat keeps WRK: space %v project %v", moved["space"], moved["project"])
	}
	moved = e.want("PATCH", "/chats/"+id, map[string]any{"revision": rev(), "space_id": work, "project": "PAY"}, http.StatusOK)
	if moved["project"] != "PAY" {
		t.Fatalf("an explicit project lost to the space's: %v", moved["project"])
	}
	moved = e.want("PATCH", "/chats/"+id, map[string]any{"revision": rev(), "space_id": ""}, http.StatusOK)
	if moved["space"] != nil || moved["project"] != "PAY" {
		t.Fatalf("out of any space: space %v project %v", moved["space"], moved["project"])
	}
	e.invalid("PATCH", "/chats/"+id, map[string]any{"revision": rev(), "space_id": "nope"}, "space_id", "must be a space id")
}
