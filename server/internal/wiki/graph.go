package wiki

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Node is one thing in the link graph.
type Node struct {
	Key    string       `json:"key"` // kind:id
	Hop    int          `json:"hop"` // distance from the centre
	Target links.Target `json:"target"`
}

// Edge is a resolved link between two nodes (source → target).
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

const maxGraphNodes = 60

func nodeKey(kind string, id uuid.UUID) string { return kind + ":" + id.String() }

// LocalGraph is everything within depth (1–2) links of a centre node, in
// either direction, capped at 60 nodes nearest first.
func LocalGraph(ctx context.Context, q *store.Queries, ws uuid.UUID, centre Ref, depth int) ([]Node, []Edge, error) {
	depth = max(1, min(depth, 2))
	hop := map[string]int{nodeKey(centre.Kind, centre.ID): 0}
	ids := map[string]Ref{nodeKey(centre.Kind, centre.ID): centre}
	edges := map[Edge]bool{}
	frontier := []uuid.UUID{centre.ID}
	for d := 1; d <= depth && len(frontier) > 0; d++ {
		rows, err := q.GraphEdges(ctx, store.GraphEdgesParams{WorkspaceID: ws, Ids: frontier, Viewer: pageview.From(ctx).UserID, IncludeAgent: pageview.From(ctx).IncludeAgent})
		if err != nil {
			return nil, nil, err
		}
		frontier = nil
		for _, r := range rows {
			from, to := nodeKey(r.SourceKind, r.SourceID), nodeKey(r.TargetKind, r.TargetID)
			if from == to {
				continue
			}
			for _, n := range []struct {
				key string
				ref Ref
			}{{from, Ref{r.SourceKind, r.SourceID}}, {to, Ref{r.TargetKind, r.TargetID}}} {
				if _, seen := hop[n.key]; !seen {
					if len(hop) >= maxGraphNodes {
						continue
					}
					hop[n.key] = d
					ids[n.key] = n.ref
					frontier = append(frontier, n.ref.ID)
				}
			}
			_, okF := hop[from]
			_, okT := hop[to]
			if okF && okT {
				edges[Edge{from, to}] = true
			}
		}
	}

	targets, err := hydrate(ctx, q, ws, ids)
	if err != nil {
		return nil, nil, err
	}
	nodes := make([]Node, 0, len(hop))
	for key, h := range hop {
		t, ok := targets[key]
		if !ok {
			continue // deleted since the link was written
		}
		nodes = append(nodes, Node{Key: key, Hop: h, Target: t})
	}
	out := make([]Edge, 0, len(edges))
	for e := range edges {
		if _, ok := targets[e.From]; !ok {
			continue
		}
		if _, ok := targets[e.To]; !ok {
			continue
		}
		out = append(out, e)
	}
	return nodes, out, nil
}

func hydrate(ctx context.Context, q *store.Queries, ws uuid.UUID, refs map[string]Ref) (map[string]links.Target, error) {
	by := map[string][]uuid.UUID{}
	for _, r := range refs {
		by[r.Kind] = append(by[r.Kind], r.ID)
	}
	out := map[string]links.Target{}
	if ids := by[links.KindArtifact]; len(ids) > 0 {
		rows, err := q.ArtifactsByIDs(ctx, store.ArtifactsByIDsParams{WorkspaceID: ws, Ids: ids, Viewer: pageview.From(ctx).UserID, IncludeAgent: pageview.From(ctx).IncludeAgent})
		if err != nil {
			return nil, err
		}
		for _, a := range rows {
			out[nodeKey(links.KindArtifact, a.ID)] = links.ArtifactTarget(store.Artifact{ID: a.ID, Type: a.Type, Title: a.Title, Path: a.Path, Number: a.Number}, "")
		}
	}
	if ids := by[links.KindTicket]; len(ids) > 0 {
		rows, err := q.TicketsByIDs(ctx, store.TicketsByIDsParams{WorkspaceID: ws, Ids: ids})
		if err != nil {
			return nil, err
		}
		for _, t := range rows {
			out[nodeKey(links.KindTicket, t.ID)] = links.TicketTarget(store.Ticket{ID: t.ID, Key: t.Key, Title: t.Title, Status: t.Status})
		}
	}
	if ids := by[links.KindEpic]; len(ids) > 0 {
		rows, err := q.EpicsByIDs(ctx, store.EpicsByIDsParams{WorkspaceID: ws, Ids: ids})
		if err != nil {
			return nil, err
		}
		for _, e := range rows {
			out[nodeKey(links.KindEpic, e.ID)] = links.EpicTarget(store.Epic{ID: e.ID, Name: e.Name})
		}
	}
	if ids := by[links.KindAgent]; len(ids) > 0 {
		rows, err := q.AgentsByIDs(ctx, store.AgentsByIDsParams{WorkspaceID: ws, Ids: ids})
		if err != nil {
			return nil, err
		}
		for _, a := range rows {
			out[nodeKey(links.KindAgent, a.ID)] = links.AgentTarget(store.Agent{ID: a.ID, Name: a.Name, Handle: a.Handle, ColorIndex: a.ColorIndex})
		}
	}
	return out, nil
}

// ------------------------------------------------------------ search

// Hit is one knowledge search result.
type Hit struct {
	links.Target
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// Search is full-text search over artifacts (notes included) and tickets,
// with well-linked pages ranked higher. kinds filters by artifact type or
// "ticket"; empty means everything.
func Search(ctx context.Context, q *store.Queries, ws uuid.UUID, text string, kinds []string, projectID *uuid.UUID, limit int) ([]Hit, error) {
	return search(ctx, q, ws, text, kinds, projectID, limit, false)
}

// SearchFuzzy is Search for people typing: partial words and typos in titles
// match too, title matches score higher and finished tickets lower.
func SearchFuzzy(ctx context.Context, q *store.Queries, ws uuid.UUID, text string, projectID *uuid.UUID, limit int) ([]Hit, error) {
	return search(ctx, q, ws, text, nil, projectID, limit, true)
}

func search(ctx context.Context, q *store.Queries, ws uuid.UUID, text string, kinds []string, projectID *uuid.UUID, limit int, fuzzy bool) ([]Hit, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var types []string
	wantTickets := len(kinds) == 0
	for _, k := range kinds {
		if k == "ticket" {
			wantTickets = true
		} else {
			types = append(types, k)
		}
	}
	hits := []Hit{}
	if len(kinds) == 0 || len(types) > 0 {
		var tf []string
		if len(types) > 0 {
			tf = types
		}
		rows, err := q.SearchKnowledgeArtifacts(ctx, store.SearchKnowledgeArtifactsParams{
			Q: text, Fuzzy: fuzzy, WorkspaceID: ws, Types: tf, ProjectID: projectID, Lim: int32(limit), Viewer: pageview.From(ctx).UserID, IncludeAgent: pageview.From(ctx).IncludeAgent,
		})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			hits = append(hits, Hit{Target: links.ArtifactTarget(store.Artifact{ID: r.ID, Type: r.Type, Title: r.Title, Path: r.Path, Number: r.Number}, ""),
				Snippet: r.Snippet, Score: r.Score})
		}
	}
	if wantTickets {
		rows, err := q.SearchKnowledgeTickets(ctx, store.SearchKnowledgeTicketsParams{Q: text, Fuzzy: fuzzy, WorkspaceID: ws, ProjectID: projectID, Lim: int32(limit)})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			hits = append(hits, Hit{Target: links.TicketTarget(store.Ticket{ID: r.ID, Key: r.Key, Title: r.Title, Status: r.Status}),
				Snippet: r.Snippet, Score: r.Score})
		}
	}
	sortHits(hits)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func sortHits(h []Hit) {
	for i := 1; i < len(h); i++ { // tiny lists: insertion sort, stable
		for j := i; j > 0 && h[j].Score > h[j-1].Score; j-- {
			h[j], h[j-1] = h[j-1], h[j]
		}
	}
}

// Autocomplete powers [[ autocomplete.
func Autocomplete(ctx context.Context, q *store.Queries, ws uuid.UUID, text string) ([]links.Target, error) {
	rows, err := q.SuggestTargets(ctx, store.SuggestTargetsParams{WorkspaceID: ws, Q: &text, Viewer: pageview.From(ctx).UserID, IncludeAgent: pageview.From(ctx).IncludeAgent})
	if err != nil {
		return nil, err
	}
	out := make([]links.Target, 0, len(rows))
	for _, r := range rows {
		id := r.ID
		t := links.Target{ID: &id, Title: r.Label}
		switch r.Kind {
		case "note":
			t.Kind, t.ArtifactType, t.Path, t.Href = links.KindArtifact, "note", deref(r.Sub), links.NoteHref(deref(r.Sub))
		case "ticket":
			t.Kind, t.DisplayID, t.Title, t.Href = links.KindTicket, r.Label, deref(r.Sub), "/tickets/"+r.Label
		case "epic":
			t.Kind, t.Href = links.KindEpic, "/epics?epic="+id.String()
		case "agent":
			t.Kind, t.DisplayID, t.Title, t.Href = links.KindAgent, r.Label, deref(r.Sub), "/agents/"+r.Label[1:]
		default:
			t.Kind, t.ArtifactType, t.Href = links.KindArtifact, r.Kind, "/artifacts/"+id.String()
			if r.Kind == "adr" {
				t.DisplayID, t.Title = r.Label, deref(r.Sub)
			}
		}
		out = append(out, t)
	}
	return out, nil
}

// LinkText is what to write inside [[…]] for a target.
func LinkText(t links.Target) string {
	switch {
	case t.Kind == links.KindArtifact && t.ArtifactType == "note":
		return t.Path
	case t.DisplayID != "":
		return t.DisplayID
	}
	return fmt.Sprint(t.Title)
}
