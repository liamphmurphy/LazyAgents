package deck

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Op is one persona edit to a deck, keyed by stable ids.
type Op struct {
	Op    string         // set_theme, insert_slide, set_slide, move_slide, remove_slide, insert_element, set_element, remove_element
	ID    string         // the slide or element acted on
	After string         // insert/move: the slide or element to follow; empty puts it first
	Slide string         // insert_element: the slide to add to
	Value map[string]any // the slide, element, theme or fields to set
}

func opErr(i int, format string, args ...any) error {
	return invalid("op %d: %s", i+1, fmt.Sprintf(format, args...))
}

// convert round-trips v through JSON into out, refusing unknown fields.
func convert(v any, out any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytesReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

func (d *Deck) slideIndex(id string) int {
	return slices.IndexFunc(d.Slides, func(s Slide) bool { return s.ID == id })
}

// findElement finds an element by id: its slide, its index among the slide's
// elements and, for an element inside a group, its index in the group (else -1).
func (d *Deck) findElement(id string) (int, int, int) {
	for i, s := range d.Slides {
		for j, e := range s.Elements {
			if e.ID == id {
				return i, j, -1
			}
			if k := slices.IndexFunc(e.Elements, func(c Element) bool { return c.ID == id }); k >= 0 {
				return i, j, k
			}
		}
	}
	return -1, -1, -1
}

// elementAt is the element findElement found.
func (d *Deck) elementAt(i, j, k int) *Element {
	e := &d.Slides[i].Elements[j]
	if k >= 0 {
		return &e.Elements[k]
	}
	return e
}

// merge sets fields on a copy of cur (as JSON objects), keeping id and type.
func merge(cur any, fields map[string]any, out any) error {
	b, err := json.Marshal(cur)
	if err != nil {
		return err
	}
	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	for k, v := range fields {
		if k == "id" || k == "type" {
			if v != m[k] {
				return fmt.Errorf("%s can't be changed; remove the element and insert a new one", k)
			}
			continue
		}
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	return convert(m, out)
}

// Apply runs ops in order on a deck body; the result is validated as a whole.
func Apply(body string, ops []Op) (string, error) {
	d, err := Parse(body)
	if err != nil {
		return "", err
	}
	for i, op := range ops {
		if err := d.apply(op); err != nil {
			return "", opErr(i, "%v", err)
		}
	}
	if err := d.Validate(); err != nil {
		return "", err
	}
	return d.Encode()
}

func insertAt[T any](list []T, after int, v T) []T {
	return slices.Insert(list, after+1, v)
}

func (d *Deck) apply(op Op) error {
	switch op.Op {
	case "set_theme":
		var t Theme
		if name, ok := op.Value["preset"].(string); ok && len(op.Value) == 1 {
			if _, known := Themes[strings.ToLower(strings.TrimSpace(name))]; !known {
				return fmt.Errorf("unknown theme %q; built-in themes: %s", name, strings.Join(ThemeOrder, ", "))
			}
			t = ThemeNamed(name)
		} else if err := merge(d.Theme, op.Value, &t); err != nil {
			return err
		}
		d.Theme = t
	case "insert_slide":
		after := -1
		if op.After != "" {
			if after = d.slideIndex(op.After); after < 0 {
				return fmt.Errorf("no slide %s", op.After)
			}
		}
		var s Slide
		if err := convert(op.Value, &s); err != nil {
			return fmt.Errorf("value isn't a slide: %v", err)
		}
		if s.ID == "" {
			s.ID = NewID("s")
		}
		if s.Elements == nil {
			title := ""
			s.Elements = Placeholders(s.Layout, title)
		}
		for k := range s.Elements {
			if s.Elements[k].ID == "" {
				s.Elements[k].ID = NewID("e")
			}
		}
		d.Slides = insertAt(d.Slides, after, s)
	case "set_slide":
		i := d.slideIndex(op.ID)
		if i < 0 {
			return fmt.Errorf("no slide %s", op.ID)
		}
		if _, ok := op.Value["elements"]; ok {
			return fmt.Errorf("change elements with insert_element, set_element and remove_element")
		}
		var s Slide
		if err := merge(d.Slides[i], op.Value, &s); err != nil {
			return err
		}
		if s.Layout != d.Slides[i].Layout {
			relayout(&s)
		}
		d.Slides[i] = s
	case "move_slide":
		i := d.slideIndex(op.ID)
		if i < 0 {
			return fmt.Errorf("no slide %s", op.ID)
		}
		s := d.Slides[i]
		d.Slides = slices.Delete(d.Slides, i, i+1)
		after := -1
		if op.After != "" {
			if after = d.slideIndex(op.After); after < 0 {
				return fmt.Errorf("no slide %s", op.After)
			}
		}
		d.Slides = insertAt(d.Slides, after, s)
	case "remove_slide":
		i := d.slideIndex(op.ID)
		if i < 0 {
			return fmt.Errorf("no slide %s", op.ID)
		}
		d.Slides = slices.Delete(d.Slides, i, i+1)
	case "insert_element":
		i := d.slideIndex(op.Slide)
		if i < 0 {
			return fmt.Errorf("no slide %q (set slide to the slide's id)", op.Slide)
		}
		var e Element
		if err := convert(op.Value, &e); err != nil {
			return fmt.Errorf("value isn't an element: %v", err)
		}
		if e.ID == "" {
			e.ID = NewID("e")
		}
		for k := range e.Elements {
			if e.Elements[k].ID == "" {
				e.Elements[k].ID = NewID("e")
			}
		}
		list := &d.Slides[i].Elements
		after := len(*list) - 1
		if op.After != "" {
			si, j, k := d.findElement(op.After)
			if si != i {
				return fmt.Errorf("no element %s on slide %s", op.After, op.Slide)
			}
			// After an element inside a group puts the new one in that group.
			if k >= 0 {
				list, after = &d.Slides[i].Elements[j].Elements, k
			} else {
				after = j
			}
		}
		*list = insertAt(*list, after, e)
	case "set_element":
		i, j, k := d.findElement(op.ID)
		if i < 0 {
			return fmt.Errorf("no element %s", op.ID)
		}
		cur := d.elementAt(i, j, k)
		// {"style": id} applies a style preset, after any other fields.
		fields, style := op.Value, ""
		if v, ok := op.Value["style"]; ok {
			s, isStr := v.(string)
			if !isStr {
				return fmt.Errorf("style must be a style name")
			}
			style = s
			fields = maps.Clone(op.Value)
			delete(fields, "style")
		}
		var e Element
		if err := merge(*cur, fields, &e); err != nil {
			return err
		}
		// A group's frame is its children's box: setting it moves and scales them.
		if _, ok := fields["frame"]; ok && e.IsGroup() {
			to := e.Frame
			e.Frame = Bounds(e.Elements)
			e.ScaleChildren(to)
		}
		if style != "" {
			if err := ApplyStyle(&e, style); err != nil {
				return err
			}
		}
		*cur = e
	case "remove_element":
		i, j, k := d.findElement(op.ID)
		if i < 0 {
			return fmt.Errorf("no element %s", op.ID)
		}
		els := &d.Slides[i].Elements
		if k < 0 {
			*els = slices.Delete(*els, j, j+1)
			break
		}
		g := &(*els)[j]
		g.Elements = slices.Delete(g.Elements, k, k+1)
		// A group left with one element dissolves into it.
		if len(g.Elements) == 1 {
			(*els)[j] = g.Elements[0]
		}
	default:
		return fmt.Errorf("unknown op %q; use set_theme, insert_slide, set_slide, move_slide, remove_slide, insert_element, set_element or remove_element", op.Op)
	}
	return nil
}

// relayout moves a slide's placeholders to where its new layout puts them,
// matching by role, and adds the layout's missing placeholders. Elements that
// aren't placeholders stay where they are.
func relayout(s *Slide) {
	fresh := Placeholders(s.Layout, "")
	used := map[string]bool{}
	// An unfilled image placeholder has nothing to keep: drop it when the
	// new layout has no image slot, rather than leave an invisible box.
	if !slices.ContainsFunc(fresh, func(f Element) bool { return f.Placeholder == "image" }) {
		s.Elements = slices.DeleteFunc(s.Elements, func(e Element) bool { return e.Placeholder == "image" && e.Type == "text" })
	}
	for k := range s.Elements {
		e := &s.Elements[k]
		if e.Placeholder == "" {
			continue
		}
		for _, f := range fresh {
			if f.Placeholder == e.Placeholder && !used[f.Placeholder] {
				e.Frame = f.Frame
				used[f.Placeholder] = true
				break
			}
		}
	}
	for _, f := range fresh {
		if !used[f.Placeholder] {
			s.Elements = append(s.Elements, f)
			used[f.Placeholder] = true
		}
	}
}
