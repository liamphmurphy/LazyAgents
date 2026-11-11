package runs

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/playground"
	"github.com/liamphmurphy/vesper/server/internal/repositories"
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
	if spec.Repository != "" {
		url, e := s.Repositories().Resolve(ctx, repositories.Owner{WorkspaceID: ws, UserID: chat.CreatedBy}, spec.Repository)
		if e != nil {
			return store.Playground{}, e
		}
		attached, e := q.ChatRepositories(ctx, chat.ID)
		if e != nil {
			return store.Playground{}, e
		}
		found := false
		for _, repo := range attached {
			canonical, _ := repositories.Canonical(repo.RepoUrl)
			if canonical == url {
				chat.RepoUrl = &repo.RepoUrl
				found = true
				break
			}
		}
		if !found {
			return store.Playground{}, apperr.Invalid("repository", "attach this repository with use_repository before starting its playground")
		}
	}
	if chat.RepoUrl != nil {
		attached, e := q.ChatRepositories(ctx, chat.ID)
		if e != nil {
			return store.Playground{}, e
		}
		for _, repo := range attached {
			if repo.RepoUrl == *chat.RepoUrl {
				spec.Repository = repo.RepoUrl
				spec.Directory = repo.Directory
				break
			}
		}
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
	pg, err := q.StartChatPlayground(ctx, store.StartChatPlaygroundParams{WorkspaceID: ws, UserID: chat.CreatedBy, ChatID: chat.ID, RunID: &run.ID, Spec: b})
	if err != nil {
		return pg, err
	}
	return pg, savePlaygroundSpec(ctx, q, ws, chat, spec)
}

// savePlaygroundSpec keeps the spec a repository's playground started with,
// so a person can start one in any chat on it. An inferred document lives in
// this chat's workspace only, so it isn't kept.
func savePlaygroundSpec(ctx context.Context, q *store.Queries, ws uuid.UUID, chat store.ChatSession, spec playground.Spec) error {
	if chat.RepoUrl == nil {
		return nil
	}
	if spec.OpenAPIInferred {
		spec.OpenAPI, spec.OpenAPIInferred, spec.OpenAPICommit = "", false, ""
	}
	spec.Directory = ""
	b, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	return q.SavePlaygroundSpec(ctx, store.SavePlaygroundSpecParams{WorkspaceID: ws, Repo: testenv.RepoKey(*chat.RepoUrl), Spec: b, ChatID: &chat.ID})
}

// SavedPlaygroundSpec is the spec a chat's repository last ran a playground
// with, if the chat can run one; found is false otherwise.
func (s *Service) SavedPlaygroundSpec(ctx context.Context, chat store.ChatSession) (spec playground.Spec, found bool, err error) {
	if chat.RepoUrl == nil {
		return spec, false, nil
	}
	q := store.New(s.Pool)
	caps, err := chatCaps(ctx, q, chat)
	if err != nil || !caps.Code {
		return spec, false, err
	}
	row, err := q.GetPlaygroundSpec(ctx, store.GetPlaygroundSpecParams{WorkspaceID: chat.WorkspaceID, Repo: testenv.RepoKey(*chat.RepoUrl)})
	if errors.Is(err, pgx.ErrNoRows) {
		return spec, false, nil
	}
	if err != nil {
		return spec, false, err
	}
	if json.Unmarshal(row.Spec, &spec) != nil || spec.Validate() != nil {
		return playground.Spec{}, false, nil // saved under rules that changed
	}
	attached, e := q.ChatRepositories(ctx, chat.ID)
	if e != nil {
		return spec, false, e
	}
	for _, repo := range attached {
		if repo.RepoUrl == *chat.RepoUrl {
			spec.Repository = repo.RepoUrl
			spec.Directory = repo.Directory
			break
		}
	}
	return spec, true, nil
}

// SetPlaygroundOpenAPI points a chat's playground at an OpenAPI document,
// such as one the persona wrote from the code (inferred, at commit). The
// runner holding it picks the change up without a restart.
func (s *Service) SetPlaygroundOpenAPI(ctx context.Context, ws, runID uuid.UUID, path string, inferred bool, commit string) (store.Playground, error) {
	pg, err := s.PlaygroundForRun(ctx, ws, runID)
	if err != nil {
		return pg, err
	}
	var spec playground.Spec
	if err := json.Unmarshal(pg.Spec, &spec); err != nil {
		return pg, err
	}
	spec.OpenAPI, spec.OpenAPIInferred, spec.OpenAPICommit = path, inferred, commit
	if err := spec.Validate(); err != nil {
		var inv *playground.InvalidError
		if errors.As(err, &inv) {
			return pg, apperr.Invalid("openapi", "%s", inv.Msg)
		}
		return pg, err
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return pg, err
	}
	return store.New(s.Pool).SetPlaygroundSpec(ctx, store.SetPlaygroundSpecParams{ID: pg.ID, Spec: b})
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
	if errors.Is(err, pgx.ErrNoRows) && action == PlaygroundStart {
		return s.startSavedPlayground(ctx, q, ws, chatID)
	}
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

// startSavedPlayground starts a chat's first playground from the spec its
// repository last ran with, without an agent turn.
func (s *Service) startSavedPlayground(ctx context.Context, q *store.Queries, ws, chatID uuid.UUID) (store.Playground, error) {
	chat, err := q.GetChatSession(ctx, store.GetChatSessionParams{ID: chatID, WorkspaceID: ws})
	if err != nil {
		return store.Playground{}, err
	}
	spec, found, err := s.SavedPlaygroundSpec(ctx, chat)
	if err != nil {
		return store.Playground{}, err
	}
	if !found {
		return store.Playground{}, pgx.ErrNoRows // nothing to start
	}
	runID, err := s.liveChatWorkspace(ctx, q, ws, chatID)
	if err != nil {
		return store.Playground{}, err
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return store.Playground{}, err
	}
	return q.StartChatPlayground(ctx, store.StartChatPlaygroundParams{WorkspaceID: ws, UserID: chat.CreatedBy, ChatID: chat.ID, RunID: &runID, Spec: b})
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
		return uuid.Nil, apperr.Conflict("This chat has no workspace yet. Send a message first, then start the playground.")
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
