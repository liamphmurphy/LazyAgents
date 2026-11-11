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
	if len(d.Slides) != 7 || d.Slides[2].Elements[2].Crop.L != 0.3 {
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
	if len(d.Slides) != 8 || d.Slides[0].ID != "s_section" || d.Slides[3].Layout != "title-body" || len(d.Slides[3].Elements) != 2 {
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

func TestTextStyleFields(t *testing.T) {
	body, err := Normalize(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	d, _ := Parse(body)
	p := d.Slides[6].Elements[1].Paragraphs[0]
	if d.Slides[6].ID != "s_text" || p.Align != "justify" || p.LineSpacing != 1.5 || !p.Runs[1].Strike || p.Runs[3].Highlight != "#FDE68A" || p.Runs[5].Highlight != "surface" {
		t.Fatalf("lost text styles: %+v", p)
	}
	// Single spacing is the default, so it isn't stored; spacing rounds like every number.
	for src, want := range map[string]float64{`"lineSpacing":1,`: 0, `"lineSpacing":1.234,`: 1.23} {
		out, err := Normalize(strings.Replace(fixture(t), `"lineSpacing":2,`, src, 1))
		if err != nil {
			t.Fatal(err)
		}
		d, _ := Parse(out)
		if got := d.Slides[6].Elements[2].Paragraphs[0].LineSpacing; got != want {
			t.Errorf("%s: lineSpacing %v, want %v", src, got, want)
		}
	}
	for name, mut := range map[string][2]string{
		"spacing too tight":   {`"lineSpacing":2,`, `"lineSpacing":0.5,`},
		"spacing too loose":   {`"lineSpacing":2,`, `"lineSpacing":3.5,`},
		"spacing negative":    {`"lineSpacing":2,`, `"lineSpacing":-1,`},
		"bad align":           {`"align":"justify"`, `"align":"full"`},
		"bad highlight":       {`"highlight":"#FDE68A"`, `"highlight":"yellow"`},
		"short hex highlight": {`"highlight":"#FDE68A"`, `"highlight":"#FDE"`},
		"strike as string":    {`"strike":true`, `"strike":"sngStrike"`},
	} {
		good := fixture(t)
		bad := strings.Replace(good, mut[0], mut[1], 1)
		if bad == good {
			t.Fatalf("%s: mutation didn't apply", name)
		}
		if _, err := Normalize(bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
