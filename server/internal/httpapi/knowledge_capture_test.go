package httpapi

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Templates fill {{title}}, {{date}} and {{properties.*}}, from the built-in
// D&D set over REST and from any page used as a template by a persona.
func TestTemplatesFillTitleDateAndProperties(t *testing.T) {
	e := newChatEnv(t)
	today := time.Now().UTC().Format("2006-01-02")
	templates := e.want("GET", "/knowledge/templates", nil, 200)["templates"].(map[string]any)
	for _, name := range []string{"NPC", "Location", "Session", "Encounter", "Daily", "Permanent note"} {
		if _, ok := templates[name]; !ok {
			t.Errorf("template %q missing", name)
		}
	}

	session := e.want("POST", "/knowledge/pages", map[string]any{"title": "Session 12", "template": "Session"}, 200)
	if body := session["body_md"].(string); !strings.HasPrefix(body, "# Session 12\n") || !strings.Contains(body, "Date: "+today) || strings.Contains(body, "{{") {
		t.Fatalf("session template body %q", body)
	}

	e.want("POST", "/knowledge/pages", map[string]any{"title": "Faction template", "path": "Templates/Faction", "body_md": "# {{title}}\n\nLed by {{properties.leader}} since {{date}}.\n"}, 200)
	chat := e.newChat(nil)
	token := e.canvasToken(uuid.MustParse(chat["id"].(string)))
	code, out := e.tool(token, "create_page", map[string]any{"title": "Harbour guild", "template": "Templates/Faction", "properties": map[string]any{"leader": "Mara"}})
	if code != 200 {
		t.Fatalf("create_page %d %v", code, out)
	}
	page := e.want("GET", "/knowledge/pages/"+out["id"].(string), nil, 200)
	if want := "# Harbour guild\n\nLed by Mara since " + today + ".\n"; page["body_md"] != want {
		t.Fatalf("template body %q, want %q", page["body_md"], want)
	}
}

// A literature note made from a saved link or a video transcript records its
// source as properties; a saved link also supplies the title and a first body.
func TestLiteratureNotesFromSavedLinksAndTranscripts(t *testing.T) {
	e := newChatEnv(t)
	ctx := t.Context()
	link, err := e.q.CreateLibraryLink(ctx, store.CreateLibraryLinkParams{WorkspaceID: e.me.WorkspaceID, UserID: e.me.ID, Title: "How to take smart notes", Url: "https://example.com/smart-notes", Site: "example.com", Note: "For the Zettelkasten setup"})
	if err != nil {
		t.Fatal(err)
	}
	chat := e.newChat(nil)
	token := e.canvasToken(uuid.MustParse(chat["id"].(string)))

	code, out := e.tool(token, "create_page", map[string]any{"source": link.Url})
	if code != 200 {
		t.Fatalf("from saved link %d %v", code, out)
	}
	page := e.want("GET", "/knowledge/pages/"+out["id"].(string), nil, 200)
	props := page["properties"].(map[string]any)
	if page["title"] != link.Title || props["type"] != "literature" || props["source"] != link.Url || props["source_title"] != link.Title || props["source_site"] != "example.com" {
		t.Fatalf("saved-link literature note %v", page)
	}
	if body := page["body_md"].(string); !strings.Contains(body, "("+link.Url+")") || !strings.Contains(body, "For the Zettelkasten setup") {
		t.Fatalf("saved-link body %q", body)
	}

	video := "https://www.youtube.com/watch?v=Z3Kal4Xazf4"
	code, out = e.tool(token, "create_page", map[string]any{"title": "Talk notes", "source": video, "body": "# Talk notes\n\nFrom the transcript: keep notes atomic.\n"})
	if code != 200 {
		t.Fatalf("from transcript %d %v", code, out)
	}
	page = e.want("GET", "/knowledge/pages/"+out["id"].(string), nil, 200)
	props = page["properties"].(map[string]any)
	if props["type"] != "literature" || props["source"] != video || !strings.Contains(page["body_md"].(string), "keep notes atomic") {
		t.Fatalf("transcript literature note %v", page)
	}
}

// A new Knowledge page or capture may start empty, as an Obsidian note does;
// a persona still can't blank a page with write_note.
func TestEmptyPagesAllowedButNotBlankedByPersonas(t *testing.T) {
	e := newChatEnv(t)
	page := e.want("POST", "/knowledge/pages", map[string]any{"title": "Untitled", "body_md": ""}, 200)
	if page["body_md"] != "" {
		t.Fatalf("empty page body %q", page["body_md"])
	}
	e.want("POST", "/knowledge/capture", map[string]any{"title": "Call the harbour master"}, 200)

	chat := e.newChat(nil)
	token := e.canvasToken(uuid.MustParse(chat["id"].(string)))
	filled := e.want("POST", "/knowledge/pages", map[string]any{"title": "Keep", "path": "Keep", "body_md": "Text to keep.", "audience": "shared"}, 200)
	if code, out := e.tool(token, "write_note", map[string]any{"path": "Keep", "body": "  ", "mode": "replace"}); code == 200 {
		t.Fatalf("write_note blanked a page: %v", out)
	}
	if again := e.want("GET", "/knowledge/pages/"+filled["id"].(string), nil, 200); again["body_md"] != "Text to keep." {
		t.Fatalf("page changed: %v", again["body_md"])
	}
}
