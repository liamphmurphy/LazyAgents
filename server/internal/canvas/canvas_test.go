package canvas

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

const doc = `<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">
  <!-- keep -->
  <g fill='red'>
    <rect x="1" width="2"/>
    <text x="3">Stop &amp; go</text>
  </g>
</svg>
`

var vidOut = regexp.MustCompile(` data-vid="[a-z0-9]+"`)

func stamped(t *testing.T, body string) string {
	t.Helper()
	out, err := Stamp(body)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func vids(t *testing.T, body string) []string {
	t.Helper()
	all, err := scan(body)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range all[1:] {
		out = append(out, e.vid)
	}
	return out
}

func TestStampAddsIDsAndLeavesTheSourceAlone(t *testing.T) {
	out := stamped(t, doc)
	if got := vidOut.ReplaceAllString(out, ""); got != doc {
		t.Fatalf("source changed:\n%s", got)
	}
	ids := vids(t, out)
	if len(ids) != 3 || ids[0] == "" || ids[0] == ids[1] || ids[1] == ids[2] {
		t.Fatalf("ids = %v", ids)
	}
	if again := stamped(t, out); again != out {
		t.Fatalf("stamping twice changed it:\n%s", again)
	}
	if strings.Contains(strings.SplitN(out, "\n", 3)[1], "data-vid") {
		t.Fatal("root was stamped")
	}
}

func TestStampReplacesCopiedAndInvalidIDs(t *testing.T) {
	ids := vids(t, stamped(t, `<svg xmlns="http://www.w3.org/2000/svg"><a data-vid="abc"/><b data-vid='abc'/><c data-vid="NOT OK"/></svg>`))
	if ids[0] != "abc" || ids[1] == "abc" || ids[2] == "NOT OK" || !vidRe.MatchString(ids[1]) || !vidRe.MatchString(ids[2]) {
		t.Fatalf("ids = %v", ids)
	}
}

func TestStampIgnoresLookalikesInsideOtherValues(t *testing.T) {
	out := stamped(t, `<svg xmlns="http://www.w3.org/2000/svg"><a data-vid="ok" title=" data-vid='x'"/></svg>`)
	if !strings.Contains(out, `title=" data-vid='x'"`) || vids(t, out)[0] != "ok" {
		t.Fatal(out)
	}
}

func TestStampRejectsBadDocuments(t *testing.T) {
	for name, body := range map[string]string{
		"malformed":  `<svg xmlns="http://www.w3.org/2000/svg"><a></svg>`,
		"not svg":    `<div xmlns="http://www.w3.org/2000/svg"/>`,
		"no ns":      `<svg/>`,
		"doctype":    `<!DOCTYPE svg [<!ENTITY a "b">]><svg xmlns="http://www.w3.org/2000/svg"/>`,
		"two roots":  `<svg xmlns="http://www.w3.org/2000/svg"/><svg xmlns="http://www.w3.org/2000/svg"/>`,
		"stray text": `hello<svg xmlns="http://www.w3.org/2000/svg"/>`,
		"empty":      `   `,
	} {
		if _, err := Stamp(body); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func svgFixture() (string, string, string, string) {
	body := `<svg xmlns="http://www.w3.org/2000/svg"><g data-vid="grp"><rect data-vid="box" x="1"/><text data-vid="lbl">Old</text></g></svg>`
	return body, "grp", "box", "lbl"
}

func TestApplySVGOps(t *testing.T) {
	body, grp, box, lbl := svgFixture()
	for name, tc := range map[string]struct {
		op   Op
		want string
	}{
		"set":     {Op{Op: "set_attrs", VID: box, Attrs: map[string]string{"x": "9", "fill": "a<b"}}, `<rect fill="a&lt;b" data-vid="box" x="9"/>`},
		"unset":   {Op{Op: "set_attrs", VID: box, Attrs: map[string]string{"x": ""}}, `<rect data-vid="box"/>`},
		"text":    {Op{Op: "set_text", VID: lbl, Text: "A & B"}, `<text data-vid="lbl">A &amp; B</text>`},
		"remove":  {Op{Op: "remove", VID: box}, `<g data-vid="grp"><text`},
		"replace": {Op{Op: "replace", VID: box, Markup: `<circle r="4"/>`}, `<circle data-vid="box" r="4"/>`},
		"before":  {Op{Op: "insert", VID: box, Position: "before", Markup: `<line data-vid="new"/>`}, `<line data-vid="new"/><rect`},
		"after":   {Op{Op: "insert", VID: lbl, Position: "after", Markup: `<line data-vid="new"/>`}, `Old</text><line data-vid="new"/>`},
		"append":  {Op{Op: "insert", VID: grp, Position: "append", Markup: `<line data-vid="new"/>`}, `<line data-vid="new"/></g>`},
		"append0": {Op{Op: "insert", Position: "append", Markup: `<line data-vid="new"/>`}, `</g><line data-vid="new"/></svg>`},
	} {
		out, err := Apply(KindSVG, body, []Op{tc.op})
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: %s", name, out)
		}
	}
}

func TestApplySVGRefusals(t *testing.T) {
	body, _, box, lbl := svgFixture()
	for name, op := range map[string]Op{
		"unknown vid":   {Op: "remove", VID: "nope"},
		"root":          {Op: "remove", VID: ""},
		"own id":        {Op: "set_attrs", VID: box, Attrs: map[string]string{VIDAttr: "x"}},
		"bad attr":      {Op: "set_attrs", VID: box, Attrs: map[string]string{`x" onload="y`: "1"}},
		"text on empty": {Op: "set_text", VID: box, Text: "x"},
		"two elements":  {Op: "replace", VID: lbl, Markup: `<a/><b/>`},
		"bad markup":    {Op: "replace", VID: lbl, Markup: `<a>`},
		"escape parent": {Op: "replace", VID: lbl, Markup: `</g><a/><g>`},
		"bad position":  {Op: "insert", VID: lbl, Position: "under", Markup: `<a/>`},
		"unknown op":    {Op: "shrink", VID: lbl},
	} {
		if _, err := Apply(KindSVG, body, []Op{op}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestApplyKeepsIDsThroughASequence(t *testing.T) {
	body, _, box, lbl := svgFixture()
	out, err := Apply(KindSVG, body, []Op{
		{Op: "insert", VID: box, Position: "before", Markup: `<circle r="1"/>`},
		{Op: "set_text", VID: lbl, Text: "New"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := vids(t, out)
	if len(ids) != 4 || ids[0] != "grp" || ids[1] == "" || ids[2] != box || ids[3] != lbl {
		t.Fatalf("ids = %v in %s", ids, out)
	}
}

func TestApplyText(t *testing.T) {
	out, err := Apply(KindText, "one two three", []Op{{Find: "two", Replace: "2"}, {Find: " three"}})
	if err != nil || out != "one 2" {
		t.Fatalf("%q %v", out, err)
	}
	for name, op := range map[string]Op{"missing": {Find: "zzz"}, "ambiguous": {Find: "o"}, "empty": {}} {
		_, err := Apply(KindText, "one two", []Op{op})
		var v *apperr.ValidationError
		if !errors.As(err, &v) || v.Field != "ops" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := Apply(KindText, "x", nil); err == nil {
		t.Error("no ops accepted")
	}
}
