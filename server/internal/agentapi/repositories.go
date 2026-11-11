package agentapi

import (
	"context"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/auth"
	"github.com/liamphmurphy/vesper/server/internal/repositories"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type SearchRepositoriesIn struct {
	Query   string `json:"query,omitempty" jsonschema:"Filter saved repositories and repositories under saved organizations"`
	Refresh bool   `json:"refresh,omitempty" jsonschema:"Refresh organization repository lists"`
}

func searchRepositories(ctx context.Context, d Deps, c agents.Caller, in SearchRepositoriesIn) (any, error) {
	scope, err := requireChat(ctx, d, c)
	if err != nil {
		return nil, err
	}
	entries, err := d.Runs.Repositories().Search(ctx, repositories.Owner{WorkspaceID: c.Agent.WorkspaceID, UserID: scope.UserID}, in.Query, in.Refresh)
	if err != nil {
		return nil, err
	}
	total := len(entries)
	if len(entries) > 100 {
		entries = entries[:100]
	}
	return map[string]any{"repositories": entries, "matches": total, "has_more": total > len(entries), "hint": "Use query to narrow large catalogues. An explicit owner/repository works without saving."}, nil
}

type UseRepositoryIn struct {
	Repository string `json:"repository" jsonschema:"Saved repository short name, owner/repository or GitHub URL. An ambiguous name requires the person's choice."`
}

func useRepository(ctx context.Context, d Deps, c agents.Caller, in UseRepositoryIn) (any, error) {
	scope, err := requireChat(ctx, d, c)
	if err != nil {
		return nil, err
	}
	if c.RunID == nil {
		return nil, apperr.Conflict("requires a running chat")
	}
	run, err := d.Q.GetRun(ctx, store.GetRunParams{ID: *c.RunID, WorkspaceID: c.Agent.WorkspaceID})
	if err != nil {
		return nil, err
	}
	spec, err := runs.Spec(run)
	if err != nil {
		return nil, err
	}
	if spec.Mode("repo:write") != agentdef.Allowed {
		return nil, apperr.Conflict("this turn cannot write code")
	}
	chat, err := d.Runs.SetChatRepository(ctx, c.Agent.WorkspaceID, scope.UserID, scope.ChatID, in.Repository, c.RunID)
	if err != nil {
		return nil, err
	}
	repos, err := d.Q.ChatRepositories(ctx, chat.ID)
	if err != nil {
		return nil, err
	}
	user, err := d.Q.GetUser(ctx, store.GetUserParams{ID: scope.UserID, WorkspaceID: c.Agent.WorkspaceID})
	if err != nil {
		return nil, err
	}
	for _, repo := range repos {
		if repo.RepoUrl == *chat.RepoUrl {
			return map[string]any{"repo_url": repo.RepoUrl, "directory": repo.Directory, "work_branches": repo.WorkBranches,
				"setup_command": repositories.SetupScript(repo.RepoUrl, repo.Directory, repo.WorkBranches, repo.CheckpointBranch, user.Name, auth.CommitEmail(user, chat.CommitEmail)),
				"next":          "Run setup_command from the workspace root before working in directory. If it fails, report the error and stop. Read this repository's AGENTS.md. Keep all git, build and test commands in its directory; other attached checkouts are preserved."}, nil
		}
	}
	return nil, apperr.ErrNotFound
}
