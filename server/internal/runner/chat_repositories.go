package runner

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/liamphmurphy/vesper/server/internal/harness"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type gitDirKey struct{}

func gitDirectory(ctx context.Context) string { d, _ := ctx.Value(gitDirKey{}).(string); return d }
func inRepository(ctx context.Context, dir string) context.Context {
	if dir == "." {
		dir = ""
	}
	return context.WithValue(ctx, gitDirKey{}, dir)
}

func (r *Runner) chatRepositoryBases(ctx context.Context, c runCtx) map[string]string {
	out := map[string]string{}
	chat, err := r.q().ChatSessionForRun(ctx, &c.run.ID)
	if err != nil {
		return out
	}
	repos, err := r.q().ChatRepositories(ctx, chat.ID)
	if err != nil {
		return out
	}
	for _, repo := range repos {
		out[repo.RepoUrl] = r.workspaceBase(inRepository(ctx, repo.Directory), c.run.ID.String())
	}
	return out
}
func (r *Runner) chatRepositoryCodeChanges(ctx context.Context, c runCtx, bases map[string]string, legacy string, env map[string]string, res harness.Result, code int, waitErr error) []byte {
	if len(bases) == 0 {
		return r.chatCodeChanges(ctx, c, legacy, env, res, code, waitErr)
	}
	chat, err := r.q().ChatSessionForRun(ctx, &c.run.ID)
	if err != nil {
		return []byte("[]")
	}
	repos, err := r.q().ChatRepositories(ctx, chat.ID)
	if err != nil {
		return []byte("[]")
	}
	all := []CodeChange{}
	for _, repo := range repos {
		rc := inRepository(ctx, repo.Directory)
		base := bases[repo.RepoUrl]
		if base == "" {
			out, _, status, err := r.gitCmd(rc, c.run.ID.String(), nil, 1024, "config", "--get", "vesper.base")
			if err == nil && status == 0 {
				base = strings.TrimSpace(string(out))
			}
		}
		if !commitRe.MatchString(base) {
			continue
		}
		raw := r.chatCodeChanges(rc, c, base, env, res, code, waitErr)
		var changes []CodeChange
		_ = json.Unmarshal(raw, &changes)
		for i := range changes {
			changes[i].Repository = repo.RepoUrl
		}
		all = append(all, changes...)
	}
	raw, _ := json.Marshal(ApplyCodeChangeBudget(all))
	return raw
}
func (r *Runner) chatRepositoryBranches(ctx context.Context, c runCtx) map[string][]string {
	out := map[string][]string{}
	chat, err := r.q().ChatSessionForRun(ctx, &c.run.ID)
	if err != nil {
		return out
	}
	repos, err := r.q().ChatRepositories(ctx, chat.ID)
	if err != nil {
		return out
	}
	for _, repo := range repos {
		out[repo.RepoUrl] = r.chatPushedBranches(inRepository(ctx, repo.Directory), c)
	}
	return out
}
func (r *Runner) chatTestEnvironmentScript(ctx context.Context, c runCtx) string {
	chat, err := r.q().ChatSessionForRun(ctx, &c.run.ID)
	if err != nil || chat.RepoUrl == nil {
		return ""
	}
	repos, err := r.q().ChatRepositories(ctx, chat.ID)
	if err != nil {
		return ""
	}
	for _, repo := range repos {
		if repo.RepoUrl == *chat.RepoUrl {
			recipe, found := r.loadTestEnv(ctx, c, testEnvRepo(ctx, r.q(), c))
			infer := ""
			if !found {
				infer = testFilesScript()
			}
			return "(cd " + shellQuote(repo.Directory) + "\n" + dependencySetupScript() + "\n" + recipe.NixInstallScript() + infer + ")\n"
		}
	}
	return ""
}

// chatRepositoryNote is refreshed even when a harness session is resumed.
func chatRepositoryNote(ctx context.Context, q *store.Queries, chat store.ChatSession) (string, error) {
	repos, err := q.ChatRepositories(ctx, chat.ID)
	if err != nil {
		return "", err
	}
	sources, err := q.ListRepositorySources(ctx, store.ListRepositorySourcesParams{WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("\n## Repositories\nGitHub credential access determines which repositories you can work on. Saved sources are discovery shortcuts, never an allowlist. Use search_repositories to find names, and use_repository to attach or select another repository in this conversation. Run its setup_command before working there. Never guess when a short name is ambiguous.\nAll paths below are relative to the workspace root. Keep commands in the correct checkout; read its own AGENTS.md before changing it.\n")
	for _, repo := range repos {
		active := ""
		if chat.RepoUrl != nil && repo.RepoUrl == *chat.RepoUrl {
			active = " (active for this turn)"
		}
		b.WriteString("- " + repo.RepoUrl + " at " + repo.Directory + active + "; this chat's branches: " + strings.Join(repo.WorkBranches, ", ") + "\n")
	}
	for i, source := range sources {
		if i >= 30 {
			b.WriteString("More saved sources are available through search_repositories.\n")
			break
		}
		b.WriteString("- Saved " + source.Kind + ": " + source.Name + "\n")
	}
	if len(repos) == 0 {
		b.WriteString("No repository selected yet. You can use_repository now; coding credentials are available.\n")
	}
	return b.String(), nil
}
