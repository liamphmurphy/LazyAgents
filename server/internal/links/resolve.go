package links

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Target kinds stored in knowledge_links.target_kind.
const (
	KindArtifact   = "artifact"
	KindSection    = "section"
	KindBlock      = "block" // a paragraph or list item marked "^id"; Anchor is "^id"
	KindAttachment = "attachment"
	KindDesign     = "design"  // a Designs canvas; Anchor is a frame's node id
	KindLibrary    = "library" // a Library item
	KindTicket     = "ticket"
	KindEpic       = "epic"
	KindAgent      = "agent"
	KindUnresolved = "unresolved"
)

// Target is what a link points at, with enough to render it.
type Target struct {
	Kind         string     `json:"kind"`
	ID           *uuid.UUID `json:"id,omitempty"`
	Anchor       string     `json:"anchor,omitempty"` // section slug
	Title        string     `json:"title"`
	ArtifactType string     `json:"artifact_type,omitempty"`
	Path         string     `json:"path,omitempty"` // notes
	DisplayID    string     `json:"display_id,omitempty"`
	Href         string     `json:"href,omitempty"`
	Status       string     `json:"status,omitempty"`      // tickets
	ColorIndex   int        `json:"color_index,omitempty"` // agents
	ContentType  string     `json:"content_type,omitempty"` // attachments and Library items
}

// IsPageKind reports whether a target kind is an artifact or a part of one.
func IsPageKind(kind string) bool {
	return kind == KindArtifact || kind == KindSection || kind == KindBlock
}

// AnchorKey is how an anchor is stored: "^id" for a block, else the heading slug.
func AnchorKey(anchor string) string {
	if anchor = strings.TrimSpace(anchor); anchor == "" {
		return ""
	}
	if id, ok := BlockID(anchor); ok {
		return "^" + id
	}
	return Slug(anchor)
}

var (
	ticketKey = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]{1,9})-(\d+)$`)
	adrID     = regexp.MustCompile(`(?i)^ADR-(\d+)$`)
)

// NoteHref is the wiki URL for a note path, one escaped segment per folder.
func NoteHref(path string) string {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return "/wiki/" + strings.Join(segs, "/")
}

// ArtifactTarget describes an artifact (and optionally one section of it).
func ArtifactTarget(a store.Artifact, anchor string) Target {
	t := Target{Kind: KindArtifact, ID: &a.ID, Title: a.Title, ArtifactType: a.Type}
	if a.Type == "note" && a.Path != nil {
		t.Path = *a.Path
		t.Href = NoteHref(*a.Path)
	} else {
		t.Href = "/artifacts/" + a.ID.String()
	}
	if a.Type == "adr" && a.Number != nil {
		t.DisplayID = fmt.Sprintf("ADR-%03d", *a.Number)
	}
	if anchor != "" {
		t.Kind = KindSection
		if strings.HasPrefix(anchor, "^") {
			t.Kind = KindBlock
		}
		t.Anchor = anchor
		t.Href += "#" + anchor
	}
	return t
}

// AttachmentHref is where an attachment's bytes are served.
func AttachmentHref(id uuid.UUID) string { return "/api/v1/wiki/attachments/" + id.String() + "/content" }

func AttachmentTarget(a store.PageAttachment) Target {
	return Target{Kind: KindAttachment, ID: &a.ID, Title: a.Name, ContentType: a.ContentType, Href: AttachmentHref(a.ID)}
}

func DesignTarget(id uuid.UUID, title, node string) Target {
	t := Target{Kind: KindDesign, ID: &id, Title: title, Anchor: node, Href: "/apps/designs/" + id.String()}
	if node != "" {
		t.Href += "?node=" + url.QueryEscape(node)
	}
	return t
}

func LibraryTarget(l store.LibraryItem) Target {
	return Target{Kind: KindLibrary, ID: &l.ID, Title: l.Title, ContentType: l.ContentType, Href: "/library?item=" + l.ID.String()}
}

// Embed prefixes for other apps: ![[design:<canvas id>#<node id>]] and
// ![[library:<item id>]].
const (
	DesignPrefix  = "design:"
	LibraryPrefix = "library:"
)

// IsFileName reports whether a link names a file (an attachment) rather than
// a page: it has an extension other than ".md".
func IsFileName(target string) bool {
	ext := strings.ToLower(path.Ext(target))
	return ext != "" && ext != ".md" && len(ext) <= 6 && !strings.ContainsAny(ext, " ")
}

func TicketTarget(t store.Ticket) Target {
	return Target{Kind: KindTicket, ID: &t.ID, Title: t.Title, DisplayID: t.Key, Href: "/tickets/" + t.Key, Status: t.Status}
}

func EpicTarget(e store.Epic) Target {
	return Target{Kind: KindEpic, ID: &e.ID, Title: e.Name, Href: "/epics?epic=" + e.ID.String()}
}

func AgentTarget(a store.Agent) Target {
	return Target{Kind: KindAgent, ID: &a.ID, Title: a.Name, DisplayID: "@" + a.Handle, Href: "/agents/" + a.Handle, ColorIndex: int(a.ColorIndex)}
}

func found(err error) (bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// Resolve finds what a link text points at. Order: @agent, ADR-NNN, ticket
// key, note path, note title, another artifact's title, epic name, then a
// renamed note's old path. Nothing found is an unresolved (ghost) target.
func Resolve(ctx context.Context, q *store.Queries, ws uuid.UUID, target, anchor string) (Target, error) {
	return ResolveFrom(ctx, q, ws, nil, target, anchor)
}

// ResolveFrom is Resolve for a link written on page from (nil elsewhere):
// ![[file]] prefers that page's own attachment. Designs, Library items and
// attachments resolve only for a viewer who may see them, as pages do.
func ResolveFrom(ctx context.Context, q *store.Queries, ws uuid.UUID, from *uuid.UUID, target, anchor string) (Target, error) {
	target = strings.TrimSpace(target)
	slug := AnchorKey(anchor)
	anchor = strings.TrimSpace(anchor)
	ghost := Target{Kind: KindUnresolved, Title: target}
	if target == "" {
		return ghost, nil
	}
	viewer := pageview.From(ctx).UserID

	if rest, ok := cutPrefixFold(target, DesignPrefix); ok {
		id, err := uuid.Parse(strings.TrimSpace(rest))
		if err != nil || viewer == nil {
			return ghost, nil
		}
		cv, err := q.GetMyCanvas(ctx, store.GetMyCanvasParams{ID: id, WorkspaceID: ws, UserID: *viewer})
		if ok, err := found(err); err != nil || !ok || cv.Kind != "design" {
			return ghost, err
		}
		return DesignTarget(cv.ID, cv.Title, anchor), nil
	}
	if rest, ok := cutPrefixFold(target, LibraryPrefix); ok {
		id, err := uuid.Parse(strings.TrimSpace(rest))
		if err != nil || viewer == nil {
			return ghost, nil
		}
		l, err := q.GetLibraryItem(ctx, store.GetLibraryItemParams{ID: id, WorkspaceID: ws, UserID: *viewer})
		if ok, err := found(err); err != nil || !ok {
			return ghost, err
		}
		return LibraryTarget(l), nil
	}

	if h, ok := strings.CutPrefix(target, "@"); ok {
		a, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: strings.ToLower(h)})
		if ok, err := found(err); err != nil || ok {
			return AgentTarget(a), err
		}
		return ghost, nil
	}
	if m := adrID.FindStringSubmatch(target); m != nil {
		n, _ := strconv.Atoi(m[1])
		num := int32(n)
		a, err := q.GetADRByNumber(ctx, store.GetADRByNumberParams{WorkspaceID: ws, Number: &num, Viewer: pageview.From(ctx).UserID})
		if ok, err := found(err); err != nil || ok {
			return ArtifactTarget(a, slug), err
		}
	}
	if ticketKey.MatchString(target) {
		t, err := q.GetTicketByKey(ctx, store.GetTicketByKeyParams{WorkspaceID: ws, Key: strings.ToUpper(target)})
		if ok, err := found(err); err != nil || ok {
			return TicketTarget(t), err
		}
	}
	path := NormalizePath(target)
	a, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: path, Viewer: pageview.From(ctx).UserID})
	if ok, err := found(err); err != nil {
		return ghost, err
	} else if ok && a.CurrentVersionID != nil { // a suggested page isn't there yet
		return ArtifactTarget(a, slug), nil
	}
	notes, err := q.FindNotesByTitle(ctx, store.FindNotesByTitleParams{WorkspaceID: ws, Title: target, Viewer: pageview.From(ctx).UserID})
	if err != nil {
		return ghost, err
	}
	if len(notes) > 0 {
		return ArtifactTarget(notes[0], slug), nil
	}
	a, err = q.FindArtifactByTitle(ctx, store.FindArtifactByTitleParams{WorkspaceID: ws, Title: target, Viewer: pageview.From(ctx).UserID})
	if ok, err := found(err); err != nil || ok {
		return ArtifactTarget(a, slug), err
	}
	e, err := q.FindEpicByName(ctx, store.FindEpicByNameParams{WorkspaceID: ws, Name: target})
	if ok, err := found(err); err != nil || ok {
		return EpicTarget(e), err
	}
	a, err = q.FindNoteByAlias(ctx, store.FindNoteByAliasParams{WorkspaceID: ws, Path: path, Viewer: pageview.From(ctx).UserID})
	if ok, err := found(err); err != nil || ok {
		return ArtifactTarget(a, slug), err
	}
	if IsFileName(target) {
		pa, err := q.FindVisiblePageAttachment(ctx, store.FindVisiblePageAttachmentParams{WorkspaceID: ws, Name: TitleOf(path), Viewer: viewer, FromPage: from})
		if ok, err := found(err); err != nil || ok {
			return AttachmentTarget(pa), err
		}
	}
	return ghost, nil
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return s, false
}

// NormalizePath trims each segment and drops empty ones and a ".md" suffix:
// " Payments//Ledger/Runbook.md " → "Payments/Ledger/Runbook".
func NormalizePath(p string) string {
	p = strings.TrimSuffix(strings.TrimSpace(strings.ReplaceAll(p, "\\", "/")), ".md")
	var segs []string
	for s := range strings.SplitSeq(p, "/") {
		if s = strings.Join(strings.Fields(s), " "); s != "" {
			segs = append(segs, s)
		}
	}
	return strings.Join(segs, "/")
}

// TitleOf is a note path's last segment.
func TitleOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
