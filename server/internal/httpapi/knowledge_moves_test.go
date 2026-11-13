package httpapi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// A persona in the person's chat moves pages and folders under the page's
// persona controls: direct pages move at once, paused and review-first pages
// refuse, and a folder with one refused page moves nothing.
func TestPersonaMovesPagesAndFolders(t *testing.T) {
	e := newChatEnv(t)
	harbour := e.want("POST", "/knowledge/pages", map[string]any{"title": "Harbour", "path": "Inbox/Harbour", "body_md": "A place by the sea."}, 200)
	first := e.want("POST", "/knowledge/pages", map[string]any{"title": "One", "path": "Campaign/Sessions/One", "body_md": "First session."}, 200)
	second := e.want("POST", "/knowledge/pages", map[string]any{"title": "Two", "path": "Campaign/Two", "body_md": "Second session."}, 200)
	chat := e.newChat(nil)
	token := e.canvasToken(uuid.MustParse(chat["id"].(string)))
	path := func(page map[string]any) any {
		return e.want("GET", "/knowledge/pages/"+page["id"].(string), nil, 200)["path"]
	}
	controls := func(page map[string]any, direct, paused bool) {
		e.want("PUT", "/knowledge/pages/"+page["id"].(string)+"/controls", map[string]any{"direct": direct, "paused": paused}, 200)
	}

	if code, out := e.tool(token, "move_page", map[string]any{"ref": "Inbox/Harbour", "to": "Places/Harbour"}); code != 200 {
		t.Fatalf("move_page %d %v", code, out)
	}
	if got := path(harbour); got != "Places/Harbour" {
		t.Fatalf("moved page path %v", got)
	}
	// The old path still finds the page.
	if code, out := e.tool(token, "page_outline", map[string]any{"ref": "Inbox/Harbour"}); code != 200 || out["id"] != harbour["id"] {
		t.Fatalf("old path no longer resolves %d %v", code, out)
	}
	for _, to := range []string{"agents/atlas/Harbour", "Patterns/Go/Harbour"} {
		if code, out := e.tool(token, "move_page", map[string]any{"ref": harbour["id"], "to": to}); code != 422 || !strings.Contains(fmt.Sprint(out["message"]), "agents/ or patterns/") {
			t.Fatalf("moved into %s: %v", to, out)
		}
	}

	move := func() (int, map[string]any) {
		return e.tool(token, "move_folder", map[string]any{"from": "Campaign", "to": "Archive/Campaign"})
	}
	for _, c := range []struct {
		name, reason   string
		direct, paused bool
	}{{"paused", "paused", true, true}, {"review-first", "wait for review", false, false}} {
		controls(second, c.direct, c.paused)
		if code, out := move(); code != 409 || !strings.Contains(fmt.Sprint(out["message"]), c.reason) {
			t.Fatalf("%s page moved with its folder: %d %v", c.name, code, out)
		}
		if path(first) != "Campaign/Sessions/One" || path(second) != "Campaign/Two" {
			t.Fatalf("a refused folder move moved part of it (%s)", c.name)
		}
		if code, out := e.tool(token, "move_page", map[string]any{"ref": second["id"], "to": "Elsewhere/Two"}); code == 200 {
			t.Fatalf("%s page moved: %v", c.name, out)
		}
	}

	controls(second, true, false)
	if code, out := move(); code != 200 || out["moved"] != float64(2) {
		t.Fatalf("move_folder %d %v", code, out)
	}
	if path(first) != "Archive/Campaign/Sessions/One" || path(second) != "Archive/Campaign/Two" {
		t.Fatalf("folder pages %v %v", path(first), path(second))
	}
}
