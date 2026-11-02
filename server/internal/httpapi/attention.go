package httpapi

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/links"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
	"github.com/liamphmurphy/lazyagents/server/internal/wiki"
)

func (s *Server) ListInbox(ctx context.Context, req api.ListInboxRequestObject) (api.ListInboxResponseObject, error) {
	u, _, ws := me(ctx)
	rows, err := s.Q.ListInbox(ctx, store.ListInboxParams{UserID: u.ID, WorkspaceID: ws, Project: req.Params.Project})
	if err != nil {
		return nil, err
	}
	out := api.ListInbox200JSONResponse{Items: make([]api.InboxItem, 0, len(rows))}
	for _, r := range rows {
		if r.Kind == "suggestion" {
			r.Href = links.NoteHref(strings.TrimPrefix(r.Href, "/wiki/"))
		}
		out.Items = append(out.Items, api.InboxItem{Id: r.ID, Kind: api.InboxItemKind(r.Kind), Title: r.Title, Summary: r.Summary, Href: r.Href, Project: r.Project, TicketKey: r.TicketKey, CreatedAt: r.CreatedAt, Resolved: r.Resolved, Read: r.Read})
	}
	return out, nil
}
func (s *Server) ReadInboxItem(ctx context.Context, req api.ReadInboxItemRequestObject) (api.ReadInboxItemResponseObject, error) {
	if req.Body == nil || len(req.Body.Id) == 0 || len(req.Body.Id) > 200 {
		return nil, &tracker.ValidationError{Field: "id", Msg: "invalid item id"}
	}
	u, _, _ := me(ctx)
	if err := s.Q.ReadInboxItem(ctx, store.ReadInboxItemParams{UserID: u.ID, ItemID: req.Body.Id}); err != nil {
		return nil, err
	}
	return api.ReadInboxItem204Response{}, nil
}
func (s *Server) Search(ctx context.Context, req api.SearchRequestObject) (api.SearchResponseObject, error) {
	_, _, ws := me(ctx)
	out := api.Search200JSONResponse{Hits: []api.SearchHit{}}
	text := strings.TrimSpace(req.Params.Q)
	if text == "" {
		return out, nil
	}
	if len(text) > 500 {
		return nil, &tracker.ValidationError{Field: "q", Msg: "query too long"}
	}
	var project *store.Project
	if req.Params.Project != nil && *req.Params.Project != "" {
		p, err := s.Q.GetProjectByKey(ctx, store.GetProjectByKeyParams{WorkspaceID: ws, Key: *req.Params.Project})
		if errors.Is(err, pgx.ErrNoRows) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		project = &p
	}
	// Reuse full-text ranking and snippets, and name matching for epics/agents.
	var hits []wiki.Hit
	var err error
	if project != nil {
		hits, err = wiki.Search(ctx, s.Q, ws, text, nil, &project.ID, 30)
	} else {
		hits, err = wiki.Search(ctx, s.Q, ws, text, nil, nil, 30)
	}
	if err != nil {
		return nil, err
	}
	for _, h := range hits {
		out.Hits = append(out.Hits, api.SearchHit{Target: toTarget(h.Target), Snippet: h.Snippet, Score: float32(h.Score)})
	}
	targets, err := s.Q.SearchAttentionTargets(ctx, store.SearchAttentionTargetsParams{WorkspaceID: ws, Q: text, Project: req.Params.Project})
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		id, href := t.ID, t.Href
		out.Hits = append(out.Hits, api.SearchHit{Target: api.LinkTarget{Kind: api.LinkTargetKind(t.Kind), Id: &id, Title: t.Title, Href: &href}, Snippet: "", Score: 0})
	}
	return out, nil
}
