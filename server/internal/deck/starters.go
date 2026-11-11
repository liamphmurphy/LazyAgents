package deck

import (
	"embed"
	"fmt"
)

// Starter decks are deck JSON fixtures a new deck can start from instead of
// one blank title slide. Each is a complete deck in the light theme using
// theme colour names only, so it takes any theme it's created in.
//
//go:embed starters/*.json
var starterFiles embed.FS

// A Starter is one entry in the New deck chooser.
type Starter struct {
	Key         string
	Name        string
	Description string
}

// Starters lists the starter decks in chooser order.
var Starters = []Starter{
	{Key: "project-update", Name: "Project update", Description: "Status, milestones, timeline and asks"},
	{Key: "pitch", Name: "Pitch", Description: "Problem, solution, traction and the ask"},
	{Key: "retro", Name: "Retro", Description: "What went well, what didn't, action items"},
	{Key: "teaching", Name: "Teaching / lesson", Description: "Objectives, explanation and a quick check"},
	{Key: "dnd-recap", Name: "D&D session recap", Description: "Previously on, the party, loot and next time"},
}

// StarterNamed is a starter's catalogue entry.
func StarterNamed(key string) (Starter, bool) {
	for _, s := range Starters {
		if s.Key == key {
			return s, true
		}
	}
	return Starter{}, false
}

// StarterDeck is a starter's fixture as stored, validated, with its own ids.
func StarterDeck(key string) (Deck, error) {
	if _, ok := StarterNamed(key); !ok {
		return Deck{}, invalid("unknown starter %q", key)
	}
	b, err := starterFiles.ReadFile("starters/" + key + ".json")
	if err != nil {
		return Deck{}, fmt.Errorf("starter %s: %w", key, err)
	}
	d, err := Parse(string(b))
	if err != nil {
		return d, fmt.Errorf("starter %s: %w", key, err)
	}
	return d, d.Validate()
}

// FromStarter is a new deck from a starter in theme, with fresh slide and
// element ids. A non-empty title replaces the first slide's title.
func FromStarter(key string, theme Theme, title string) (Deck, error) {
	d, err := StarterDeck(key)
	if err != nil {
		return d, err
	}
	d.Theme = theme
	for i := range d.Slides {
		s := &d.Slides[i]
		s.ID = NewID("s")
		for j := range s.Elements {
			e := &s.Elements[j]
			e.ID = NewID("e")
			if i == 0 && title != "" && e.Placeholder == "title" && len(e.Paragraphs) > 0 && len(e.Paragraphs[0].Runs) > 0 {
				e.Paragraphs = e.Paragraphs[:1]
				e.Paragraphs[0].Runs = e.Paragraphs[0].Runs[:1]
				e.Paragraphs[0].Runs[0].Text = title
			}
		}
	}
	return d, d.Validate()
}
