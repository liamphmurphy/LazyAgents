package deck

import (
	"testing"
)

func TestApplyStyleMapsPresetsToProperties(t *testing.T) {
	type want struct {
		fill, stroke, text, dash string
		width                    float64
	}
	cases := map[string]want{
		"filled-accent":  {fill: "accent", text: "background"},
		"filled-accent2": {fill: "accent2", text: "background"},
		"outline-accent": {stroke: "accent", width: 6, text: "accent"},
		"subtle":         {fill: "surface", stroke: "muted", width: 3, text: "text"},
		"emphasis":       {fill: "heading", text: "background"},
		"ghost":          {stroke: "muted", width: 3, text: "muted", dash: "dash"},
	}
	if len(cases) != len(StyleIDs) || len(Styles) != len(StyleIDs) {
		t.Fatalf("every style needs a case: %d cases, %d ids, %d styles", len(cases), len(StyleIDs), len(Styles))
	}
	for id, w := range cases {
		half := 0.5
		e := Element{Type: "shape", Fill: &Paint{Gradient: &Gradient{Type: "linear", Stops: []string{"accent", "muted"}}}, FillOpacity: &half, StrokeOpacity: &half, Dash: "dot", Stroke: "#654321", StrokeWidth: 9, Text: &TextBox{Paragraphs: []Paragraph{{Runs: []Run{{Text: "a", Color: "#FFFFFF"}, {Text: "b"}}}}}}
		if err := ApplyStyle(&e, id); err != nil {
			t.Fatal(err)
		}
		fill := ""
		if e.Fill != nil {
			fill = e.Fill.Color
		}
		if fill != w.fill || (e.Fill != nil && e.Fill.Gradient != nil) || e.Stroke != w.stroke || e.StrokeWidth != w.width || e.Dash != w.dash || e.FillOpacity != nil || e.StrokeOpacity != nil {
			t.Errorf("%s: %+v, want %+v", id, e, w)
		}
		for _, r := range e.Text.Paragraphs[0].Runs {
			if r.Color != w.text {
				t.Errorf("%s: run colour %q, want %q", id, r.Color, w.text)
			}
		}
		l := Element{Type: "line", Stroke: "#000000", StrokeWidth: 1, Dash: "dot", StrokeOpacity: &half}
		if err := ApplyStyle(&l, id); err != nil || l.Stroke != Styles[id].LineStroke || l.StrokeWidth <= 0 || l.Dash != w.dash || l.StrokeOpacity != nil {
			t.Errorf("%s on a line: %+v %v", id, l, err)
		}
	}
	// Only theme names, so styled elements follow the theme.
	for id, s := range Styles {
		for _, c := range []string{s.Fill, s.Stroke, s.Text, s.LineStroke} {
			if c != "" && !isThemeColor(c) {
				t.Errorf("%s uses %q, not a theme colour", id, c)
			}
		}
	}
	if err := ApplyStyle(&Element{Type: "shape"}, "neon"); err == nil {
		t.Fatal("unknown style accepted")
	}
	if err := ApplyStyle(&Element{Type: "text"}, "subtle"); err == nil {
		t.Fatal("style applied to a text box")
	}
}

func isThemeColor(c string) bool {
	switch c {
	case "background", "text", "heading", "accent", "accent2", "muted", "surface":
		return true
	}
	return false
}

func TestSetElementStyleOp(t *testing.T) {
	body, _ := Normalize(fixture(t))
	out, err := Apply(body, []Op{
		{Op: "set_element", ID: "e_s1", Value: map[string]any{"style": "outline-accent"}},
		{Op: "set_element", ID: "e_s2", Value: map[string]any{"style": "ghost", "radius": 20}},
		{Op: "set_element", ID: "e_s4", Value: map[string]any{"style": "emphasis"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := Parse(out)
	i, j, _ := d.findElement("e_s1")
	s1 := d.Slides[i].Elements[j]
	if s1.Fill != nil || s1.Stroke != "accent" || s1.StrokeWidth != 6 || s1.Text.Paragraphs[0].Runs[0].Color != "accent" {
		t.Fatalf("outline-accent not applied: %+v", s1)
	}
	i, j, _ = d.findElement("e_s2")
	if s2 := d.Slides[i].Elements[j]; s2.Stroke != "muted" || s2.Fill != nil || s2.Dash != "dash" || s2.Radius != 20 {
		t.Fatalf("ghost with radius not applied: %+v", s2)
	}
	i, j, _ = d.findElement("e_s4")
	if s4 := d.Slides[i].Elements[j]; s4.Stroke != "heading" || s4.StrokeWidth != 10 {
		t.Fatalf("emphasis line not applied: %+v", s4)
	}
	for _, v := range []any{"neon", 3} {
		if _, err := Apply(body, []Op{{Op: "set_element", ID: "e_s1", Value: map[string]any{"style": v}}}); err == nil {
			t.Fatalf("style %v accepted", v)
		}
	}
}
