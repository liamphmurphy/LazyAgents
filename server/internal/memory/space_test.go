package memory_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/memory"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

// A space's memory stays in the space: chats in it see their own space's
// entries and the general memory; chats elsewhere, and other spaces, never see
// them. Near-duplicates don't merge across the line, and the memory manager
// (ScopeAll) sees everything.
func TestSpaceMemoryIsolation(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	q := store.New(pool)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	me, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	space := func(name string) uuid.UUID {
		sp, err := q.CreateChatSpace(ctx, store.CreateChatSpaceParams{WorkspaceID: me.WorkspaceID, UserID: me.ID, Name: name, Color: "green", ConnectorIds: []uuid.UUID{}, SpaceMemory: true})
		if err != nil {
			t.Fatal(err)
		}
		return sp.ID
	}
	health, novel := space("Health"), space("Novel")
	svc := memory.New(q)
	person := memory.Owner{WorkspaceID: me.WorkspaceID, UserID: me.ID}
	inHealth, inNovel := person.InSpace(&health), person.InSpace(&novel)
	src := memory.Source{Label: "manual"}
	save := func(o memory.Owner, title, summary string) memory.SaveResult {
		t.Helper()
		r, err := svc.Save(ctx, o, memory.Input{Title: title, Summary: summary}, src)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	general := save(person, "Prefers metric units", "Use kilograms and kilometres")
	protein := save(inHealth, "Protein target", "Aims for about 140 g a day")
	if protein.Memory.SpaceID == nil || *protein.Memory.SpaceID != health {
		t.Fatalf("saved in a space: %+v", protein.Memory.SpaceID)
	}
	plot := save(inNovel, "Second act twist", "The broker is the narrator's sister")

	list := func(o memory.Owner, text string) []string {
		t.Helper()
		hits, err := svc.Find(ctx, o, memory.Query{Text: text})
		if err != nil {
			t.Fatal(err)
		}
		return titles(hits)
	}
	if got := list(person, ""); slices.Contains(got, "Protein target") || slices.Contains(got, "Second act twist") || !slices.Contains(got, "Prefers metric units") {
		t.Fatalf("outside any space: %v", got)
	}
	if got := list(inHealth, ""); !slices.Contains(got, "Protein target") || !slices.Contains(got, "Prefers metric units") || slices.Contains(got, "Second act twist") {
		t.Fatalf("in Health: %v", got)
	}
	if got := list(person, "protein"); len(got) != 0 {
		t.Fatalf("search outside: %v", got)
	}
	if got := list(inNovel, "protein"); len(got) != 0 {
		t.Fatalf("search in another space: %v", got)
	}
	if _, err := svc.Get(ctx, person, protein.Memory.ID); err == nil {
		t.Fatal("read a space's entry from outside it")
	}
	if _, err := svc.Get(ctx, inHealth, general.Memory.ID); err != nil {
		t.Fatalf("general memory from a space: %v", err)
	}
	if err := svc.Forget(ctx, inNovel, protein.Memory.ID); err == nil {
		t.Fatal("forgot another space's entry")
	}

	// The index in a space marks the space's entries.
	idx, _, err := svc.Index(ctx, inHealth)
	if err != nil {
		t.Fatal(err)
	}
	marked := 0
	for _, e := range idx {
		if e.Space {
			marked++
			if e.Title != "Protein target" {
				t.Fatalf("marked %q", e.Title)
			}
		}
	}
	if marked != 1 {
		t.Fatalf("index %+v", idx)
	}
	if md := memory.IndexMarkdown(idx, len(idx)); !strings.Contains(md, "[other, this space] Protein target") {
		t.Fatalf("index markdown:\n%s", md)
	}

	// A near-duplicate saved outside the space adds a general entry rather
	// than updating the space's.
	again := save(person, "Protein target", "Aims for about 140 g a day")
	if again.Updated || again.Memory.SpaceID != nil {
		t.Fatalf("merged across the space line: %+v", again)
	}
	// Saved again inside the space, it updates the space's own entry.
	if r := save(inHealth, "Protein target", "Aims for 150 g a day now"); !r.Updated || r.Memory.ID != protein.Memory.ID {
		t.Fatalf("near-duplicate in the space: %+v", r)
	}

	// The manager sees every space's entries, and can keep to one space.
	all := person
	all.Scope = memory.ScopeAll
	if got := list(all, ""); !slices.Contains(got, "Second act twist") || !slices.Contains(got, "Protein target") {
		t.Fatalf("manager: %v", got)
	}
	hits, err := svc.Find(ctx, all, memory.Query{Space: &novel})
	if err != nil || len(hits) != 1 || hits[0].ID != plot.Memory.ID {
		t.Fatalf("manager in Novel: %v %v", titles(hits), err)
	}

	// A persona's own memory never belongs to a space, even saved from one.
	ps := &personas.Service{Pool: pool}
	gwen, err := ps.Create(ctx, personas.Owner{WorkspaceID: me.WorkspaceID, UserID: me.ID}, personas.NewInput{Name: "Gwen"})
	if err != nil {
		t.Fatal(err)
	}
	own := save(inHealth.Persona(gwen.ID), "Likes the gym", "")
	if own.Memory.SpaceID != nil {
		t.Fatal("persona memory got a space")
	}

	// Deleting the space forgets its memory.
	if _, err := q.DeleteChatSpace(ctx, store.DeleteChatSpaceParams{ID: health, WorkspaceID: me.WorkspaceID, UserID: me.ID}); err != nil {
		t.Fatal(err)
	}
	// Only the general "Protein target" is left.
	if got := list(all, ""); countTitle(got, "Protein target") != 1 || !slices.Contains(got, "Second act twist") {
		t.Fatalf("after delete: %v", got)
	}
}

func countTitle(list []string, title string) int {
	n := 0
	for _, s := range list {
		if s == title {
			n++
		}
	}
	return n
}
