package agentapi

import (
	"context"
	"strings"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// The person's Inbox from a chat, read-only: a persona can answer "what needs
// me?" but every decision stays on the item's own card, and reading here
// doesn't mark anything read.

const (
	listInboxDefault = 30
	listInboxMax     = 100
)

type ListInboxIn struct {
	Project         string `json:"project,omitempty" jsonschema:"Only items in this project key, e.g. PAY"`
	Kind            string `json:"kind,omitempty" jsonschema:"Only one kind, e.g. approval, question, review, blocked, mockup, suggestion, scheduled or workflow_gate"`
	IncludeResolved bool   `json:"include_resolved,omitempty" jsonschema:"Also list items already decided or done"`
	Limit           int    `json:"limit,omitempty" jsonschema:"Up to 100, default 30"`
}

type inboxItem struct {
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Summary  string `json:"summary,omitempty"`
	Href     string `json:"href"`
	Project  string `json:"project,omitempty"`
	Ticket   string `json:"ticket,omitempty"`
	Agent    string `json:"agent,omitempty"`
	At       string `json:"at"`
	Read     bool   `json:"read"`
	Resolved bool   `json:"resolved,omitempty"`
}

func listInbox(ctx context.Context, d Deps, c agents.Caller, in ListInboxIn) (any, error) {
	s, err := personChatScope(ctx, d, c, "list_inbox")
	if err != nil {
		return nil, err
	}
	if in.Limit > listInboxMax {
		return nil, apperr.Invalid("limit", "at most %d", listInboxMax)
	}
	if in.Limit <= 0 {
		in.Limit = listInboxDefault
	}
	arg := store.ListInboxParams{WorkspaceID: c.Agent.WorkspaceID, UserID: s.UserID}
	if key := strings.ToUpper(strings.TrimSpace(in.Project)); key != "" {
		if _, err := projectFilter(ctx, d, c.Agent.WorkspaceID, key); err != nil {
			return nil, err
		}
		arg.Project = &key
	}
	rows, err := d.Q.ListInbox(ctx, arg)
	if err != nil {
		return nil, err
	}
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	out := []inboxItem{}
	unread := 0
	for _, r := range rows {
		unread = int(r.UnreadCount)
		if (r.Resolved && !in.IncludeResolved) || (kind != "" && r.Kind != kind) {
			continue
		}
		if len(out) == in.Limit {
			break
		}
		out = append(out, inboxItem{Kind: r.Kind, Title: r.Title, Summary: r.Summary, Href: r.Href, Project: r.Project, Ticket: r.TicketKey,
			Agent: r.AgentHandle, At: r.CreatedAt.Format(time.RFC3339), Read: r.Read, Resolved: r.Resolved})
	}
	return map[string]any{"items": out, "unread": unread, "href": "/inbox"}, nil
}
