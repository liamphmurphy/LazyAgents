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

func (d *Deck) findElement(id string) (int, int) {
	for i, s := range d.Slides {
		if j := slices.IndexFunc(s.Elements, func(e Element) bool { return e.ID == id }); j >= 0 {
			return i, j
		}
	}
	return -1, -1
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
			if _, known := Themes[strings.ToLower(name)]; !known {
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
		after := len(d.Slides[i].Elements) - 1
		if op.After != "" {
			if after = slices.IndexFunc(d.Slides[i].Elements, func(e Element) bool { return e.ID == op.After }); after < 0 {
				return fmt.Errorf("no element %s on slide %s", op.After, op.Slide)
			}
		}
		var e Element
		if err := convert(op.Value, &e); err != nil {
			return fmt.Errorf("value isn't an element: %v", err)
		}
		if e.ID == "" {
			e.ID = NewID("e")
		}
		d.Slides[i].Elements = insertAt(d.Slides[i].Elements, after, e)
	case "set_element":
		i, j := d.findElement(op.ID)
		if i < 0 {
			return fmt.Errorf("no element %s", op.ID)
		}
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
		if err := merge(d.Slides[i].Elements[j], fields, &e); err != nil {
			return err
		}
		if style != "" {
			if err := ApplyStyle(&e, style); err != nil {
				return err
			}
		}
		d.Slides[i].Elements[j] = e
	case "remove_element":
		i, j := d.findElement(op.ID)
		if i < 0 {
			return fmt.Errorf("no element %s", op.ID)
		}
		d.Slides[i].Elements = slices.Delete(d.Slides[i].Elements, j, j+1)
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
