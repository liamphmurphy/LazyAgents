package wiki

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/links"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// ContextPage is one wiki page a run starts with (PLAN.md §12.3).
type ContextPage struct {
	Target    links.Target `json:"target"`
	Ref       string       `json:"ref"`    // what to pass to read_page
	Reason    string       `json:"reason"` // why it's here
	Markdown  string       `json:"markdown"`
	Truncated bool         `json:"truncated,omitempty"`
}

// approxTokens is a rough count (4 bytes a token) for trimming context.
func approxTokens(s string) int { return (len(s) + 3) / 4 }

type candidate struct {
	a      store.Artifact
	reason string
}

// RunContext picks the wiki pages a run on ticket starts with, most
// relevant first, trimmed to mem.MaxTokens:
//   - index:  the agent's Memory page;
//   - linked: notes linked from the ticket (description, comments) and its
//     attached artifacts, then notes that link to them, the agent's own first;
//   - search: the top note hits for the ticket's title.
func RunContext(ctx context.Context, q *store.Queries, agent store.Agent, ticket store.Ticket, mem agentdef.Memory) ([]ContextPage, error) {
	if !mem.Enabled {
		return []ContextPage{}, nil
	}
	ws := agent.WorkspaceID
	var cands []candidate
	seen := map[uuid.UUID]bool{}
	add := func(a store.Artifact, reason string) {
		if a.Type != "note" || a.CurrentVersionID == nil || seen[a.ID] {
			return
		}
		seen[a.ID] = true
		cands = append(cands, candidate{a, reason})
	}
	note := func(id uuid.UUID) (store.Artifact, bool) {
		a, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: id, WorkspaceID: ws})
		return a, err == nil
	}

	idx, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: MemoryIndex(agent.Handle)})
	if err == nil {
		add(idx, "Your memory index")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	if mem.Inject == agentdef.InjectLinked || mem.Inject == agentdef.InjectSearch {
		attached, err := q.TicketArtifactIDs(ctx, ticket.ID)
		if err != nil {
			return nil, err
		}
		// Outgoing: notes the ticket and its artifacts link to.
		out, err := q.LinksFromTicket(ctx, &ticket.ID)
		if err != nil {
			return nil, err
		}
		for _, l := range out {
			if l.TargetID != nil && (l.TargetKind == links.KindArtifact || l.TargetKind == links.KindSection) {
				if a, ok := note(*l.TargetID); ok {
					add(a, "Linked from "+ticket.Key)
				}
			}
		}
		titles := map[uuid.UUID]string{}
		for _, aid := range attached {
			src, ok := note(aid)
			if !ok {
				continue
			}
			titles[aid] = displayName(src)
			ls, err := q.LinksFromSource(ctx, store.LinksFromSourceParams{SourceKind: links.SourceArtifact, SourceID: aid})
			if err != nil {
				return nil, err
			}
			for _, l := range ls {
				if l.TargetID != nil && (l.TargetKind == links.KindArtifact || l.TargetKind == links.KindSection) {
					if a, ok := note(*l.TargetID); ok {
						add(a, "Linked from "+titles[aid])
					}
				}
			}
		}
		// Incoming: notes that link to the ticket or its artifacts. The
		// agent's own memory goes first: it's what it learned last time.
		var mine, others []candidate
		in := []struct {
			ref  Ref
			name string
		}{{Ref{links.KindTicket, ticket.ID}, ticket.Key}}
		for _, aid := range attached {
			in = append(in, struct {
				ref  Ref
				name string
			}{Ref{links.KindArtifact, aid}, titles[aid]})
		}
		root := strings.ToLower(MemoryRoot(agent.Handle))
		for _, t := range in {
			bl, err := Backlinks(ctx, q, ws, t.ref)
			if err != nil {
				return nil, err
			}
			for _, m := range bl {
				if m.SourceKind != links.SourceArtifact || m.Source.ArtifactType != "note" {
					continue
				}
				a, ok := note(m.SourceID)
				if !ok {
					continue
				}
				c := candidate{a, "Links to " + t.name}
				if strings.HasPrefix(strings.ToLower(m.Source.Path), root) {
					mine = append(mine, c)
				} else {
					others = append(others, c)
				}
			}
		}
		for _, c := range append(mine, others...) {
			add(c.a, c.reason)
		}
	}

	if mem.Inject == agentdef.InjectSearch && strings.TrimSpace(ticket.Title) != "" {
		hits, err := Search(ctx, q, ws, orQuery(ticket.Title), []string{"note"}, nil, 5)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			if a, ok := note(*h.ID); ok {
				add(a, "Matches the ticket title")
			}
		}
	}

	budget := mem.MaxTokens
	pages := []ContextPage{}
	for _, c := range cands {
		if budget <= 0 {
			break
		}
		body, _, err := current(ctx, q, c.a)
		if err != nil {
			return nil, err
		}
		p := ContextPage{Target: links.ArtifactTarget(c.a, ""), Ref: *c.a.Path, Reason: c.reason, Markdown: body}
		if n := approxTokens(body); n > budget {
			cut := min(budget*4, len(body))
			for cut > 0 && !utf8.RuneStart(body[cut]) && cut < len(body) {
				cut--
			}
			if i := strings.LastIndexByte(body[:cut], '\n'); i > cut/2 {
				cut = i
			}
			p.Markdown, p.Truncated = body[:cut]+"\n\n…", true
		}
		budget -= approxTokens(p.Markdown)
		pages = append(pages, p)
		if p.Truncated {
			break // the budget is spent
		}
	}
	return pages, nil
}

// orQuery turns a title into an any-word web search ("a OR b OR c"), so a
// note sharing a few distinctive words still matches.
func orQuery(title string) string {
	var words []string
	for _, w := range strings.Fields(title) {
		w = strings.Trim(w, `"'.,:;()[]`)
		if len(w) > 3 {
			words = append(words, w)
		}
	}
	return strings.Join(words, " OR ")
}

func displayName(a store.Artifact) string {
	if id := artifacts.DisplayID(a.Type, a.Number); id != nil {
		return *id
	}
	return a.Title
}

// RecordContextReads logs each delivered page as one read by the agent
// (§6.5: files count as one read per file when the run starts).
func RecordContextReads(ctx context.Context, q *store.Queries, ws, agentID uuid.UUID, runID *uuid.UUID, pages []ContextPage) error {
	now := time.Now()
	for _, p := range pages {
		a, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: *p.Target.ID, WorkspaceID: ws})
		if err != nil {
			continue
		}
		if err := artifacts.RecordRead(ctx, q, a, agentID, runID, "", &now); err != nil {
			return err
		}
	}
	return nil
}
