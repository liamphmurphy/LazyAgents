package artifacts

import (
	"reflect"
	"strings"
	"testing"
)

func TestOutline(t *testing.T) {
	md := strings.Join([]string{
		"# Ledger v2 — Product requirements",
		"Intro.",
		"## 1. Problem & goals",
		"```sql",
		"# not a heading",
		"```",
		"## Users and **merchant** impact ##",
		"### 4.2 Partition by `posted_at`",
		"#hashtag is not a heading",
		"## Problem & goals",
		"## [Out of scope](https://x)",
		"####### too deep",
	}, "\n")
	got := Outline(md)
	want := []Section{
		{1, "Ledger v2 — Product requirements", "ledger-v2-product-requirements"},
		{2, "1. Problem & goals", "1-problem-goals"},
		{2, "Users and merchant impact", "users-and-merchant-impact"},
		{3, "4.2 Partition by posted_at", "4-2-partition-by-posted-at"},
		{2, "Problem & goals", "problem-goals"},
		{2, "Out of scope", "out-of-scope"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestDuplicateAnchors(t *testing.T) {
	got := Outline("## Notes\n## Notes\n## Notes")
	for i, want := range []string{"notes", "notes-2", "notes-3"} {
		if got[i].Anchor != want {
			t.Errorf("anchor %d = %q, want %q", i, got[i].Anchor, want)
		}
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"§4.2 Partition by month": "4-2-partition-by-month",
		"Émigré café":             "émigré-café",
		"!!!":                     "section",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSummary(t *testing.T) {
	md := "# Title\n\n> quote\n\nGoals and constraints for the **ledger** rebuild.\nSecond line.\n\nNext paragraph."
	if got := Summary(md); got != "Goals and constraints for the ledger rebuild. Second line." {
		t.Fatalf("Summary = %q", got)
	}
	long := strings.Repeat("word ", 100)
	if got := Summary(long); len([]rune(got)) > 241 || !strings.HasSuffix(got, "…") {
		t.Fatalf("long summary not trimmed: %d", len(got))
	}
}

func TestSectionText(t *testing.T) {
	md := "# Doc\n\nIntro\n\n## Goals\n\nFast.\n\n### Sub\n\nDetail.\n\n## Risks\n\nLocks.\n"
	got, ok := SectionText(md, "goals")
	if !ok || got != "## Goals\n\nFast.\n\n### Sub\n\nDetail.\n" {
		t.Fatalf("goals = %q", got)
	}
	if got, _ := SectionText(md, "risks"); got != "## Risks\n\nLocks.\n" {
		t.Fatalf("risks = %q", got)
	}
	if _, ok := SectionText(md, "nope"); ok {
		t.Fatal("unknown anchor found")
	}
}
