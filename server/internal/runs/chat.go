package runs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// ChatSpec freezes a conversation's read-only capabilities after project overrides.
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
	r.Instructions = "You are chatting with a person. Answer directly, use context tools to look things up, and cite sources with [[…]]. Do not claim tickets, execute project work, or write memory.\n\n" + r.Instructions
	return r
}

// ChatInput opens an idle conversation; its first message creates the segment run.
// Harness/model selection is resolved from the agent and project in this version.
type ChatInput struct {
	Project string
	AgentID uuid.UUID
	Title   string
	Message *ChatMessageInput
}

// ChatMessageInput is a single human prompt. Context attachment delivery is handled
// by the REST/context integration, which must validate project membership first.
type ChatMessageInput struct{ Body string }

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
		a, err := q.GetAgent(ctx, store.GetAgentParams{ID: in.AgentID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Invalid("agent_id", "unknown agent")
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
		out, err = q.CreateChatSession(ctx, store.CreateChatSessionParams{WorkspaceID: ws, ProjectID: p.ID, Title: in.Title, CreatedBy: user, AgentID: a.ID, Harness: spec.Harness, Model: spec.Model})
		if err != nil {
			return err
		}
		if in.Message != nil {
			out, _, err = postChatMessage(ctx, q, out, *in.Message)
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
		_, out, err = postChatMessage(ctx, q, chat, in)
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

func postChatMessage(ctx context.Context, q *store.Queries, chat store.ChatSession, in ChatMessageInput) (store.ChatSession, store.ChatMessage, error) {
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
	var run store.Run
	if chat.CurrentRunID != nil {
		run, err = q.LockRun(ctx, *chat.CurrentRunID)
		if err != nil {
			return chat, msg, err
		}
		if run.State != "waiting" {
			return chat, msg, apperr.Conflict("the chat run is not ready for a message")
		}
	} else {
		a, err := q.GetAgent(ctx, store.GetAgentParams{ID: chat.AgentID, WorkspaceID: chat.WorkspaceID})
		if err != nil {
			return chat, msg, err
		}
		spec, err := agents.Resolve(ctx, q, chat.WorkspaceID, a, p.Key)
		if err != nil {
			return chat, msg, err
		}
		// Selection is frozen at chat creation. Switching belongs to LAZYAGENTS-10.
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
	}
	state := "running"
	if run.WorkspaceVolume == nil {
		state = "queued"
	}
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: state}); err != nil {
		return chat, msg, err
	}
	turn, err := q.StartTurn(ctx, store.StartTurnParams{RunID: run.ID, Trigger: "comment", InputMessage: body})
	if err != nil {
		return chat, msg, err
	}
	appended, err := q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chat.ID, Role: "user", BodyMd: body, Harness: &run.Harness, Model: &run.Model, RunID: &run.ID, RunTurnID: &turn.ID, ContextRefs: []byte("[]"), Citations: []byte("[]")})
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
			return nil
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
