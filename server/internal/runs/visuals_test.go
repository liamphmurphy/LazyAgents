package runs

import (
	"strings"
	"testing"
)

const goodSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 50"><title>Box</title><defs><marker id="m"><path d="M0 0" fill="var(--viz-line)"/></marker></defs><rect fill="var(--viz-1-soft)" stroke="var(--viz-1)"/><path stroke="var(--viz-line)" fill="none" marker-end="url(#m)"/><use href="#m"/></svg>`

func TestCheckLessonDiagram(t *testing.T) {
	stepped := `<svg viewBox="0 0 9 9"><g data-step="1"/><g data-step="2-3"/></svg>`
	cases := []struct {
		name, src string
		steps     int
		want      string
	}{
		{"good", goodSVG, 0, ""},
		{"stepped", stepped, 3, ""},
		{"malformed", `<svg viewBox="0 0 1 1"><rect></svg>`, 0, "well-formed"},
		{"empty", ``, 0, "root element"},
		{"not svg", `<div viewBox="0"/>`, 0, "root element"},
		{"two roots", `<svg viewBox="0 0 1 1"/><svg viewBox="0 0 1 1"/>`, 0, "root element"},
		{"no viewBox", `<svg/>`, 0, "viewBox"},
		{"script", `<svg viewBox="0 0 1 1"><script/></svg>`, 0, "isn't allowed"},
		{"foreignObject", `<svg viewBox="0 0 1 1"><foreignObject/></svg>`, 0, "isn't allowed"},
		{"handler", `<svg viewBox="0 0 1 1" onload="x()"/>`, 0, "event handlers"},
		{"style element", `<svg viewBox="0 0 1 1"><style>rect{}</style></svg>`, 0, "<style>"},
		{"external href", `<svg viewBox="0 0 1 1"><use href="http://x/y#z"/></svg>`, 0, "inside itself"},
		{"external xlink", `<svg xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 1 1"><use xlink:href="http://x/y"/></svg>`, 0, "inside itself"},
		{"external url", `<svg viewBox="0 0 1 1"><rect fill="url(http://x/y)"/></svg>`, 0, "inside itself"},
		{"hex fill", `<svg viewBox="0 0 1 1"><rect fill="#fff"/></svg>`, 0, "palette color"},
		{"named fill", `<svg viewBox="0 0 1 1"><rect stroke="red"/></svg>`, 0, "palette color"},
		{"hex style", `<svg viewBox="0 0 1 1"><rect style="fill:#fff"/></svg>`, 0, "fixed color"},
		{"unknown token", `<svg viewBox="0 0 1 1"><rect fill="var(--viz-9)"/></svg>`, 0, "no palette color"},
		{"too big", `<svg viewBox="0 0 1 1">` + strings.Repeat(" ", visualMaxSource) + `</svg>`, 0, "over"},
		{"step without steps", stepped, 0, "steps list"},
		{"steps without marks", goodSVG, 2, "data-step on"},
		{"step out of range", `<svg viewBox="0 0 1 1"><g data-step="4"/></svg>`, 3, "outside"},
		{"range out of range", `<svg viewBox="0 0 1 1"><g data-step="2-9"/></svg>`, 3, "outside"},
		{"backward range", `<svg viewBox="0 0 1 1"><g data-step="3-2"/></svg>`, 3, "outside"},
		{"step format", `<svg viewBox="0 0 1 1"><g data-step="one"/></svg>`, 3, "write a step"},
	}
	for _, c := range cases {
		err := CheckLessonDiagram(c.src, c.steps)
		if c.want == "" && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

func TestCheckLessonChart(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"bar":         {`{"type":"bar","title":"T","categories":["a","b"],"y":{"unit":"%"},"series":[{"name":"s","values":[1,null]}]}`, ""},
		"scatter":     {`{"type":"scatter","series":[{"name":"s","points":[[1,2]]}]}`, ""},
		"not json":    {`{`, "valid spec"},
		"unknown key": {`{"type":"bar","colour":"red"}`, "valid spec"},
		"bad type":    {`{"type":"pie"}`, "type must be"},
		"no cats":     {`{"type":"line","series":[{"name":"s","values":[]}]}`, "categories"},
		"no series":   {`{"type":"bar","categories":["a"],"series":[]}`, "series"},
		"short":       {`{"type":"bar","categories":["a","b"],"series":[{"name":"s","values":[1]}]}`, "one value per category"},
		"all null":    {`{"type":"bar","categories":["a"],"series":[{"name":"s","values":[null]}]}`, "one value per category"},
		"unnamed":     {`{"type":"bar","categories":["a"],"series":[{"values":[1]},{"name":"b","values":[1]}]}`, "needs a name"},
		"no points":   {`{"type":"scatter","series":[{"name":"s","points":[]}]}`, "points"},
		"bad point":   {`{"type":"scatter","series":[{"name":"s","points":[["a",2]]}]}`, "valid spec"},
		"too scatter": {`{"type":"scatter","series":[{"name":"a","points":[[1,1]]},{"name":"b","points":[[1,1]]},{"name":"c","points":[[1,1]]},{"name":"d","points":[[1,1]]}]}`, "1 to 3"},
	}
	for name, c := range cases {
		err := CheckLessonChart(c.src)
		if c.want == "" && err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: got %v, want %q", name, err, c.want)
		}
	}
}
