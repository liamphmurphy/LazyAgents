package agentapi

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// chatTools is the whole MCP surface of a chat run: project context and tracker actions.
// Everything else, including tools added later, is refused for chats.
var chatTools = []string{"whoami", "search_knowledge", "read_page", "read_artifact", "get_ticket", "get_backlinks", "create_ticket", "create_epic", "list_epics",
	"search_memory", "read_memory", "save_memory", "update_memory", "forget_memory"}

// chatScope is the project a chat run may read from.
type chatScope struct {
	ChatID     uuid.UUID
	ProjectID  uuid.UUID
	ProjectKey string
	// UserID is the person the chat belongs to: the owner of its memory.
	UserID uuid.UUID
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
	return &chatScope{ChatID: chat.ID, ProjectID: chat.ProjectID, ProjectKey: p.Key, UserID: chat.CreatedBy}, nil
}

// allowsArtifact: workspace-level pages (no project) are shared; the rest must
// belong to the chat's project. A nil scope (not a chat) allows everything.
func (s *chatScope) allowsArtifact(a store.Artifact) bool {
	return s == nil || a.ProjectID == nil || *a.ProjectID == s.ProjectID
}

func (s *chatScope) allowsTicket(t store.Ticket) bool { return s == nil || t.ProjectID == s.ProjectID }

const chatInstructions = `You are answering a person in a chat about project %s (chat %s). Answer directly.
Look things up with search_knowledge, read_page, read_artifact, get_ticket and get_backlinks, and cite what you used with [[…]] links.
Use create_ticket, create_epic and list_epics to manage work in this project when asked. Never claim tickets, execute project work, or delegate. Return links or identifiers for anything you create.
You keep a private memory of this person: find entries with search_memory and read_memory, save with save_memory (it updates near-duplicates), and tell the person whenever you save something. Never store credentials or tokens. Honour "forget that" with forget_memory, and save nothing when told "don't remember this".`

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

type NewEpicIn struct {
	Name           string   `json:"name"`
	Objective      string   `json:"objective,omitempty"`
	RequiresMockup bool     `json:"requires_mockup,omitempty" jsonschema:"Tickets in the epic need a person-approved UI mockup before they can be finished"`
	Criteria       []string `json:"criteria,omitempty" jsonschema:"Optional final acceptance criteria for the epic, one checkable sentence each"`
}

// chatEpic is a list_epics row: the epic plus its status, points and criteria counts.
type chatEpic struct {
	store.Epic
	CriteriaTotal int32
	CriteriaDone  int32
	PointsTotal   int32
	PointsDone    int32
	TicketsTotal  int32
	TicketsOpen   int32
}

func listChatEpics(ctx context.Context, d Deps, c agents.Caller, _ Empty) (any, error) {
	s, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, apperr.Conflict("this tool requires a chat")
	}
	rows, err := d.Q.ListEpics(ctx, c.Agent.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := []chatEpic{}
	ids := []uuid.UUID{}
	for _, e := range rows {
		if e.ProjectID == s.ProjectID {
			out = append(out, chatEpic{Epic: e})
			ids = append(ids, e.ID)
		}
	}
	rolls, err := d.Q.EpicRollups(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]store.EpicRollupsRow, len(rolls))
	for _, r := range rolls {
		byID[r.EpicID] = r
	}
	for i := range out {
		r := byID[out[i].ID]
		out[i].CriteriaTotal, out[i].CriteriaDone = r.CriteriaTotal, r.CriteriaDone
		out[i].PointsTotal, out[i].PointsDone = r.PointsTotal, r.PointsDone
		out[i].TicketsTotal, out[i].TicketsOpen = r.TicketsTotal, r.TicketsOpen
	}
	return map[string]any{"epics": out}, nil
}

func createChatEpic(ctx context.Context, d Deps, c agents.Caller, in NewEpicIn) (any, error) {
	s, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, apperr.Conflict("this tool requires a chat")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > 200 {
		return nil, apperr.Invalid("name", "must be 1–200 characters")
	}
	p, err := d.Q.GetProject(ctx, store.GetProjectParams{ID: s.ProjectID, WorkspaceID: c.Agent.WorkspaceID})
	if err != nil {
		return nil, err
	}
	if p.Archived {
		return nil, apperr.Conflict("restore the archived project before creating an epic")
	}
	e, err := d.Tracker.CreateEpic(ctx, store.CreateEpicParams{WorkspaceID: c.Agent.WorkspaceID, ProjectID: s.ProjectID, Name: name, Objective: in.Objective, Health: "on_track", RequiresMockup: in.RequiresMockup}, in.Criteria)
	if err != nil {
		return nil, err
	}
	return map[string]any{"epic_id": e.ID, "name": e.Name, "criteria": len(in.Criteria)}, nil
}
