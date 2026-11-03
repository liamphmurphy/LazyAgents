package runner

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/secrets"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/wiki"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// BranchName is <handle>/<ticket-key>-<slug> (§6.5), at most ~60 characters.
func BranchName(handle, key, title string) string {
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(slug) > 40 {
		slug = strings.TrimRight(slug[:40], "-")
	}
	b := handle + "/" + strings.ToLower(key)
	if slug != "" {
		b += "-" + slug
	}
	return b
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// SSHKeyEnv is the env var the git:ssh secret is injected as. It is not
// GIT_SSH, which git itself reads as the path of an ssh program.
const SSHKeyEnv = "LAZYAGENTS_SSH_KEY"

// githubHostKeys pins GitHub's published SSH host keys so clones never
// trust-on-first-use (docs.github.com: GitHub's SSH key fingerprints).
const githubHostKeys = `github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl
github.com ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBEmKSENjQEezOmxkZMy7opKgwFB9nkt5YRrYMjNuG5N87uRgg6CLrbo5wAdT/y6v0mKV0U2w0WZ2YB/++Tpockg=
`

// sshSetup writes the pinned known_hosts (and the key when injected) under
// .lazyagents/ssh, which stays out of git, and points git at them. Without an
// injected key ssh falls back to the machine's own keys or agent, so a local
// runner keeps working with the user's login.
var sshSetup = `mkdir -p .lazyagents/ssh
chmod 700 .lazyagents/ssh
cat > .lazyagents/ssh/known_hosts <<'LAZYAGENTS_KNOWN_HOSTS'
` + githubHostKeys + `LAZYAGENTS_KNOWN_HOSTS
sshopts="-o UserKnownHostsFile=$PWD/.lazyagents/ssh/known_hosts -o StrictHostKeyChecking=yes -o HostKeyAlgorithms=ssh-ed25519,ecdsa-sha2-nistp256 -o BatchMode=yes"
if [ -n "${` + SSHKeyEnv + `:-}" ]; then
  (umask 077; printf '%s\n' "$` + SSHKeyEnv + `" > .lazyagents/ssh/id)
  sshopts="$sshopts -i $PWD/.lazyagents/ssh/id -o IdentitiesOnly=yes"
fi
export GIT_SSH_COMMAND="ssh $sshopts"
`

// SetupScript prepares a fresh workspace (§6.5): clone the project's repo
// (or start an empty one), create the run's branch, keep LazyAgents' files
// out of git, and run the project's setup commands. The working directory
// is the workspace; GH_TOKEN, when present, authenticates git and gh.
func SetupScript(repoURL, branch, authorName, authorEmail string, setup []string) string {
	return SetupScriptForBranch(repoURL, "", branch, authorName, authorEmail, setup)
}

// SetupScriptForBranch clones the approved base branch rather than the remote's HEAD.
func SetupScriptForBranch(repoURL, baseBranch, branch, authorName, authorEmail string, setup []string) string {
	var b strings.Builder
	b.WriteString("set -eu\n")
	// Everything is repo-local: on the local backend this is the user's own
	// machine, and their global git config must not change.
	b.WriteString(`helper='!f() { echo username=x-access-token; echo "password=$GH_TOKEN"; }; f'` + "\n")
	b.WriteString(sshSetup)
	b.WriteString("if [ ! -d .git ]; then\n")
	if repoURL != "" {
		b.WriteString("  echo '→ cloning " + strings.ReplaceAll(repoURL, "'", "") + "'\n")
		// The workspace already holds .lazyagents/ (ssh setup), so `git clone … .`
		// would refuse; init + fetch works in a non-empty directory.
		fetchRef := "HEAD"
		if baseBranch != "" {
			fetchRef = shellQuote(baseBranch)
		}
		b.WriteString("  git init --quiet .\n")
		b.WriteString("  git remote add origin " + shellQuote(repoURL) + "\n")
		b.WriteString(`  if [ -n "${GH_TOKEN:-}" ]; then git -c credential.helper= -c "credential.helper=$helper" fetch --quiet origin ` + fetchRef + "; else git fetch --quiet origin " + fetchRef + "; fi\n")
		b.WriteString("  git checkout --quiet --detach FETCH_HEAD\n")
	} else {
		b.WriteString("  echo '→ no repository configured for this project: starting an empty one'\n")
		b.WriteString("  git init --quiet -b main .\n")
	}
	b.WriteString("  git config user.name " + shellQuote(authorName) + "\n")
	b.WriteString("  git config user.email " + shellQuote(authorEmail) + "\n")
	b.WriteString(`  if [ -n "${GH_TOKEN:-}" ]; then git config credential.helper "$helper"; fi` + "\n")
	b.WriteString(`  git config core.sshCommand "$GIT_SSH_COMMAND"` + "\n")
	b.WriteString("  git checkout --quiet -B " + shellQuote(branch) + "\n")
	b.WriteString("fi\n")
	// Our files never get committed: .lazyagents/ is excluded and a tracked
	// AGENTS.md is marked skip-worktree while we write our section into it.
	b.WriteString("mkdir -p .lazyagents/context\n")
	b.WriteString("grep -qx '.lazyagents/' .git/info/exclude 2>/dev/null || echo '.lazyagents/' >> .git/info/exclude\n")
	b.WriteString("if [ -f AGENTS.md ] && ! grep -q 'lazyagents:start' AGENTS.md; then cp AGENTS.md .lazyagents/AGENTS.repo.md; fi\n")
	b.WriteString("if git ls-files --error-unmatch AGENTS.md >/dev/null 2>&1; then git update-index --skip-worktree AGENTS.md; else grep -qx 'AGENTS.md' .git/info/exclude || echo 'AGENTS.md' >> .git/info/exclude; fi\n")
	for _, c := range setup {
		b.WriteString("echo " + shellQuote("→ "+c) + "\n")
		b.WriteString(c + "\n")
	}
	b.WriteString("echo '→ workspace ready on branch " + strings.ReplaceAll(branch, "'", "") + "'\n")
	return b.String()
}

// SaveScript pushes the run's branch before its workspace is removed. It exits
// 0 when there's nothing to save, and non-zero when the work can't be saved
// (tracked changes are uncommitted, or the push fails). It never commits,
// forces, or pushes any ref but the branch.
func SaveScript(branch string) string {
	b := shellQuote(branch)
	return `branch=` + b + `
[ -d .git ] || exit 0
git remote get-url origin >/dev/null 2>&1 || exit 0
git rev-parse --verify --quiet "refs/heads/$branch" >/dev/null || exit 0
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  echo "uncommitted changes to tracked files; not pushing $branch:"
  git status --porcelain --untracked-files=no
  exit 1
fi
n=$(git rev-list --count "refs/heads/$branch" --not --remotes=origin) || exit 1
[ "$n" -gt 0 ] || exit 0
echo "pushing $n commit(s) on $branch"
if [ -n "${GH_TOKEN:-}" ]; then helper='!f() { echo username=x-access-token; echo "password=$GH_TOKEN"; }; f'; git -c credential.helper= -c "credential.helper=$helper" push origin "refs/heads/$branch:refs/heads/$branch"; else git push origin "refs/heads/$branch:refs/heads/$branch"; fi
`
}

// TurnWrapper git-excludes skill files, writes AGENTS.md (our section above the repo's own) and then
// execs the harness, so every harness reads the same instructions file.
var TurnWrapper = []string{"sh", "-c",
	`if [ -f .lazyagents/skills-exclude.txt ] && [ -d .git ]; then mkdir -p .git/info; while read -r p; do grep -qxF "$p" .git/info/exclude 2>/dev/null || echo "$p" >> .git/info/exclude; done < .lazyagents/skills-exclude.txt; fi; cat .lazyagents/agents-section.md > AGENTS.md && if [ -f .lazyagents/AGENTS.repo.md ]; then printf '\n' >> AGENTS.md && cat .lazyagents/AGENTS.repo.md >> AGENTS.md; fi && exec "$@"`,
	"lazyagents-turn"}

var fileSlug = regexp.MustCompile(`[^a-z0-9]+`)

func contextName(typ, title string) string {
	s := strings.Trim(fileSlug.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	return ".lazyagents/context/" + typ + "-" + s + ".md"
}

// ContextFile is one file delivered into the workspace.
type ContextFile struct {
	Path       string
	Body       []byte
	ArtifactID uuid.UUID
	VersionID  *uuid.UUID // pinned planning source, rather than the current version
}

// ContextFiles are the ticket's markdown artifacts plus the wiki memory the
// run starts with (§6.5, §12.3).
func ContextFiles(ctx context.Context, q *store.Queries, agent store.Agent, t store.Ticket, mem agentdef.Memory) ([]ContextFile, error) {
	var out []ContextFile
	seen := map[string]bool{}
	add := func(path string, body string, id uuid.UUID) {
		for n := 2; seen[path]; n++ {
			path = strings.TrimSuffix(path, ".md") + fmt.Sprintf("-%d.md", n)
		}
		seen[path] = true
		out = append(out, ContextFile{Path: path, Body: []byte(body), ArtifactID: id})
	}
	docs, err := q.ListTicketArtifacts(ctx, store.ListTicketArtifactsParams{TicketID: t.ID, AssigneeAgentID: &agent.ID})
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		a, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: d.ID, WorkspaceID: t.WorkspaceID})
		if err != nil || a.CurrentVersionID == nil {
			continue
		}
		v, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: t.WorkspaceID})
		if err != nil || v.BodyMd == nil {
			continue // files (designs) aren't delivered as text
		}
		title := a.Title
		if id := artifacts.DisplayID(a.Type, a.Number); id != nil {
			title = *id + " " + title
		}
		add(contextName(a.Type, title), *v.BodyMd, a.ID)
	}
	pages, err := wiki.RunContext(ctx, q, agent, t, mem)
	if err != nil {
		return nil, err
	}
	for _, p := range pages {
		header := fmt.Sprintf("<!-- wiki page %s (%s) -->\n", p.Ref, p.Reason)
		add(contextName("wiki", strings.ReplaceAll(p.Ref, "/", " ")), header+p.Markdown, *p.Target.ID)
	}
	return out, nil
}

// envName turns a secret name into an env var: db:staging → DB_STAGING.
func envName(secret string) string {
	return strings.Trim(strings.ToUpper(regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(secret, "_")), "_")
}

// HarnessKeys are the API-key secrets each harness authenticates with
// headless, and the env vars they're injected as.
var HarnessKeys = map[string]struct {
	Secret string
	Env    []string
}{
	agentdef.HarnessClaudeCode: {"harness:anthropic", []string{"ANTHROPIC_API_KEY"}},
	agentdef.HarnessCodex:      {"harness:openai", []string{"CODEX_API_KEY", "OPENAI_API_KEY"}},
}

// Revealer decrypts secrets (secrets.Service).
type Revealer interface {
	Reveal(ctx context.Context, ws uuid.UUID, project *uuid.UUID, name string) (string, error)
}

// SecretDecision says what happens to one secret in a run.
type SecretDecision struct {
	Name     string
	Scope    string // the scope it grants; "" for harness keys
	Injected bool
	Env      []string
	Reason   string
}

// PlanSecrets decides which secrets a run gets (§6.3 credential layer): a
// secret is injected only when the scope it grants is allowed; the harness
// key is always injected. git:pat also becomes GH_TOKEN/GITHUB_TOKEN.
func PlanSecrets(ctx context.Context, q *store.Queries, ws uuid.UUID, project *uuid.UUID, spec agentdef.Resolved) ([]SecretDecision, error) {
	rows, err := q.SecretsForRun(ctx, store.SecretsForRunParams{WorkspaceID: ws, ProjectID: project})
	if err != nil {
		return nil, err
	}
	modes := map[string]agentdef.Mode{}
	for _, s := range spec.Scopes {
		modes[s.Scope] = s.Mode
	}
	key := HarnessKeys[spec.Harness]
	var out []SecretDecision
	for _, r := range rows {
		d := SecretDecision{Name: r.Name}
		switch {
		case r.Name == key.Secret:
			d.Injected, d.Env, d.Reason = true, key.Env, "Authenticates "+spec.Harness
		case strings.HasPrefix(r.Name, "harness:"):
			continue // another harness's key
		case r.GrantsScope == nil:
			d.Reason = "Grants no scope, so it's never injected"
		default:
			d.Scope = *r.GrantsScope
			mode, ok := agentdef.ModeFor(modes, d.Scope)
			switch {
			case ok && mode == agentdef.Allowed:
				d.Injected, d.Env = true, []string{envName(r.Name)}
				if r.Name == "git:ssh" {
					d.Env = []string{SSHKeyEnv}
				}
				if r.Name == "git:pat" {
					d.Env = append(d.Env, "GH_TOKEN", "GITHUB_TOKEN")
				}
				d.Reason = d.Scope + " is allowed"
			case ok && mode == agentdef.RequiresApproval:
				d.Reason = d.Scope + " requires approval: LazyAgents uses it server-side after a person approves"
			default:
				d.Reason = d.Scope + " is denied"
			}
		}
		out = append(out, d)
	}
	return out, nil
}

// ErrNoHarnessKey means a docker run can't authenticate its harness.
var ErrNoHarnessKey = errors.New("runner: no harness API key")

// SecretEnv reveals the injected secrets into env vars.
func SecretEnv(ctx context.Context, rv Revealer, ws uuid.UUID, project *uuid.UUID, plan []SecretDecision) (map[string]string, error) {
	env := map[string]string{}
	for _, d := range plan {
		if !d.Injected {
			continue
		}
		v, err := rv.Reveal(ctx, ws, project, d.Name)
		if errors.Is(err, apperr.ErrNotFound) || errors.Is(err, secrets.ErrNoKey) {
			continue // no SECRETS_KEY: nothing can be revealed
		} else if err != nil {
			return nil, err
		}
		for _, e := range d.Env {
			env[e] = v
		}
	}
	return env, nil
}

// PlanningContextFile delivers exactly the source version approved at start,
// even if a new version is published while the run is queued.
func PlanningContextFile(ctx context.Context, q *store.Queries, run store.Run) (*ContextFile, error) {
	p, err := q.PlanningSessionForRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	if p.ArtifactID == nil || p.SourceVersionID == nil {
		return nil, nil
	}
	v, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *p.SourceVersionID, ArtifactID: *p.ArtifactID, WorkspaceID: run.WorkspaceID})
	if err != nil {
		return nil, err
	}
	if v.BodyMd == nil {
		return nil, apperr.Invalid("source", "planning source is not markdown")
	}
	return &ContextFile{Path: ".lazyagents/planning-source.md", Body: []byte(*v.BodyMd), ArtifactID: *p.ArtifactID, VersionID: p.SourceVersionID}, nil
}
