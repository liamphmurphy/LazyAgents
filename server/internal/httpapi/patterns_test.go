package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
)

func TestFindPatternsEndpoint(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	w := &wiki.Service{Pool: pool, Artifacts: arts, Tracker: &tracker.Service{Pool: pool}}
	for path, body := range map[string]string{
		"Patterns/Go/Wrap errors with context":      "Topic: Go\nTags: errors, context\n\n## When to use\n\nCrossing a package boundary.\n",
		"Patterns/Testing/Use throwaway databases":  "Topic: Testing\nTags: testing, database\n\n## When to use\n\nIntegration tests.\n",
		"Patterns/Proposed/Wrap errors in handlers": "Topic: Go\nTags: errors, handlers\n\nnot published\n",
	} {
		if _, err := w.SaveAsUser(ctx, me.WorkspaceID, me.ID, wiki.Save{Path: path, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	handler := (&Server{AnonymousAPI: true, Pool: pool, Q: q}).Handler()
	get := func(url string, code int) (out struct{ Patterns []api.WikiPattern }) {
		t.Helper()
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", url, nil))
		if r.Code != code {
			t.Fatalf("%s: %d %s", url, r.Code, r.Body.String())
		}
		if code == 200 {
			if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
		}
		return out
	}
	got := get("/api/v1/wiki/patterns?q=eror+wrap", 200)
	if len(got.Patterns) != 1 {
		t.Fatalf("fuzzy = %+v", got.Patterns)
	}
	p := got.Patterns[0]
	if p.Title != "Wrap errors with context" || p.Topic != "Go" || p.Summary != "Crossing a package boundary." || len(p.Tags) != 2 || p.Score <= 0 {
		t.Fatalf("pattern = %+v", p)
	}
	if all := get("/api/v1/wiki/patterns", 200); len(all.Patterns) != 2 || all.Patterns[0].Topic != "Go" || all.Patterns[1].Topic != "Testing" {
		t.Fatalf("browse = %+v", all.Patterns)
	}
	if one := get("/api/v1/wiki/patterns?topic=testing&limit=1", 200); len(one.Patterns) != 1 || one.Patterns[0].Topic != "Testing" {
		t.Fatalf("topic = %+v", one.Patterns)
	}
	get("/api/v1/wiki/patterns?topic=Cooking", 422)
}

// Agents find, propose, publish and reject patterns through the Agent API.
func TestPatternTools(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	tr := &tracker.Service{Pool: pool}
	handler := (&Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: &runs.Service{Pool: pool, Tracker: tr, Artifacts: arts}, Artifacts: arts,
		Wiki: &wiki.Service{Pool: pool, Artifacts: arts, Tracker: tr}}).Handler()

	tokenFor := func(handle string) string {
		a, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: handle})
		if err != nil {
			t.Fatal(err)
		}
		tok, _, err := agents.IssueToken(ctx, q, a.ID, "unmanaged", "test", nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	call := func(token, name string, args any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(args)
		req := httptest.NewRequest("POST", "/api/v1/agent/tools/"+name, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	// echo becomes the librarian: wiki:write allowed.
	echo, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "echo"})
	spec, _, err := agents.CurrentSpec(ctx, q, echo)
	if err != nil {
		t.Fatal(err)
	}
	spec.Scopes["wiki:write"] = agentdef.Allowed
	if _, err := agents.WriteVersion(ctx, q, echo, agents.Identity{Name: echo.Name, Role: echo.Role, Specialty: echo.Specialty, ColorIndex: int(echo.ColorIndex)}, spec, "Librarian", &me.ID, nil); err != nil {
		t.Fatal(err)
	}
	atlas, librarian := tokenFor("atlas"), tokenFor("echo")

	body := "## When to use\n\nAny error crossing a package boundary.\n\n## Rule\n\nWrap with %w.\n\n## Related\n"
	if code, out := call(atlas, "propose_pattern", map[string]any{"title": "Wrap errors with context", "topic": "go", "tags": []string{"errors", "context"}, "body": body, "source_ticket": "pay-214"}); code != 200 || out["path"] != "Patterns/Proposed/Wrap errors with context" {
		t.Fatalf("propose = %d %v", code, out)
	}
	if code, _ := call(atlas, "propose_pattern", map[string]any{"title": "X", "topic": "Cooking", "tags": []string{"a", "b"}, "body": body}); code != 422 {
		t.Fatalf("bad topic = %d", code)
	}
	if code, _ := call(atlas, "propose_pattern", map[string]any{"title": "Y", "topic": "Go", "tags": []string{"a", "b"}, "body": body, "source_ticket": "NOPE-1"}); code != 422 {
		t.Fatalf("bad ticket = %d", code)
	}
	// Proposals aren't findable; only curators list or decide them.
	if _, out := call(atlas, "find_pattern", map[string]any{"q": "wrap"}); len(out["patterns"].([]any)) != 0 {
		t.Fatalf("proposal found: %v", out)
	}
	for _, name := range []string{"list_proposed_patterns", "publish_pattern", "reject_pattern"} {
		if code, out := call(atlas, name, map[string]any{"proposed_title": "Wrap errors with context", "reason": "no"}); code != 422 {
			t.Fatalf("%s as non-curator = %d %v", name, code, out)
		}
	}
	_, out := call(librarian, "list_proposed_patterns", nil)
	if l := out["proposed"].([]any); len(l) != 1 || l[0].(map[string]any)["proposer"] != "@atlas" {
		t.Fatalf("list = %v", out)
	}
	if code, out := call(librarian, "publish_pattern", map[string]any{"proposed_title": "Wrap errors with context"}); code != 200 || out["path"] != "Patterns/Go/Wrap errors with context" {
		t.Fatalf("publish = %d %v", code, out)
	}
	_, out = call(atlas, "find_pattern", map[string]any{"q": "eror wrap"})
	ps := out["patterns"].([]any)
	if len(ps) != 1 {
		t.Fatalf("find = %v", out)
	}
	p := ps[0].(map[string]any)
	if p["ref"] != "Patterns/Go/Wrap errors with context" || p["topic"] != "Go" || p["summary"] != "Any error crossing a package boundary." || len(p["tags"].([]any)) != 2 {
		t.Fatalf("pattern = %v", p)
	}
	if code, pg := call(atlas, "read_page", map[string]any{"ref": p["ref"]}); code != 200 || !strings.Contains(pg["markdown"].(string), "[[PAY-214]]") {
		t.Fatalf("read_page = %d %v", code, pg)
	}
	call(atlas, "propose_pattern", map[string]any{"title": "Bad idea", "topic": "Process", "tags": []string{"a", "b"}, "body": body})
	if code, out := call(librarian, "reject_pattern", map[string]any{"proposed_title": "Bad idea", "reason": "too vague"}); code != 200 || out["proposed_by"] != "@atlas" {
		t.Fatalf("reject = %d %v", code, out)
	}
	if code, out := call(librarian, "reject_pattern", map[string]any{"proposed_title": "Bad idea", "reason": "again"}); code != 422 {
		t.Fatalf("reject twice = %d %v", code, out)
	}
}
