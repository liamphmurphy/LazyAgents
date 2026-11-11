package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

func slideThemeBody(name, bg string) map[string]any {
	return map[string]any{
		"name":  name,
		"fonts": map[string]any{"heading": "serif", "body": "mono"},
		"colors": map[string]any{"background": bg, "text": "#222222", "heading": "#111111", "accent": "#e4572e",
			"accent2": "#29335C", "muted": "#555555", "surface": "#F2F2F2"},
	}
}

func TestSlideThemeLifecycle(t *testing.T) {
	e := newSlidesEnv(t)
	if got := e.want("GET", "/slide-themes", nil, http.StatusOK)["themes"].([]any); len(got) != 0 {
		t.Fatalf("new person has themes: %v", got)
	}
	th := e.want("POST", "/slide-themes", slideThemeBody(" Acme brand ", "#FFFFFF"), http.StatusCreated)
	id := th["id"].(string)
	if th["name"] != "Acme brand" || th["colors"].(map[string]any)["accent"] != "#E4572E" || th["fonts"].(map[string]any)["body"] != "mono" {
		t.Fatalf("created = %v", th)
	}
	// Names are unique per person ignoring case, and built-in names are taken.
	if code, out := e.do("POST", "/slide-themes", slideThemeBody("ACME BRAND", "#FFFFFF")); code != http.StatusConflict {
		t.Fatalf("duplicate name = %d %v", code, out)
	}
	if code, out := e.do("POST", "/slide-themes", slideThemeBody("ocean", "#FFFFFF")); code != http.StatusConflict {
		t.Fatalf("built-in name = %d %v", code, out)
	}
	e.invalid("POST", "/slide-themes", slideThemeBody("  ", "#FFFFFF"), "name", "")
	bad := slideThemeBody("Bad", "#FFFFFF")
	bad["fonts"] = map[string]any{"heading": "comic", "body": "sans"}
	if code, _ := e.do("POST", "/slide-themes", bad); code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("bad font = %d", code)
	}

	// Rename keeps the colours; a colour change keeps the name.
	r := e.want("PATCH", "/slide-themes/"+id, map[string]any{"name": "Acme 2026"}, http.StatusOK)
	if r["name"] != "Acme 2026" || r["colors"].(map[string]any)["accent"] != "#E4572E" {
		t.Fatalf("renamed = %v", r)
	}
	r = e.want("PATCH", "/slide-themes/"+id, map[string]any{"colors": slideThemeBody("", "#FAFAFA")["colors"]}, http.StatusOK)
	if r["name"] != "Acme 2026" || r["colors"].(map[string]any)["background"] != "#FAFAFA" {
		t.Fatalf("recoloured = %v", r)
	}
	e.want("POST", "/slide-themes", slideThemeBody("Workshop", "#FFFFFF"), http.StatusCreated)
	if code, _ := e.do("PATCH", "/slide-themes/"+id, map[string]any{"name": "workshop"}); code != http.StatusConflict {
		t.Fatalf("rename onto another theme = %d", code)
	}
	list := e.want("GET", "/slide-themes", nil, http.StatusOK)["themes"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["name"] != "Acme 2026" || list[1].(map[string]any)["name"] != "Workshop" {
		t.Fatalf("list = %v", list)
	}
	e.want("DELETE", "/slide-themes/"+id, nil, http.StatusNoContent)
	e.want("DELETE", "/slide-themes/"+id, nil, http.StatusNotFound)
	if list := e.want("GET", "/slide-themes", nil, http.StatusOK)["themes"].([]any); len(list) != 1 {
		t.Fatalf("after delete = %v", list)
	}
}

// Saved themes are the owner's alone: another person's are neither listed nor
// changeable (404), and their persona can't apply them.
func TestSlideThemeOwnerIsolation(t *testing.T) {
	e := newSlidesEnv(t)
	ctx := t.Context()
	other := e.otherUser()
	colors, _ := json.Marshal(slideThemeBody("", "#FFFFFF")["colors"])
	fonts, _ := json.Marshal(map[string]string{"heading": "sans", "body": "sans"})
	theirs, err := e.q.CreateDeckTheme(ctx, store.CreateDeckThemeParams{WorkspaceID: e.me.WorkspaceID, UserID: other.ID, Name: "Secret", Fonts: fonts, Colors: colors})
	if err != nil {
		t.Fatal(err)
	}
	// Store: the same name, differently cased, is fine for another person
	// but not for the same one.
	if _, err := e.q.CreateDeckTheme(ctx, store.CreateDeckThemeParams{WorkspaceID: e.me.WorkspaceID, UserID: other.ID, Name: "SECRET", Fonts: fonts, Colors: colors}); err == nil {
		t.Fatal("store allowed a duplicate name for one person")
	}
	if _, err := e.q.GetDeckTheme(ctx, store.GetDeckThemeParams{ID: theirs.ID, WorkspaceID: e.me.WorkspaceID, UserID: e.me.ID}); err == nil {
		t.Fatal("store returned another person's theme")
	}
	mine := e.want("POST", "/slide-themes", slideThemeBody("secret", "#FFFFFF"), http.StatusCreated)

	list := e.want("GET", "/slide-themes", nil, http.StatusOK)["themes"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["id"] != mine["id"] {
		t.Fatalf("list shows another person's theme: %v", list)
	}
	e.want("PATCH", "/slide-themes/"+theirs.ID.String(), map[string]any{"name": "Mine now"}, http.StatusNotFound)
	e.want("DELETE", "/slide-themes/"+theirs.ID.String(), nil, http.StatusNotFound)
	if got, err := e.q.GetDeckTheme(ctx, store.GetDeckThemeParams{ID: theirs.ID, WorkspaceID: e.me.WorkspaceID, UserID: other.ID}); err != nil || got.Name != "Secret" {
		t.Fatalf("their theme = %v %v", got, err)
	}
	e.want("DELETE", "/slide-themes/"+mine["id"].(string), nil, http.StatusNoContent)

	// My persona can't reach their theme by name.
	id, chat := e.deck("Mine")
	token := e.canvasToken(uuid.MustParse(chat))
	code, out := e.tool(token, "canvas_patch", map[string]any{"canvas_id": id, "base_version": 1, "ops": []any{map[string]any{"op": "set_theme", "value": map[string]any{"preset": "Secret"}}}})
	if msg, _ := out["message"].(string); code != http.StatusUnprocessableEntity || !strings.Contains(msg, `no theme named "Secret"`) {
		t.Fatalf("persona applied another person's theme: %d %v", code, out)
	}
}

// The persona switches a deck to a built-in or saved theme by name; the deck
// keeps its copy when the saved theme is deleted, and the theme works in
// every deck.
func TestPersonaSetsThemeByName(t *testing.T) {
	e := newSlidesEnv(t)
	saved := e.want("POST", "/slide-themes", slideThemeBody("Acme brand", "#FFFFFF"), http.StatusCreated)
	setTheme := func(id, chat, name string) {
		t.Helper()
		_, seq := e.deckBody(chat, id)
		token := e.canvasToken(uuid.MustParse(chat))
		if code, out := e.tool(token, "canvas_patch", map[string]any{"canvas_id": id, "base_version": seq, "ops": []any{map[string]any{"op": "set_theme", "value": map[string]any{"preset": name}}}}); code != http.StatusOK {
			t.Fatalf("set_theme %s = %d %v", name, code, out)
		}
	}
	themeOf := func(id, chat string) map[string]any {
		t.Helper()
		b, _ := e.deckBody(chat, id)
		return b["theme"].(map[string]any)
	}
	a, achat := e.deck("A")
	setTheme(a, achat, "Ocean")
	if th := themeOf(a, achat); th["name"] != "Ocean" || th["colors"].(map[string]any)["background"] != "#0B2540" {
		t.Fatalf("built-in = %v", th)
	}
	setTheme(a, achat, "acme BRAND")
	b, bchat := e.deck("B")
	setTheme(b, bchat, "Acme brand")
	for _, d := range [][2]string{{a, achat}, {b, bchat}} {
		th := themeOf(d[0], d[1])
		if th["name"] != "Acme brand" || th["colors"].(map[string]any)["accent"] != "#E4572E" || th["fonts"].(map[string]any)["heading"] != "serif" {
			t.Fatalf("saved theme in deck = %v", th)
		}
	}
	e.want("DELETE", "/slide-themes/"+saved["id"].(string), nil, http.StatusNoContent)
	if th := themeOf(a, achat); th["name"] != "Acme brand" || th["colors"].(map[string]any)["accent"] != "#E4572E" {
		t.Fatalf("deleting the saved theme changed the deck: %v", th)
	}
}
