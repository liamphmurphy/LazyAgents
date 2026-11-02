package links

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	md := strings.Join([]string{
		"# Rollback",
		"- See [[ADR-012#Decision|the decision]] and [[PAY-214]].",
		"Inline `[[not a link]]` stays code.",
		"```",
		"[[also not]]",
		"```",
		"| a | [[Runbook\\|table alias]] |",
		"Embed: ![[Payments/Ledger/Runbook#Steps]] and [[@atlas]]",
		"Empty [[ ]] and [[#Only anchor]].",
	}, "\n")
	got := Parse(md)
	want := []Ref{
		{Target: "ADR-012", Anchor: "Decision", Display: "the decision", Line: 1},
		{Target: "PAY-214", Line: 1},
		{Target: "Runbook", Display: "table alias", Line: 6},
		{Target: "Payments/Ledger/Runbook", Anchor: "Steps", Embed: true, Line: 7},
		{Target: "@atlas", Line: 7},
		{Target: "", Anchor: "Only anchor", Line: 8},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d links: %+v", len(got), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Target != w.Target || g.Anchor != w.Anchor || g.Display != w.Display || g.Embed != w.Embed || g.Line != w.Line {
			t.Errorf("link %d: got %+v, want %+v", i, g, w)
		}
	}
	if got[0].Snippet != "See [[ADR-012#Decision|the decision]] and [[PAY-214]]." {
		t.Errorf("snippet drops the list marker: %q", got[0].Snippet)
	}
	if got[0].Raw != "[[ADR-012#Decision|the decision]]" || got[3].Raw != "![[Payments/Ledger/Runbook#Steps]]" {
		t.Errorf("raw: %q, %q", got[0].Raw, got[3].Raw)
	}
}

func TestSnippetClipsLongLines(t *testing.T) {
	line := strings.Repeat("word ", 100) + "[[Target]]" + strings.Repeat(" tail", 100)
	s := Parse(line)[0].Snippet
	if !strings.Contains(s, "[[Target]]") || !strings.HasPrefix(s, "…") || !strings.HasSuffix(s, "…") || len([]rune(s)) > 210 {
		t.Fatalf("snippet %q", s)
	}
}

func TestParseRefAndPaths(t *testing.T) {
	r := ParseRef("[[Ledger v2 — Product requirements#Reconciliation performance]]")
	if r.Target != "Ledger v2 — Product requirements" || r.Anchor != "Reconciliation performance" {
		t.Fatalf("%+v", r)
	}
	if p := NormalizePath("  Payments//Ledger /  Rollback   runbook.md "); p != "Payments/Ledger/Rollback runbook" {
		t.Fatalf("normalize: %q", p)
	}
	if TitleOf("a/b/Rollback runbook") != "Rollback runbook" || TitleOf("Top") != "Top" {
		t.Fatal("TitleOf")
	}
	if h := NoteHref("agents/atlas/Partition gotchas"); h != "/wiki/agents/atlas/Partition%20gotchas" {
		t.Fatalf("href %q", h)
	}
	if Slug("§4.2 Partition by month") != "4-2-partition-by-month" {
		t.Fatal("slug")
	}
}
