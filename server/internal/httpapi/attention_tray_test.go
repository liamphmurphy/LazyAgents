package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/auth"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// The tray's routing rule (LAZYAGENTS-162): news goes to Updates, and a
// mention that asks a question goes to Needs you like any other question.
func TestInboxNeedsYou(t *testing.T) {
	cases := []struct {
		kind, body string
		want       bool
	}{
		{"approval", "Run migration 0042", true},
		{"question", "The agent is waiting for a reply", true},
		{"blocked", "exit 1", true},
		{"review", "Ready for your review", true},
		{"dispatch", "Atlas is waiting for dispatch", true},
		{"planning", "Draft tickets are ready", true},
		{"mockup", "Mockup v2 is waiting for your approval", true},
		{"suggestion", "Fix a typo", true},
		{"workflow_gate", "A workflow step is waiting for your approval", true},
		{"scheduled", "Daily digest: 3 PRs merged. Anything else?", false},
		{"merged", "Pull request #12 merged", false},
		{"mention", "@liam FYI the deploy finished.", false},
		{"mention", "@liam can you confirm the retry cap?", true},
		{"mention", "@liam Should it be 5 or 8 ?\nThanks", true},
		{"mention", "@liam see https://example.com/pr?tab=files for the diff.", false},
		{"mention", "@liam is this fine? https://example.com/pr?tab=files", true},
		{"mention", "@liam does https://example.com/x?", true},
		{"mention", "@liam the `x ?: y` operator is in.", false},
		{"mention", "@liam done.\n```js\nconst a = b ? c : d\n```", false},
	}
	for _, c := range cases {
		if got := inboxNeedsYou(c.kind, c.body); got != c.want {
			t.Errorf("inboxNeedsYou(%q, %q) = %v, want %v", c.kind, c.body, got, c.want)
		}
	}
}

func TestInboxTraySections(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: &artifacts.Service{Pool: pool}}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	u, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tr := &tracker.Service{Pool: pool}
	srv := &Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: &runs.Service{Pool: pool, Tracker: tr}}
	authctx := auth.WithUser(ctx, u)
	ticket, err := q.GetTicketByKey(ctx, store.GetTicketByKeyParams{WorkspaceID: u.WorkspaceID, Key: "PAY-214"})
	if err != nil {
		t.Fatal(err)
	}
	atlas, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: u.WorkspaceID, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	needsYou := func() int {
		t.Helper()
		res, err := srv.GetBootstrap(authctx, api.GetBootstrapRequestObject{})
		if err != nil {
			t.Fatal(err)
		}
		return res.(api.GetBootstrap200JSONResponse).Counts.NeedsYou
	}
	before := needsYou()

	handle := strings.Split(u.Email, "@")[0]
	long := "@" + handle + " " + strings.Repeat("context ", 40) + "which cap do you want?"
	var fyi, ask string
	for body, id := range map[string]*string{"@" + handle + " FYI the deploy finished.": &fyi, long: &ask} {
		if err := pool.QueryRow(ctx, `INSERT INTO comments(workspace_id,ticket_id,author_type,author_id,body_md) VALUES($1,$2,'agent',$3,$4) RETURNING 'mention:'||id`,
			u.WorkspaceID, ticket.ID, atlas.ID, body).Scan(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := tracker.Record(ctx, q, u.WorkspaceID, ticket.ID, ticket.EpicID, tracker.Actor{Type: "system"}, runs.VerbPRMerged, map[string]any{"number": 12, "url": "https://example.com/pr/12"}, nil); err != nil {
		t.Fatal(err)
	}

	res, err := srv.ListInbox(authctx, api.ListInboxRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]api.InboxItem{}
	var merged *api.InboxItem
	for _, it := range res.(api.ListInbox200JSONResponse).Items {
		byID[it.Id] = it
		if it.Kind == api.InboxItemKindMerged && it.TicketKey == "PAY-214" {
			merged = &it
		}
	}
	if it, ok := byID[fyi]; !ok || it.NeedsYou {
		t.Fatalf("an FYI mention belongs in Updates: %+v", it)
	}
	// The question is past the 240 characters the summary keeps: it is read from the whole comment.
	if it, ok := byID[ask]; !ok || !it.NeedsYou || len([]rune(it.Summary)) != 240 {
		t.Fatalf("a mention that asks belongs in Needs you, with a clipped summary: %+v", it)
	}
	if merged == nil || merged.NeedsYou || merged.Summary != "Pull request #12 merged" || merged.Href != "/tickets/PAY-214" {
		t.Fatalf("merged PR item: %+v", merged)
	}
	if got := needsYou(); got != before+1 {
		t.Fatalf("needs_you count = %d, want %d (only the asking mention counts)", got, before+1)
	}
}
