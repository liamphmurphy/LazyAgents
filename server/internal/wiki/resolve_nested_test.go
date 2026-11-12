package wiki_test

import (
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/links"
)

// TestBareLinkToNestedPage records how a bare [[Note]] resolves today when the
// page lives in a folder (docs/knowledge/ADR-knowledge.md §5). Obsidian
// resolves a bare name by file name anywhere in the vault; the cases marked
// "gap" are where we differ, and the Obsidian import ticket closes them.
func TestBareLinkToNestedPage(t *testing.T) {
	e := setup(t)
	deep := e.save(t, "Payments/Ledger/Partition gotchas", "Triggers block ALTER.")

	resolve := func(text string) links.Target {
		t.Helper()
		tg, err := links.Resolve(e.ctx, e.q, e.ws, text, "")
		if err != nil {
			t.Fatalf("resolve %q: %v", text, err)
		}
		return tg
	}
	is := func(text, want string) {
		t.Helper()
		tg := resolve(text)
		if want == "" {
			if tg.Kind != links.KindUnresolved {
				t.Errorf("[[%s]] resolved to %q, expected a ghost", text, tg.Path)
			}
			return
		}
		if tg.Kind != links.KindArtifact || tg.Path != want {
			t.Errorf("[[%s]] = %s %q, want %q", text, tg.Kind, tg.Path, want)
		}
	}

	// 1. A bare name finds the nested page: a note's title is its last path
	// segment, and FindNotesByTitle matches it anywhere.
	is("Partition gotchas", "Payments/Ledger/Partition gotchas")
	is("partition GOTCHAS", "Payments/Ledger/Partition gotchas") // case-insensitive
	is("Payments/Ledger/Partition gotchas", "Payments/Ledger/Partition gotchas")

	// 2. Gap: Obsidian accepts a trailing part of the path; we need the full path.
	is("Ledger/Partition gotchas", "")

	// 3. Gap: [[Name.md]] works as a full path (".md" is stripped) but not as a
	// bare name, because the title lookup uses the raw text.
	is("Payments/Ledger/Partition gotchas.md", "Payments/Ledger/Partition gotchas")
	is("Partition gotchas.md", "")

	// 4. Same name in several folders: the shortest path (in characters) wins,
	// then alphabetical. Obsidian would ask the writer to qualify the link.
	shallow := e.save(t, "Ops/Partition gotchas", "Ops copy.")
	is("Partition gotchas", "Ops/Partition gotchas")
	e.save(t, "Archive/Partition gotchas", "Archive copy.") // longer path: loses
	is("Partition gotchas", "Ops/Partition gotchas")
	e.save(t, "Abc/Partition gotchas", "Same length as Ops/: alphabetical wins.")
	is("Partition gotchas", "Abc/Partition gotchas")

	// 5. A link is stored by id when it's written, so adding a same-named page
	// later doesn't move links that already resolved.
	src := e.save(t, "Notes/Uses ledger", "See [[Ledger notes]].")
	ledger := e.save(t, "Payments/Deep/Ledger notes", "x") // resolves the ghost
	e.save(t, "Ledger notes", "a shorter, later page")
	if ls := e.linksFrom(t, src.Note.ID); len(ls) != 1 || ls[0].TargetID == nil || *ls[0].TargetID != ledger.Note.ID {
		t.Errorf("stored link moved: %+v", ls)
	}
	is("Ledger notes", "Ledger notes") // but a new link takes the shorter one

	// 6. Gap: after a rename to a new name, the old full path is an alias, but
	// the old bare name is not.
	if _, err := e.svc.Rename(e.ctx, e.ws, shallow.Note.ID, "Ops/Hot partitions"); err != nil {
		t.Fatal(err)
	}
	is("Ops/Partition gotchas", "Ops/Hot partitions")
	is("Partition gotchas", "Abc/Partition gotchas") // another page with the name
	if _, err := e.svc.Rename(e.ctx, e.ws, deep.Note.ID, "Payments/Ledger/Trigger locks"); err != nil {
		t.Fatal(err)
	}
	is("Payments/Ledger/Partition gotchas", "Payments/Ledger/Trigger locks")
	solo := e.save(t, "Runbooks/Deep/Failover drill", "x")
	if _, err := e.svc.Rename(e.ctx, e.ws, solo.Note.ID, "Runbooks/Deep/Region failover"); err != nil {
		t.Fatal(err)
	}
	is("Runbooks/Deep/Failover drill", "Runbooks/Deep/Region failover")
	is("Failover drill", "") // a new bare link to the old name is a ghost

	// 7. A name a ticket key or ADR id would match is never a page name.
	e.save(t, "Glossary/PAY-214", "a page named like a ticket")
	if tg := resolve("PAY-214"); tg.Kind != links.KindTicket {
		t.Errorf("[[PAY-214]] = %s, want the ticket", tg.Kind)
	}
}
