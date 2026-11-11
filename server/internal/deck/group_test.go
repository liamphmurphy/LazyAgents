package deck

import (
	"strings"
	"testing"
)

func groupSlide(t *testing.T, d Deck) Slide {
	t.Helper()
	i := d.slideIndex("s_group")
	if i < 0 {
		t.Fatal("no group slide in the fixture")
	}
	return d.Slides[i]
}

func TestGroupFrameIsChildrenBox(t *testing.T) {
	// A stale group frame is replaced by the children's box.
	d, _ := Parse(fixture(t))
	d.Slides[d.slideIndex("s_group")].Elements[1].Frame = Frame{W: 1, H: 1}
	body, _ := d.Encode()
	out, err := Normalize(body)
	if err != nil {
		t.Fatal(err)
	}
	d, _ = Parse(out)
	g := groupSlide(t, d).Elements[1]
	if g.Type != "group" || g.Frame != (Frame{X: 120, Y: 300, W: 760, H: 240}) || len(g.Elements) != 2 {
		t.Fatalf("group frame not derived: %+v", g)
	}
	// Lines with a negative height count from both ends.
	if b := Bounds([]Element{{Frame: Frame{X: 10, Y: 100, W: 50, H: -80}}, {Frame: Frame{X: 0, Y: 50, W: 5, H: 5}}}); b != (Frame{X: 0, Y: 20, W: 60, H: 80}) {
		t.Fatalf("bounds: %+v", b)
	}
}

func TestGroupRules(t *testing.T) {
	good := fixture(t)
	for name, mut := range map[string]func(string) string{
		"one child": func(s string) string {
			return strings.Replace(s, `{"fill":"accent","frame":{"h":240,"w":240,"x":120,"y":300},"id":"e_gp2","shape":"ellipse","type":"shape"},`, ``, 1)
		},
		"nested group": func(s string) string {
			return strings.Replace(s, `"id":"e_gp2","shape":"ellipse","type":"shape"`, `"id":"e_gp2","type":"group","elements":[{"id":"e_n1","type":"line","frame":{"x":0,"y":0,"w":10,"h":0},"stroke":"text","strokeWidth":2},{"id":"e_n2","type":"line","frame":{"x":0,"y":0,"w":10,"h":0},"stroke":"text","strokeWidth":2}]`, 1)
		},
		"child id reused at top level":     func(s string) string { return strings.Replace(s, `"id":"e_gp3"`, `"id":"e_gp0"`, 1) },
		"child id reused in another group": func(s string) string { return strings.Replace(s, `"id":"e_gp5"`, `"id":"e_gp2"`, 1) },
		"group id reused by a child":       func(s string) string { return strings.Replace(s, `"id":"e_gp2"`, `"id":"e_gp1"`, 1) },
		"rotated group":                    func(s string) string { return strings.Replace(s, `"id":"e_gp1",`, `"id":"e_gp1","rotation":10,`, 1) },
		"styled group":                     func(s string) string { return strings.Replace(s, `"id":"e_gp1",`, `"id":"e_gp1","fill":"accent",`, 1) },
		"placeholder child": func(s string) string {
			return strings.Replace(s, `"id":"e_gp3",`, `"id":"e_gp3","placeholder":"body",`, 1)
		},
		"bad child": func(s string) string {
			return strings.Replace(s, `"id":"e_gp2","shape":"ellipse"`, `"id":"e_gp2","shape":"star"`, 1)
		},
		"elements on a shape": func(s string) string {
			return strings.Replace(s, `"id":"e_gp8",`, `"id":"e_gp8","elements":[{"id":"e_n1","type":"line","frame":{"x":0,"y":0,"w":10,"h":0},"stroke":"text","strokeWidth":2}],`, 1)
		},
	} {
		bad := mut(good)
		if bad == good {
			t.Fatalf("%s: mutation didn't apply", name)
		}
		if _, err := Normalize(bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestGroupChildrenCountTowardMaxElements(t *testing.T) {
	d, _ := Parse(fixture(t))
	s := &d.Slides[d.slideIndex("s_group")]
	// 4 top-level elements, 5 in groups: 9. Fill up to MaxElements, then one more.
	for CountElements(s.Elements) < MaxElements {
		s.Elements = append(s.Elements, Element{ID: NewID("e"), Type: "line", Frame: Frame{W: 10}, Stroke: "text", StrokeWidth: 2})
	}
	if len(s.Elements) != MaxElements-5 {
		t.Fatalf("count doesn't include children: %d top-level", len(s.Elements))
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
	s.Elements = append(s.Elements, Element{ID: NewID("e"), Type: "line", Frame: Frame{W: 10}, Stroke: "text", StrokeWidth: 2})
	if err := d.Validate(); err == nil || !strings.Contains(err.Error(), "counting those in groups") {
		t.Fatalf("over the limit: %v", err)
	}
}

func TestGroupOps(t *testing.T) {
	body, _ := Normalize(fixture(t))
	out, err := Apply(body, []Op{
		// A child is found by id wherever it is.
		{Op: "set_element", ID: "e_gp3", Value: map[string]any{"paragraphs": []any{map[string]any{"runs": []any{map[string]any{"text": "Renamed", "size": 56}}}}}},
		// Inserting after a child puts the new element in that group.
		{Op: "insert_element", Slide: "s_group", After: "e_gp5", Value: map[string]any{"id": "e_gp9", "type": "line", "frame": map[string]any{"x": 1000, "y": 900, "w": 900, "h": 0}, "stroke": "text", "strokeWidth": 2}},
		// Setting a group's frame scales its children.
		{Op: "set_element", ID: "e_gp1", Value: map[string]any{"frame": map[string]any{"x": 120, "y": 300, "w": 380, "h": 120}}},
		// A new group from scratch, children's ids assigned.
		{Op: "insert_element", Slide: "s_shapes", Value: map[string]any{"type": "group", "elements": []any{
			map[string]any{"type": "line", "frame": map[string]any{"x": 0, "y": 0, "w": 100, "h": 0}, "stroke": "text", "strokeWidth": 2},
			map[string]any{"type": "line", "frame": map[string]any{"x": 0, "y": 50, "w": 100, "h": 0}, "stroke": "text", "strokeWidth": 2},
		}}},
		// A style on a group styles its shapes and lines.
		{Op: "set_element", ID: "e_gp4", Value: map[string]any{"style": "emphasis"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := Parse(out)
	s := groupSlide(t, d)
	g1 := s.Elements[1]
	if g1.Elements[1].Paragraphs[0].Runs[0].Text != "Renamed" {
		t.Fatalf("child not set: %+v", g1.Elements[1])
	}
	if g1.Frame != (Frame{X: 120, Y: 300, W: 380, H: 120}) || g1.Elements[0].Frame != (Frame{X: 120, Y: 300, W: 120, H: 120}) || g1.Elements[1].Frame != (Frame{X: 260, Y: 330, W: 240, H: 60}) {
		t.Fatalf("children not scaled: %+v", g1)
	}
	g4 := s.Elements[2]
	if len(g4.Elements) != 4 || g4.Elements[1].ID != "e_gp9" || g4.Frame.W != 900 || g4.Frame.H != 600 {
		t.Fatalf("insert into group: %+v", g4)
	}
	if f := g4.Elements[0].Fill; f == nil || f.Color != "heading" || g4.Elements[2].Stroke != "heading" {
		t.Fatalf("style not applied to the group's shapes: %+v", g4.Elements)
	}
	shapes := d.Slides[d.slideIndex("s_shapes")].Elements
	if n := shapes[len(shapes)-1]; n.Type != "group" || n.Frame != (Frame{W: 100, H: 50}) || !elementID.MatchString(n.Elements[1].ID) {
		t.Fatalf("new group: %+v", n)
	}

	// Removing a group's child; a group left with one element dissolves into it.
	out, err = Apply(out, []Op{{Op: "remove_element", ID: "e_gp9"}, {Op: "remove_element", ID: "e_gp2"}})
	if err != nil {
		t.Fatal(err)
	}
	d, _ = Parse(out)
	s = groupSlide(t, d)
	if s.Elements[1].ID != "e_gp3" || s.Elements[1].Type != "text" || len(s.Elements[2].Elements) != 3 {
		t.Fatalf("remove from group: %+v", s.Elements)
	}
	// A group can't be rotated or given a child that's a group.
	for _, op := range []Op{
		{Op: "set_element", ID: "e_gp4", Value: map[string]any{"rotation": 15}},
		{Op: "set_element", ID: "e_gp4", Value: map[string]any{"type": "shape"}},
	} {
		if _, err := Apply(out, []Op{op}); err == nil {
			t.Errorf("%+v: accepted", op)
		}
	}
}

func TestInsertSlideWithGroupAssignsChildIDs(t *testing.T) {
	body, _ := Normalize(fixture(t))
	line := func(y float64) map[string]any {
		return map[string]any{"type": "line", "frame": map[string]any{"x": 0, "y": y, "w": 100, "h": 0}, "stroke": "text", "strokeWidth": 2}
	}
	out, err := Apply(body, []Op{{Op: "insert_slide", Value: map[string]any{"id": "s_new", "layout": "blank", "elements": []any{
		map[string]any{"type": "group", "elements": []any{line(0), line(50)}},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := Parse(out)
	for _, s := range d.Slides {
		if s.ID == "s_new" {
			if g := s.Elements[0]; !g.IsGroup() || g.Elements[0].ID == "" || g.Elements[1].ID == "" || g.Elements[0].ID == g.Elements[1].ID {
				t.Fatalf("children ids not assigned: %+v", g)
			}
			return
		}
	}
	t.Fatal("slide not inserted")
}
