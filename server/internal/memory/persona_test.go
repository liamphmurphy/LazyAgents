package memory_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/memory"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

func titles(hits []memory.Hit) []string {
	out := []string{}
	for _, h := range hits {
		out = append(out, h.Title)
	}
	return out
}

func invalid(err error) bool {
	var v *apperr.ValidationError
	return errors.As(err, &v)
}

// The person's memory and each persona's own memory are apart: the index,
// listing, search and near-duplicate merging stay in one memory, moments only
// live in a persona's, and no owner reaches another persona's entries.
func TestPersonaMemoryScoping(t *testing.T) {
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
	ps := &personas.Service{Pool: pool}
	po := personas.Owner{WorkspaceID: me.WorkspaceID, UserID: me.ID}
	gwen, err := ps.Create(ctx, po, personas.NewInput{Name: "Gwen"})
	if err != nil {
		t.Fatal(err)
	}
	coach, err := ps.Create(ctx, po, personas.NewInput{Name: "Coach"})
	if err != nil {
		t.Fatal(err)
	}
	svc := memory.New(q)
	person := memory.Owner{WorkspaceID: me.WorkspaceID, UserID: me.ID}
	src := memory.Source{Label: "manual"}
	save := func(o memory.Owner, in memory.Input) memory.SaveResult {
		t.Helper()
		r, err := svc.Save(ctx, o, in, src)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	save(person, memory.Input{Title: "Has two cats", Type: "fact"})
	look := save(person.Persona(gwen.ID), memory.Input{Title: "Has auburn hair", Body: "Tortoiseshell glasses too."})
	// The same title in another memory is a new entry, not a near-duplicate.
	if r := save(person.Persona(gwen.ID), memory.Input{Title: "Has two cats"}); r.Updated {
		t.Fatal("persona save merged into the person's entry")
	}
	save(person.Persona(coach.ID), memory.Input{Title: "Coached rowing"})
	m1 := save(person.Persona(gwen.ID), memory.Input{Title: "Walk by the river", Type: memory.TypeMoment})
	// Moments are never merged, even with an identical title.
	if r := save(person.Persona(gwen.ID), memory.Input{Title: "Walk by the river", Type: memory.TypeMoment}); r.Updated || r.Memory.ID == m1.Memory.ID {
		t.Fatal("a moment was merged into another")
	}
	if _, err := svc.Save(ctx, person, memory.Input{Title: "Nice day", Type: memory.TypeMoment}, src); !invalid(err) {
		t.Fatalf("moment in the person's memory: %v", err)
	}
	// Secrets are refused in a persona's memory too.
	if _, err := svc.Save(ctx, person.Persona(gwen.ID), memory.Input{Title: "Deploy", Body: "key AKIAABCDEFGHIJKLMNOP"}, src); !invalid(err) {
		t.Fatalf("secret saved to persona memory: %v", err)
	}
	// Someone else's (or a made-up) persona is refused.
	if _, err := svc.Save(ctx, person.Persona(uuid.New()), memory.Input{Title: "x"}, src); !invalid(err) {
		t.Fatalf("save to an unknown persona: %v", err)
	}

	idx, total, err := svc.Index(ctx, person)
	if err != nil || total != 1 || len(idx) != 1 || idx[0].Title != "Has two cats" {
		t.Fatalf("index = %+v total %d, %v; want only the person's entry", idx, total, err)
	}
	find := func(o memory.Owner, text string) []string {
		t.Helper()
		hits, err := svc.Find(ctx, o, memory.Query{Text: text, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		return titles(hits)
	}
	if got := find(person, "cats"); len(got) != 1 {
		t.Errorf("person search = %v", got)
	}
	if got := find(person.Persona(gwen.ID), ""); len(got) != 4 {
		t.Errorf("gwen's memory = %v, want 4 entries", got)
	}
	if got := find(person.Persona(coach.ID), ""); len(got) != 1 || got[0] != "Coached rowing" {
		t.Errorf("coach's memory = %v", got)
	}
	chat := memory.Owner{WorkspaceID: me.WorkspaceID, UserID: me.ID, PersonaID: &gwen.ID, Scope: memory.ScopeChat}
	if got := find(chat, ""); len(got) != 5 || strings.Contains(strings.Join(got, ","), "rowing") {
		t.Errorf("gwen chat reach = %v, want the person's and gwen's only", got)
	}
	all := person
	all.Scope = memory.ScopeAll
	if got := find(all, ""); len(got) != 6 {
		t.Errorf("all = %v", got)
	}
	personasOnly := person
	personasOnly.Scope = memory.ScopePersonas
	if n, _ := svc.Count(ctx, personasOnly, memory.Query{}); n != 5 {
		t.Errorf("personas count = %d, want 5", n)
	}

	// Gwen's chat can't read, change or forget Coach's entries.
	coachEntry, _ := svc.Find(ctx, person.Persona(coach.ID), memory.Query{})
	cid := coachEntry[0].ID
	if _, err := svc.Get(ctx, chat, cid); !errors.Is(err, apperr.ErrNotFound) {
		t.Errorf("chat read another persona's entry: %v", err)
	}
	if _, err := svc.Update(ctx, chat, cid, memory.Patch{Title: new("Hacked")}, src); !errors.Is(err, apperr.ErrNotFound) {
		t.Errorf("chat edited another persona's entry: %v", err)
	}
	if err := svc.Forget(ctx, chat, cid); !errors.Is(err, apperr.ErrNotFound) {
		t.Errorf("chat forgot another persona's entry: %v", err)
	}
	// The person's own scope doesn't reach persona entries.
	if _, err := svc.Get(ctx, person, look.Memory.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Errorf("person scope read a persona entry: %v", err)
	}

	// Moving: a person entry into gwen's memory and back; never into a
	// persona that isn't the person's, and a moment can't leave a persona.
	cats, _ := svc.Find(ctx, person, memory.Query{})
	moved, err := svc.Update(ctx, chat, cats[0].ID, memory.Patch{Move: true, ToPersona: &gwen.ID}, src)
	if err != nil || moved.PersonaID == nil || *moved.PersonaID != gwen.ID {
		t.Fatalf("move to persona = %+v, %v", moved, err)
	}
	if _, total, _ := svc.Index(ctx, person); total != 0 {
		t.Errorf("moved entry still in the person's index (%d)", total)
	}
	if back, err := svc.Update(ctx, chat, moved.ID, memory.Patch{Move: true}, src); err != nil || back.PersonaID != nil {
		t.Fatalf("move back = %+v, %v", back, err)
	}
	other := uuid.New()
	if _, err := svc.Update(ctx, all, moved.ID, memory.Patch{Move: true, ToPersona: &other}, src); !invalid(err) {
		t.Errorf("move to an unknown persona: %v", err)
	}
	if _, err := svc.Update(ctx, chat, m1.Memory.ID, memory.Patch{Move: true}, src); !invalid(err) {
		t.Errorf("moment moved to the person's memory: %v", err)
	}
	if _, err := svc.Update(ctx, chat, moved.ID, memory.Patch{Type: new(memory.TypeMoment)}, src); !invalid(err) {
		t.Errorf("person entry became a moment: %v", err)
	}
	// Retiring only works within one memory.
	if _, err := svc.Supersede(ctx, chat, moved.ID, &look.Memory.ID, "test"); !invalid(err) {
		t.Errorf("superseded across memories: %v", err)
	}

	// Deleting a persona deletes its memory; the person's stays.
	if _, err := ps.Delete(ctx, po, coach.ID, nil); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.Count(ctx, all, memory.Query{}); n != 6-1 {
		t.Errorf("after deleting coach: %d entries, want 5", n)
	}
}

// Consolidation merges duplicates within one memory only, and never moments.
func TestConsolidateStaysInOneMemory(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	q := store.New(pool)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	gwen, err := (&personas.Service{Pool: pool}).Create(ctx, personas.Owner{WorkspaceID: me.WorkspaceID, UserID: me.ID}, personas.NewInput{Name: "Gwen"})
	if err != nil {
		t.Fatal(err)
	}
	person := memory.Owner{WorkspaceID: me.WorkspaceID, UserID: me.ID}
	mk := func(o memory.Owner, title, typ string) {
		t.Helper()
		// Straight inserts: Save would merge the near-duplicates itself.
		if _, err := q.CreateMemory(ctx, store.CreateMemoryParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID, PersonaID: o.PersonaID,
			Title: title, Type: typ, Tags: []string{}}); err != nil {
			t.Fatal(err)
		}
	}
	mk(person, "Loves hiking in the gorge", "fact")
	mk(person.Persona(gwen.ID), "Loves hiking in the gorge", "fact")
	mk(person.Persona(gwen.ID), "Hiking day at the gorge", memory.TypeMoment)
	mk(person.Persona(gwen.ID), "Hiking day at the gorge", memory.TypeMoment)
	merges, err := memory.New(q).Consolidate(ctx)
	if err != nil || len(merges) != 0 {
		t.Fatalf("merged across memories or moments: %+v, %v", merges, err)
	}
	mk(person.Persona(gwen.ID), "Loves hiking in the gorge!", "fact")
	merges, err = memory.New(q).Consolidate(ctx)
	if err != nil || len(merges) != 1 || merges[0].Owner.PersonaID == nil || *merges[0].Owner.PersonaID != gwen.ID {
		t.Fatalf("persona duplicate merges = %+v, %v", merges, err)
	}
}

// The always-loaded persona memory keeps bodies, lists moments newest first
// with their date, and stays within its budget, saying how many didn't fit.
func TestPersonaMarkdown(t *testing.T) {
	if got := memory.PersonaMarkdown(nil, 0); got != "Nothing remembered yet.\n" {
		t.Errorf("empty = %q", got)
	}
	day := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	entries := []store.Memory{
		{ID: uuid.New(), Title: "Who I am", Summary: "Gwen", Body: "Late twenties, auburn hair.", Type: "other", UpdatedAt: day},
		{ID: uuid.New(), Title: "River walk", Summary: "We talked about the book", Type: memory.TypeMoment, CreatedAt: day},
	}
	got := memory.PersonaMarkdown(entries, 2)
	for _, want := range []string{"### Who you are", "#### Who I am", "Late twenties, auburn hair.", "### Shared moments", "- 2026-11-07 `" + entries[1].ID.String() + "` River walk: We talked about the book"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "aren't loaded") {
		t.Errorf("everything fit, but says some didn't:\n%s", got)
	}

	var big []store.Memory
	for i := range 40 {
		big = append(big, store.Memory{ID: uuid.New(), Title: fmt.Sprintf("Trait %d", i), Body: strings.Repeat("x", 1000), Type: "other"})
	}
	for i := range 60 {
		big = append(big, store.Memory{ID: uuid.New(), Title: fmt.Sprintf("Moment %d", i), Summary: "a summary", Type: memory.TypeMoment, CreatedAt: day})
	}
	got = memory.PersonaMarkdown(big, 150)
	if len(got) > memory.PersonaBudget+500 {
		t.Errorf("rendered %d characters, budget %d", len(got), memory.PersonaBudget)
	}
	if !strings.Contains(got, "Moment 0") || strings.Contains(got, "Moment 59") {
		t.Errorf("moments not newest-first and capped:\n%s", got)
	}
	if !strings.Contains(got, "body not loaded") || !strings.Contains(got, "more of your entries aren't loaded here") {
		t.Errorf("overflow not reported:\n%s", got[len(got)-300:])
	}
}
