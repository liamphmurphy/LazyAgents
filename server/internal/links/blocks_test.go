package links

import (
	"strings"
	"testing"
)

func TestParseLabelHeadingAndBlockLinks(t *testing.T) {
	got := Parse("See [[Runbook|the runbook]], [[Runbook#Roll back]], [[Runbook#^step-2]] and ![[Runbook#^step-2]].")
	if len(got) != 4 {
		t.Fatalf("%+v", got)
	}
	if got[0].Target != "Runbook" || got[0].Display != "the runbook" || got[0].Anchor != "" {
		t.Errorf("label: %+v", got[0])
	}
	if got[1].Anchor != "Roll back" {
		t.Errorf("heading: %+v", got[1])
	}
	if id, ok := BlockID(got[2].Anchor); !ok || id != "step-2" || got[2].Embed {
		t.Errorf("block link: %+v", got[2])
	}
	if id, ok := BlockID(got[3].Anchor); !ok || id != "step-2" || !got[3].Embed {
		t.Errorf("block embed: %+v", got[3])
	}
	if _, ok := BlockID("Roll back"); ok {
		t.Error("a heading is not a block id")
	}
	if _, ok := BlockID("^has space"); ok {
		t.Error("block ids are letters, digits and hyphens")
	}
}

func TestBlocks(t *testing.T) {
	md := strings.Join([]string{
		"# Steps",
		"First paragraph line",
		"continues here. ^para",
		"",
		"- one",
		"- two ^item",
		"  - nested under two",
		"- three",
		"",
		"> a quote",
		"^quote",
		"",
		"```",
		"code ^notablock",
		"```",
		"Again ^para",
	}, "\n")
	want := map[string]string{
		"para":  "First paragraph line\ncontinues here.",
		"item":  "- two\n  - nested under two",
		"quote": "> a quote",
	}
	bs := Blocks(md)
	if len(bs) != len(want) {
		t.Fatalf("blocks %+v", bs)
	}
	for _, b := range bs {
		if want[b.ID] != b.Text {
			t.Errorf("block %s: %q, want %q", b.ID, b.Text, want[b.ID])
		}
	}
	if _, ok := FindBlock(md, "notablock"); ok {
		t.Error("ids inside code fences don't count")
	}
}
