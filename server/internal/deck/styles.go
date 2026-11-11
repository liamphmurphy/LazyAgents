package deck

import (
	"fmt"
	"strings"
)

// A Style is a named bundle of theme colour tokens applied to a shape or a
// line. Styles are applied, not referenced: the element keeps the resulting
// fill, stroke, strokeWidth, dash and run colours (and loses any opacity),
// so it recolours with the theme
// because the values are theme names. web/lib/slides/styles.ts has the same
// table; keep the two in step.
type Style struct {
	Label       string
	Fill        string  // shapes; empty is no fill
	Stroke      string  // empty is no outline
	StrokeWidth float64 // shapes with a stroke
	Text        string  // shapes: every run's colour
	LineWidth   float64 // lines: stroke width
	LineStroke  string  // lines: stroke colour
	Dash        string  // the outline's or line's dash; empty is solid
}

// StyleIDs is the gallery order.
var StyleIDs = []string{"filled-accent", "filled-accent2", "outline-accent", "subtle", "emphasis", "ghost"}

// Styles are the shape style presets, keyed by id.
var Styles = map[string]Style{
	"filled-accent":  {Label: "Filled accent", Fill: "accent", Text: "background", LineStroke: "accent", LineWidth: 10},
	"filled-accent2": {Label: "Filled accent 2", Fill: "accent2", Text: "background", LineStroke: "accent2", LineWidth: 10},
	"outline-accent": {Label: "Outline accent", Stroke: "accent", StrokeWidth: 6, Text: "accent", LineStroke: "accent", LineWidth: 5},
	"subtle":         {Label: "Subtle", Fill: "surface", Stroke: "muted", StrokeWidth: 3, Text: "text", LineStroke: "muted", LineWidth: 4},
	"emphasis":       {Label: "Emphasis", Fill: "heading", Text: "background", LineStroke: "heading", LineWidth: 10},
	"ghost":          {Label: "Ghost", Stroke: "muted", StrokeWidth: 3, Text: "muted", LineStroke: "muted", LineWidth: 3, Dash: "dash"},
}

// ApplyStyle sets a preset's properties on a shape or line.
func ApplyStyle(e *Element, id string) error {
	s, ok := Styles[id]
	if !ok {
		return fmt.Errorf("unknown style %q; use %s", id, strings.Join(StyleIDs, ", "))
	}
	switch e.Type {
	case "shape":
		e.Fill, e.Stroke, e.StrokeWidth, e.Dash = nil, s.Stroke, 0, s.Dash
		if s.Fill != "" {
			e.Fill = &Paint{Color: s.Fill}
		}
		e.FillOpacity, e.StrokeOpacity = nil, nil
		if s.Stroke != "" {
			e.StrokeWidth = s.StrokeWidth
		}
		if e.Text != nil {
			for i := range e.Text.Paragraphs {
				for j := range e.Text.Paragraphs[i].Runs {
					e.Text.Paragraphs[i].Runs[j].Color = s.Text
				}
			}
		}
	case "line":
		e.Stroke, e.StrokeWidth, e.Dash, e.StrokeOpacity = s.LineStroke, s.LineWidth, s.Dash, nil
	case "group":
		// A group's shapes and lines take the style; its text and images stay as they are.
		styled := false
		for k := range e.Elements {
			if c := &e.Elements[k]; c.Type == "shape" || c.Type == "line" {
				_ = ApplyStyle(c, id)
				styled = true
			}
		}
		if !styled {
			return fmt.Errorf("this group has no shapes or lines to style")
		}
	default:
		return fmt.Errorf("styles apply to shapes and lines, not %s elements", e.Type)
	}
	return nil
}
