package runs

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// The limits on a lesson's visuals. Charts match web/lib/chat-chart.ts, which
// draws them; a step count matches MAX_STEPS in web/lib/chat-svg.ts.
const (
	visualMaxSource     = 20000
	visualMaxSteps      = 12
	chartMaxSeries      = 8
	chartMaxScatter     = 3
	chartMaxCategories  = 60
	chartMaxSeriesPoint = 500
)

var (
	vizTokenRe = regexp.MustCompile(`var\(\s*--viz-([a-z0-9-]+)\s*\)`)
	vizNameRe  = regexp.MustCompile(`^(surface|panel|ink|muted|line|[1-8](-soft)?)$`)
	stepRe     = regexp.MustCompile(`^(\d+)(?:-(\d+))?$`)
	hexColorRe = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	// Refs within the document start with #; anything else would reach outside it.
	externalURLRe = regexp.MustCompile(`(?i)url\(\s*['"]?[^#'"\s]`)
)

// colorAttrs take a color, so they must use the palette for dark mode to work.
var colorAttrs = map[string]bool{"fill": true, "stroke": true, "color": true, "stop-color": true, "flood-color": true, "lighting-color": true}

var bannedSVGElements = map[string]bool{"script": true, "foreignobject": true, "iframe": true, "image": true, "a": true, "animate": true, "animatetransform": true, "animatemotion": true, "set": true}

// CheckLessonDiagram refuses an SVG that wouldn't draw well in every theme, or
// that isn't self-contained. Lessons show diagrams through <img>, so this is
// about quality and limits: nothing inside could run anyway. Steps is the
// number of captions, 0 for a static diagram.
func CheckLessonDiagram(source string, steps int) error {
	if len(source) > visualMaxSource {
		return fmt.Errorf("the SVG is over %d characters: simplify it", visualMaxSource)
	}
	for _, m := range vizTokenRe.FindAllStringSubmatch(source, -1) {
		if !vizNameRe.MatchString(m[1]) {
			return fmt.Errorf("there's no palette color --viz-%s", m[1])
		}
	}
	if externalURLRe.MatchString(source) || strings.Contains(strings.ToLower(source), "@import") {
		return errors.New("the SVG can only refer to things inside itself")
	}
	d := xml.NewDecoder(strings.NewReader(source))
	depth, roots, stepped := 0, 0, 0
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("the SVG isn't well-formed: %v", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(t)) > 0 {
				return errors.New("the SVG has text outside its root element")
			}
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if depth == 0 {
				roots++
				if name != "svg" || roots > 1 {
					return errors.New("give exactly one <svg> root element")
				}
				if !hasAttr(t, "viewBox") {
					return errors.New("the <svg> needs a viewBox")
				}
			}
			depth++
			if bannedSVGElements[name] {
				return fmt.Errorf("<%s> isn't allowed in a diagram", t.Name.Local)
			}
			if name == "style" {
				return errors.New("<style> isn't allowed: use presentation attributes")
			}
			n, err := checkSVGAttrs(t, steps)
			if err != nil {
				return err
			}
			stepped += n
		}
	}
	if roots == 0 {
		return errors.New("give exactly one <svg> root element")
	}
	if steps > 0 && stepped == 0 {
		return errors.New("a step-through needs data-step on the elements each step reveals")
	}
	return nil
}

func hasAttr(el xml.StartElement, name string) bool {
	for _, a := range el.Attr {
		if a.Name.Local == name {
			return true
		}
	}
	return false
}

// checkSVGAttrs returns how many data-step attributes the element had.
func checkSVGAttrs(el xml.StartElement, steps int) (int, error) {
	marked := 0
	for _, a := range el.Attr {
		name := strings.ToLower(a.Name.Local)
		v := strings.TrimSpace(a.Value)
		switch {
		case strings.HasPrefix(name, "on"):
			return 0, fmt.Errorf("event handlers like %s aren't allowed", a.Name.Local)
		case name == "href" && !strings.HasPrefix(v, "#"):
			return 0, errors.New("the SVG can only refer to things inside itself")
		case colorAttrs[name]:
			if v != "none" && v != "transparent" && v != "currentColor" && !strings.HasPrefix(v, "url(#") && !vizTokenRe.MatchString(v) {
				return 0, fmt.Errorf("%s=%q: use a var(--viz-*) palette color so the diagram follows the theme", a.Name.Local, v)
			}
		case name == "style" && hexColorRe.MatchString(v):
			return 0, errors.New("a style attribute uses a fixed color: use a var(--viz-*) palette color")
		case name == "data-step":
			if err := checkStep(v, steps); err != nil {
				return 0, err
			}
			marked++
		}
	}
	return marked, nil
}

func checkStep(v string, steps int) error {
	if steps == 0 {
		return errors.New("data-step needs a steps list: add one caption per step, or remove it")
	}
	m := stepRe.FindStringSubmatch(v)
	if m == nil {
		return fmt.Errorf("data-step %q: write a step like 2, or a range like 2-3", v)
	}
	from, _ := strconv.Atoi(m[1])
	to := from
	if m[2] != "" {
		to, _ = strconv.Atoi(m[2])
	}
	if from < 1 || to < from || to > steps || from > steps {
		return fmt.Errorf("data-step %q is outside steps 1 to %d", v, steps)
	}
	return nil
}

// CheckLessonChart refuses a chart spec the renderer couldn't draw. Numbers go
// through as JSON, so NaN and Infinity can't get in.
func CheckLessonChart(source string) error {
	if len(source) > visualMaxSource {
		return fmt.Errorf("the chart is over %d characters: simplify it", visualMaxSource)
	}
	var c struct {
		Type       string   `json:"type"`
		Title      string   `json:"title"`
		Subtitle   string   `json:"subtitle"`
		Categories []string `json:"categories"`
		X          struct {
			Label string `json:"label"`
		} `json:"x"`
		Y struct {
			Label string `json:"label"`
			Unit  string `json:"unit"`
		} `json:"y"`
		Series []struct {
			Name   string       `json:"name"`
			Values []*float64   `json:"values"`
			Points [][2]float64 `json:"points"`
		} `json:"series"`
	}
	d := json.NewDecoder(strings.NewReader(source))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return fmt.Errorf("the chart isn't a valid spec: %v", err)
	}
	maxSeries := chartMaxSeries
	switch c.Type {
	case "scatter":
		maxSeries = chartMaxScatter
	case "bar", "line":
		if len(c.Categories) == 0 || len(c.Categories) > chartMaxCategories {
			return fmt.Errorf("a %s chart needs 1 to %d categories", c.Type, chartMaxCategories)
		}
	default:
		return errors.New("a chart's type must be bar, line or scatter")
	}
	if len(c.Series) == 0 || len(c.Series) > maxSeries {
		return fmt.Errorf("a %s chart needs 1 to %d series", c.Type, maxSeries)
	}
	for _, s := range c.Series {
		if strings.TrimSpace(s.Name) == "" && len(c.Series) > 1 {
			return errors.New("each series needs a name")
		}
		if c.Type == "scatter" {
			if len(s.Points) == 0 || len(s.Points) > chartMaxSeriesPoint {
				return fmt.Errorf("series %q needs 1 to %d points", s.Name, chartMaxSeriesPoint)
			}
			continue
		}
		numbers := 0
		for _, v := range s.Values {
			if v != nil {
				numbers++
			}
		}
		if len(s.Values) != len(c.Categories) || numbers == 0 {
			return fmt.Errorf("series %q needs one value per category", s.Name)
		}
	}
	return nil
}
