package canvas

import "testing"

func TestTextAnchorsCountUTF16(t *testing.T) {
	body := "😀 héllo world"
	a, err := newTextAnchor(body, 3, 8) // the emoji is two units
	if err != nil || a.Quote != "héllo" || a.Prefix != "😀 " {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := newTextAnchor(body, 1, 4); err == nil {
		t.Fatal("accepted a selection splitting the emoji")
	}
	for _, r := range [][2]int{{5, 5}, {8, 3}, {-1, 2}, {0, 99}} {
		if _, err := newTextAnchor(body, r[0], r[1]); err == nil {
			t.Fatalf("accepted %v", r)
		}
	}
}

func TestMoveTextAnchor(t *testing.T) {
	base := "Archive after 30 days. Delete after 90 days."
	a, err := newTextAnchor(base, 14, 21) // "30 days"
	if err != nil || a.Quote != "30 days" {
		t.Fatal(a, err)
	}
	moved, ok := moveTextAnchor("Intro added. "+base, a)
	if !ok || *moved.Start != 27 || moved.Quote != "30 days" {
		t.Fatalf("shifted = %+v %v", moved, ok)
	}
	if _, ok := moveTextAnchor("Archive after 60 weeks.", a); ok {
		t.Fatal("found a passage that was rewritten")
	}

	// The same words twice: context picks the right one.
	twice := "x days. Archive after 30 days. Then 30 days later."
	moved, ok = moveTextAnchor(twice, a)
	if !ok || twice[:utf16Index(twice, *moved.Start)] != "x days. Archive after " {
		t.Fatalf("twice = %+v %v", moved, ok)
	}

	// Recurring with none of its context: gone, not a guess.
	word, _ := newTextAnchor("alpha the omega", 6, 9)
	if _, ok := moveTextAnchor("the one and the other", word); ok {
		t.Fatal("guessed among occurrences with no matching context")
	}
	if moved, ok := moveTextAnchor("a single the here", word); !ok || moved.Quote != "the" {
		t.Fatalf("single occurrence = %+v %v", moved, ok)
	}
}

func utf16Index(s string, u16 int) int {
	i, _ := byteOffset(s, u16)
	return i
}

func TestSVGAnchorDescribesTheElement(t *testing.T) {
	body, _, box, lbl := svgFixture()
	a, err := svgAnchor(body, lbl, &Point{X: .5, Y: .5})
	if err != nil || a.Tag != "text" || a.Label != "Old" {
		t.Fatalf("%+v %v", a, err)
	}
	if a, err = svgAnchor(body, box, nil); err != nil || a.Tag != "rect" || a.Label != "" {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err = svgAnchor(body, "nope", nil); err == nil {
		t.Fatal("accepted an unknown element")
	}
	if _, err = svgAnchor(body, box, &Point{X: 2}); err == nil {
		t.Fatal("accepted a point off the drawing")
	}
}
