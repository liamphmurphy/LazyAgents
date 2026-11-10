package runs

import (
	"strings"
	"testing"
)

// mockup is an svg-image reply of about n characters that ends in </svg>, so
// a clipped copy is easy to tell from a whole one.
func mockup(label string, n int) string {
	pad := strings.Repeat(`<rect x="1" y="1" width="2" height="2"/>`, n/40)
	return "Here's the " + label + " mockup.\n\n```svg-image\n<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 10 10\"><title>" + label + "</title>" + pad + "</svg>\n```\n\nWant changes?"
}

func TestRenderHandoffKeepsNewestMockupWhole(t *testing.T) {
	// A mockup larger than both the per-message clip and the whole history
	// budget used to reach a fresh segment cut mid-element, or not at all.
	h := []HandoffMessage{
		{Seq: 4, Role: "user", Body: "Make the sidebar narrower."},
		{Seq: 3, Role: "assistant", Persona: "Maya", Body: mockup("v2", 30000)},
		{Seq: 2, Role: "user", Body: "Redo it with a sidebar."},
		{Seq: 1, Role: "assistant", Persona: "Maya", Body: mockup("v1", 30000)},
	}
	got := RenderHandoff(h, 4, DefaultHandoffChars, HandoffContext{}, "Now redraw it.")

	if !strings.Contains(got, "<title>v2</title>") || !strings.Contains(got, "</svg>\n```") {
		t.Fatal("newest mockup is not in the handoff whole")
	}
	if strings.Contains(got, "<title>v1</title>") {
		t.Error("an earlier version was handed over too; only the latest should be")
	}
	if !strings.Contains(got, "[earlier svg-image block omitted") {
		t.Error("no note where the earlier version was")
	}
	// With the blocks compacted, every message fits the budget again.
	for _, kept := range []string{"[1] assistant Maya", "Redo it with a sidebar.", "[3] assistant Maya", "Want changes?"} {
		if !strings.Contains(got, kept) {
			t.Errorf("missing %q", kept)
		}
	}
	if strings.Contains(got, "omitted to fit") {
		t.Error("messages were dropped for an artifact that no longer counts against the budget")
	}
	if strings.Index(got, "Latest artifacts") > strings.Index(got, handoffFooter) || !strings.HasSuffix(got, "Now redraw it.") {
		t.Error("artifacts must come after the history and before the new message")
	}
}

func TestRenderHandoffPinsNewestOfEachKind(t *testing.T) {
	h := []HandoffMessage{
		{Seq: 3, Role: "assistant", Body: "```app\n<p>calc v2</p>\n```\n```svg-steps\n<svg>steps</svg>\n1. one\n```"},
		{Seq: 2, Role: "assistant", Body: "```app\n<p>calc v1</p>\n```"},
		{Seq: 1, Role: "assistant", Body: mockup("only", 100)},
	}
	got := RenderHandoff(h, 3, 0, HandoffContext{}, "NEW")
	for _, want := range []string{"From message [1] (svg-image)", "<title>only</title>", "From message [3] (app)", "calc v2", "From message [3] (svg-steps)", "1. one"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(got, "calc v1") {
		t.Error("earlier app version handed over")
	}
}

func TestRenderHandoffLeavesOtherBlocksAndPlainHistoryAlone(t *testing.T) {
	body := "Run this:\n```go\nfmt.Println(1)\n```\n```chart\n{}\n```"
	got := RenderHandoff([]HandoffMessage{{Seq: 1, Role: "assistant", Body: body}}, 1, 0, HandoffContext{}, "NEW")
	if !strings.Contains(got, body) {
		t.Error("ordinary code blocks must stay in place")
	}
	if strings.Contains(got, "Latest artifacts") {
		t.Error("an artifacts section with nothing in it")
	}
}

func TestRenderHandoffShowsCanvases(t *testing.T) {
	canvases := []HandoffCanvas{
		{ID: "c-old", Kind: "text", Title: "Plan", Version: 2},
		{ID: "c-new", Kind: "svg", Title: "Settings mockup", Version: 5, Body: "<svg>```tricky```</svg>"},
	}
	h := []HandoffMessage{{Seq: 1, Role: "assistant", Body: "```canvas\nc-new\n```"}}
	got := RenderHandoff(h, 1, 0, HandoffContext{Canvases: canvases}, "NEW")
	for _, want := range []string{"canvas_read", `c-old "Plan" (text, version 2)`, `c-new "Settings mockup" (svg, version 5)`, "````svg\n<svg>```tricky```</svg>\n````"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Count(got, "Most recently changed canvas") != 1 {
		t.Error("only the newest canvas is inline")
	}
}

func TestRenderHandoffShowsImagePrompts(t *testing.T) {
	// A generated image is an attachment, not message text: without its prompt
	// a fresh segment can't revise it.
	hc := HandoffContext{ImagePrompts: []string{"a teal fox, flat vector", "a red fox"}}
	got := RenderHandoff([]HandoffMessage{{Seq: 1, Role: "user", Body: "draw a fox"}}, 1, 0, hc, "make it bigger")
	if !strings.Contains(got, "1. a teal fox, flat vector\n2. a red fox") {
		t.Fatalf("image prompts missing or out of order:\n%s", got)
	}
}

func TestFencedBlocks(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		kinds []string
	}{
		{"backticks and tildes", "a\n```svg-image\nx\n```\n~~~app\ny\n~~~\n", []string{"svg-image", "app"}},
		{"longer fence holds a shorter one", "````svg-image\n```\nstill inside\n````\n", []string{"svg-image"}},
		{"closing fence must be the same character", "```app\n~~~\n```", []string{"app"}},
		{"unclosed runs to the end", "text\n```svg-image\n<svg>", []string{"svg-image"}},
		{"indented in a list, kind case-folded", "- item\n  ```SVG-Image title\n  <svg/>\n  ```", []string{"svg-image"}},
		{"two backticks is not a fence", "``app``\n", nil},
	}
	for _, c := range cases {
		var kinds []string
		for _, b := range fencedBlocks(c.body) {
			kinds = append(kinds, b.kind)
		}
		if strings.Join(kinds, ",") != strings.Join(c.kinds, ",") {
			t.Errorf("%s: got %v, want %v", c.name, kinds, c.kinds)
		}
	}
	body := "pre\n```app\nx\n```\npost"
	b := fencedBlocks(body)[0]
	if body[b.start:b.end] != "```app\nx\n```\n" {
		t.Errorf("offsets cover %q", body[b.start:b.end])
	}
}
