package design

import (
	"strings"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

// Box is a node's box. The persona-facing views carry it as {x, y, w, h}.
type Box struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// OutlineNode is one row of design_outline.
type OutlineNode struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Name       string         `json:"name,omitempty"`
	Box        *Box           `json:"box,omitempty"`
	Children   []*OutlineNode `json:"children,omitempty"`
	ChildCount int            `json:"child_count"`
}

// OutlinePage is a page with its top-level nodes.
type OutlinePage struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Children   []*OutlineNode `json:"children,omitempty"`
	ChildCount int            `json:"child_count"`
}

func fnum(v any) float64 { f, _ := v.(float64); return f }

// Label names a node for a person: its name, else (text, sticky) its first
// words, else its type.
func Label(n map[string]any) string {
	if s, _ := n["name"].(string); s != "" {
		return s
	}
	var b strings.Builder
	if t, _ := n["text"].(string); t != "" {
		b.WriteString(t)
	} else {
		eachRun(n, func(r map[string]any) {
			if s, _ := r["text"].(string); s != "" && b.Len() < 60 {
				b.WriteString(s)
			}
		})
	}
	s := strings.Join(strings.Fields(b.String()), " ")
	if r := []rune(s); len(r) > 40 {
		s = string(r[:40]) + "…"
	}
	return s
}

// Outline is the compact tree of a design (or one page), down to depth levels
// below the page. boxes are settled boxes in page coordinates keyed by node
// id; a node missing from it gets its stored box, with the parents' offsets
// added, which ignores auto layout (stored=true tells the caller).
func Outline(body, page string, depth int, boxes map[string]Box) ([]OutlinePage, error) {
	d, err := Parse(body)
	if err != nil {
		return nil, err
	}
	if depth < 1 {
		depth = 3
	}
	var out []OutlinePage
	var conv func(n map[string]any, ox, oy float64, level int) *OutlineNode
	conv = func(n map[string]any, ox, oy float64, level int) *OutlineNode {
		o := &OutlineNode{ID: sstr(n["id"]), Type: sstr(n["type"]), Name: Label(n)}
		kids := children(n)
		o.ChildCount = len(kids)
		x, y := ox+fnum(n["x"]), oy+fnum(n["y"])
		if b, ok := boxes[o.ID]; ok {
			o.Box = &b
		} else if o.Type != "connector" {
			o.Box = &Box{X: x, Y: y, W: fnum(n["w"]), H: fnum(n["h"])}
		}
		if level < depth {
			for _, c := range kids {
				o.Children = append(o.Children, conv(c, x, y, level+1))
			}
		}
		return o
	}
	for _, p := range pages(d) {
		if page != "" && p["id"] != page {
			continue
		}
		op := OutlinePage{ID: sstr(p["id"]), Name: sstr(p["name"])}
		kids := children(p)
		op.ChildCount = len(kids)
		for _, c := range kids {
			op.Children = append(op.Children, conv(c, 0, 0, 1))
		}
		out = append(out, op)
	}
	if page != "" && len(out) == 0 {
		return nil, apperr.Invalid("page", "no page %s in this design", page)
	}
	return out, nil
}

func sstr(v any) string { s, _ := v.(string); return s }

// MaxReadNodes bounds how many nodes one design_read returns.
const MaxReadNodes = 500

// TokenValue is how a $token shows up in a read: the reference and the value
// it has in the node's mode.
type TokenValue struct {
	Token string `json:"token"`
	Value any    `json:"value"`
}

// Read is design_read's result.
type Read struct {
	Mode  string           `json:"mode"`
	Nodes []map[string]any `json:"nodes"`
	Links []map[string]any `json:"links,omitempty"`
	// Truncated lists nodes whose children were left out to stay within
	// MaxReadNodes; read them by id.
	Truncated []string `json:"truncated,omitempty"`
}

// tokenValues finds a token's value in the set the design uses, for a mode
// (light falls back to dark).
func tokenValues(d Doc) func(mode, name string) (any, bool) {
	var modes map[string]any
	if ref, _ := d["tokens_ref"].(map[string]any); ref != nil {
		sets, _ := d["token_sets"].(map[string]any)
		set, _ := sets[sstr(ref["set"])].(map[string]any)
		modes, _ = set["modes"].(map[string]any)
	}
	return func(mode, name string) (any, bool) {
		for _, m := range []string{mode, "dark"} {
			toks, _ := modes[m].(map[string]any)
			if t, ok := toks[name].(map[string]any); ok {
				return t["value"], true
			}
		}
		return nil, false
	}
}

// resolveTokens copies v with every "$name" string replaced by
// {token, value}; a token the set doesn't have keeps value null.
func resolveTokens(v any, mode string, lookup func(mode, name string) (any, bool)) any {
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(x, "$") && reColor.MatchString(x) {
			val, _ := lookup(mode, x[1:])
			return map[string]any{"token": x, "value": val}
		}
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = resolveTokens(e, mode, lookup)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = resolveTokens(e, mode, lookup)
		}
		return out
	}
	return v
}

// ReadNodes returns full nodes for design_read: those named by ids, or a
// page's top-level nodes. Tokens are resolved for the mode in effect at each
// node (a frame's own mode wins over the design's). Links touching a returned
// node come with them.
func ReadNodes(body string, ids []string, page string) (Read, error) {
	d, err := Parse(body)
	if err != nil {
		return Read{}, err
	}
	if (len(ids) == 0) == (page == "") {
		return Read{}, apperr.Invalid("node_ids", "give node_ids or a page, not both and not neither")
	}
	lookup := tokenValues(d)
	designMode := sstr(d["mode"])
	out := Read{Mode: designMode, Nodes: []map[string]any{}}
	budget := MaxReadNodes
	touched := map[string]bool{}

	var emit func(n map[string]any, mode string) map[string]any
	emit = func(n map[string]any, mode string) map[string]any {
		budget--
		touched[sstr(n["id"])] = true
		if m := sstr(n["mode"]); m != "" {
			mode = m
		}
		cp := map[string]any{}
		for k, v := range n {
			if k != "children" {
				cp[k] = resolveTokens(v, mode, lookup)
			}
		}
		kids := children(n)
		if len(kids) == 0 {
			return cp
		}
		if budget < len(kids) {
			cp["child_count"] = len(kids)
			out.Truncated = append(out.Truncated, sstr(n["id"]))
			return cp
		}
		list := make([]any, 0, len(kids))
		for _, c := range kids {
			list = append(list, emit(c, mode))
		}
		cp["children"] = list
		return cp
	}
	modeAt := func(target map[string]any) string {
		mode := designMode
		// The mode in effect is the nearest ancestor's.
		var visit func(n map[string]any, m string) (string, bool)
		visit = func(n map[string]any, m string) (string, bool) {
			if x := sstr(n["mode"]); x != "" {
				m = x
			}
			if n["id"] == target["id"] {
				return m, true
			}
			for _, c := range children(n) {
				if r, ok := visit(c, m); ok {
					return r, true
				}
			}
			return "", false
		}
		for _, p := range pages(d) {
			for _, c := range children(p) {
				if r, ok := visit(c, mode); ok {
					return r
				}
			}
		}
		return mode
	}
	if page != "" {
		found := false
		for _, p := range pages(d) {
			if p["id"] != page {
				continue
			}
			found = true
			for _, c := range children(p) {
				out.Nodes = append(out.Nodes, emit(c, designMode))
			}
		}
		if !found {
			return Read{}, apperr.Invalid("page", "no page %s in this design", page)
		}
	} else {
		if len(ids) > 50 {
			return Read{}, apperr.Invalid("node_ids", "read at most 50 nodes at once")
		}
		for _, id := range ids {
			l, ok := find(d, id)
			if !ok {
				return Read{}, apperr.Invalid("node_ids", "no node %s in this design", id)
			}
			out.Nodes = append(out.Nodes, emit(l.n, modeAt(l.n)))
		}
	}
	for _, l := range links(d) {
		if touched[sstr(l["from"])] || touched[sstr(l["to"])] {
			out.Links = append(out.Links, l)
		}
	}
	return out, nil
}

// NodeSummary is what a selection's context doc lists about a node.
type NodeSummary struct {
	ID, Type, Name string
	Box            *Box
	Found          bool
}

// Describe looks a node up by id, with its box in page coordinates computed
// from stored boxes (ignoring auto layout).
func Describe(body string, id string) (NodeSummary, error) {
	d, err := Parse(body)
	if err != nil {
		return NodeSummary{}, err
	}
	out := NodeSummary{ID: id}
	var visit func(n map[string]any, ox, oy float64) bool
	visit = func(n map[string]any, ox, oy float64) bool {
		x, y := ox+fnum(n["x"]), oy+fnum(n["y"])
		if n["id"] == id {
			out.Found, out.Type, out.Name = true, sstr(n["type"]), Label(n)
			if out.Type != "connector" {
				out.Box = &Box{X: x, Y: y, W: fnum(n["w"]), H: fnum(n["h"])}
			}
			return true
		}
		for _, c := range children(n) {
			if visit(c, x, y) {
				return true
			}
		}
		return false
	}
	for _, p := range pages(d) {
		for _, c := range children(p) {
			if visit(c, 0, 0) {
				return out, nil
			}
		}
	}
	return out, nil
}

// HasNode reports whether a node id is in the body.
func HasNode(body, id string) bool {
	d, err := Parse(body)
	if err != nil {
		return false
	}
	_, ok := find(d, id)
	return ok
}

// HasPage reports whether a page id is in the body.
func HasPage(body, id string) bool {
	d, err := Parse(body)
	if err != nil {
		return false
	}
	_, _, ok := findPage(d, id)
	return ok
}
