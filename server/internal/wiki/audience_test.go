package wiki_test

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
	"strings"
	"testing"
)

func TestPrivatePageNeverReachesTicketRun(t *testing.T) {
	e := setup(t)
	tk, err := tracker.Get(e.ctx, e.q, e.ws, "PAY-214")
	if err != nil {
		t.Fatal(err)
	}
	secret := e.save(t, "Hidden/"+tk.Title, "Private payload 9cfe [[ADR-012]] [[PAY-214]]")
	// Legacy data may already have links and attachments; readers must still hide it.
	pool := e.svc.Pool
	if _, err := pool.Exec(e.ctx, `UPDATE artifacts SET audience='private',owner_user_id=$2 WHERE id=$1`, secret.Note.ID, e.me.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.q.AttachArtifact(e.ctx, store.AttachArtifactParams{TicketID: tk.ID, ArtifactID: secret.Note.ID, Relation: "context"}); err != nil {
		t.Fatal(err)
	}
	if err := e.q.InsertLink(e.ctx, store.InsertLinkParams{WorkspaceID: e.ws, SourceKind: "ticket", SourceID: tk.ID, SourceTicketID: &tk.ID, TargetKind: "artifact", TargetID: &secret.Note.ID, TargetText: *secret.Note.Path, RawText: "[[" + *secret.Note.Path + "]]", Snippet: "Ticket-authored text"}); err != nil {
		t.Fatal(err)
	}
	rows, err := e.q.ListNotes(e.ctx, store.ListNotesParams{WorkspaceID: e.ws, IncludeAgent: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == secret.Note.ID {
			t.Fatal("note list leak")
		}
	}
	typ := "note"
	listed, err := e.q.ListArtifacts(e.ctx, store.ListArtifactsParams{WorkspaceID: e.ws, Type: &typ, IncludeAgent: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range listed {
		if r.ID == secret.Note.ID {
			t.Fatal("artifact list leak")
		}
	}
	attached, err := e.q.ListTicketArtifacts(e.ctx, store.ListTicketArtifactsParams{TicketID: tk.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range attached {
		if r.ID == secret.Note.ID {
			t.Fatal("attachment list leak")
		}
	}
	owner := pageview.Person(e.ctx, e.me.ID)
	if _, err := e.q.GetArtifact(owner, store.GetArtifactParams{ID: secret.Note.ID, WorkspaceID: e.ws, Viewer: &e.me.ID}); err != nil {
		t.Fatal(err)
	}
	for _, inject := range []string{agentdef.InjectLinked, agentdef.InjectSearch, agentdef.InjectSelective} { // caller identity must be dropped even in a person's dispatch preview
		pages, err := wiki.RunContext(owner, e.q, e.atlas, tk, agentdef.Memory{Enabled: true, Inject: inject, MaxTokens: 50000})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range pages {
			if p.Target.ID != nil && *p.Target.ID == secret.Note.ID {
				t.Fatalf("%s context leaked private page", inject)
			}
		}
	}
	hits, err := wiki.Search(e.ctx, e.q, e.ws, tk.Title, nil, nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.ID != nil && *h.ID == secret.Note.ID {
			t.Fatal("search leak")
		}
	}
	ac, err := wiki.Autocomplete(e.ctx, e.q, e.ws, tk.Title)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range ac {
		if h.ID != nil && *h.ID == secret.Note.ID {
			t.Fatal("autocomplete leak")
		}
	}
	bl, err := wiki.Backlinks(e.ctx, e.q, e.ws, wiki.Ref{Kind: "artifact", ID: e.adr.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bl {
		if b.SourceID == secret.Note.ID {
			t.Fatal("backlink leak")
		}
	}
	if bl, err := wiki.Backlinks(e.ctx, e.q, e.ws, wiki.Ref{Kind: "artifact", ID: secret.Note.ID}); err != nil || len(bl) != 0 {
		t.Fatalf("hidden target backlinks: %v %v", bl, err)
	}
	for _, centre := range []wiki.Ref{{Kind: "ticket", ID: tk.ID}, {Kind: "artifact", ID: secret.Note.ID}} {
		nodes, edges, err := wiki.LocalGraph(e.ctx, e.q, e.ws, centre, 2)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal([]any{nodes, edges})
		if strings.Contains(string(b), secret.Note.ID.String()) {
			t.Fatal("graph leak")
		}
	}
	if _, _, err := wiki.LookupRef(e.ctx, e.q, e.ws, secret.Note.ID.String()); err == nil {
		t.Fatal("id lookup leaked")
	}
	if target, err := links.Resolve(e.ctx, e.q, e.ws, *secret.Note.Path, ""); err != nil || target.Kind != "unresolved" {
		t.Fatalf("resolver %+v %v", target, err)
	}
	_, err = e.q.GetArtifactVersion(e.ctx, store.GetArtifactVersionParams{ID: secret.Version.ID, ArtifactID: secret.Note.ID, WorkspaceID: e.ws, Viewer: pageview.From(e.ctx).UserID})
	if err != pgx.ErrNoRows {
		t.Fatalf("version leak %v", err)
	}
	if err := e.svc.Tracker.AttachArtifact(owner, e.ws, tracker.Actor{Type: "user", ID: &e.me.ID}, tk.Key, secret.Note.ID, "context"); err == nil || !strings.Contains(err.Error(), "Share the page first") {
		t.Fatalf("attach: %v", err)
	}
	// Runs may create a shared page at the same path without learning that a private page exists.
	shared, err := e.svc.AgentWrite(e.ctx, e.atlas, map[string]agentdef.Mode{"wiki:write": agentdef.Allowed}, wiki.Save{Path: *secret.Note.Path, Body: "Public independent page", Mode: wiki.ModeCreate})
	if err != nil || shared.Note.ID == secret.Note.ID {
		t.Fatalf("run write: %+v %v", shared, err)
	}
}

func TestPersonDiscoveryExcludesAgentPagesUnlessRequested(t *testing.T) {
	e := setup(t)
	r, err := e.svc.AgentWrite(e.ctx, e.atlas, nil, wiki.Save{Path: "agents/atlas/Uniqueneedle", Body: "Uniqueneedle [[ADR-012]]"})
	if err != nil {
		t.Fatal(err)
	}
	for _, include := range []bool{false, true} {
		ctx := pageview.IncludeAgents(pageview.Person(e.ctx, e.me.ID), include)
		hits, err := wiki.Search(ctx, e.q, e.ws, "Uniqueneedle", nil, nil, 20)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, h := range hits {
			found = found || (h.ID != nil && *h.ID == r.Note.ID)
		}
		if found != include {
			t.Fatalf("search include=%v found=%v", include, found)
		}
		ac, err := wiki.Autocomplete(ctx, e.q, e.ws, "Uniqueneedle")
		if err != nil {
			t.Fatal(err)
		}
		found = false
		for _, h := range ac {
			found = found || (h.ID != nil && *h.ID == r.Note.ID)
		}
		if found != include {
			t.Fatalf("autocomplete include=%v found=%v", include, found)
		}
		nodes, _, err := wiki.LocalGraph(ctx, e.q, e.ws, wiki.Ref{Kind: "artifact", ID: e.adr.ID}, 1)
		if err != nil {
			t.Fatal(err)
		}
		found = false
		for _, n := range nodes {
			found = found || (n.Target.ID != nil && *n.Target.ID == r.Note.ID)
		}
		if found != include {
			t.Fatalf("graph include=%v found=%v", include, found)
		}
	}
}

func TestPrivatePageOwnerIsolation(t *testing.T) {
	e := setup(t)
	r := e.save(t, "Owner's note", "Private text")
	if _, err := e.svc.Pool.Exec(e.ctx, `UPDATE artifacts SET audience='private',owner_user_id=$2 WHERE id=$1`, r.Note.ID, e.me.ID); err != nil {
		t.Fatal(err)
	}
	other := uuid.New()
	for _, viewer := range []*uuid.UUID{nil, &other, &e.me.ID} {
		a, err := e.q.GetNoteByPath(e.ctx, store.GetNoteByPathParams{WorkspaceID: e.ws, Path: *r.Note.Path, Viewer: viewer})
		if viewer == &e.me.ID {
			if err != nil || a.ID != r.Note.ID {
				t.Fatalf("owner can't see note: %v", err)
			}
		} else if err != pgx.ErrNoRows {
			t.Fatalf("non-owner saw note: %v", err)
		}
	}
}

func TestPrivateCreationAndSharingConflicts(t *testing.T) {
	e := setup(t)
	e.save(t, "Same path", "Shared text")
	p, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Path: "Same path", Body: "Private text", Mode: wiki.ModeCreate})
	if err != nil {
		t.Fatal(err)
	}
	if p.Note.Audience != "private" || p.Note.OwnerUserID == nil || *p.Note.OwnerUserID != e.me.ID {
		t.Fatalf("private default: %+v", p.Note)
	}
	if _, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Path: "Same path", Body: "Private text", Audience: "shared"}); err == nil || !strings.Contains(err.Error(), "Rename it first") {
		t.Fatalf("sharing collision: %v", err)
	}
	if _, err := e.svc.AgentWrite(e.ctx, e.atlas, nil, wiki.Save{Path: "Same path", Body: "Agent text", Audience: "private"}); err == nil {
		t.Fatal("agent accepted audience")
	}
}
