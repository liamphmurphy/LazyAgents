package httpapi

import (
	"testing"
)

// The explorer's context menu: delete a page, and move, rename or delete a
// whole folder.

func (e *chatEnv) unresolvedPages() map[string]bool {
	e.t.Helper()
	out := map[string]bool{}
	for _, r := range e.want("POST", "/knowledge/query", map[string]any{"unresolved": true}, 200)["rows"].([]any) {
		out[r.(map[string]any)["path"].(string)] = true
	}
	return out
}

func (e *chatEnv) pagePath(id string) string {
	e.t.Helper()
	return e.want("GET", "/knowledge/pages/"+id, nil, 200)["path"].(string)
}

// Deleting a page leaves [[links]] to it in place as unresolved links, which
// reattach if a page by that name is made again.
func TestDeletingAPageUnresolvesLinksToIt(t *testing.T) {
	e := newChatEnv(t)
	harbour := e.want("POST", "/knowledge/pages", map[string]any{"title": "Harbour", "path": "Places/Harbour", "body_md": "Ferries."}, 200)["id"].(string)
	e.want("POST", "/knowledge/pages", map[string]any{"title": "Log", "path": "Notes/Log", "body_md": "Docked at [[Harbour]]."}, 200)
	if e.unresolvedPages()["Notes/Log"] {
		t.Fatal("link to an existing page counted as unresolved")
	}

	e.want("DELETE", "/knowledge/pages/"+harbour, nil, 200)
	e.want("GET", "/knowledge/pages/"+harbour, nil, 404)
	if !e.unresolvedPages()["Notes/Log"] {
		t.Fatal("link to the deleted page didn't go back to unresolved")
	}
	e.want("DELETE", "/knowledge/pages/"+harbour, nil, 404)

	e.want("POST", "/knowledge/pages", map[string]any{"title": "Harbour", "path": "Places/Harbour", "body_md": "Rebuilt."}, 200)
	if e.unresolvedPages()["Notes/Log"] {
		t.Fatal("link didn't reattach to the recreated page")
	}
}

// Renaming a folder moves every page under it, nested ones too, and links
// into it keep resolving; a clash anywhere moves nothing.
func TestMovingAFolderMovesEveryPageUnderIt(t *testing.T) {
	e := newChatEnv(t)
	mara := e.want("POST", "/knowledge/pages", map[string]any{"title": "Mara", "path": "Campaign/NPCs/Mara", "body_md": "Harbour master."}, 200)["id"].(string)
	bren := e.want("POST", "/knowledge/pages", map[string]any{"title": "Bren", "path": "Campaign/NPCs/Retired/Bren", "body_md": "Old sailor."}, 200)["id"].(string)
	harbour := e.want("POST", "/knowledge/pages", map[string]any{"title": "Harbour", "path": "Campaign/Locations/Harbour", "body_md": "Run by [[Mara]]."}, 200)["id"].(string)

	moved := e.want("POST", "/knowledge/folders/move", map[string]any{"from": "Campaign/NPCs", "to": "Campaign/People"}, 200)["moved"].([]any)
	if len(moved) != 2 {
		t.Fatalf("moved %v, want Mara and Bren", moved)
	}
	if got := e.pagePath(mara); got != "Campaign/People/Mara" {
		t.Errorf("Mara at %q", got)
	}
	if got := e.pagePath(bren); got != "Campaign/People/Retired/Bren" {
		t.Errorf("Bren at %q", got)
	}
	if got := e.pagePath(harbour); got != "Campaign/Locations/Harbour" {
		t.Errorf("a page outside the folder moved to %q", got)
	}
	if e.unresolvedPages()["Campaign/Locations/Harbour"] {
		t.Error("link into the moved folder stopped resolving")
	}

	e.want("POST", "/knowledge/folders/move", map[string]any{"from": "Campaign/People", "to": "Campaign/People/Inner"}, 422)
	e.want("POST", "/knowledge/folders/move", map[string]any{"from": "Nowhere", "to": "Elsewhere"}, 404)

	e.want("POST", "/knowledge/pages", map[string]any{"title": "Bren", "path": "Archive/Retired/Bren", "body_md": "Taken."}, 200)
	e.want("POST", "/knowledge/folders/move", map[string]any{"from": "Campaign/People", "to": "Archive"}, 422)
	if got := e.pagePath(mara); got != "Campaign/People/Mara" {
		t.Errorf("a failed move still moved Mara to %q", got)
	}
}

// Deleting a folder deletes the pages under it and nothing beside it, and
// never reaches into agent knowledge from the person's view.
func TestDeletingAFolderDeletesOnlyItsPages(t *testing.T) {
	e := newChatEnv(t)
	mara := e.want("POST", "/knowledge/pages", map[string]any{"title": "Mara", "path": "Campaign/NPCs/Mara", "body_md": "Harbour master."}, 200)["id"].(string)
	bren := e.want("POST", "/knowledge/pages", map[string]any{"title": "Bren", "path": "Campaign/NPCs/Retired/Bren", "body_md": "Old sailor."}, 200)["id"].(string)
	harbour := e.want("POST", "/knowledge/pages", map[string]any{"title": "Harbour", "path": "Campaign/Locations/Harbour", "body_md": "Run by [[Mara]]."}, 200)["id"].(string)

	e.want("POST", "/knowledge/folders/delete", map[string]any{"folder": "Campaign/NPCs", "agent": true}, 404)
	deleted := e.want("POST", "/knowledge/folders/delete", map[string]any{"folder": "Campaign/NPCs"}, 200)["deleted"].([]any)
	if len(deleted) != 2 {
		t.Fatalf("deleted %v, want Mara and Bren", deleted)
	}
	e.want("GET", "/knowledge/pages/"+mara, nil, 404)
	e.want("GET", "/knowledge/pages/"+bren, nil, 404)
	e.want("GET", "/knowledge/pages/"+harbour, nil, 200)
	if !e.unresolvedPages()["Campaign/Locations/Harbour"] {
		t.Error("link into the deleted folder didn't go back to unresolved")
	}
	e.want("POST", "/knowledge/folders/delete", map[string]any{"folder": "  "}, 422)
}
