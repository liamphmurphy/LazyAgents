package runs

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/playground"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testenv"
)

// StartChatPlayground records what a coding chat's playground runs and asks
// a runner to start it (or restart it with this spec). Services left out are
// the repository's test environment's.
func (s *Service) StartChatPlayground(ctx context.Context, ws, runID uuid.UUID, spec playground.Spec) (store.Playground, error) {
	q := store.New(s.Pool)
	run, err := q.GetRun(ctx, store.GetRunParams{ID: runID, WorkspaceID: ws})
	if err != nil {
		return store.Playground{}, err
	}
	if run.Purpose != "chat" || run.State != "running" {
		return store.Playground{}, apperr.Conflict("playgrounds only start from a chat answering a message")
	}
	chat, err := q.ChatSessionForRun(ctx, &run.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Playground{}, apperr.Conflict("this chat run is no longer attached to a chat")
	}
	if err != nil {
		return store.Playground{}, err
	}
	caps, err := chatCaps(ctx, q, chat)
	if err != nil {
		return store.Playground{}, err
	}
	if !caps.Code {
		return store.Playground{}, apperr.Conflict("A playground needs a chat that can run code; tell the person this chat can't.")
	}
	// What the agent leaves out comes from the repository's test
	// environment: the services its tests use, and the tools agents installed.
	if (spec.Services == nil || spec.NixPackages == nil) && chat.RepoUrl != nil {
		row, err := q.GetTestEnvironment(ctx, store.GetTestEnvironmentParams{WorkspaceID: ws, Repo: testenv.RepoKey(*chat.RepoUrl)})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return store.Playground{}, err
		}
		var rec testenv.Recipe
		if err == nil && json.Unmarshal(row.Recipe, &rec) == nil && rec.Validate() == nil {
			if spec.Services == nil {
				spec.Services = rec.Services
			}
			if spec.NixPackages == nil {
				spec.NixPackages = rec.NixPackages
			}
		}
	}
	if err := spec.Validate(); err != nil {
		var inv *playground.InvalidError
		if errors.As(err, &inv) {
			return store.Playground{}, apperr.Invalid("spec", "%s", inv.Msg)
		}
		return store.Playground{}, err
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return store.Playground{}, err
	}
	return q.StartChatPlayground(ctx, store.StartChatPlaygroundParams{WorkspaceID: ws, UserID: chat.CreatedBy, ChatID: chat.ID, RunID: &run.ID, Spec: b})
}

// PlaygroundForRun is the playground of the chat a run belongs to.
func (s *Service) PlaygroundForRun(ctx context.Context, ws, runID uuid.UUID) (store.Playground, error) {
	q := store.New(s.Pool)
	chat, err := q.ChatSessionForRun(ctx, &runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Playground{}, apperr.Conflict("playgrounds belong to chats")
	}
	if err != nil {
		return store.Playground{}, err
	}
	pg, err := q.GetChatPlayground(ctx, store.GetChatPlaygroundParams{ChatID: chat.ID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return pg, apperr.Conflict("this chat has no playground yet; start one with start_playground")
	}
	return pg, err
}

// Playground actions a person (or the agent, stop) can ask for.
const (
	PlaygroundStart = "start"
	PlaygroundReset = "reset"
	PlaygroundStop  = "stop"
)

// PlaygroundAction asks for a chat's playground to start again (with its last
// spec, in the chat's current workspace), reset its data, or stop.
func (s *Service) PlaygroundAction(ctx context.Context, ws, chatID uuid.UUID, action string) (store.Playground, error) {
	q := store.New(s.Pool)
	pg, err := q.GetChatPlayground(ctx, store.GetChatPlaygroundParams{ChatID: chatID, WorkspaceID: ws})
	if err != nil {
		return pg, err // pgx.ErrNoRows: the chat has none
	}
	switch action {
	case PlaygroundStop, PlaygroundReset:
		out, err := q.RequestPlaygroundAction(ctx, store.RequestPlaygroundActionParams{Action: &action, ChatID: chatID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			if action == PlaygroundReset {
				return pg, apperr.Conflict("the playground can only reset its data while it's running")
			}
			return pg, nil // already stopped
		}
		return out, err
	case PlaygroundStart:
		if pg.State == "starting" || pg.State == "running" {
			return pg, nil
		}
		runID, err := s.liveChatWorkspace(ctx, q, ws, chatID)
		if err != nil {
			return pg, err
		}
		return q.RestartStoppedPlayground(ctx, store.RestartStoppedPlaygroundParams{ChatID: chatID, WorkspaceID: ws, RunID: &runID})
	}
	return pg, apperr.Invalid("action", "must be start, reset or stop")
}

// liveChatWorkspace is the chat segment whose workspace a playground can run
// from: the current one, unless it ended (its workspace is then removed).
func (s *Service) liveChatWorkspace(ctx context.Context, q *store.Queries, ws, chatID uuid.UUID) (uuid.UUID, error) {
	chat, err := q.GetChatSession(ctx, store.GetChatSessionParams{ID: chatID, WorkspaceID: ws})
	if err != nil {
		return uuid.Nil, err
	}
	gone := apperr.Conflict("This chat's workspace was cleaned up after it went idle. Send a message and ask for the playground again.")
	if chat.CurrentRunID == nil {
		return uuid.Nil, gone
	}
	run, err := q.GetRun(ctx, store.GetRunParams{ID: *chat.CurrentRunID, WorkspaceID: ws})
	if err != nil {
		return uuid.Nil, err
	}
	switch run.State {
	case "completed", "failed", "cancelled":
		return uuid.Nil, gone
	}
	return run.ID, nil
}
