package deck

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// fixture is the shared acceptance deck, minus the PoC's local file names.
func fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../../docs/slides/fixtures/acceptance-deck.json")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, a := range m["assets"].(map[string]any) {
		delete(a.(map[string]any), "file")
	}
	out, _ := json.Marshal(m)
	return string(out)
}

func TestNormalizeAcceptanceDeck(t *testing.T) {
	body, err := Normalize(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	again, err := Normalize(body)
	if err != nil || again != body {
		t.Fatalf("normalize isn't idempotent: %v", err)
	}
	d, _ := Parse(body)
	if len(d.Slides) != 6 || d.Slides[2].Elements[2].Crop.L != 0.3 {
		t.Fatalf("lost content: %+v", d.Slides[2].Elements[2])
	}
}

func TestNormalizeRefuses(t *testing.T) {
	good := fixture(t)
	for name, mut := range map[string]func(string) string{
		"unknown field":  func(s string) string { return strings.Replace(s, `"schema":1`, `"schema":1,"bogus":true`, 1) },
		"bad colour":     func(s string) string { return strings.Replace(s, `"color":"accent"`, `"color":"blue"`, 1) },
		"missing asset":  func(s string) string { return strings.Replace(s, `"asset":"a_landscape"`, `"asset":"a_missing"`, 1) },
		"duplicate id":   func(s string) string { return strings.Replace(s, `"id":"e_t2"`, `"id":"e_t1"`, 1) },
		"newline in run": func(s string) string { return strings.Replace(s, `Quarterly review`, `Quarterly\nreview`, 1) },
		"wrong schema":   func(s string) string { return strings.Replace(s, `"schema":1`, `"schema":2`, 1) },
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

func TestApplyOps(t *testing.T) {
	body, _ := Normalize(fixture(t))
	out, err := Apply(body, []Op{
		{Op: "insert_slide", After: "s_bullets", Value: map[string]any{"layout": "title-body"}},
		{Op: "set_element", ID: "e_t1", Value: map[string]any{"paragraphs": []any{map[string]any{"runs": []any{map[string]any{"text": "Q4 review", "size": 112, "bold": true}}}}}},
		{Op: "move_slide", ID: "s_section"},
		{Op: "remove_element", ID: "e_s5"},
		{Op: "set_slide", ID: "s_image", Value: map[string]any{"layout": "section"}},
		{Op: "set_theme", Value: map[string]any{"preset": "dark"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := Parse(out)
	if len(d.Slides) != 7 || d.Slides[0].ID != "s_section" || d.Slides[3].Layout != "title-body" || len(d.Slides[3].Elements) != 2 {
		t.Fatalf("unexpected slides: %+v", d.Slides)
	}
	if d.Slides[1].Elements[0].Paragraphs[0].Runs[0].Text != "Q4 review" || d.Theme.Name != "Dark" {
		t.Fatal("set_element or set_theme lost")
	}
	if _, err := Apply(body, []Op{{Op: "set_element", ID: "e_t1", Value: map[string]any{"type": "image"}}}); err == nil {
		t.Fatal("type change accepted")
	}
	if _, err := Apply(body, []Op{{Op: "remove_slide", ID: "s_nope"}}); err == nil {
		t.Fatal("unknown slide accepted")
	}
}

func TestNewDeckIsValid(t *testing.T) {
	for name := range Themes {
		d := New("Hello", name)
		for _, l := range Layouts {
			d.Slides = append(d.Slides, NewSlide(l, ""))
		}
		if err := d.Validate(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestPatchesCannotExceedTheBodyCap(t *testing.T) {
	body, _ := Normalize(fixture(t))
	notes := strings.Repeat("n", MaxNotesRunes)
	var ops []Op
	for i := 0; i < 80; i++ {
		ops = append(ops, Op{Op: "insert_slide", Value: map[string]any{"layout": "blank", "notes": notes}})
	}
	if _, err := Apply(body, ops); err == nil || !strings.Contains(err.Error(), "at most 512 KB") {
		t.Fatalf("oversized patch accepted or wrong error: %v", err)
	}
}

func TestFieldsForOtherTypesAreRefused(t *testing.T) {
	good := fixture(t)
	for name, mut := range map[string]func(string) string{
		"image with shape": func(s string) string {
			return strings.Replace(s, `"asset":"a_landscape"`, `"asset":"a_landscape","shape":"rect"`, 1)
		},
		"line with crop": func(s string) string {
			return strings.Replace(s, `"endArrow":true`, `"endArrow":true,"crop":{"l":0.1,"t":0,"r":0,"b":0}`, 1)
		},
		"long asset alt": func(s string) string {
			return strings.Replace(s, `"alt":"Stylised`, `"alt":"`+strings.Repeat("x", 600)+`Stylised`, 1)
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

// styled is a one-slide deck whose single element (or background) is el.
func styled(t *testing.T, background, el string) string {
	t.Helper()
	d := New("", "light")
	d.Assets = map[string]Asset{"a_pic": {Mime: "image/png", W: 10, H: 10}}
	body, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(body), &m)
	s := map[string]any{"id": "s_x", "layout": "blank", "elements": []any{}}
	if background != "" {
		var bg any
		if err := json.Unmarshal([]byte(background), &bg); err != nil {
			t.Fatal(err)
		}
		s["background"] = bg
	}
	if el != "" {
		var e any
		if err := json.Unmarshal([]byte(el), &e); err != nil {
			t.Fatal(err)
		}
		s["elements"] = []any{e}
	}
	m["slides"] = []any{s}
	out, _ := json.Marshal(m)
	return string(out)
}

const frame = `"frame":{"x":0,"y":0,"w":100,"h":100}`

func TestStyleFieldsNormalize(t *testing.T) {
	for name, c := range map[string]struct{ bg, el, want string }{
		"v1 string fill stays a string": {"", `{"id":"e_a","type":"shape","shape":"rect",` + frame + `,"fill":"accent"}`, `"fill":"accent"`},
		"gradient fill": {"", `{"id":"e_a","type":"shape","shape":"rect",` + frame + `,"fill":{"type":"linear","angle":45.004,"stops":["accent","#00ff00"]},"fillOpacity":0.5}`,
			`"fill":{"type":"linear","angle":45,"stops":["accent","#00ff00"]},"fillOpacity":0.5`},
		"angle 360 is 0":       {"", `{"id":"e_a","type":"shape","shape":"rect",` + frame + `,"fill":{"type":"linear","angle":360,"stops":["accent","muted"]}}`, `"angle":0,`},
		"opaque is left out":   {"", `{"id":"e_a","type":"shape","shape":"rect",` + frame + `,"fill":"accent","fillOpacity":1,"stroke":"text","strokeWidth":4,"strokeOpacity":1,"dash":"solid"}`, `"fill":"accent","stroke":"text","strokeWidth":4}`},
		"transparent is kept":  {"", `{"id":"e_a","type":"shape","shape":"ellipse",` + frame + `,"fill":"accent","fillOpacity":0}`, `"fillOpacity":0`},
		"dashed line":          {"", `{"id":"e_a","type":"line",` + frame + `,"stroke":"text","strokeWidth":4,"strokeOpacity":0.25,"dash":"dashDot"}`, `"strokeOpacity":0.25,"dash":"dashDot"`},
		"image opacity":        {"", `{"id":"e_a","type":"image",` + frame + `,"asset":"a_pic","opacity":0.4}`, `"opacity":0.4`},
		"gradient background":  {`{"color":{"type":"linear","angle":90,"stops":["background","surface"]}}`, "", `"background":{"color":{"type":"linear","angle":90,"stops":["background","surface"]}}`},
		"v1 colour background": {`{"color":"surface"}`, "", `"background":{"color":"surface"}`},
	} {
		out, err := Normalize(styled(t, c.bg, c.el))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: want %s in %s", name, c.want, out)
		}
		if again, err := Normalize(out); err != nil || again != out {
			t.Errorf("%s: not idempotent: %v", name, err)
		}
	}
}

func TestStyleFieldsRefused(t *testing.T) {
	shape := func(extra string) string {
		return `{"id":"e_a","type":"shape","shape":"rect",` + frame + `,` + extra + `}`
	}
	for name, c := range map[string]struct{ bg, el string }{
		"opacity above 1":        {"", shape(`"fillOpacity":1.5`)},
		"negative opacity":       {"", shape(`"strokeOpacity":-0.1`)},
		"unknown dash":           {"", shape(`"dash":"wavy"`)},
		"radial gradient":        {"", shape(`"fill":{"type":"radial","angle":0,"stops":["accent","muted"]}`)},
		"three stops":            {"", shape(`"fill":{"type":"linear","angle":0,"stops":["accent","muted","text"]}`)},
		"one stop":               {"", shape(`"fill":{"type":"linear","angle":0,"stops":["accent"]}`)},
		"bad stop colour":        {"", shape(`"fill":{"type":"linear","angle":0,"stops":["accent","blue"]}`)},
		"angle out of range":     {"", shape(`"fill":{"type":"linear","angle":400,"stops":["accent","muted"]}`)},
		"unknown gradient field": {"", shape(`"fill":{"type":"linear","angle":0,"stops":["accent","muted"],"spread":1}`)},
		"fill is a number":       {"", shape(`"fill":3`)},
		"opacity on a shape":     {"", shape(`"opacity":0.5`)},
		"fillOpacity on a line":  {"", `{"id":"e_a","type":"line",` + frame + `,"stroke":"text","strokeWidth":4,"fillOpacity":0.5}`},
		"dash on an image":       {"", `{"id":"e_a","type":"image",` + frame + `,"asset":"a_pic","dash":"dot"}`},
		"opacity on text":        {"", `{"id":"e_a","type":"text",` + frame + `,"paragraphs":[{"runs":[{"text":"x","size":40}]}],"fillOpacity":0.5}`},
		"bad background stop":    {`{"color":{"type":"linear","angle":0,"stops":["#12345","muted"]}}`, ""},
	} {
		if _, err := Normalize(styled(t, c.bg, c.el)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
