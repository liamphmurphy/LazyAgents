package deck

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// Every starter fixture is listed, is a valid deck that survives a canonical
// round trip unchanged, has 5–8 slides of real content using a mix of
// layouts and shapes, and uses theme colour names only, so it recolours with
// any theme it's created in.
func TestStarterFixturesValidate(t *testing.T) {
	files, err := fs.Glob(starterFiles, "starters/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(Starters) || len(Starters) < 5 {
		t.Fatalf("%d fixtures for %d starters", len(files), len(Starters))
	}
	hex := regexp.MustCompile(`"(color|fill|stroke|highlight)":"#`)
	for _, st := range Starters {
		raw, err := starterFiles.ReadFile("starters/" + st.Key + ".json")
		if err != nil {
			t.Fatalf("%s: %v", st.Key, err)
		}
		canon, err := Normalize(string(raw))
		if err != nil {
			t.Fatalf("%s: %v", st.Key, err)
		}
		if again, err := Normalize(canon); err != nil || again != canon {
			t.Fatalf("%s: not stable in canonical form: %v", st.Key, err)
		}
		if hex.MatchString(canon) {
			t.Errorf("%s: uses a hex colour; use theme colour names so it follows the theme", st.Key)
		}
		d, _ := Parse(canon)
		if n := len(d.Slides); n < 5 || n > 8 {
			t.Errorf("%s: %d slides, want 5–8", st.Key, n)
		}
		layouts, shapes := map[string]bool{}, 0
		for i, s := range d.Slides {
			layouts[s.Layout] = true
			text := ""
			for _, e := range s.Elements {
				if e.Type == "shape" {
					shapes++
				}
				if e.Placeholder == "image" {
					t.Errorf("%s slide %d: an empty image slot; starters carry no images", st.Key, i+1)
				}
				for _, p := range append(e.Paragraphs, textBoxParas(e.Text)...) {
					for _, r := range p.Runs {
						text += r.Text
					}
				}
			}
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s slide %d: no placeholder content", st.Key, i+1)
			}
		}
		if len(layouts) < 4 || shapes == 0 {
			t.Errorf("%s: %d layouts and %d shapes; want a mix", st.Key, len(layouts), shapes)
		}
	}
}

func textBoxParas(t *TextBox) []Paragraph {
	if t == nil {
		return nil
	}
	return t.Paragraphs
}

// A starter is created valid in every built-in theme, with ids no other
// deck from the same starter shares, and the title replaces the first title.
func TestFromStarter(t *testing.T) {
	for _, st := range Starters {
		for _, theme := range ThemeOrder {
			d, err := FromStarter(st.Key, ThemeNamed(theme), "")
			if err != nil {
				t.Fatalf("%s in %s: %v", st.Key, theme, err)
			}
			if d.Theme.Name != Themes[theme].Name {
				t.Fatalf("%s in %s: theme %s", st.Key, theme, d.Theme.Name)
			}
		}
		fixture, _ := StarterDeck(st.Key)
		a, _ := FromStarter(st.Key, ThemeNamed("dark"), "Our own title")
		b, _ := FromStarter(st.Key, ThemeNamed("dark"), "")
		old := map[string]bool{}
		for _, d := range []Deck{fixture, b} {
			for _, s := range d.Slides {
				old[s.ID] = true
				for _, e := range s.Elements {
					old[e.ID] = true
				}
			}
		}
		for _, s := range a.Slides {
			if old[s.ID] {
				t.Fatalf("%s: slide id %s reused", st.Key, s.ID)
			}
			for _, e := range s.Elements {
				if old[e.ID] {
					t.Fatalf("%s: element id %s reused", st.Key, e.ID)
				}
			}
		}
		if got := a.Slides[0].Elements[0].Paragraphs[0].Runs[0].Text; got != "Our own title" {
			t.Fatalf("%s: first title %q", st.Key, got)
		}
		if got := b.Slides[0].Elements[0].Paragraphs[0].Runs[0].Text; got != fixture.Slides[0].Elements[0].Paragraphs[0].Runs[0].Text {
			t.Fatalf("%s: untitled first title %q", st.Key, got)
		}
	}
	if _, err := FromStarter("nope", ThemeNamed(""), ""); err == nil {
		t.Fatal("unknown starter accepted")
	}
}
