package links

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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
		t.Anchor = anchor
		t.Href += "#" + anchor
	}
	return t
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
	target = strings.TrimSpace(target)
	slug := ""
	if anchor = strings.TrimSpace(anchor); anchor != "" {
		slug = Slug(anchor)
		if strings.HasPrefix(anchor, "^") {
			slug = anchor
		}
	}
	ghost := Target{Kind: KindUnresolved, Title: target}
	if target == "" {
		return ghost, nil
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
	return ghost, nil
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
