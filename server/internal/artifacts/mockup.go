package artifacts

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// MockupContentType is how mockup versions are stored: SVG source in body_md.
const MockupContentType = "image/svg+xml"

// Mockup review states.
const (
	ReviewPending          = "pending"
	ReviewApproved         = "approved"
	ReviewChangesRequested = "changes_requested"
)

// IsSVG reports whether an upload is an SVG file.
func IsSVG(name, contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return strings.EqualFold(extOfName(name), ".svg") || ct == MockupContentType
}

func extOfName(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i:]
	}
	return ""
}

var (
	forbiddenSVGElements = map[string]bool{"script": true, "foreignobject": true, "iframe": true, "object": true, "embed": true}
	safeImageHref        = regexp.MustCompile(`(?i)^data:image/(png|jpeg|gif|webp)[;,]`)
	unsafeCSS            = regexp.MustCompile(`(?i)@import|url\(\s*['"]?\s*[^#'"\s)]|expression\(|javascript:`)
	// Any url( in an attribute must point at a local #id.
	externalURL = regexp.MustCompile(`(?i)url\(\s*['"]?\s*[^#'"\s)]`)
	// Scriptable URLs anywhere, e.g. <animate attributeName="href" values="javascript:…">.
	scriptURL = regexp.MustCompile(`(?i)(j\s*a\s*v\s*a|v\s*b)\s*s\s*c\s*r\s*i\s*p\s*t\s*:`)
)

const (
	xhtmlNS  = "http://www.w3.org/1999/xhtml"
	mathmlNS = "http://www.w3.org/1998/Math/MathML"
)

// ValidSVG checks that body is a self-contained SVG document: an svg root, no
// script or embedded foreign content, no event handlers, no external or
// scriptable references, and no DOCTYPE or entity declarations.
func ValidSVG(body string) error {
	if len(body) > MaxMarkdownBytes {
		return apperr.Invalid("body_md", "is limited to 2 MB")
	}
	if !utf8.ValidString(body) {
		return apperr.Invalid("body_md", "must be UTF-8 text")
	}
	bad := func(format string, args ...any) error { return apperr.Invalid("svg", format, args...) }
	dec := xml.NewDecoder(strings.NewReader(body))
	dec.Strict = true
	dec.Entity = nil // undefined entities are errors
	seenRoot, inStyle := false, false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return bad("isn't well-formed SVG: %v", err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			return bad("can't contain a DOCTYPE or entity declaration")
		case xml.ProcInst:
			// <?xml ...?> is fine; anything else (stylesheets) is not.
			if !strings.EqualFold(t.Target, "xml") {
				return bad("can't contain processing instructions")
			}
		case xml.CharData:
			if inStyle && (unsafeCSS.Match(t) || bytes.ContainsRune(t, '\\')) {
				return bad("<style> can't import or load external resources")
			}
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "style") {
				inStyle = false
			}
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if inStyle {
				return bad("<style> can only contain text")
			}
			inStyle = name == "style"
			if t.Name.Space == xhtmlNS || t.Name.Space == mathmlNS {
				return bad("<%s> isn't an SVG element; HTML and MathML aren't allowed in mockups", t.Name.Local)
			}
			if !seenRoot {
				seenRoot = true
				if name != "svg" {
					return bad("the root element must be <svg>")
				}
			}
			if forbiddenSVGElements[name] {
				return bad("<%s> isn't allowed in mockups", t.Name.Local)
			}
			for _, a := range t.Attr {
				local := strings.ToLower(a.Name.Local)
				if scriptURL.MatchString(a.Value) {
					return bad("scriptable URLs aren't allowed")
				}
				if local == "attributename" && strings.HasSuffix(strings.ToLower(strings.TrimSpace(a.Value)), "href") {
					return bad("animating references isn't allowed")
				}
				if strings.HasPrefix(local, "on") {
					return bad("event handler attributes like %s aren't allowed", a.Name.Local)
				}
				if externalURL.MatchString(a.Value) {
					return bad("url() may only point at a #id")
				}
				if local == "style" && (unsafeCSS.MatchString(a.Value) || strings.Contains(a.Value, `\`)) {
					return bad("style attributes can't import or load external resources")
				}
				if (local == "src" || local == "action" || local == "formaction") && !strings.HasPrefix(strings.TrimSpace(a.Value), "#") {
					return bad("%s attributes aren't allowed", a.Name.Local)
				}
				if local == "href" {
					v := strings.TrimSpace(a.Value)
					if !strings.HasPrefix(v, "#") && !safeImageHref.MatchString(v) {
						return bad("references must be #ids or data:image/png|jpeg|gif|webp URIs; external and scriptable URLs aren't allowed")
					}
				}
			}
		}
	}
	if !seenRoot {
		return bad("must contain an <svg> element")
	}
	return nil
}

// SVGText gathers the visible text of an SVG (text, tspan, title, desc) for search.
func SVGText(body string) string {
	dec := xml.NewDecoder(strings.NewReader(body))
	dec.Strict = false
	var b strings.Builder
	depth := 0 // nesting inside a text-bearing element
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch strings.ToLower(t.Name.Local) {
			case "text", "tspan", "title", "desc":
				depth++
			}
		case xml.EndElement:
			switch strings.ToLower(t.Name.Local) {
			case "text", "tspan", "title", "desc":
				if depth > 0 {
					depth--
				}
				if depth == 0 {
					b.WriteByte('\n')
				}
			}
		case xml.CharData:
			if depth > 0 {
				if s := string(bytes.TrimSpace(t)); s != "" {
					b.WriteString(s)
					b.WriteByte(' ')
				}
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// GateMockup is one candidate mockup (its current version) in a ticket's gate.
type GateMockup struct {
	ID           uuid.UUID
	Title        string
	VersionID    uuid.UUID
	VersionLabel string
	ReviewState  string
	ReviewNote   string
}

// Gate says whether a ticket needs a person-approved mockup, and whether it has one.
type Gate struct {
	Required bool
	FromEpic bool // required only because the ticket's epic says so
	Approved bool
	Mockups  []GateMockup
}

// MockupGate evaluates a ticket's mockup gate. Candidates are mockups attached
// to the ticket or belonging to its epic; one approved current version is enough.
func MockupGate(ctx context.Context, q *store.Queries, t store.Ticket) (Gate, error) {
	g := Gate{Required: t.RequiresMockup, Mockups: []GateMockup{}}
	if t.EpicID != nil {
		e, err := q.GetEpic(ctx, store.GetEpicParams{ID: *t.EpicID, WorkspaceID: t.WorkspaceID})
		if err != nil {
			return g, err
		}
		if e.RequiresMockup && !t.RequiresMockup {
			g.Required, g.FromEpic = true, true
		}
	}
	rows, err := q.MockupGateCandidates(ctx, store.MockupGateCandidatesParams{WorkspaceID: t.WorkspaceID, TicketID: t.ID, EpicID: t.EpicID})
	if err != nil {
		return g, err
	}
	for _, r := range rows {
		m := GateMockup{ID: r.ID, Title: r.Title, VersionID: r.VersionID, VersionLabel: r.VersionLabel, ReviewNote: r.ReviewNote}
		if r.ReviewState != nil {
			m.ReviewState = *r.ReviewState
		}
		if m.ReviewState == ReviewApproved {
			g.Approved = true
		}
		g.Mockups = append(g.Mockups, m)
	}
	return g, nil
}

// Unblocked reports whether the ticket can finish: no mockup needed, or one is approved.
func (g Gate) Unblocked() bool { return !g.Required || g.Approved }
