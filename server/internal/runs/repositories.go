package runs

import (
	"context"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/repositories"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func (s *Service) Repositories() *repositories.Service {
	if s.RepositoryCatalog != nil {
		return s.RepositoryCatalog
	}
	catalog := &repositories.Service{Q: store.New(s.Pool)}
	if s.Secrets != nil {
		catalog.Secrets = s.Secrets
	}
	return catalog
}

// SetChatRepository attaches a checkout without replacing the conversation.
// Browser changes during a turn are queued; a tool call explicitly selects
// a repository for the running agent and may take effect immediately.
func (s *Service) SetChatRepository(ctx context.Context, ws, user, id uuid.UUID, requested string, fromRun *uuid.UUID) (out store.ChatSession, err error) {
	// Check ownership and coding before any credential-backed name lookup.
	err = s.tx(ctx, func(q *store.Queries) error {
		chat, e := lockChat(ctx, q, ws, user, id)
		if e != nil {
			return e
		}
		caps, e := chatCaps(ctx, q, chat)
		if e != nil {
			return e
		}
		if !caps.Code {
			return apperr.Conflict("this persona cannot write code")
		}
		if chat.Archived {
			return apperr.Conflict("restore the chat before selecting a repository")
		}
		return nil
	})
	if err != nil {
		return
	}
	url, e := s.Repositories().Resolve(ctx, repositories.Owner{WorkspaceID: ws, UserID: user}, requested)
	if e != nil {
		return out, e
	}
	err = s.tx(ctx, func(q *store.Queries) error {
		chat, e := lockChat(ctx, q, ws, user, id)
		if e != nil {
			return e
		}
		caps, e := chatCaps(ctx, q, chat)
		if e != nil {
			return e
		}
		if !caps.Code {
			return apperr.Conflict("this persona cannot write code")
		}
		if fromRun != nil && (chat.CurrentRunID == nil || *chat.CurrentRunID != *fromRun || chat.State != "running") {
			return apperr.Conflict("this run is no longer answering the chat")
		}
		attached, e := q.ChatRepositories(ctx, id)
		if e != nil {
			return e
		}
		for _, repo := range attached {
			canonical, err := repositories.Canonical(repo.RepoUrl)
			if err == nil && canonical == url {
				url = repo.RepoUrl
				break
			}
		}
		if len(attached) >= 50 {
			found := false
			for _, repo := range attached {
				found = found || repo.RepoUrl == url
			}
			if !found {
				return apperr.Conflict("a chat can attach at most 50 repositories")
			}
		}
		if _, e = q.AttachChatRepository(ctx, store.AttachChatRepositoryParams{ChatID: id, RepoUrl: url, Directory: repositories.Directory(url)}); e != nil {
			return e
		}
		if chat.State == "running" && fromRun == nil {
			out, e = q.QueueChatRepository(ctx, store.QueueChatRepositoryParams{ID: id, PendingRepoUrl: &url})
		} else {
			if fromRun == nil {
				if e = q.ClearPendingChatRepository(ctx, id); e != nil {
					return e
				}
			}
			out, e = q.ActivateChatRepository(ctx, store.ActivateChatRepositoryParams{ID: id, RepoUrl: url})
		}
		return e
	})
	return
}
