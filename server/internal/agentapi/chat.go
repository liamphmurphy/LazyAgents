package agentapi

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// chatTools is the whole MCP surface of a chat run: project context and tracker actions.
// Everything else, including tools added later, is refused for chats.
var chatTools = []string{"whoami", "search_knowledge", "read_page", "read_artifact", "get_ticket", "get_backlinks", "create_ticket", "create_epic", "list_epics", "list_runs", "get_run",
	"search_memory", "read_memory", "save_memory", "update_memory", "forget_memory", "set_chat_title", "generate_image", "get_video_transcript", "propose_persona", "list_saved_links", "submit_research_report"}

// chatScope is the project a chat run may read from.
type chatScope struct {
	ChatID     uuid.UUID
	ProjectID  uuid.UUID
	ProjectKey string
	// UserID is the person the chat belongs to: the owner of its memory.
	UserID uuid.UUID
	// PersonaID is the persona answering the chat (its speaker, else its host),
	// the one persona memory it may reach.
	PersonaID *uuid.UUID
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
	return &chatScope{ChatID: chat.ID, ProjectID: chat.ProjectID, ProjectKey: p.Key, UserID: chat.CreatedBy, PersonaID: runs.ChatSpeakerID(chat)}, nil
}

// allowsArtifact: workspace-level pages (no project) are shared; the rest must
// belong to the chat's project. A nil scope (not a chat) allows everything.
func (s *chatScope) allowsArtifact(a store.Artifact) bool {
	return s == nil || a.ProjectID == nil || *a.ProjectID == s.ProjectID
}

func (s *chatScope) allowsTicket(t store.Ticket) bool { return s == nil || t.ProjectID == s.ProjectID }

const chatInstructions = `You are the person's Personal Assistant, chatting about project %s (chat %s). Answer directly.
Look things up with list_runs, get_run, get_ticket, list_epics, search_knowledge, read_page, read_artifact and get_backlinks; search first and open only what matters. Cite what you used with [[…]] links.
Use create_ticket and create_epic when asked and return their identifiers. Never claim tickets, execute project work, or delegate.
You keep a private memory of this person: find entries with search_memory and read_memory, save with save_memory (it updates near-duplicates), and tell the person whenever you save something. Never store credentials or tokens. Honour "forget that" with forget_memory, and save nothing when told "don't remember this". In a persona chat, about: "persona" reaches the persona's own memory (its identity and shared moments) instead.
set_chat_title names the conversation on its first message. When the person shares a YouTube link, read it with get_video_transcript before reacting to the video.`

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

type GenerateImageIn struct {
	Prompt     string   `json:"prompt" jsonschema:"A detailed description of the image to create (1–4000 characters)"`
	References []string `json:"references,omitempty" jsonschema:"Likenesses to keep: persona when the image shows you, person when it shows the person. Each uses the reference image set in the person's Library; omit when the image shows neither."`
}

// generateImage queues an image for the runner to make with Codex after this
// reply (see runs.Service.QueueChatImage).
func generateImage(ctx context.Context, d Deps, c agents.Caller, in GenerateImageIn) (any, error) {
	s, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, apperr.Conflict("generate_image only works in a chat with a person")
	}
	if _, err := d.Runs.QueueChatImage(ctx, c.Agent.WorkspaceID, *c.RunID, in.Prompt, in.References); err != nil {
		return nil, err
	}
	return map[string]any{"queued": true, "tell_the_person": "Queued. Codex tries to make the image right after your reply; it may still fail (for example if moderation refuses it). " +
		"Tell the person you're trying to make it; don't describe it as finished or promise it will appear. " +
		"You'll get a follow-up turn with the image to look at, or the reason it failed, to react to."}, nil
}

type ProposePersonaIn struct {
	Name         string `json:"name" jsonschema:"The new persona's name, 1–60 characters, unique among the person's personas; others mention it as @Name"`
	Tone         string `json:"tone,omitempty" jsonschema:"One of neutral, friendly, concise, formal, playful (default neutral)"`
	Color        string `json:"color,omitempty" jsonschema:"Its colour in the chat as #rrggbb (default #64748b)"`
	Instructions string `json:"instructions,omitempty" jsonschema:"Its style and focus, at most 2000 characters, written to it in the second person"`
	Reason       string `json:"reason,omitempty" jsonschema:"One sentence for the person on why it helps, at most 500 characters"`
}

// proposePersona asks the person to approve a new persona (see
// runs.Service.ProposePersona); only a persona allowed to create personas may.
func proposePersona(ctx context.Context, d Deps, c agents.Caller, in ProposePersonaIn) (any, error) {
	s, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, apperr.Conflict("propose_persona only works in a chat with a person")
	}
	p, err := d.Runs.ProposePersona(ctx, c.Agent.WorkspaceID, *c.RunID, runs.PersonaProposalInput{Name: in.Name, Tone: in.Tone, Color: in.Color, Instructions: in.Instructions, Reason: in.Reason})
	if err != nil {
		return nil, err
	}
	return map[string]any{"proposal_id": p.ID, "name": p.Name, "state": p.State, "tell_the_person": "Proposed. The person sees Approve and Decline in the chat; " + p.Name + " is only created if they approve. Don't describe it as created yet."}, nil
}
