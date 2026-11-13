package httpapi

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
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
		needsYou := inboxNeedsYou(r.Kind, r.Summary)
		if r.Kind == "mention" {
			r.Summary = clip(r.Summary, 240)
		}
		item := api.InboxItem{Id: r.ID, Kind: api.InboxItemKind(r.Kind), NeedsYou: needsYou, Title: r.Title, Summary: r.Summary, Href: r.Href, Project: r.Project, TicketKey: r.TicketKey, CreatedAt: r.CreatedAt, Resolved: r.Resolved, Read: r.Read, RunId: r.RunID, ArtifactId: r.ArtifactID}
		if r.AgentID != nil {
			item.Agent = &api.AgentRef{Id: *r.AgentID, Handle: r.AgentHandle, Name: r.AgentName, ColorIndex: int(r.AgentColorIndex)}
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// inboxNeedsYou sorts an inbox item into the tray's Needs you tab (true, and
// counted on the sidebar badge) or its Updates tab (false): news that asks the
// person nothing. A mention is news unless it asks a question, because a
// comment that asks "@you can you check X?" is a question like any await_human.
func inboxNeedsYou(kind, body string) bool {
	switch kind {
	case "scheduled", "merged":
		return false
	case "mention":
		return asksQuestion(body)
	}
	return true
}

var (
	codeFence  = regexp.MustCompile("(?s)```.*?(```|$)")
	inlineCode = regexp.MustCompile("`[^`\n]*`")
	quoteLine  = regexp.MustCompile(`(?m)^\s*>.*$`)
	linkURL    = regexp.MustCompile(`https?://\S*[^\s?.,;:!)]`)
)

// asksQuestion: the text has a question mark outside code, links and quoted
// lines (a URL's query string, a code sample or a quoted earlier message
// doesn't ask anything).
func asksQuestion(md string) bool {
	md = codeFence.ReplaceAllString(md, "")
	md = quoteLine.ReplaceAllString(md, "")
	md = inlineCode.ReplaceAllString(md, "")
	md = linkURL.ReplaceAllString(md, "")
	return strings.Contains(md, "?")
}

// clip shortens s to at most n runes.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
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
	ctx = pageview.IncludeAgents(ctx, deref(req.Params.IncludeAgent))
	u, _, ws := me(ctx)
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
	// Reuse full-text ranking and snippets, and name matching for everything else.
	var projectID *uuid.UUID
	if project != nil {
		projectID = &project.ID
	}
	hits, err := wiki.SearchFuzzy(ctx, s.Q, ws, text, projectID, 30)
	if err != nil {
		return nil, err
	}
	for _, h := range hits {
		out.Hits = append(out.Hits, api.SearchHit{Target: toTarget(h.Target), Snippet: h.Snippet, Score: float32(h.Score)})
	}
	targets, err := s.Q.SearchAttentionTargets(ctx, store.SearchAttentionTargetsParams{WorkspaceID: ws, UserID: u.ID, Q: text, Project: req.Params.Project})
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		id, href := t.ID, t.Href
		target := api.LinkTarget{Kind: api.LinkTargetKind(t.Kind), Id: &id, Title: t.Title, Href: &href}
		if t.Status != "" {
			status := t.Status
			target.Status = &status
		}
		out.Hits = append(out.Hits, api.SearchHit{Target: target, Snippet: "", Score: float32(t.Score)})
	}
	// Message text is the costliest search, so it waits for a real word.
	if len(text) < 3 {
		return out, nil
	}
	messages, err := s.Q.SearchChatMessages(ctx, store.SearchChatMessagesParams{Query: text, WorkspaceID: ws, UserID: u.ID, RowLimit: 8})
	if err != nil {
		return nil, err
	}
	for _, m := range messages {
		if m.Archived || (project != nil && (m.ProjectKey == nil || *m.ProjectKey != project.Key)) {
			continue
		}
		id, href := m.ID, "/chat/"+m.SessionID.String()+"?message="+m.ID.String()
		snippet := strings.NewReplacer("\ue000", "«", "\ue001", "»").Replace(m.Snippet)
		// Below title matches: a word somewhere in a conversation is weak evidence.
		out.Hits = append(out.Hits, api.SearchHit{Target: api.LinkTarget{Kind: api.LinkTargetKind("chat"), Id: &id, Title: m.ChatTitle, Href: &href}, Snippet: snippet, Score: 0.05})
	}
	return out, nil
}
