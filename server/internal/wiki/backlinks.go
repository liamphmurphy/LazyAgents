package wiki

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/links"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

// Ref names something links can point at.
type Ref struct {
	Kind string // artifact | ticket | epic | agent
	ID   uuid.UUID
}

// Mention is one source that mentions a target, with where.
type Mention struct {
	SourceKind string       `json:"source_kind"` // artifact | ticket | comment
	SourceID   uuid.UUID    `json:"source_id"`
	Source     links.Target `json:"source"` // how to show and open the source
	Snippets   []string     `json:"snippets"`
	Anchors    []string     `json:"anchors,omitempty"` // sections of the target it cites
}

// Backlinks are the linked mentions of a target, one entry per source.
func Backlinks(ctx context.Context, q *store.Queries, ws uuid.UUID, ref Ref) ([]Mention, error) {
	rows, err := q.Backlinks(ctx, store.BacklinksParams{WorkspaceID: ws, TargetID: &ref.ID, TargetKind: ref.Kind})
	if err != nil {
		return nil, err
	}
	out := []Mention{}
	at := map[string]int{}
	for _, r := range rows {
		if r.SourceKind == links.SourceArtifact && r.SourceID == ref.ID {
			continue // a page linking to its own sections
		}
		key := r.SourceKind + ":" + r.SourceID.String()
		i, ok := at[key]
		if !ok {
			i = len(out)
			at[key] = i
			out = append(out, Mention{SourceKind: r.SourceKind, SourceID: r.SourceID, Source: sourceTarget(r), Snippets: []string{}})
		}
		m := &out[i]
		if len(m.Snippets) < 3 && !contains(m.Snippets, r.Snippet) {
			m.Snippets = append(m.Snippets, r.Snippet)
		}
		if r.TargetKind == links.KindSection && r.TargetAnchor != "" && !contains(m.Anchors, r.TargetAnchor) {
			m.Anchors = append(m.Anchors, r.TargetAnchor)
		}
	}
	return out, nil
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func sourceTarget(r store.BacklinksRow) links.Target {
	switch r.SourceKind {
	case links.SourceArtifact:
		a := store.Artifact{ID: r.SourceID, Title: deref(r.SourceTitle), Type: deref(r.SourceType), Path: r.SourcePath, Number: r.SourceNumber}
		return links.ArtifactTarget(a, "")
	case links.SourceComment:
		return links.Target{Kind: "comment", ID: &r.SourceID, Title: "Comment on " + deref(r.TicketKey), DisplayID: deref(r.TicketKey),
			Href: "/tickets/" + deref(r.TicketKey)}
	}
	return links.Target{Kind: links.KindTicket, ID: &r.SourceID, Title: deref(r.TicketTitle), DisplayID: deref(r.TicketKey), Href: "/tickets/" + deref(r.TicketKey)}
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}

// Phrases are what an unlinked mention of a target looks like in text.
func Phrases(ctx context.Context, q *store.Queries, ws uuid.UUID, ref Ref) (phrases []string, link string, err error) {
	switch ref.Kind {
	case links.KindArtifact:
		a, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: ref.ID, WorkspaceID: ws})
		if err != nil {
			return nil, "", err
		}
		if id := artifacts.DisplayID(a.Type, a.Number); id != nil {
			return []string{*id, a.Title}, *id, nil
		}
		if a.Path != nil {
			// Link by full path when another note shares the title.
			same, err := q.FindNotesByTitle(ctx, store.FindNotesByTitleParams{WorkspaceID: ws, Title: a.Title})
			if err != nil {
				return nil, "", err
			}
			if len(same) > 1 {
				return []string{a.Title}, *a.Path, nil
			}
		}
		return []string{a.Title}, a.Title, nil
	case links.KindTicket:
		t, err := q.GetTicketByID(ctx, ref.ID)
		if err != nil || t.WorkspaceID != ws {
			return nil, "", apperr.ErrNotFound
		}
		return []string{t.Key}, t.Key, nil
	case links.KindEpic:
		e, err := q.GetEpic(ctx, store.GetEpicParams{ID: ref.ID, WorkspaceID: ws})
		if err != nil {
			return nil, "", err
		}
		return []string{e.Name}, e.Name, nil
	}
	return nil, "", nil // agents: names are too common to match as text
}

var codeOrLink = regexp.MustCompile("(?s)```.*?```|~~~.*?~~~|`[^`\n]*`|!?\\[\\[[^\\]\n]*\\]\\]|\\[[^\\]\n]*\\]\\([^)\n]*\\)|<[^>\n]*>")

// findMention returns the byte offset of the first mention of phrase that
// isn't already a link, in code, or part of a longer word; -1 if none.
func findMention(body, phrase string) int {
	if len(phrase) < 3 {
		return -1
	}
	masked := []byte(body)
	for _, m := range codeOrLink.FindAllStringIndex(body, -1) {
		for i := m[0]; i < m[1]; i++ {
			if masked[i] != '\n' {
				masked[i] = 0
			}
		}
	}
	lower := strings.ToLower(string(masked))
	needle := strings.ToLower(phrase)
	from := 0
	for {
		i := strings.Index(lower[from:], needle)
		if i < 0 {
			return -1
		}
		i += from
		end := i + len(needle)
		if boundary(lower, i-1, true) && boundary(lower, end, false) {
			return i
		}
		from = i + 1
	}
}

func boundary(s string, i int, before bool) bool {
	if i < 0 || i >= len(s) {
		return true
	}
	var r rune
	if before {
		r, _ = utf8.DecodeLastRuneInString(s[:i+1])
	} else {
		r, _ = utf8.DecodeRuneInString(s[i:])
	}
	return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == 0)
}

func lineAround(body string, at int) string {
	start := strings.LastIndexByte(body[:at], '\n') + 1
	end := strings.IndexByte(body[at:], '\n')
	if end < 0 {
		end = len(body)
	} else {
		end += at
	}
	line := strings.TrimSpace(body[start:end])
	line = strings.TrimLeft(line, "-+># ")
	line = strings.ReplaceAll(line, "**", "")
	if utf8.RuneCountInString(line) > 200 {
		r := []rune(line)
		line = string(r[:200]) + "…"
	}
	return line
}

// UnlinkedMentions finds sources that name the target in plain text but
// don't link to it.
func UnlinkedMentions(ctx context.Context, q *store.Queries, ws uuid.UUID, ref Ref, linked []Mention) ([]Mention, error) {
	phrases, _, err := Phrases(ctx, q, ws, ref)
	if err != nil {
		return nil, err
	}
	skip := map[uuid.UUID]bool{ref.ID: true}
	for _, m := range linked {
		skip[m.SourceID] = true
	}
	out := []Mention{}
	for _, p := range phrases {
		arts, err := q.MentionCandidatesArtifacts(ctx, store.MentionCandidatesArtifactsParams{WorkspaceID: ws, ExcludeID: ref.ID, Phrase: p})
		if err != nil {
			return nil, err
		}
		for _, a := range arts {
			if skip[a.ID] || a.BodyMd == nil {
				continue
			}
			if i := findMention(*a.BodyMd, p); i >= 0 {
				skip[a.ID] = true
				out = append(out, Mention{SourceKind: links.SourceArtifact, SourceID: a.ID,
					Source:   links.ArtifactTarget(store.Artifact{ID: a.ID, Type: a.Type, Title: a.Title, Path: a.Path, Number: a.Number}, ""),
					Snippets: []string{lineAround(*a.BodyMd, i)}})
			}
		}
		ts, err := q.MentionCandidatesTickets(ctx, store.MentionCandidatesTicketsParams{WorkspaceID: ws, ExcludeID: ref.ID, Phrase: p})
		if err != nil {
			return nil, err
		}
		for _, t := range ts {
			if skip[t.ID] {
				continue
			}
			if i := findMention(t.DescriptionMd, p); i >= 0 {
				skip[t.ID] = true
				out = append(out, Mention{SourceKind: links.SourceTicket, SourceID: t.ID,
					Source:   links.Target{Kind: links.KindTicket, ID: &t.ID, Title: t.Title, DisplayID: t.Key, Href: "/tickets/" + t.Key},
					Snippets: []string{lineAround(t.DescriptionMd, i)}})
			}
		}
	}
	return out, nil
}

// LinkMention turns the first unlinked mention of a target in a source into
// a [[link]]: a new version of a markdown artifact, or a description edit.
func (s *Service) LinkMention(ctx context.Context, ws, userID uuid.UUID, ref Ref, sourceKind string, sourceID uuid.UUID) error {
	var (
		body   string
		ticket store.Ticket
		art    store.Artifact
	)
	q := store.New(s.Pool)
	phrases, link, err := Phrases(ctx, q, ws, ref)
	if err != nil {
		return err
	}
	switch sourceKind {
	case links.SourceArtifact:
		if art, err = q.GetArtifact(ctx, store.GetArtifactParams{ID: sourceID, WorkspaceID: ws}); err != nil {
			return apperr.ErrNotFound
		}
		if body, _, err = current(ctx, q, art); err != nil {
			return err
		}
	case links.SourceTicket:
		if ticket, err = q.GetTicketByID(ctx, sourceID); err != nil || ticket.WorkspaceID != ws {
			return apperr.ErrNotFound
		}
		body = ticket.DescriptionMd
	default:
		return apperr.Invalid("source_kind", "only artifacts and ticket descriptions can be edited this way")
	}
	next := ""
	for _, p := range phrases {
		if i := findMention(body, p); i >= 0 {
			written := body[i : i+len(p)]
			repl := "[[" + link + "]]"
			if written != link {
				repl = "[[" + link + "|" + written + "]]"
			}
			next = body[:i] + repl + body[i+len(p):]
			break
		}
	}
	if next == "" {
		return apperr.Invalid("source", "no unlinked mention left to link")
	}
	note := fmt.Sprintf("Linked %s", link)
	if sourceKind == links.SourceTicket {
		_, err := s.Tracker.Update(ctx, ws, tracker.UserActor(userID), ticket.Key, tracker.Patch{DescriptionMd: &next})
		return err
	}
	if art.Type == "note" {
		_, err := s.SaveAsUser(ctx, ws, userID, Save{Path: *art.Path, Body: next, Note: note})
		return err
	}
	_, err = s.Artifacts.AddVersion(ctx, ws, artifacts.Actor{Type: "user", ID: userID}, art.ID, artifacts.NewVersion{Note: note, Content: artifacts.Content{BodyMD: &next}})
	return err
}

// LookupRef finds what a typed reference ("ADR-012", "PAY-214", a page
// title or path, "@atlas") names. Unresolved refs are ErrNotFound.
func LookupRef(ctx context.Context, q *store.Queries, ws uuid.UUID, s string) (Ref, links.Target, error) {
	r := links.ParseRef(s)
	if id, err := uuid.Parse(r.Target); err == nil {
		a, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: id, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return Ref{}, links.Target{}, apperr.ErrNotFound
		} else if err != nil {
			return Ref{}, links.Target{}, err
		}
		return Ref{Kind: links.KindArtifact, ID: id}, links.ArtifactTarget(a, sectionSlug(r.Anchor)), nil
	}
	t, err := links.Resolve(ctx, q, ws, r.Target, r.Anchor)
	if err != nil {
		return Ref{}, t, err
	}
	if t.Kind == links.KindUnresolved {
		return Ref{}, t, apperr.ErrNotFound
	}
	kind := t.Kind
	if kind == links.KindSection {
		kind = links.KindArtifact
	}
	return Ref{Kind: kind, ID: *t.ID}, t, nil
}

func sectionSlug(anchor string) string {
	if anchor == "" {
		return ""
	}
	return links.Slug(anchor)
}
