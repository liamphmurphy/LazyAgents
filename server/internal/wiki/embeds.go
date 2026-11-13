package wiki

import (
	"context"
	"errors"
	"net/url"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/design"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Embed is one ![[…]] resolved for the viewer in ctx, so a client can render
// it in place: a page, heading or block as markdown; a file, Design frame or
// Library item as a URL to its bytes. An embed the viewer can't see answers
// exactly as one that doesn't exist (Available false, ADR-knowledge R-A4).
type Embed struct {
	Ref         string       `json:"ref"`
	Target      links.Target `json:"target"`
	Available   bool         `json:"available"`
	Markdown    string       `json:"markdown,omitempty"`
	ContentURL  string       `json:"content_url,omitempty"`
	ContentType string       `json:"content_type,omitempty"`
}

// MaxEmbeds bounds one ResolveEmbeds call.
const MaxEmbeds = 100

// ResolveEmbeds resolves embed texts (as written inside ![[…]]) for the
// viewer in ctx. from is the page they're written on, for ![[#Heading]] and
// for preferring that page's own attachments.
func ResolveEmbeds(ctx context.Context, q *store.Queries, ws uuid.UUID, from *uuid.UUID, refs []string) ([]Embed, error) {
	out := []Embed{}
	seen := map[string]bool{}
	for _, raw := range refs {
		if seen[raw] || len(out) >= MaxEmbeds {
			continue
		}
		seen[raw] = true
		e, err := resolveEmbed(ctx, q, ws, from, raw)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func resolveEmbed(ctx context.Context, q *store.Queries, ws uuid.UUID, from *uuid.UUID, raw string) (Embed, error) {
	r := links.ParseRef(raw)
	var t links.Target
	if r.Target == "" && from != nil {
		a, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: *from, WorkspaceID: ws, Viewer: pageview.From(ctx).UserID})
		if ok, err := visible(err); err != nil || !ok {
			return Embed{Ref: raw, Target: links.Target{Kind: links.KindUnresolved}}, err
		}
		t = links.ArtifactTarget(a, links.AnchorKey(r.Anchor))
	} else {
		var err error
		if t, err = links.ResolveFrom(ctx, q, ws, from, r.Target, r.Anchor); err != nil {
			return Embed{}, err
		}
	}
	e := Embed{Ref: raw, Target: t}
	unavailable := Embed{Ref: raw, Target: links.Target{Kind: links.KindUnresolved, Title: r.Target}}
	switch t.Kind {
	case links.KindUnresolved:
		return unavailable, nil
	case links.KindArtifact, links.KindSection, links.KindBlock:
		md, ok, err := pageContent(ctx, q, ws, *t.ID, t.Kind, t.Anchor)
		if err != nil || !ok {
			return unavailable, err
		}
		e.Markdown, e.ContentType = md, "text/markdown"
	case links.KindAttachment:
		e.ContentURL, e.ContentType = t.Href, t.ContentType
	case links.KindLibrary:
		e.ContentURL, e.ContentType = "/api/v1/library/"+t.ID.String()+"/content", t.ContentType
	case links.KindDesign:
		v, err := q.LatestChatCanvasVersion(ctx, *t.ID)
		if ok, err := visible(err); err != nil || !ok {
			return unavailable, err
		}
		e.ContentURL, e.ContentType = "/api/v1/designs/"+t.ID.String()+"/render", "image/png"
		if t.Anchor != "" {
			n, ok := design.FindNode(v.Body, t.Anchor)
			if !ok {
				return unavailable, nil
			}
			e.Target.Title = t.Title + " / " + design.Label(n)
			e.ContentURL += "?node=" + url.QueryEscape(t.Anchor)
		}
	}
	// Tickets, epics and agents need nothing beyond their target.
	e.Available = true
	return e, nil
}

func visible(err error) (bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// pageContent is a visible page's current markdown, or one heading's section
// or one block of it.
func pageContent(ctx context.Context, q *store.Queries, ws, id uuid.UUID, kind, anchor string) (string, bool, error) {
	viewer := pageview.From(ctx).UserID
	a, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: id, WorkspaceID: ws, Viewer: viewer})
	if ok, err := visible(err); err != nil || !ok || a.CurrentVersionID == nil {
		return "", false, err
	}
	v, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: ws, Viewer: viewer})
	if ok, err := visible(err); err != nil || !ok || v.BodyMd == nil || a.Type == "mockup" {
		return "", false, err
	}
	return anchoredText(*v.BodyMd, kind, anchor)
}

// anchoredText is the part of a body a page, section or block link names.
func anchoredText(body, kind, anchor string) (string, bool, error) {
	switch {
	case anchor == "" || kind == links.KindArtifact:
		return body, true, nil
	case kind == links.KindBlock:
		id, _ := links.BlockID(anchor)
		b, ok := links.FindBlock(body, id)
		return b.Text, ok, nil
	default:
		s, ok := artifacts.SectionText(body, anchor)
		return s, ok, nil
	}
}

// EmbedRefs are the distinct ![[…]] texts of a body, in order.
func EmbedRefs(body string) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range links.Parse(body) {
		if !r.Embed {
			continue
		}
		inner := r.Raw[3 : len(r.Raw)-2]
		if !seen[inner] {
			seen[inner] = true
			out = append(out, inner)
		}
	}
	return out
}
