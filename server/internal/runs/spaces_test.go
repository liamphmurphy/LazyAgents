package runs

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

func source(title string, tokens int32) store.ChatSpaceSource {
	return store.ChatSpaceSource{ID: uuid.New(), Kind: "text", Title: title, Tokens: tokens, Body: "x"}
}

// Sources fill the inline budget in the order they were added: one too long
// on its own, or past what's left, is searched instead, and a later small one
// can still fit.
func TestSpaceDelivery(t *testing.T) {
	a := source("a", 15_000)
	big := source("big", SpaceInlineSourceMax+1)
	b := source("b", 20_000)
	c := source("c", 6_000)
	d := source("d", 4_000)
	file := store.ChatSpaceSource{ID: uuid.New(), Kind: "file", Title: "scan.pdf", BlobKey: new(string)}
	all := []store.ChatSpaceSource{a, big, b, c, file, d}
	got := SpaceDelivery(all)
	want := map[string]string{"a": SpaceInline, "big": SpaceSearched, "b": SpaceInline, "c": SpaceSearched, "scan.pdf": SpaceInline, "d": SpaceInline}
	for _, s := range all {
		if got[s.ID] != want[s.Title] {
			t.Errorf("%s: %s, want %s", s.Title, got[s.ID], want[s.Title])
		}
	}
	if n := SpaceInlineTokens(all); n != 39_000 {
		t.Fatalf("inline tokens %d", n)
	}
}

func TestEstimateTokens(t *testing.T) {
	if EstimateTokens("") != 0 || EstimateTokens("abcd") != 1 || EstimateTokens("abcde") != 2 || EstimateTokens("héllo wörld") != 3 {
		t.Fatal("estimates")
	}
}

// Paths come from the position and a slug, never the raw title, and keep a
// file's extension.
func TestSpaceFilesPaths(t *testing.T) {
	name := "Bloodwork Oct 2026.PDF"
	files := SpaceFiles([]store.ChatSpaceSource{
		{ID: uuid.New(), Title: "../../etc/passwd", Body: "x"},
		{ID: uuid.New(), Title: name, Filename: &name, BlobKey: new(string)},
		{ID: uuid.New(), Title: "Diagram", Body: "<svg/>", ContentType: "image/svg+xml"},
		{ID: uuid.New(), Title: "!!!", Body: "x"},
	})
	want := []string{".lazyagents/context/space-1-etc-passwd.md", ".lazyagents/context/space-2-bloodwork-oct-2026-pdf.pdf",
		".lazyagents/context/space-3-diagram.svg", ".lazyagents/context/space-4-source.md"}
	for i, f := range files {
		if f.Path != want[i] {
			t.Errorf("path %d = %s, want %s", i, f.Path, want[i])
		}
	}
}

func TestSpaceSection(t *testing.T) {
	if SpaceSection(nil, nil) != "" {
		t.Fatal("no space, no section")
	}
	sp := &store.ChatSpace{Name: "Health", Description: "Workouts and food", Instructions: "Be my coach.\n```\nbreak out", SpaceMemory: true}
	long := source("Creatine study", SpaceInlineSourceMax+5)
	long.Kind = "website"
	files := SpaceFiles([]store.ChatSpaceSource{source("Training plan", 500), long})
	got := SpaceSection(sp, files)
	for _, want := range []string{"## Space: Health", "Workouts and food", "### Space instructions", "````text\nBe my coach.\n```\nbreak out\n````",
		"Read the ones that bear on the person's message", "`.lazyagents/context/space-1-training-plan.md`: pasted text \"Training plan\"",
		"search inside them", "web page \"Creatine study\" (about 20005 tokens)", "### Space memory", "stays in \"Health\""} {
		if !strings.Contains(got, want) {
			t.Errorf("section lacks %q:\n%s", want, got)
		}
	}
	sp.SpaceMemory, sp.Instructions = false, ""
	got = SpaceSection(sp, nil)
	if strings.Contains(got, "### Space memory") || strings.Contains(got, "### Space instructions") || strings.Contains(got, "### Space sources") {
		t.Fatalf("empty parts rendered:\n%s", got)
	}
}

// A report as a source keeps its bottom line, body and the pages its [n]
// citations point to.
func TestResearchSourceText(t *testing.T) {
	r := ResearchReport{BottomLine: "Use Kokoro.", BodyMD: "Fast [1].\n", Sources: []ResearchNumberedSource{{N: 1, Title: "Repo [main]", URL: "https://github.com/hexgrad/kokoro"}, {N: 2, URL: "https://example.com/a"}}}
	got := ResearchSourceText("Local TTS", r)
	for _, want := range []string{"# Local TTS\n", "**Bottom line:** Use Kokoro.", "Fast [1].", "## Sources", `1. [Repo \[main\]](https://github.com/hexgrad/kokoro)`, "2. [https://example.com/a](https://example.com/a)"} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
}

// The space section says where research reports come from only when it has one.
func TestSpaceSectionNotesResearchSources(t *testing.T) {
	sp := &store.ChatSpace{Name: "Voice"}
	plain := SpaceFiles([]store.ChatSpaceSource{source("notes", 10)})
	if s := SpaceSection(sp, plain); strings.Contains(s, "deep research") {
		t.Fatalf("research note without a report:\n%s", s)
	}
	rep := source("Local TTS", 10)
	rep.Kind = "research"
	s := SpaceSection(sp, SpaceFiles([]store.ChatSpaceSource{rep}))
	if !strings.Contains(s, "deep research") || !strings.Contains(s, `research report "Local TTS"`) {
		t.Fatalf("section:\n%s", s)
	}
}
