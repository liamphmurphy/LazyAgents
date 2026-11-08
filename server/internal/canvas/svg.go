package canvas

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"encoding/xml"
	"errors"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

const svgNS = "http://www.w3.org/2000/svg"

// VIDAttr identifies an element across versions of an SVG canvas, unlike its
// position in the tree, which shifts when an earlier sibling is added.
const VIDAttr = "data-vid"

var (
	vidRe   = regexp.MustCompile(`^[a-z0-9]{1,24}$`)
	nameRe  = regexp.MustCompile(`^[A-Za-z_:][\w:.-]*$`)
	attrRe  = regexp.MustCompile(`\s+([^\s=/>]+)\s*=\s*("[^"]*"|'[^']*')`)
	vidCode = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)
)

// element is where one element sits in a document, in byte offsets.
type element struct {
	vid                         string
	depth                       int
	start, tagEnd, closeAt, end int
	hasChild                    bool
}

func (e *element) selfClosed() bool { return e.closeAt == e.end }

// scan indexes well-formed XML by byte offset, in document order. Offsets, not
// a re-serialization, so edits leave the rest of the source untouched.
func scan(body string) ([]*element, error) {
	d := xml.NewDecoder(strings.NewReader(body))
	var all, stack []*element
	for {
		at := int(d.InputOffset())
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, apperr.Invalid("body", "not well-formed SVG: %v", err)
		}
		after := int(d.InputOffset())
		switch t := tok.(type) {
		case xml.Directive:
			return nil, apperr.Invalid("body", "DOCTYPE and other directives aren't allowed")
		case xml.ProcInst:
			if t.Target != "xml" {
				return nil, apperr.Invalid("body", "processing instructions aren't allowed")
			}
		case xml.CharData:
			if len(stack) == 0 && len(bytes.TrimSpace(t)) > 0 {
				return nil, apperr.Invalid("body", "text outside the root element")
			}
		case xml.StartElement:
			e := &element{depth: len(stack), start: at, tagEnd: after}
			for _, a := range t.Attr {
				if a.Name.Space == "" && a.Name.Local == VIDAttr {
					e.vid = a.Value
				}
			}
			if len(stack) > 0 {
				stack[len(stack)-1].hasChild = true
			}
			all = append(all, e)
			stack = append(stack, e)
		case xml.EndElement:
			e := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			e.closeAt, e.end = at, after
		}
	}
	if len(all) == 0 {
		return nil, apperr.Invalid("body", "no root element")
	}
	return all, nil
}

// checkRoot requires a single <svg> root in the SVG namespace.
func checkRoot(body string, all []*element) error {
	for _, e := range all[1:] {
		if e.depth == 0 {
			return apperr.Invalid("body", "more than one root element")
		}
	}
	d := xml.NewDecoder(strings.NewReader(body[all[0].start:all[0].tagEnd]))
	tok, err := d.RawToken()
	if err != nil {
		return apperr.Invalid("body", "not well-formed SVG: %v", err)
	}
	start := tok.(xml.StartElement)
	if start.Name.Local != "svg" {
		return apperr.Invalid("body", "the root element must be <svg>")
	}
	for _, a := range start.Attr {
		if a.Name.Space == "" && a.Name.Local == "xmlns" && a.Value == svgNS {
			return nil
		}
	}
	return apperr.Invalid("body", `the root <svg> needs xmlns="%s"`, svgNS)
}

func newVID() (string, error) {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return vidCode.EncodeToString(b[:]), nil
}

func nameEnd(tag string) int {
	if i := strings.IndexAny(tag, " \t\r\n/>"); i > 0 {
		return i
	}
	return len(tag)
}

// setAttr returns the start tag with an attribute set, or removed when value is nil.
func setAttr(tag, name string, value *string) string {
	off := nameEnd(tag)
	var out string
	if value != nil {
		out = " " + name + `="` + escape(*value) + `"`
	}
	for _, m := range attrRe.FindAllStringSubmatchIndex(tag[off:], -1) {
		if tag[off+m[2]:off+m[3]] == name {
			return tag[:off+m[0]] + out + tag[off+m[1]:]
		}
	}
	if value == nil {
		return tag
	}
	return tag[:off] + out + tag[off:]
}

func escape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Stamp gives every element below the root a unique data-vid, keeping valid
// ones. A repeated id (an agent copying an element) is replaced on the later
// element, and anything else in the source is left byte for byte.
func Stamp(body string) (string, error) {
	all, err := scan(body)
	if err != nil {
		return "", err
	}
	if err := checkRoot(body, all); err != nil {
		return "", err
	}
	seen := map[string]bool{}
	var b strings.Builder
	last := 0
	for _, e := range all[1:] {
		if vidRe.MatchString(e.vid) && !seen[e.vid] {
			seen[e.vid] = true
			continue
		}
		vid, err := newVID()
		if err != nil {
			return "", err
		}
		seen[vid] = true
		b.WriteString(body[last:e.start])
		b.WriteString(setAttr(body[e.start:e.tagEnd], VIDAttr, &vid))
		last = e.tagEnd
	}
	b.WriteString(body[last:])
	return b.String(), nil
}

func find(all []*element, vid string) *element {
	if i := slices.IndexFunc(all[min(1, len(all)):], func(e *element) bool { return e.vid == vid }); i >= 0 {
		return all[i+1]
	}
	return nil
}

// oneElement checks markup is a single well-formed element and returns its span.
func oneElement(markup string) (*element, error) {
	const open = `<svg xmlns="` + svgNS + `">`
	wrapped := open + markup + "</svg>"
	all, err := scan(wrapped)
	if err != nil {
		return nil, apperr.Invalid("markup", "not well-formed: %v", err)
	}
	var top []*element
	for _, e := range all {
		if e.depth == 1 {
			top = append(top, e)
		}
	}
	if all[len(all)-1].depth == 0 || len(top) != 1 {
		return nil, apperr.Invalid("markup", "provide exactly one element")
	}
	return &element{start: top[0].start - len(open), tagEnd: top[0].tagEnd - len(open), vid: top[0].vid}, nil
}

func applySVGOp(body string, all []*element, op Op) (string, error) {
	target := find(all, op.VID)
	if target == nil && !(op.Op == "insert" && op.Position == "append" && op.VID == "") {
		return "", apperr.Invalid("vid", "no element %q in this version", op.VID)
	}
	splice := func(from, to int, s string) string { return body[:from] + s + body[to:] }
	switch op.Op {
	case "set_attrs":
		if len(op.Attrs) == 0 {
			return "", apperr.Invalid("attrs", "set at least one attribute")
		}
		tag := body[target.start:target.tagEnd]
		for _, name := range slices.Sorted(maps.Keys(op.Attrs)) {
			v := op.Attrs[name]
			if !nameRe.MatchString(name) || name == VIDAttr {
				return "", apperr.Invalid("attrs", "can't set %q", name)
			}
			var value *string
			if v != "" {
				value = &v
			}
			tag = setAttr(tag, name, value)
		}
		return splice(target.start, target.tagEnd, tag), nil
	case "set_text":
		if target.hasChild || target.selfClosed() {
			return "", apperr.Invalid("vid", "set_text needs an element holding only text; use replace")
		}
		return splice(target.tagEnd, target.closeAt, escape(op.Text)), nil
	case "remove":
		return splice(target.start, target.end, ""), nil
	case "replace":
		e, err := oneElement(op.Markup)
		if err != nil {
			return "", err
		}
		// The replacement is still the same element, so comments stay on it.
		tag := setAttr(op.Markup[e.start:e.tagEnd], VIDAttr, &op.VID)
		return splice(target.start, target.end, op.Markup[:e.start]+tag+op.Markup[e.tagEnd:]), nil
	case "insert":
		if _, err := oneElement(op.Markup); err != nil {
			return "", err
		}
		switch op.Position {
		case "before":
			return splice(target.start, target.start, op.Markup), nil
		case "after":
			return splice(target.end, target.end, op.Markup), nil
		case "append":
			parent := target
			if parent == nil {
				parent = all[0]
			}
			if parent.selfClosed() {
				return "", apperr.Invalid("vid", "can't append to an empty self-closed element; use replace")
			}
			return splice(parent.closeAt, parent.closeAt, op.Markup), nil
		}
		return "", apperr.Invalid("position", "use before, after or append")
	}
	return "", apperr.Invalid("op", "unknown SVG op %q", op.Op)
}
