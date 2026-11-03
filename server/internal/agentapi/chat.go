package agentapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// chatTools is the whole MCP surface of a chat run: read-only context lookups.
// Everything else, including tools added later, is refused for chats.
var chatTools = []string{"whoami", "search_knowledge", "read_page", "read_artifact", "get_ticket", "get_backlinks"}

// chatScope is the project a chat run may read from.
type chatScope struct {
	ChatID     uuid.UUID
	ProjectID  uuid.UUID
	ProjectKey string
}

// chatScopeFor is nil unless the caller's token belongs to a chat run.
func chatScopeFor(ctx context.Context, d Deps, c agents.Caller) (*chatScope, error) {
	if c.RunID == nil {
		return nil, nil
	}
	run, err := d.Q.GetRun(ctx, store.GetRunParams{ID: *c.RunID, WorkspaceID: c.Agent.WorkspaceID})
	if err != nil {
		return nil, err
	}
	if run.Purpose != "chat" {
		return nil, nil
	}
	chat, err := d.Q.ChatSessionForRun(ctx, c.RunID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.Conflict("this chat run is no longer attached to a chat")
	}
	if err != nil {
		return nil, err
	}
	p, err := d.Q.GetProject(ctx, store.GetProjectParams{ID: chat.ProjectID, WorkspaceID: chat.WorkspaceID})
	if err != nil {
		return nil, err
	}
	return &chatScope{ChatID: chat.ID, ProjectID: chat.ProjectID, ProjectKey: p.Key}, nil
}

// allowsArtifact: workspace-level pages (no project) are shared; the rest must
// belong to the chat's project. A nil scope (not a chat) allows everything.
func (s *chatScope) allowsArtifact(a store.Artifact) bool {
	return s == nil || a.ProjectID == nil || *a.ProjectID == s.ProjectID
}

func (s *chatScope) allowsTicket(t store.Ticket) bool { return s == nil || t.ProjectID == s.ProjectID }

const chatInstructions = `You are answering a person in a chat about project %s (chat %s). Answer directly.
Look things up with search_knowledge, read_page, read_artifact, get_ticket and get_backlinks, and cite what you used with [[…]] links.
Never claim tickets, plan, delegate, or edit or write anything: this chat is read-only, and its other tools are unavailable.`

// sourceVisible reports whether a link target or backlink source of this kind
// (artifact, ticket, comment) is in the chat's project or workspace-wide.
// Kinds without a project (epics, agents) are visible.
func (s *chatScope) sourceVisible(ctx context.Context, d Deps, kind string, id uuid.UUID) (bool, error) {
	switch kind {
	case "artifact", "section", "ticket", "comment":
	default:
		return true, nil
	}
	if kind == "section" {
		kind = "artifact"
	}
	return d.Q.LinkSourceInProject(ctx, store.LinkSourceInProjectParams{SourceKind: kind, SourceID: id, ProjectID: s.ProjectID})
}
