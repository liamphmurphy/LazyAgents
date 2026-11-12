// Package design is the Designs body schema (docs/design/ADR-designs.md §3.1):
// validation, normalisation, ids and the ops a persona edits a design with. A
// design is the JSON body of a canvas of kind "design"; this package knows
// nothing about storage. web/lib/designs/schema.ts mirrors Normalize, and both
// run the shared invalid bodies in web/lib/designs/fixtures/invalid.
package design

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

// Schema is the version this package reads and writes.
const Schema = 1

// Caps (ADR §3.1). MaxBody is the only body cap for kind design; canvas
// applies it through maxBodyFor.
const (
	MaxBody      = 2 * 1024 * 1024
	MaxNodes     = 10000
	MaxDepth     = 32
	MaxTextChars = 5000
	MaxPages     = 50
	MaxLinks     = 2000
	MaxTokenSets = 8
	MaxAssets    = 200
	MaxTokens    = 400
)

var (
	reNodeID    = regexp.MustCompile(`^n_[a-z0-9_]{1,40}$`)
	rePageID    = regexp.MustCompile(`^p_[a-z0-9_]{1,40}$`)
	reLinkID    = regexp.MustCompile(`^k_[a-z0-9_]{1,40}$`)
	reAssetID   = regexp.MustCompile(`^a_[a-z0-9_]{1,40}$`)
	reKey       = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	reTokenName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	reColor     = regexp.MustCompile(`^(#[0-9A-F]{6}([0-9A-F]{2})?|\$[a-z0-9][a-z0-9-]{0,63})$`)
	reHex       = regexp.MustCompile(`^#[0-9A-F]{6}([0-9A-F]{2})?$`)
	reAnyHex    = regexp.MustCompile(`^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`)
	reDimension = regexp.MustCompile(`^-?\d+(\.\d+)?px$`)
	reHash      = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	reDateTime  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`)
	rePreset    = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	reTicket    = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,15}-[0-9]{1,7}$`)
	rePathRef   = regexp.MustCompile(`^[^/\s][^\s]{0,299}$`)
	rePathD     = regexp.MustCompile(`^[MLCQZmlcqz0-9eE.,\s-]+$`)
)

// NodeTypes are the node types of schema 1.
var NodeTypes = []string{"frame", "component", "group", "section", "shape", "path", "text", "image", "instance", "sticky", "connector"}

// Containers may hold children.
var Containers = map[string]bool{"frame": true, "component": true, "group": true, "section": true}

// ShapePresets are rect, roundRect, ellipse and the Slides presets.
var ShapePresets = []string{"rect", "roundRect", "ellipse", "triangle", "rtTriangle", "diamond", "pentagon", "hexagon", "octagon", "rightArrow", "leftArrow", "upArrow", "downArrow", "chevron", "homePlate", "wedgeRectCallout", "wedgeRoundRectCallout", "star5", "plus", "donut", "heart"}

// Doc is a parsed design body: generic JSON, so unknown fields are seen and
// refused rather than dropped.
type Doc = map[string]any

func invalid(format string, args ...any) error { return apperr.Invalid("body", format, args...) }

// Parse decodes a body without validating it.
func Parse(body string) (Doc, error) {
	var d Doc
	dec := json.NewDecoder(strings.NewReader(body))
	if err := dec.Decode(&d); err != nil {
		return nil, invalid("isn't valid JSON: %v", err)
	}
	if dec.More() {
		return nil, invalid("has data after the design")
	}
	if d == nil {
		return nil, invalid("must be a JSON object")
	}
	return d, nil
}

// Normalize parses, normalises and validates a body and returns its canonical
// JSON: numbers rounded to 0.01, hex upper-cased, bold as weight 700, default
// values dropped, keys sorted, no whitespace.
func Normalize(body string) (string, error) {
	if len(body) > MaxBody {
		return "", invalid("is %d KB; a design is at most %d KB", len(body)/1024, MaxBody/1024)
	}
	d, err := Parse(body)
	if err != nil {
		return "", err
	}
	return Finish(d)
}

// Finish normalises and validates a parsed design in place and encodes it.
func Finish(d Doc) (string, error) {
	if s, ok := d["schema"].(float64); ok && s > Schema {
		return "", invalid("schema %v is newer than this server knows (%d)", s, Schema)
	}
	normalizeDoc(d)
	if err := Validate(d); err != nil {
		return "", err
	}
	return Encode(d)
}

// Encode writes canonical JSON (sorted keys, no HTML escaping) and enforces MaxBody.
func Encode(d Doc) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(d); err != nil {
		return "", invalid("couldn't be encoded: %v", err)
	}
	out := strings.TrimSuffix(buf.String(), "\n")
	if len(out) > MaxBody {
		return "", invalid("is %d KB; a design is at most %d KB", len(out)/1024, MaxBody/1024)
	}
	return out, nil
}

// ---- normalisation ----

// round matches JavaScript's Math.round(x*100)/100 so both sides agree.
func round(x float64) float64 {
	r := math.Floor(x*100+0.5) / 100
	if r == 0 {
		return 0 // no -0
	}
	return r
}

func roundAll(v any) any {
	switch t := v.(type) {
	case float64:
		return round(t)
	case map[string]any:
		for k, x := range t {
			t[k] = roundAll(x)
		}
	case []any:
		for i, x := range t {
			t[i] = roundAll(x)
		}
	}
	return v
}

func upperHex(v any) any {
	if s, ok := v.(string); ok && reAnyHex.MatchString(s) {
		return strings.ToUpper(s)
	}
	return v
}

func normPaint(m map[string]any, k string) {
	switch p := m[k].(type) {
	case string:
		m[k] = upperHex(p)
	case map[string]any:
		if stops, ok := p["stops"].([]any); ok {
			for i, s := range stops {
				stops[i] = upperHex(s)
			}
		}
	}
}

// dropDefault removes k when it holds its default value.
func dropDefault(m map[string]any, k string, def any) {
	if v, ok := m[k]; ok && v == def {
		delete(m, k)
	}
}

var nodeDefaults = map[string]any{"rotation": 0.0, "opacity": 1.0, "visible": true, "locked": false, "absolute": false}

func normParagraphs(ps any) {
	list, _ := ps.([]any)
	for _, p := range list {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		dropDefault(pm, "level", 0.0)
		dropDefault(pm, "spaceBefore", 0.0)
		dropDefault(pm, "lineSpacing", 1.0)
		runs, _ := pm["runs"].([]any)
		for _, r := range runs {
			rm, ok := r.(map[string]any)
			if !ok {
				continue
			}
			if b, ok := rm["bold"].(bool); ok {
				delete(rm, "bold")
				if b {
					rm["weight"] = 700.0
				}
			}
			dropDefault(rm, "weight", 400.0)
			for _, k := range []string{"italic", "underline", "strike"} {
				dropDefault(rm, k, false)
			}
			for _, k := range []string{"color", "highlight"} {
				if v, ok := rm[k]; ok {
					rm[k] = upperHex(v)
				}
			}
		}
	}
}

func normNode(v any) {
	n, ok := v.(map[string]any)
	if !ok {
		return
	}
	for k, def := range nodeDefaults {
		dropDefault(n, k, def)
	}
	normPaint(n, "fill")
	if s, ok := n["stroke"]; ok {
		n["stroke"] = upperHex(s)
	}
	normParagraphs(n["paragraphs"])
	if tb, ok := n["text"].(map[string]any); ok {
		normParagraphs(tb["paragraphs"])
	}
	if ov, ok := n["overrides"].(map[string]any); ok {
		for _, o := range ov {
			if om, ok := o.(map[string]any); ok {
				normPaint(om, "fill")
			}
		}
	}
	if cs, ok := n["children"].([]any); ok {
		for _, c := range cs {
			normNode(c)
		}
	}
}

func normalizeDoc(d Doc) {
	roundAll(d)
	if pages, ok := d["pages"].([]any); ok {
		for _, p := range pages {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if bg, ok := pm["background"]; ok {
				pm["background"] = upperHex(bg)
			}
			if cs, ok := pm["children"].([]any); ok {
				for _, c := range cs {
					normNode(c)
				}
			}
		}
	}
	if sets, ok := d["token_sets"].(map[string]any); ok {
		for _, s := range sets {
			sm, _ := s.(map[string]any)
			modes, _ := sm["modes"].(map[string]any)
			for _, m := range modes {
				toks, _ := m.(map[string]any)
				for _, t := range toks {
					if tm, ok := t.(map[string]any); ok && tm["kind"] == "color" {
						tm["value"] = upperHex(tm["value"])
					}
				}
			}
		}
	}
}

// ---- structural checks (the JSON Schema, by hand) ----

type checker func(v any, path string) error

func at(path, format string, args ...any) error {
	if path == "" {
		path = "design"
	}
	return invalid("%s: %s", path, fmt.Sprintf(format, args...))
}

func join(path, k string) string {
	if path == "" {
		return k
	}
	return path + "." + k
}

func str(re *regexp.Regexp, min, max int) checker {
	return func(v any, path string) error {
		s, ok := v.(string)
		if !ok {
			return at(path, "must be a string")
		}
		n := utf8.RuneCountInString(s)
		if n < min || (max > 0 && n > max) {
			if min > 0 {
				return at(path, "must be %d to %d characters", min, max)
			}
			return at(path, "must be at most %d characters", max)
		}
		if re != nil && !re.MatchString(s) {
			return at(path, "%q isn't in the expected form", s)
		}
		return nil
	}
}

func num(min, max float64) checker {
	return func(v any, path string) error {
		f, ok := v.(float64)
		if !ok {
			return at(path, "must be a number")
		}
		if f < min || f > max {
			return at(path, "must be between %v and %v", min, max)
		}
		return nil
	}
}

func integer(min, max float64) checker {
	return func(v any, path string) error {
		f, ok := v.(float64)
		if !ok || f != math.Trunc(f) {
			return at(path, "must be a whole number")
		}
		if f < min || f > max {
			return at(path, "must be between %v and %v", min, max)
		}
		return nil
	}
}

func enum(vals ...any) checker {
	return func(v any, path string) error {
		for _, x := range vals {
			if v == x {
				return nil
			}
		}
		parts := make([]string, len(vals))
		for i, x := range vals {
			parts[i] = fmt.Sprint(x)
		}
		return at(path, "must be one of %s", strings.Join(parts, ", "))
	}
}

func strEnum(vals ...string) checker {
	a := make([]any, len(vals))
	for i, s := range vals {
		a[i] = s
	}
	return enum(a...)
}

func boolean(v any, path string) error {
	if _, ok := v.(bool); !ok {
		return at(path, "must be true or false")
	}
	return nil
}

func arr(item checker, min, max int) checker {
	return func(v any, path string) error {
		a, ok := v.([]any)
		if !ok {
			return at(path, "must be a list")
		}
		if len(a) < min || (max >= 0 && len(a) > max) {
			if max < 0 {
				return at(path, "must have at least %d items", min)
			}
			return at(path, "must have %d to %d items", min, max)
		}
		for i, x := range a {
			if err := item(x, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	}
}

func uniqueStrings(item checker, max int) checker {
	inner := arr(item, 0, max)
	return func(v any, path string) error {
		if err := inner(v, path); err != nil {
			return err
		}
		seen := map[any]bool{}
		for _, x := range v.([]any) {
			if seen[x] {
				return at(path, "lists %v twice", x)
			}
			seen[x] = true
		}
		return nil
	}
}

type fields map[string]checker

// object checks an object against its known fields; anything else is refused.
func object(fs fields, required ...string) checker {
	return func(v any, path string) error {
		m, ok := v.(map[string]any)
		if !ok {
			return at(path, "must be an object")
		}
		return checkFields(m, path, fs, required)
	}
}

func checkFields(m map[string]any, path string, fs fields, required []string) error {
	for _, k := range required {
		if _, ok := m[k]; !ok {
			return at(path, "%s is required", k)
		}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		c, ok := fs[k]
		if !ok {
			return at(path, "unknown field %s", k)
		}
		if err := c(m[k], join(path, k)); err != nil {
			return err
		}
	}
	return nil
}

func mapOf(key *regexp.Regexp, val checker, max int) checker {
	return func(v any, path string) error {
		m, ok := v.(map[string]any)
		if !ok {
			return at(path, "must be an object")
		}
		if len(m) > max {
			return at(path, "has %d entries; at most %d", len(m), max)
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if !key.MatchString(k) {
				return at(path, "%q isn't a valid key", k)
			}
			if err := val(m[k], join(path, k)); err != nil {
				return err
			}
		}
		return nil
	}
}

var (
	cColor = str(reColor, 0, 0)
	cPaint = func(v any, path string) error {
		if _, ok := v.(string); ok {
			return cColor(v, path)
		}
		return object(fields{"angle": num(0, 360), "stops": arr(cColor, 2, 2)}, "angle", "stops")(v, path)
	}
	cNum   = num(-1000000, 1000000)
	cSize  = num(0, 100000)
	cMode  = strEnum("dark", "light")
	cDash  = strEnum("solid", "dash", "dot", "dashDot", "longDash")
	cToken = func(v any, path string) error {
		m, ok := v.(map[string]any)
		if !ok {
			return at(path, "must be an object")
		}
		switch m["kind"] {
		case "color":
			return checkFields(m, path, fields{"kind": enum("color"), "value": str(reHex, 0, 0)}, []string{"kind", "value"})
		case "dimension":
			return checkFields(m, path, fields{"kind": enum("dimension"), "value": str(reDimension, 0, 0)}, []string{"kind", "value"})
		}
		return at(path, "kind must be color or dimension")
	}
	cTokens   = mapOf(reTokenName, cToken, MaxTokens)
	cTokenSet = object(fields{
		"name":   str(nil, 1, 80),
		"source": func(v any, path string) error { return nullable(str(nil, 0, 200))(v, path) },
		"modes":  object(fields{"dark": cTokens, "light": cTokens}, "dark"),
	}, "name", "modes")
	cAsset = object(fields{
		"mime": strEnum("image/png", "image/jpeg"),
		"w":    integer(1, 8192), "h": integer(1, 8192),
		"alt":  str(nil, 0, 500),
		"file": str(nil, 0, 0),
	}, "mime", "w", "h")
	cLink = object(fields{
		"id": str(reLinkID, 0, 0), "from": str(reNodeID, 0, 0), "to": str(reNodeID, 0, 0),
		"trigger":     strEnum("click", "hover", "after-delay"),
		"delay_ms":    integer(0, 60000),
		"transition":  strEnum("instant", "dissolve", "slide-left", "slide-right", "slide-up", "slide-down", "smart-animate"),
		"duration_ms": integer(0, 5000),
	}, "id", "from", "to", "trigger", "transition")
	cRun = object(fields{
		"text": str(nil, 0, MaxTextChars), "size": num(4, 400),
		"weight": enum(400.0, 500.0, 600.0, 700.0),
		"bold":   boolean, "italic": boolean, "underline": boolean, "strike": boolean,
		"color": cColor, "highlight": cColor,
	}, "text", "size")
	cParagraph = object(fields{
		"runs":        arr(cRun, 1, -1),
		"align":       strEnum("left", "center", "right", "justify"),
		"bullet":      strEnum("disc", "number"),
		"level":       integer(0, 4),
		"spaceBefore": num(0, 1000),
		"lineSpacing": num(0.8, 3),
	}, "runs")
	cParagraphs = arr(cParagraph, 1, -1)
	cValign     = strEnum("top", "middle", "bottom")
	cTextBox    = object(fields{"paragraphs": cParagraphs, "valign": cValign}, "paragraphs")
	cEndpoint   = object(fields{"node": str(reNodeID, 0, 0), "anchor": strEnum("auto", "top", "right", "bottom", "left")}, "node")
	cOverride   = func(v any, path string) error {
		if m, ok := v.(map[string]any); ok && len(m) == 0 {
			return at(path, "must set text, fill or visible")
		}
		return object(fields{"text": str(nil, 0, MaxTextChars), "fill": cPaint, "visible": boolean})(v, path)
	}
	cAutoLayout = object(fields{
		"direction": strEnum("horizontal", "vertical"),
		"gap":       num(0, 10000),
		"padding":   arr(num(0, 10000), 4, 4),
		"align":     strEnum("start", "center", "end"),
		"justify":   strEnum("start", "center", "end", "space-between"),
	}, "direction")
	cSizing      = object(fields{"w": strEnum("fixed", "hug", "fill"), "h": strEnum("fixed", "hug", "fill")})
	cConstraints = object(fields{"h": strEnum("left", "right", "left-right", "center", "scale"), "v": strEnum("top", "bottom", "top-bottom", "center", "scale")})
	cCrop        = object(fields{"l": num(0, 0.99), "t": num(0, 0.99), "r": num(0, 0.99), "b": num(0, 0.99)}, "l", "t", "r", "b")
	cNodeID      = str(reNodeID, 0, 0)
)

func nullable(c checker) checker {
	return func(v any, path string) error {
		if v == nil {
			return nil
		}
		return c(v, path)
	}
}

func merge(sets ...fields) fields {
	out := fields{}
	for _, s := range sets {
		for k, v := range s {
			out[k] = v
		}
	}
	return out
}

var (
	boxFields = fields{
		"id": cNodeID, "type": func(any, string) error { return nil }, "name": str(nil, 0, 100),
		"x": cNum, "y": cNum, "w": cSize, "h": cSize,
		"rotation": num(-360, 360), "opacity": num(0, 1),
		"visible": boolean, "locked": boolean, "sizing": cSizing, "absolute": boolean, "constraints": cConstraints,
	}
	boxRequired    = []string{"id", "type", "x", "y", "w", "h"}
	styledFields   = fields{"fill": cPaint, "stroke": cColor, "stroke_width": num(0, 1000), "dash": cDash}
	childrenFields = fields{"children": func(v any, path string) error { return arr(cNode, 0, -1)(v, path) }}
	nodeFields     map[string]fields
)

func init() {
	container := merge(childrenFields, fields{
		"radius": num(0, 10000), "clip": boolean, "mode": cMode, "auto_layout": cAutoLayout,
		"preset":         str(rePreset, 0, 0),
		"linked_tickets": uniqueStrings(str(reTicket, 0, 0), 20),
		"linked_paths":   uniqueStrings(str(rePathRef, 0, 0), 20),
	})
	frame := merge(boxFields, styledFields, container)
	nodeFields = map[string]fields{
		"frame":     frame,
		"component": frame,
		"group":     merge(boxFields, fields{"children": func(v any, path string) error { return arr(cNode, 1, -1)(v, path) }}),
		"section":   merge(boxFields, childrenFields, fields{"fill": cColor}),
		"shape": merge(boxFields, styledFields, fields{
			"shape": strEnum(ShapePresets...), "radius": num(0, 10000), "text": cTextBox,
		}),
		"path": merge(boxFields, styledFields, fields{
			"d": str(rePathD, 3, 20000), "cap": strEnum("butt", "round", "square"), "join": strEnum("miter", "round", "bevel"),
		}),
		"text": merge(boxFields, fields{"paragraphs": cParagraphs, "valign": cValign}),
		"image": merge(boxFields, fields{
			"asset": str(reAssetID, 0, 0), "fit": strEnum("cover", "contain", "fill"), "crop": cCrop,
			"radius": num(0, 10000), "alt": str(nil, 0, 500),
		}),
		"instance": merge(boxFields, fields{"component": cNodeID, "overrides": mapOf(reNodeID, cOverride, 200)}),
		"sticky": merge(boxFields, fields{
			"text": str(nil, 0, 2000), "color": strEnum("yellow", "pink", "green", "blue", "violet", "grey"), "author": str(nil, 0, 80),
		}),
		"connector": {
			"id": cNodeID, "type": func(any, string) error { return nil }, "name": str(nil, 0, 100),
			"from": cEndpoint, "to": cEndpoint, "route": strEnum("orthogonal", "straight"), "label": str(nil, 0, 200),
			"stroke": cColor, "stroke_width": num(0.5, 50), "dash": cDash,
			"start_arrow": strEnum("none", "arrow", "dot"), "end_arrow": strEnum("none", "arrow", "dot"),
			"visible": boolean,
			"points":  arr(arr(cNum, 2, 2), 0, 64),
		},
	}
}

var nodeRequired = map[string][]string{
	"frame": {"children"}, "component": {"children"}, "group": {"children"}, "section": {"children"},
	"shape": {"shape"}, "path": {"d"}, "text": {"paragraphs"}, "image": {"asset"}, "instance": {"component"}, "sticky": {"text"},
}

func cNode(v any, path string) error {
	m, ok := v.(map[string]any)
	if !ok {
		return at(path, "must be an object")
	}
	t, _ := m["type"].(string)
	fs, ok := nodeFields[t]
	if !ok {
		return at(join(path, "type"), "must be one of %s", strings.Join(NodeTypes, ", "))
	}
	req := append(append([]string{}, boxRequired...), nodeRequired[t]...)
	if t == "connector" {
		req = []string{"id", "type", "from", "to"}
	}
	return checkFields(m, path, fs, req)
}

var cPage = object(fields{
	"id": str(rePageID, 0, 0), "name": str(nil, 1, 100), "background": cColor,
	"children": func(v any, path string) error { return arr(cNode, 0, -1)(v, path) },
}, "id", "name", "children")

var cDesign = object(fields{
	"schema": enum(1.0),
	"mode":   cMode,
	"tokens_ref": object(fields{
		"set": str(reKey, 0, 0), "source": str(nil, 0, 200), "hash": str(reHash, 0, 0), "synced_at": str(reDateTime, 0, 0),
	}, "set", "hash", "synced_at"),
	"token_sets": mapOf(reKey, cTokenSet, MaxTokenSets),
	"assets":     mapOf(reAssetID, cAsset, MaxAssets),
	"pages":      arr(cPage, 1, MaxPages),
	"links":      arr(cLink, 0, MaxLinks),
}, "schema", "mode", "pages", "assets", "links", "token_sets")

// ---- semantic checks (ADR §3.1 rules JSON Schema can't express) ----

type place struct {
	n      map[string]any
	parent map[string]any // nil at top level
	page   map[string]any
	depth  int
}

// Walk visits every node in paint order, parents first.
func Walk(d Doc, fn func(n, parent, page map[string]any, depth int)) {
	var visit func(n, parent, page map[string]any, depth int)
	visit = func(n, parent, page map[string]any, depth int) {
		fn(n, parent, page, depth)
		for _, c := range children(n) {
			visit(c, n, page, depth+1)
		}
	}
	for _, p := range pages(d) {
		for _, n := range children(p) {
			visit(n, nil, p, 1)
		}
	}
}

func pages(d Doc) []map[string]any {
	list, _ := d["pages"].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		if m, ok := p.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func children(n map[string]any) []map[string]any {
	list, _ := n["children"].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		if m, ok := c.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func links(d Doc) []map[string]any {
	list, _ := d["links"].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, l := range list {
		if m, ok := l.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func paragraphsOf(n map[string]any) []any {
	if ps, ok := n["paragraphs"].([]any); ok {
		return ps
	}
	if tb, ok := n["text"].(map[string]any); ok {
		ps, _ := tb["paragraphs"].([]any)
		return ps
	}
	return nil
}

func eachRun(n map[string]any, fn func(r map[string]any)) {
	for _, p := range paragraphsOf(n) {
		runs, _ := p.(map[string]any)["runs"].([]any)
		for _, r := range runs {
			fn(r.(map[string]any))
		}
	}
}

func hasValue(m map[string]any, v string) bool {
	for _, x := range m {
		if x == v {
			return true
		}
	}
	return false
}

// Validate checks a normalised design: structure first, then the rules.
func Validate(d Doc) error {
	if err := cDesign(d, ""); err != nil {
		return err
	}
	ids := map[string]place{}
	seen := map[string]bool{}
	var firstErr error
	fail := func(format string, args ...any) {
		if firstErr == nil {
			firstErr = invalid(format, args...)
		}
	}
	dup := func(id string) {
		if seen[id] {
			fail("duplicate id %s", id)
		}
		seen[id] = true
	}
	for _, p := range pages(d) {
		dup(p["id"].(string))
	}
	for _, l := range links(d) {
		dup(l["id"].(string))
	}
	assets, _ := d["assets"].(map[string]any)
	count := 0
	Walk(d, func(n, parent, page map[string]any, depth int) {
		id := n["id"].(string)
		dup(id)
		ids[id] = place{n: n, parent: parent, page: page, depth: depth}
		count++
		t := n["type"].(string)
		if depth > MaxDepth {
			fail("%s is nested %d deep; the cap is %d", id, depth, MaxDepth)
		}
		if parent != nil && !Containers[parent["type"].(string)] {
			fail("%s: a %s can't have children", id, parent["type"])
		}
		if t == "connector" && parent != nil && parent["type"] == "group" {
			fail("%s: connectors can't sit in a group", id)
		}
		chars := 0
		eachRun(n, func(r map[string]any) { chars += utf8.RuneCountInString(r["text"].(string)) })
		if chars > MaxTextChars {
			fail("%s has %d characters of text; the cap is %d", id, chars, MaxTextChars)
		}
		if t == "image" {
			if _, ok := assets[n["asset"].(string)]; !ok {
				fail("%s: asset %s isn't in assets", id, n["asset"])
			}
			if c, ok := n["crop"].(map[string]any); ok && (c["l"].(float64)+c["r"].(float64) >= 1 || c["t"].(float64)+c["b"].(float64) >= 1) {
				fail("%s: crop leaves nothing of the image", id)
			}
		}
		if sz, ok := n["sizing"].(map[string]any); ok {
			if hasValue(sz, "fill") && (parent == nil || parent["auto_layout"] == nil) {
				fail("%s: fill sizing needs a parent with auto layout", id)
			}
			if hasValue(sz, "hug") && t != "text" && t != "instance" && n["auto_layout"] == nil {
				fail("%s: hug sizing needs text, an instance or auto layout", id)
			}
		}
	})
	if firstErr != nil {
		return firstErr
	}
	if count > MaxNodes {
		return invalid("has %d nodes; the cap is %d", count, MaxNodes)
	}

	// Tokens.
	sets, _ := d["token_sets"].(map[string]any)
	var set map[string]any
	setName := "(no token set)"
	if ref, ok := d["tokens_ref"].(map[string]any); ok {
		setName = ref["set"].(string)
		s, ok := sets[setName].(map[string]any)
		if !ok {
			return invalid("tokens_ref.set %s isn't in token_sets", setName)
		}
		set = s
	}
	isColorToken := func(name string) bool {
		if set == nil {
			return false
		}
		dark := set["modes"].(map[string]any)["dark"].(map[string]any)
		t, ok := dark[name].(map[string]any)
		return ok && t["kind"] == "color"
	}
	checkColor := func(v any, where string) error {
		var cs []string
		switch c := v.(type) {
		case string:
			cs = []string{c}
		case map[string]any:
			for _, s := range c["stops"].([]any) {
				cs = append(cs, s.(string))
			}
		}
		for _, c := range cs {
			if strings.HasPrefix(c, "$") && !isColorToken(c[1:]) {
				return invalid("%s: token %s isn't a colour in %s", where, c, setName)
			}
		}
		return nil
	}
	for _, p := range pages(d) {
		if err := checkColor(p["background"], p["id"].(string)); err != nil {
			return err
		}
	}
	var err error
	Walk(d, func(n, _, _ map[string]any, _ int) {
		if err != nil {
			return
		}
		id := n["id"].(string)
		for _, k := range []string{"fill", "stroke"} {
			if err == nil {
				err = checkColor(n[k], id)
			}
		}
		eachRun(n, func(r map[string]any) {
			for _, k := range []string{"color", "highlight"} {
				if err == nil {
					err = checkColor(r[k], id)
				}
			}
		})
		if ov, ok := n["overrides"].(map[string]any); ok {
			for _, k := range sortedKeys(ov) {
				if err == nil {
					err = checkColor(ov[k].(map[string]any)["fill"], id)
				}
			}
		}
	})
	if err != nil {
		return err
	}

	// References, in paint order so the first error is stable.
	Walk(d, func(n, _, page map[string]any, _ int) {
		if err != nil {
			return
		}
		id := n["id"].(string)
		switch n["type"] {
		case "instance":
			comp := n["component"].(string)
			main, ok := ids[comp]
			if !ok || main.n["type"] != "component" {
				err = invalid("%s: component %s isn't a component", id, comp)
				return
			}
			inMain := map[string]bool{}
			var mark func(m map[string]any)
			mark = func(m map[string]any) {
				inMain[m["id"].(string)] = true
				for _, c := range children(m) {
					mark(c)
				}
			}
			mark(main.n)
			if inMain[id] {
				err = invalid("%s: a component can't contain an instance of itself", id)
				return
			}
			if ov, ok := n["overrides"].(map[string]any); ok {
				for _, k := range sortedKeys(ov) {
					if !inMain[k] || k == comp {
						err = invalid("%s: override %s isn't inside %s", id, k, comp)
						return
					}
				}
			}
		case "connector":
			for _, end := range []string{"from", "to"} {
				target := n[end].(map[string]any)["node"].(string)
				t, ok := ids[target]
				switch {
				case !ok:
					err = invalid("%s: endpoint %s doesn't exist", id, target)
				case !samePage(t.page, page):
					err = invalid("%s: endpoint %s is on another page", id, target)
				case t.n["type"] == "connector":
					err = invalid("%s: can't connect to a connector", id)
				}
				if err != nil {
					return
				}
			}
		}
	})
	if err != nil {
		return err
	}
	top := map[string]bool{}
	for _, p := range pages(d) {
		for _, n := range children(p) {
			if n["type"] == "frame" {
				top[n["id"].(string)] = true
			}
		}
	}
	for _, l := range links(d) {
		id := l["id"].(string)
		if _, ok := ids[l["from"].(string)]; !ok {
			return invalid("link %s: from %s doesn't exist", id, l["from"])
		}
		if !top[l["to"].(string)] {
			return invalid("link %s: to %s isn't a top-level frame", id, l["to"])
		}
		if _, ok := l["delay_ms"]; l["trigger"] == "after-delay" && !ok {
			return invalid("link %s: after-delay needs delay_ms", id)
		}
	}
	return nil
}

func samePage(a, b map[string]any) bool { return a["id"] == b["id"] }

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
