package runs

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/chatmodels"
	"github.com/liamphmurphy/lazyagents/server/internal/links"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// ChatSpec freezes a conversation's capabilities after project overrides.
func ChatSpec(r agentdef.Resolved) agentdef.Resolved {
	r.Scopes = append([]agentdef.ResolvedScope{}, r.Scopes...)
	for i := range r.Scopes {
		r.Scopes[i].Mode = agentdef.Denied
	}
	r.Setup = []string{}
	r.MCPServers = []agentdef.MCPServer{}
	r.HarnessOptions = map[string]string{"sandbox": "read-only", "readonly": "true"}
	r.Repository = nil
	r.Fallbacks = nil // A chat segment never switches harness or model implicitly.
	r.Instructions = "You are chatting with a person. Answer directly, use context tools to look things up, and cite sources with [[…]]. Use create_ticket, create_epic and list_epics for requested project changes. Do not claim tickets, execute project work, or write memory.\n\n" + r.Instructions
	return r
}

// ChatInput opens an idle conversation; its first message creates the segment run.
// AgentID may be the zero UUID, which picks the default agent (see DefaultChatAgent).
// An empty Harness/Model keeps what the agent and project resolve to.
type ChatInput struct {
	Project string
	AgentID uuid.UUID
	Title   string
	Harness string
	Model   string
	Message *ChatMessageInput
}

// ChatMessageInput is a single human prompt. A non-empty Harness/Model is
// validated against the catalog and stored on the chat; the runner applies it
// in LAZYAGENTS-10, so an existing segment keeps running its own harness/model.
// ContextRefs are resolved inside the chat's project and delivered to the turn.
type ChatMessageInput struct {
	Body        string
	Harness     string
	Model       string
	ContextRefs []ChatContextRef
}

// ChatCatalog is the harness/model catalog chats validate against.
func (s *Service) ChatCatalog() chatmodels.Catalog {
	return chatmodels.Catalog{RequireKey: s.RequireHarnessKey, Secrets: s.Secrets, Discover: s.ModelDiscovery}
}

// StartChat creates a chat and optionally queues its first message atomically.
func (s *Service) StartChat(ctx context.Context, ws, user uuid.UUID, in ChatInput) (store.ChatSession, error) {
	var out store.ChatSession
	in.Title = strings.TrimSpace(in.Title)
	if len([]rune(in.Title)) > 200 {
		return out, apperr.Invalid("title", "must be at most 200 characters")
	}
	err := s.tx(ctx, func(q *store.Queries) error {
		p, err := q.LockProjectByKey(ctx, store.LockProjectByKeyParams{WorkspaceID: ws, Key: in.Project})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Invalid("project", "unknown project")
		}
		if err != nil {
			return err
		}
		if p.Archived {
			return apperr.Conflict("restore the archived project before starting a chat")
		}
		var a store.Agent
		if in.AgentID == uuid.Nil {
			a, err = q.DefaultChatAgent(ctx, ws)
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.Invalid("agent_id", "the workspace has no main agent to chat with")
			}
		} else {
			a, err = q.GetAgent(ctx, store.GetAgentParams{ID: in.AgentID, WorkspaceID: ws})
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.Invalid("agent_id", "unknown agent")
			}
		}
		if err != nil {
			return err
		}
		if a.Kind == "sub" {
			return apperr.Invalid("agent_id", "pick a main agent")
		}
		spec, err := agents.Resolve(ctx, q, ws, a, p.Key)
		if err != nil {
			return err
		}
		h, m, err := s.ChatCatalog().Resolve(ctx, q, ws, spec.Harness, spec.Model, in.Harness, in.Model)
		if err != nil {
			return err
		}
		title := in.Title
		if title == "" && in.Message != nil {
			title = firstLine(in.Message.Body, 80)
		}
		out, err = q.CreateChatSession(ctx, store.CreateChatSessionParams{WorkspaceID: ws, ProjectID: p.ID, Title: title, CreatedBy: user, AgentID: a.ID, Harness: h, Model: m})
		if err != nil {
			return err
		}
		if in.Message != nil {
			out, _, err = postChatMessage(ctx, q, s.ChatCatalog(), out, *in.Message, s.HandoffChars)
		}
		return err
	})
	return out, err
}

// PostChatMessage serializes posts on the session row, including first-run creation.
// ConflictError is mapped to HTTP 409 by the shared API error handler.
func (s *Service) PostChatMessage(ctx context.Context, ws, user, id uuid.UUID, in ChatMessageInput) (store.ChatMessage, error) {
	var out store.ChatMessage
	err := s.tx(ctx, func(q *store.Queries) error {
		chat, err := lockChat(ctx, q, ws, user, id)
		if err != nil {
			return err
		}
		_, out, err = postChatMessage(ctx, q, s.ChatCatalog(), chat, in, s.HandoffChars)
		return err
	})
	return out, err
}

func lockChat(ctx context.Context, q *store.Queries, ws, user, id uuid.UUID) (store.ChatSession, error) {
	chat, err := q.LockChatSession(ctx, store.LockChatSessionParams{ID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && chat.CreatedBy != user) {
		return chat, apperr.ErrNotFound
	}
	return chat, err
}

func postChatMessage(ctx context.Context, q *store.Queries, cat chatmodels.Catalog, chat store.ChatSession, in ChatMessageInput, handoffChars int) (store.ChatSession, store.ChatMessage, error) {
	var msg store.ChatMessage
	body := strings.TrimSpace(in.Body)
	if body == "" || len([]rune(body)) > 100000 {
		return chat, msg, apperr.Invalid("body", "provide 1–100000 characters")
	}
	if chat.Archived || chat.State == "running" {
		return chat, msg, apperr.Conflict("the chat is archived or a turn is still running")
	}
	p, err := q.GetProject(ctx, store.GetProjectParams{ID: chat.ProjectID, WorkspaceID: chat.WorkspaceID})
	if err != nil {
		return chat, msg, err
	}
	if p.Archived {
		return chat, msg, apperr.Conflict("restore the archived project before chatting")
	}
	// Validate the selection and the refs before anything is written; a 422
	// leaves the stored selection unchanged (the whole transaction rolls back).
	h, m, err := cat.Resolve(ctx, q, chat.WorkspaceID, chat.Harness, chat.Model, in.Harness, in.Model)
	if err != nil {
		return chat, msg, err
	}
	docs, err := ResolveChatRefs(ctx, q, chat.WorkspaceID, chat.ProjectID, in.ContextRefs)
	if err != nil {
		return chat, msg, err
	}
	refs := make([]ChatContextRef, len(docs))
	for i, d := range docs {
		refs[i] = d.Ref
	}
	rawRefs, err := json.Marshal(refs)
	if err != nil {
		return chat, msg, err
	}
	if h != chat.Harness || m != chat.Model {
		if chat, err = q.SetChatSelection(ctx, store.SetChatSelectionParams{ID: chat.ID, Harness: h, Model: m}); err != nil {
			return chat, msg, err
		}
	}
	var run store.Run
	prompt := body
	fresh := chat.CurrentRunID == nil
	if !fresh {
		run, err = q.LockRun(ctx, *chat.CurrentRunID)
		if err != nil {
			return chat, msg, err
		}
		// Only an idle/waiting segment may switch; a busy one conflicts here.
		if run.State != "waiting" {
			return chat, msg, apperr.Conflict("the chat run is not ready for a message")
		}
		switch {
		case chat.Harness != run.Harness:
			// Every harness switch starts a fresh session, even back to an earlier
			// harness: the native session is stale. The runner sweep removes the
			// ended segment's workspace.
			outcome := "Chat switched to " + chat.Harness + "."
			if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "completed", Outcome: &outcome}); err != nil {
				return chat, msg, err
			}
			if err = event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "chat_segment_ended", "reason": "harness_switch", "harness": chat.Harness}); err != nil {
				return chat, msg, err
			}
			fresh = true
		case chat.Model != run.Model:
			// Same harness: keep the CLI session and workspace, change the model.
			if err = q.SetRunModel(ctx, store.SetRunModelParams{ID: run.ID, Model: chat.Model}); err != nil {
				return chat, msg, err
			}
			run.Model = chat.Model
		}
	}
	if fresh {
		a, err := q.GetAgent(ctx, store.GetAgentParams{ID: chat.AgentID, WorkspaceID: chat.WorkspaceID})
		if err != nil {
			return chat, msg, err
		}
		spec, err := agents.Resolve(ctx, q, chat.WorkspaceID, a, p.Key)
		if err != nil {
			return chat, msg, err
		}
		spec.Harness, spec.Model = chat.Harness, chat.Model
		spec = ChatSpec(spec)
		raw, err := json.Marshal(spec)
		if err != nil {
			return chat, msg, err
		}
		run, err = q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: chat.WorkspaceID, AgentID: a.ID, Mode: "managed", State: "queued", ResolvedSpec: raw, DefinitionVersionID: a.CurrentDefinitionVersionID, Harness: spec.Harness, Model: spec.Model, Purpose: "chat"})
		if err != nil {
			return chat, msg, err
		}
		if err = q.SetRunDispatcher(ctx, store.SetRunDispatcherParams{ID: run.ID, DispatchedByUserID: &chat.CreatedBy}); err != nil {
			return chat, msg, err
		}
		// A segment that follows earlier messages gets them as its first prompt.
		total, err := q.CountChatHistory(ctx, chat.ID)
		if err != nil {
			return chat, msg, err
		}
		if total > 0 {
			rows, err := q.ListRecentChatMessages(ctx, store.ListRecentChatMessagesParams{SessionID: chat.ID, RowLimit: handoffRows, MaxChars: handoffRowChars})
			if err != nil {
				return chat, msg, err
			}
			hist := make([]HandoffMessage, len(rows))
			for i, r := range rows {
				hist[i] = HandoffMessage{Seq: r.Seq, Role: r.Role, Harness: deref(r.Harness), Model: deref(r.Model), Body: r.BodyMd}
			}
			prompt = RenderHandoff(hist, int(total), handoffChars, body)
		}
	}
	state := "running"
	if run.WorkspaceVolume == nil {
		state = "queued"
	}
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: state}); err != nil {
		return chat, msg, err
	}
	turn, err := q.StartTurn(ctx, store.StartTurnParams{RunID: run.ID, Trigger: "comment", InputMessage: prompt})
	if err != nil {
		return chat, msg, err
	}
	appended, err := q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chat.ID, Role: "user", BodyMd: body, Harness: &run.Harness, Model: &run.Model, RunID: &run.ID, RunTurnID: &turn.ID, ContextRefs: rawRefs, Citations: []byte("[]")})
	if err != nil {
		return chat, msg, err
	}
	msg = store.ChatMessage(appended)
	chat, err = q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: chat.ID, CurrentRunID: &run.ID, State: "running"})
	if err != nil {
		return chat, msg, err
	}
	return chat, msg, event(ctx, q, run.ID, "lifecycle", map[string]any{"state": state, "purpose": "chat", "chat_id": chat.ID})
}

// CancelChat stops only the current turn. While its process exits, new posts
// conflict; the runner then returns the same segment and workspace to idle.
func (s *Service) CancelChat(ctx context.Context, ws, user, id uuid.UUID) (store.ChatSession, error) {
	var out store.ChatSession
	err := s.tx(ctx, func(q *store.Queries) error {
		chat, err := lockChat(ctx, q, ws, user, id)
		if err != nil {
			return err
		}
		out = chat
		if chat.State != "running" || chat.CurrentRunID == nil {
			return apperr.Conflict("no turn is running")
		}
		run, err := q.LockRun(ctx, *chat.CurrentRunID)
		if err != nil {
			return err
		}
		if run.State == "paused" {
			return nil
		} // cancellation is already draining setup/the turn
		// Resolve the claim boundary first. At READ COMMITTED, this UPDATE
		// waits for a winning claimant and rechecks its pending predicate;
		// the following SELECT gets a fresh snapshot and sees that running turn.
		// If we win, the row stays locked/done so a claimant cannot take it.
		if err = q.DropPendingTurns(ctx, run.ID); err != nil {
			return err
		}
		active, err := q.HasRunningChatTurn(ctx, run.ID)
		if err != nil {
			return err
		}
		state, reason := "waiting", "human_reply"
		if active || run.State == "starting" {
			state, reason = "paused", "pause"
		}
		if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: state, WaitReason: &reason}); err != nil {
			return err
		}
		if state == "waiting" {
			out, err = q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: chat.ID, CurrentRunID: &run.ID, State: "idle"})
			if err != nil {
				return err
			}
		}
		return event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "chat_turn_cancelled"})
	})
	return out, err
}

// ChatCitations is the JSON for a chat message's citations: what the run read
// since the turn began (the ChatCitation schema's target, plus the exact version
// and section). Reads are deduplicated by artifact, version and section.
func ChatCitations(ctx context.Context, q *store.Queries, runID uuid.UUID, since time.Time) ([]byte, error) {
	rows, err := q.ChatTurnCitations(ctx, store.ChatTurnCitationsParams{RunID: &runID, Since: since})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		anchor := ""
		if r.Section != nil {
			anchor = *r.Section
		}
		a := store.Artifact{ID: r.ArtifactID, Type: r.Type, Title: r.Title, Path: r.Path, Number: r.Number}
		out = append(out, map[string]any{
			"target": links.ArtifactTarget(a, anchor), "artifact_id": r.ArtifactID, "title": r.Title, "path": r.Path,
			"version_id": r.VersionID, "version": r.Version, "section": r.Section,
		})
	}
	return json.Marshal(out)
}

func firstLine(body string, max int) string {
	line, _, _ := strings.Cut(strings.TrimSpace(body), "\n")
	r := []rune(strings.TrimSpace(line))
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return string(r)
}

// ChatPatchInput is a revision-checked rename and/or archive change.
type ChatPatchInput struct {
	Revision int32
	Title    *string
	Archived *bool
}

// UpdateChat renames, archives or restores a chat. Archiving ends the chat's
// current run (the runner sweeps its workspace); a restored chat starts a new
// segment on its next message. 409 on a stale revision or while a turn runs.
func (s *Service) UpdateChat(ctx context.Context, ws, user, id uuid.UUID, in ChatPatchInput) (store.ChatSession, error) {
	var out store.ChatSession
	err := s.tx(ctx, func(q *store.Queries) error {
		chat, err := lockChat(ctx, q, ws, user, id)
		if err != nil {
			return err
		}
		title, archived := chat.Title, chat.Archived
		if in.Title != nil {
			title = strings.TrimSpace(*in.Title)
			if title == "" || len([]rune(title)) > 200 {
				return apperr.Invalid("title", "must be 1–200 characters")
			}
		}
		if in.Archived != nil {
			archived = *in.Archived
		}
		if in.Title == nil && in.Archived == nil {
			return apperr.Invalid("title", "provide a title or archived")
		}
		if chat.Revision != in.Revision {
			return apperr.Conflict("the chat changed; reload and try again")
		}
		if chat.State == "running" {
			return apperr.Conflict("a turn is still running")
		}
		if archived && !chat.Archived && chat.CurrentRunID != nil {
			run, err := q.LockRun(ctx, *chat.CurrentRunID)
			if err != nil {
				return err
			}
			if !slices.Contains(Terminal, run.State) {
				outcome := "The chat was archived."
				if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "completed", Outcome: &outcome}); err != nil {
					return err
				}
				if err = q.ExpireRunApprovals(ctx, run.ID); err != nil {
					return err
				}
				if err = event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "completed", "outcome": outcome}); err != nil {
					return err
				}
				if err = RecomputeAgentStatus(ctx, q, run.AgentID); err != nil {
					return err
				}
			}
			if _, err = q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: chat.ID, State: chat.State}); err != nil {
				return err
			}
		}
		out, err = q.UpdateChatTitleArchived(ctx, store.UpdateChatTitleArchivedParams{ID: chat.ID, Title: title, Archived: archived})
		return err
	})
	return out, err
}

// DeleteChat removes a chat and its messages. A running turn is cancelled
// first; the chat's runs stay for cost history.
func (s *Service) DeleteChat(ctx context.Context, ws, user, id uuid.UUID) error {
	return s.tx(ctx, func(q *store.Queries) error {
		chat, err := lockChat(ctx, q, ws, user, id)
		if err != nil {
			return err
		}
		if chat.CurrentRunID != nil {
			run, err := q.LockRun(ctx, *chat.CurrentRunID)
			if err != nil {
				return err
			}
			if !slices.Contains(Terminal, run.State) {
				if err = q.DropPendingTurns(ctx, run.ID); err != nil {
					return err
				}
				outcome := "The chat was deleted."
				if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "cancelled", Outcome: &outcome}); err != nil {
					return err
				}
				_ = q.EndOpenTurn(ctx, store.EndOpenTurnParams{RunID: run.ID, ExitCode: ptr[int32](143)})
				if err = q.ExpireRunApprovals(ctx, run.ID); err != nil {
					return err
				}
				if err = event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "cancelled", "outcome": outcome}); err != nil {
					return err
				}
				if err = RecomputeAgentStatus(ctx, q, run.AgentID); err != nil {
					return err
				}
			}
		}
		return q.DeleteChatSession(ctx, chat.ID)
	})
}
