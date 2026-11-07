package wiki_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/links"
	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
	"github.com/liamphmurphy/lazyagents/server/internal/wiki"
)

type env struct {
	ctx   context.Context
	q     *store.Queries
	ws    uuid.UUID
	me    store.User
	svc   *wiki.Service
	atlas store.Agent
	echo  store.Agent
	adr   store.Artifact
}

func setup(t *testing.T) env {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	n := int32(12)
	adr, err := q.GetADRByNumber(ctx, store.GetADRByNumberParams{WorkspaceID: ws, Number: &n})
	if err != nil {
		t.Fatal(err)
	}
	atlas, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	echo, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "echo"})
	return env{ctx, q, ws, me, &wiki.Service{Pool: pool, Artifacts: arts, Tracker: &tracker.Service{Pool: pool}}, atlas, echo, adr}
}

func invalid(err error) bool {
	var v *apperr.ValidationError
	return errors.As(err, &v)
}

func (e env) save(t *testing.T, path, body string) wiki.Result {
	t.Helper()
	r, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Path: path, Body: body})
	if err != nil {
		t.Fatalf("save %s: %v", path, err)
	}
	return r
}

func (e env) linksFrom(t *testing.T, id uuid.UUID) []store.KnowledgeLink {
	t.Helper()
	ls, err := e.q.LinksFromSource(e.ctx, store.LinksFromSourceParams{SourceKind: "artifact", SourceID: id})
	if err != nil {
		t.Fatal(err)
	}
	return ls
}

func TestLinksGhostsAndRename(t *testing.T) {
	e := setup(t)
	r := e.save(t, "Ops/Rollback plan",
		"# Rollback runbook\n\nFollow [[ADR-012#Decision]] for [[PAY-214]], ask [[@atlas]], see [[Ledger v2]] and [[Partition gotchas]].\n")
	if !r.Created || r.State != artifacts.Published || r.Note.Title != "Rollback plan" {
		t.Fatalf("result %+v", r)
	}
	kinds := []string{}
	for _, l := range e.linksFrom(t, r.Note.ID) {
		kinds = append(kinds, l.TargetKind)
	}
	if strings.Join(kinds, ",") != "section,ticket,agent,epic,unresolved" {
		t.Fatalf("kinds %v", kinds)
	}

	// The ghost resolves when the page is created.
	g := e.save(t, "Payments/Partition gotchas", "Triggers block ALTER on ledger_entries.")
	ls := e.linksFrom(t, r.Note.ID)
	if ls[4].TargetKind != "artifact" || *ls[4].TargetID != g.Note.ID {
		t.Fatalf("ghost not resolved: %+v", ls[4])
	}

	// Backlinks on ADR-012 count the runbook (as a section citation).
	bl, err := wiki.Backlinks(e.ctx, e.q, e.ws, wiki.Ref{Kind: "artifact", ID: e.adr.ID})
	if err != nil || len(bl) == 0 || bl[0].SourceID != r.Note.ID || len(bl[0].Anchors) != 1 || bl[0].Anchors[0] != "decision" {
		t.Fatalf("backlinks %+v %v", bl, err) // newest first
	}

	// Rename keeps links (by id) and the old path keeps resolving.
	if _, err := e.svc.Rename(e.ctx, e.ws, g.Note.ID, "Payments/Ledger/Partition gotchas"); err != nil {
		t.Fatal(err)
	}
	if ls := e.linksFrom(t, r.Note.ID); *ls[4].TargetID != g.Note.ID {
		t.Fatal("rename broke the link")
	}
	tg, err := links.Resolve(e.ctx, e.q, e.ws, "Payments/Partition gotchas", "")
	if err != nil || tg.Kind != "artifact" || *tg.ID != g.Note.ID || tg.Path != "Payments/Ledger/Partition gotchas" {
		t.Fatalf("alias: %+v %v", tg, err)
	}
	// Writing to the old name writes to the moved page.
	if w := e.save(t, "Payments/Partition gotchas", "Updated."); w.Note.ID != g.Note.ID || w.Created {
		t.Fatalf("old-path write: %+v", w)
	}
	// A new page can take the old path back.
	if n, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Path: "Payments/Partition gotchas", Body: "A new page", Mode: wiki.ModeCreate}); err != nil || !n.Created {
		t.Fatal("old path not reusable")
	}

	// Conflicting edit from a stale editor.
	stale := uuid.New()
	if _, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Path: "Ops/Rollback plan", Body: "x", BaseID: &stale}); !errors.As(err, new(*apperr.ConflictError)) {
		t.Fatalf("stale save: %v", err)
	}
	// Bad paths.
	for _, p := range []string{"", "a/[[b]]", "x#y", "../up"} {
		if _, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Path: p, Body: "x"}); !invalid(err) {
			t.Errorf("path %q: %v", p, err)
		}
	}
}

func TestTicketAndCommentLinks(t *testing.T) {
	e := setup(t)
	tr := &tracker.Service{Pool: e.svc.Pool}
	desc := "Context: [[ADR-012]] and [[Not yet written]]."
	if _, err := tr.Update(e.ctx, e.ws, tracker.UserActor(e.me.ID), "PAY-224", tracker.Patch{DescriptionMd: &desc}); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Comment(e.ctx, e.ws, tracker.UserActor(e.me.ID), "PAY-224", tracker.NewComment{BodyMd: "Also [[ADR-012#Consequences]]"}); err != nil {
		t.Fatal(err)
	}
	bl, err := wiki.Backlinks(e.ctx, e.q, e.ws, wiki.Ref{Kind: "artifact", ID: e.adr.ID})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, m := range bl {
		kinds[m.SourceKind] = m.Source.DisplayID
	}
	if kinds["ticket"] != "PAY-224" || kinds["comment"] != "PAY-224" {
		t.Fatalf("backlinks %+v", bl)
	}
	// The ghost from the description resolves when someone writes the page.
	e.save(t, "Not yet written", "Now it is.")
	tk, _ := tracker.Get(e.ctx, e.q, e.ws, "PAY-224")
	ls, _ := e.q.LinksFromSource(e.ctx, store.LinksFromSourceParams{SourceKind: "ticket", SourceID: tk.ID})
	if len(ls) != 2 || ls[1].TargetKind != "artifact" {
		t.Fatalf("ticket links %+v", ls)
	}
}

func TestUnlinkedMentions(t *testing.T) {
	e := setup(t)
	p := e.save(t, "Glossary", "We partition per ADR-012. Code: `ADR-012`. Linked: [[ADR-012]].")
	other := e.save(t, "Notes/Ledger", "Plain mention of ADR-012 here, and ADR-0123 isn't one.")
	linked, _ := wiki.Backlinks(e.ctx, e.q, e.ws, wiki.Ref{Kind: "artifact", ID: e.adr.ID})
	un, err := wiki.UnlinkedMentions(e.ctx, e.q, e.ws, wiki.Ref{Kind: "artifact", ID: e.adr.ID}, linked)
	if err != nil {
		t.Fatal(err)
	}
	found := map[uuid.UUID]bool{}
	for _, m := range un {
		found[m.SourceID] = true
	}
	if found[p.Note.ID] || !found[other.Note.ID] {
		t.Fatalf("unlinked: %+v", un)
	}
	// "Link it" rewrites the first plain mention.
	if err := e.svc.LinkMention(e.ctx, e.ws, e.me.ID, wiki.Ref{Kind: "artifact", ID: e.adr.ID}, "artifact", other.Note.ID); err != nil {
		t.Fatal(err)
	}
	a, _ := e.q.GetArtifact(e.ctx, store.GetArtifactParams{ID: other.Note.ID, WorkspaceID: e.ws})
	v, _ := e.q.GetArtifactVersion(e.ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: e.ws})
	if *v.BodyMd != "Plain mention of [[ADR-012]] here, and ADR-0123 isn't one." {
		t.Fatalf("rewritten: %q", *v.BodyMd)
	}
	if err := e.svc.LinkMention(e.ctx, e.ws, e.me.ID, wiki.Ref{Kind: "artifact", ID: e.adr.ID}, "artifact", other.Note.ID); !invalid(err) {
		t.Fatalf("second link-it: %v", err)
	}
}

func TestAgentPermissionsAndSuggestions(t *testing.T) {
	e := setup(t)
	if wiki.Decide("atlas", "agents/atlas/x", agentdef.Denied, true) != wiki.Direct ||
		wiki.Decide("atlas", "Agents/Echo/x", agentdef.Allowed, true) != wiki.Refuse ||
		wiki.Decide("atlas", "Payments/x", "", false) != wiki.Suggest ||
		wiki.Decide("atlas", "Payments/x", agentdef.Allowed, true) != wiki.Direct ||
		wiki.Decide("atlas", "Payments/x", agentdef.Denied, true) != wiki.Refuse {
		t.Fatal("Decide")
	}

	// Own memory: published directly, and listed on the Memory index.
	none := map[string]agentdef.Mode{}
	r, err := e.svc.AgentWrite(e.ctx, e.atlas, none, wiki.Save{Path: "agents/atlas/Partition rollback gotchas", Body: "Keep the legacy index 72h. See [[ADR-012]], [[PAY-214]]."})
	if err != nil || r.State != artifacts.Published {
		t.Fatalf("memory write: %+v %v", r, err)
	}
	idx, err := e.q.GetNoteByPath(e.ctx, store.GetNoteByPathParams{WorkspaceID: e.ws, Path: "agents/atlas/Memory"})
	if err != nil {
		t.Fatal("no memory index")
	}
	if ls := e.linksFrom(t, idx.ID); len(ls) != 1 || *ls[0].TargetID != r.Note.ID {
		t.Fatalf("index links %+v", ls)
	}
	// Writing again doesn't list it twice.
	if _, err := e.svc.AgentWrite(e.ctx, e.atlas, none, wiki.Save{Path: "agents/atlas/Partition rollback gotchas", Body: "More.", Mode: wiki.ModeAppend}); err != nil {
		t.Fatal(err)
	}
	if ls := e.linksFrom(t, idx.ID); len(ls) != 1 {
		t.Fatalf("index listed twice: %d", len(ls))
	}
	// Another agent's namespace: refused.
	if _, err := e.svc.AgentWrite(e.ctx, e.echo, none, wiki.Save{Path: "agents/atlas/Sneaky", Body: "x"}); !invalid(err) {
		t.Fatalf("cross-namespace: %v", err)
	}
	// Denied: refused.
	if _, err := e.svc.AgentWrite(e.ctx, e.echo, map[string]agentdef.Mode{"wiki:write": agentdef.Denied}, wiki.Save{Path: "Shared", Body: "x"}); !invalid(err) {
		t.Fatalf("denied: %v", err)
	}

	// Shared page, wiki:write unset → suggestion; the page isn't visible yet.
	s1, err := e.svc.AgentWrite(e.ctx, e.echo, none, wiki.Save{Path: "Payments/Conventions", Body: "Use [[ADR-012]]."})
	if err != nil || s1.State != artifacts.Suggested || s1.Note.CurrentVersionID != nil {
		t.Fatalf("suggest new: %+v %v", s1, err)
	}
	if tg, _ := links.Resolve(e.ctx, e.q, e.ws, "Payments/Conventions", ""); tg.Kind != "unresolved" {
		t.Fatalf("suggested page resolves before acceptance: %+v", tg)
	}
	// Rejecting the only suggestion removes the page.
	if _, err := e.svc.DecideSuggestion(e.ctx, e.ws, e.me.ID, s1.Version.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.q.GetNoteByPath(e.ctx, store.GetNoteByPathParams{WorkspaceID: e.ws, Path: "Payments/Conventions"}); err == nil {
		t.Fatal("rejected new page still exists")
	}

	before, _ := e.q.CountSuggestions(e.ctx, e.ws) // the demo seeds one from Sentry
	// Existing page: suggestion, then accept → published as v2, links indexed.
	base := e.save(t, "Payments/Conventions", "Original.")
	s2, err := e.svc.AgentWrite(e.ctx, e.echo, none, wiki.Save{Path: "Payments/Conventions", Body: "See [[ADR-012]].", Mode: wiki.ModeAppend})
	if err != nil || s2.State != artifacts.Suggested {
		t.Fatalf("suggest edit: %+v %v", s2, err)
	}
	if n, _ := e.q.CountSuggestions(e.ctx, e.ws); n != before+1 {
		t.Fatalf("pending = %d, want %d", n, before+1)
	}
	if len(e.linksFrom(t, base.Note.ID)) != 0 {
		t.Fatal("suggestion indexed before acceptance")
	}
	a, err := e.svc.DecideSuggestion(e.ctx, e.ws, e.me.ID, s2.Version.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := e.q.GetArtifactVersion(e.ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: e.ws})
	if v.Label != "v2" || *v.BodyMd != "Original.\n\nSee [[ADR-012]]." || len(e.linksFrom(t, a.ID)) != 1 {
		t.Fatalf("accepted: %s %q", v.Label, deref(v.BodyMd))
	}
	if _, err := e.svc.DecideSuggestion(e.ctx, e.ws, e.me.ID, s2.Version.ID, true); !errors.As(err, new(*apperr.ConflictError)) {
		t.Fatalf("decide twice: %v", err)
	}
	// Allowed: direct.
	d, err := e.svc.AgentWrite(e.ctx, e.echo, map[string]agentdef.Mode{"wiki:write": agentdef.Allowed}, wiki.Save{Path: "Payments/Conventions", Body: "Direct."})
	if err != nil || d.State != artifacts.Published {
		t.Fatalf("allowed: %+v %v", d, err)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// The Phase 4.5 exit: Atlas's memory note about PAY-214 reaches its next
// run on PAY-224 through ADR-012, and ADR-012 lists the note as a backlink.
func TestRunContext(t *testing.T) {
	e := setup(t)
	tr := &tracker.Service{Pool: e.svc.Pool}
	note, err := e.svc.AgentWrite(e.ctx, e.atlas, nil, wiki.Save{Path: "agents/atlas/Partition rollback gotchas",
		Body: "# Partition rollback gotchas\n\nDetaching a partition takes a lock; see [[ADR-012]]. Learned on [[PAY-214]].\n"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.AttachArtifact(e.ctx, e.ws, tracker.UserActor(e.me.ID), "PAY-224", e.adr.ID, "context"); err != nil {
		t.Fatal(err)
	}
	e.save(t, "Payments/Unrelated", "Nothing to do with ledgers.")
	tk, _ := tracker.Get(e.ctx, e.q, e.ws, "PAY-224")

	pages, err := wiki.RunContext(e.ctx, e.q, e.atlas, tk, agentdef.Memory{Enabled: true, Inject: "linked", MaxTokens: 4000})
	if err != nil {
		t.Fatal(err)
	}
	refs := []string{}
	for _, p := range pages {
		refs = append(refs, p.Ref+" ("+p.Reason+")")
	}
	// The index, then Atlas's own note, then team pages linking to the PRD.
	if len(pages) < 2 || pages[0].Ref != "agents/atlas/Memory" || pages[1].Target.ID == nil || *pages[1].Target.ID != note.Note.ID ||
		pages[1].Reason != "Links to ADR-012" {
		t.Fatalf("context: %v", refs)
	}
	for _, p := range pages {
		if p.Ref == "Payments/Unrelated" {
			t.Fatalf("unrelated page injected: %v", refs)
		}
	}
	// Index only.
	if p, _ := wiki.RunContext(e.ctx, e.q, e.atlas, tk, agentdef.Memory{Enabled: true, Inject: "index", MaxTokens: 4000}); len(p) != 1 {
		t.Fatalf("index-only: %d", len(p))
	}
	// Disabled.
	if p, _ := wiki.RunContext(e.ctx, e.q, e.atlas, tk, agentdef.Memory{}); len(p) != 0 {
		t.Fatal("disabled memory still injected")
	}
	// Budget trims: the second page is cut.
	long := strings.Repeat("A line about partitions and rollback.\n", 400)
	if _, err := e.svc.AgentWrite(e.ctx, e.atlas, nil, wiki.Save{Path: "agents/atlas/Partition rollback gotchas", Body: long + "[[ADR-012]]"}); err != nil {
		t.Fatal(err)
	}
	p, _ := wiki.RunContext(e.ctx, e.q, e.atlas, tk, agentdef.Memory{Enabled: true, Inject: "linked", MaxTokens: 500})
	if len(p) != 2 || !p[1].Truncated {
		t.Fatalf("budget: %d pages", len(p))
	}
	// Search: the ticket title finds a note nobody linked.
	e.save(t, "Payments/Ledger partition rollback checklist", "Steps for a rollback of the ledger partition.")
	p, _ = wiki.RunContext(e.ctx, e.q, e.atlas, tk, agentdef.Memory{Enabled: true, Inject: "search", MaxTokens: 50000})
	found := false
	for _, x := range p {
		found = found || x.Reason == "Matches the ticket title"
	}
	if !found {
		t.Fatal("search injection found nothing")
	}

	bl, _ := wiki.Backlinks(e.ctx, e.q, e.ws, wiki.Ref{Kind: "artifact", ID: e.adr.ID})
	if len(bl) == 0 || bl[0].Source.Path != "agents/atlas/Partition rollback gotchas" {
		t.Fatalf("ADR-012 backlinks %+v", bl)
	}
}

func TestGraphSearchVault(t *testing.T) {
	e := setup(t)
	a := e.save(t, "Hub", "[[ADR-012]] [[PAY-214]] [[Spoke]]")
	e.save(t, "Spoke", "Back to [[Hub]].")
	nodes, edges, err := wiki.LocalGraph(e.ctx, e.q, e.ws, wiki.Ref{Kind: "artifact", ID: a.Note.ID}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 4 || len(edges) != 4 {
		t.Fatalf("graph: %d nodes, %d edges", len(nodes), len(edges))
	}
	hits, err := wiki.Search(e.ctx, e.q, e.ws, "partitioning", nil, nil, 10)
	if err != nil || len(hits) == 0 {
		t.Fatalf("search: %v %v", hits, err)
	}
	if ac, err := wiki.Autocomplete(e.ctx, e.q, e.ws, "ADR-01"); err != nil || len(ac) == 0 || ac[0].DisplayID != "ADR-012" {
		t.Fatalf("autocomplete: %+v %v", ac, err)
	}

	notes, _ := e.q.ListNotes(e.ctx, e.ws)
	var buf bytes.Buffer
	if err := wiki.Export(e.ctx, e.q, e.ws, &buf); err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.Import(e.ctx, e.ws, e.me.ID, buf.Bytes())
	if err != nil || res.Unchanged != len(notes) || res.Created != 0 {
		t.Fatalf("re-import: %+v %v", res, err)
	}
	if _, err := e.svc.Import(e.ctx, e.ws, e.me.ID, []byte("not a zip")); !invalid(err) {
		t.Fatal("bad zip accepted")
	}
}

// Published patterns matching the ticket come right after the memory index.
func TestRunContextPatterns(t *testing.T) {
	e := setup(t)
	tk, _ := tracker.Get(e.ctx, e.q, e.ws, "PAY-224")
	tk.DescriptionMd = ""
	linked := agentdef.Memory{Enabled: true, Inject: "linked", MaxTokens: 4000}
	refs := func(m agentdef.Memory, tk store.Ticket) []string {
		t.Helper()
		pages, err := wiki.RunContext(e.ctx, e.q, e.atlas, tk, m)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, p := range pages {
			out = append(out, p.Ref+" ("+p.Reason+")")
		}
		return out
	}
	hasPattern := func(rs []string) bool {
		for _, r := range rs {
			if strings.HasPrefix(r, "Patterns/") {
				return true
			}
		}
		return false
	}

	tk.Title = "Retry flaky ledger webhook deliveries"
	e.save(t, "Patterns/API/Retry webhook deliveries with backoff", "Topic: API\nTags: webhooks, retries\n\n## When to use\n\nDelivering webhooks that can fail.\n\n## Rule\n\nRetry with exponential backoff.\n")
	e.save(t, "Patterns/Proposed/Retry ledger webhook deliveries forever", "Topic: API\nTags: webhooks, retries\n\n## Rule\n\nRetry ledger webhook deliveries.\n")

	// Title match: the pattern sits right after the index; the proposal never appears.
	want := "Patterns/API/Retry webhook deliveries with backoff (Pattern: matches the ticket)"
	for _, inject := range []string{"linked", "search"} {
		rs := refs(agentdef.Memory{Enabled: true, Inject: inject, MaxTokens: 4000}, tk)
		if len(rs) < 2 || rs[0] != "agents/atlas/Memory (Your memory index)" || rs[1] != want {
			t.Fatalf("%s: %v", inject, rs)
		}
		for _, r := range rs {
			if strings.Contains(r, "Proposed") {
				t.Fatalf("%s: proposed pattern injected: %v", inject, rs)
			}
		}
	}
	// A match in the description counts too.
	tk2 := tk
	tk2.Title = "Ledger work"
	tk2.DescriptionMd = "## Context\n\nOur **webhook deliveries** fail now and then; please add retries with backoff."
	if rs := refs(linked, tk2); !hasPattern(rs) {
		t.Fatalf("description match: %v", rs)
	}
	// Unrelated tickets and inject=index get none.
	tk3 := tk
	tk3.Title = "Update the onboarding illustrations"
	if rs := refs(linked, tk3); hasPattern(rs) {
		t.Fatalf("unrelated: %v", rs)
	}
	if rs := refs(agentdef.Memory{Enabled: true, Inject: "index", MaxTokens: 4000}, tk); hasPattern(rs) {
		t.Fatalf("index: %v", rs)
	}
	// The budget still bounds the total.
	pages, err := wiki.RunContext(e.ctx, e.q, e.atlas, tk, agentdef.Memory{Enabled: true, Inject: "linked", MaxTokens: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 || !pages[len(pages)-1].Truncated && len(pages) > 1 {
		t.Fatalf("budget: %d pages", len(pages))
	}
	if n := len(pages); n > 2 {
		t.Fatalf("budget not respected: %d pages", n)
	}
}

// Regression: patterns linking each other used to be injected into unrelated
// tickets that shared a single common word.
func TestRunContextPatternsNeedRealOverlap(t *testing.T) {
	e := setup(t)
	tk, _ := tracker.Get(e.ctx, e.q, e.ws, "PAY-224")
	e.save(t, "Patterns/Web/Avoid known Playwright and Next e2e traps", "Topic: Web\nTags: playwright, e2e, nextjs\n\n## Rule\n\nSee [[Patterns/Web/Put web logic in node-testable modules]]. Chat pages need stable selectors.\n")
	e.save(t, "Patterns/Web/Put web logic in node-testable modules", "Topic: Web\nTags: nextjs, testing\n\n## Rule\n\nKeep logic out of components; see [[Patterns/Web/Avoid known Playwright and Next e2e traps]]. Chat state too.\n")
	e.save(t, "Patterns/Architecture/Isolate harness tool policy", "Topic: Architecture\nTags: harness, policy\n\n## Rule\n\nGenerate the tool policy per run.\n")
	patterns := func(title, desc string) []string {
		t.Helper()
		x := tk
		x.Title, x.DescriptionMd = title, desc
		pages, err := wiki.RunContext(e.ctx, e.q, e.atlas, x, agentdef.Memory{Enabled: true, Inject: "linked", MaxTokens: 8000})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, p := range pages {
			if strings.HasPrefix(p.Ref, "Patterns/") {
				out = append(out, p.Ref)
			}
		}
		return out
	}
	// One shared common word ("chat", "testing" in a body) is not enough.
	if got := patterns("Retry chat webhook failures in the runner", ""); len(got) != 0 {
		t.Fatalf("single common word matched: %v", got)
	}
	// Two body-only words still miss: a title or tag match is required.
	if got := patterns("Chat state flakiness", "components"); len(got) != 0 {
		t.Fatalf("body-only match: %v", got)
	}
	// Real title/tag overlap picks the right pattern, not the React ones.
	got := patterns("Fix harness tool policy for chat runs", "")
	if len(got) != 1 || got[0] != "Patterns/Architecture/Isolate harness tool policy" {
		t.Fatalf("policy ticket: %v", got)
	}
}

func TestSelectiveMemoryUsesBoundedRelevantPreviews(t *testing.T) {
	e := setup(t)
	tr := &tracker.Service{Pool: e.svc.Pool}
	tk, _ := tracker.Get(e.ctx, e.q, e.ws, "PAY-214")
	long := strings.Repeat("Partition rollback locking guidance. ", 200)
	for _, name := range []string{"One", "Two", "Three", "Four"} {
		if _, err := e.svc.AgentWrite(e.ctx, e.atlas, nil, wiki.Save{Path: "agents/atlas/" + name, Body: long + " [[PAY-214]]"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tr.AttachArtifact(e.ctx, e.ws, tracker.UserActor(e.me.ID), "PAY-214", e.adr.ID, "context"); err != nil {
		t.Fatal(err)
	}
	pages, err := wiki.RunContext(e.ctx, e.q, e.atlas, tk, agentdef.DefaultMemory)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 || len(pages) > 3 {
		t.Fatalf("%d previews", len(pages))
	}
	for _, p := range pages {
		if p.Ref == "agents/atlas/Memory" || !p.Preview || len([]rune(p.Markdown)) > 1100 || !strings.Contains(p.Markdown, "read_page(") {
			t.Fatalf("%+v", p)
		}
	}
}
