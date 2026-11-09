package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/harness"
	"github.com/liamphmurphy/vesper/server/internal/memory"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
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

// keepOurFilesOutOfGit: our files never get committed. .lazyagents/ is
// excluded and a tracked AGENTS.md is marked skip-worktree while we write our
// section into it.
const keepOurFilesOutOfGit = `mkdir -p .lazyagents/context
grep -qx '.lazyagents/' .git/info/exclude 2>/dev/null || echo '.lazyagents/' >> .git/info/exclude
if [ -f AGENTS.md ] && ! grep -q 'lazyagents:start' AGENTS.md; then cp AGENTS.md .lazyagents/AGENTS.repo.md; fi
if git ls-files --error-unmatch AGENTS.md >/dev/null 2>&1; then git update-index --skip-worktree AGENTS.md; else grep -qx 'AGENTS.md' .git/info/exclude || echo 'AGENTS.md' >> .git/info/exclude; fi
`

// SetupScript prepares a fresh workspace (§6.5): clone the project's repo
// (or start an empty one), create the run's branch, keep Vesper's files
// out of git, and run the project's setup commands. The working directory
// is the workspace; GH_TOKEN, when present, authenticates git and gh.
func SetupScript(repoURL, branch, authorName, authorEmail string, setup []string) string {
	return SetupScriptForBranch(repoURL, "", branch, authorName, authorEmail, setup)
}

// SetupScriptForBranch clones the approved base branch rather than the remote's HEAD.
func SetupScriptForBranch(repoURL, baseBranch, branch, authorName, authorEmail string, setup []string) string {
	return SetupScriptForEpic(repoURL, baseBranch, "", branch, authorName, authorEmail, setup)
}

// SetupScriptForEpic is SetupScriptForBranch for a ticket in an epic: the
// run's branch is cut from the epic's shared branch once it exists on origin,
// and from the base branch before the first ticket has pushed it.
func SetupScriptForEpic(repoURL, baseBranch, epicBranch, branch, authorName, authorEmail string, setup []string) string {
	return SetupScriptWithCheckpoint(repoURL, baseBranch, epicBranch, "", branch, authorName, authorEmail, setup)
}

// checkpointRecovered is what setup prints once it has restored a checkpoint.
const checkpointRecovered = "→ recovered checkpoint"

// SetupScriptWithCheckpoint is SetupScriptForEpic for a ticket whose earlier
// run left a checkpoint branch: the run starts from it when it is still on
// origin, and as usual when it isn't.
func SetupScriptWithCheckpoint(repoURL, baseBranch, epicBranch, checkpoint, branch, authorName, authorEmail string, setup []string) string {
	var b strings.Builder
	b.WriteString("set -eu\n")
	// Everything is repo-local: on the local backend this is the user's own
	// machine, and their global git config must not change.
	b.WriteString(`helper='!f() { echo username=x-access-token; echo "password=$GH_TOKEN"; }; f'` + "\n")
	b.WriteString(sshSetup)
	b.WriteString("if [ ! -d .git ]; then\n")
	if repoURL != "" {
		b.WriteString("  echo '→ cloning " + strings.ReplaceAll(repoURL, "'", "") + "'\n")
		cloneBranch := ""
		if baseBranch != "" {
			cloneBranch = "--branch " + shellQuote(baseBranch) + " -- "
		}
		// .lazyagents/ already exists (ssh setup), so clone beside it and move .git in.
		const dest = ".lazyagents/clone"
		q := shellQuote(repoURL)
		b.WriteString("  rm -rf " + dest + "\n")
		b.WriteString(`  if [ -n "${GH_TOKEN:-}" ]; then git -c credential.helper= -c "credential.helper=$helper" clone --quiet --no-checkout ` + cloneBranch + q + " " + dest + "; else git clone --quiet --no-checkout " + cloneBranch + q + " " + dest + "; fi\n")
		b.WriteString("  mv " + dest + "/.git .git\n")
		b.WriteString("  rm -rf " + dest + "\n")
		b.WriteString("  if git rev-parse -q --verify HEAD >/dev/null 2>&1; then git reset -q --hard; fi\n")
		if epicBranch != "" {
			b.WriteString("  if git fetch --quiet origin " + shellQuote("+refs/heads/"+epicBranch+":refs/remotes/origin/"+epicBranch) + " 2>/dev/null; then echo '→ stacking on epic branch " + strings.ReplaceAll(epicBranch, "'", "") + "'; git checkout -q --detach " + shellQuote("origin/"+epicBranch) + "; else echo '→ epic branch " + strings.ReplaceAll(epicBranch, "'", "") + " not on origin yet: starting from the base branch'; fi\n")
		}
		if checkpoint != "" {
			b.WriteString("  if git fetch --quiet origin " + shellQuote("+refs/heads/"+checkpoint+":refs/remotes/origin/"+checkpoint) + " 2>/dev/null; then git checkout -q --detach " + shellQuote("origin/"+checkpoint) + "; echo '" + checkpointRecovered + " " + strings.ReplaceAll(checkpoint, "'", "") + "'; else echo '→ checkpoint " + strings.ReplaceAll(checkpoint, "'", "") + " is no longer on origin: starting without it'; fi\n")
		}
	} else {
		b.WriteString("  echo '→ no repository configured for this project: starting an empty one'\n")
		b.WriteString("  git init --quiet -b main .\n")
	}
	b.WriteString("  git config user.name " + shellQuote(authorName) + "\n")
	b.WriteString("  git config user.email " + shellQuote(authorEmail) + "\n")
	b.WriteString("  git checkout --quiet -B " + shellQuote(branch) + "\n")
	b.WriteString("fi\n")
	// Every turn, not just the first: a token or key added after the workspace
	// was created must still reach git (gh reads GH_TOKEN on its own).
	b.WriteString(`if [ -n "${GH_TOKEN:-}" ]; then git config credential.helper "$helper"; fi` + "\n")
	b.WriteString(`git config core.sshCommand "$GIT_SSH_COMMAND"` + "\n")
	b.WriteString(keepOurFilesOutOfGit)
	b.WriteString(dependencySetupScript())
	for _, c := range setup {
		b.WriteString("echo " + shellQuote("→ "+c) + "\n")
		b.WriteString(c + "\n")
	}
	b.WriteString("echo '→ workspace ready on branch " + strings.ReplaceAll(branch, "'", "") + "'\n")
	return b.String()
}

// chatGitAuth points a chat's git at its credentials. The token helper is
// scoped to github.com so it never answers for another host (a submodule, a
// second remote the agent adds); an empty $token leaves it unset.
const chatGitAuth = `if [ -n "$token" ]; then git config credential.https://github.com.helper "$helper"; fi; git config core.sshCommand "$GIT_SSH_COMMAND"` + "\n"

// ChatRepoSetupScript prepares a coding chat's workspace: the persona's repo
// cloned into the workspace root on the chat's own work branch. A branch an
// earlier segment already pushed is checked out and resumed (the previous
// workspace is gone); otherwise it starts from the remote's default branch.
// Only github.com over https gets the workspace's git token: the repo URL is
// the persona owner's choice, and the token must not go to a host they picked.
func ChatRepoSetupScript(repoURL, workBranch, authorName, authorEmail string) string {
	var b strings.Builder
	b.WriteString("set -eu\n")
	b.WriteString(`helper='!f() { echo username=x-access-token; echo "password=$GH_TOKEN"; }; f'` + "\n")
	b.WriteString(sshSetup)
	// An empty value never matches the credential helper's GH_TOKEN test below.
	b.WriteString(`token="${GH_TOKEN:-}"` + "\n")
	if !strings.HasPrefix(repoURL, "https://github.com/") {
		b.WriteString(`token=""` + "\n")
	}
	q, wb := shellQuote(repoURL), shellQuote(workBranch)
	b.WriteString("if [ ! -d .git ]; then\n")
	b.WriteString("  echo '→ cloning " + strings.ReplaceAll(repoURL, "'", "") + "'\n")
	const dest = ".lazyagents/clone"
	b.WriteString("  rm -rf " + dest + "\n")
	b.WriteString(`  if [ -n "$token" ]; then git -c credential.helper= -c "credential.helper=$helper" clone --quiet --no-checkout -- ` + q + " " + dest + "; else git clone --quiet --no-checkout -- " + q + " " + dest + "; fi\n")
	b.WriteString("  mv " + dest + "/.git .git\n")
	b.WriteString("  rm -rf " + dest + "\n")
	b.WriteString("  if git rev-parse -q --verify HEAD >/dev/null 2>&1; then git reset -q --hard; fi\n")
	b.WriteString("  git config user.name " + shellQuote(authorName) + "\n")
	b.WriteString("  git config user.email " + shellQuote(authorEmail) + "\n")
	// The clone's own fetch below needs the credentials too.
	b.WriteString("  " + chatGitAuth)
	b.WriteString("  if git fetch --quiet origin " + shellQuote("+refs/heads/"+workBranch+":refs/remotes/origin/"+workBranch) + " 2>/dev/null; then\n")
	b.WriteString("    echo '→ resuming pushed work on branch " + strings.ReplaceAll(workBranch, "'", "") + "'\n")
	b.WriteString("    git checkout --quiet -B " + wb + " " + shellQuote("origin/"+workBranch) + "\n")
	b.WriteString("  else\n")
	b.WriteString("    git checkout --quiet -B " + wb + "\n")
	b.WriteString("  fi\n")
	b.WriteString("fi\n")
	// Again every turn, so a token or key added after the workspace was
	// created reaches git too.
	b.WriteString(chatGitAuth)
	// Defense in depth, not a hard boundary: the agent can still bypass a hook
	// (--no-verify, another remote). The token's own reach is the real limit.
	hook := []string{
		"#!/bin/sh",
		"# Vesper: this chat may push only its own work branch (not a hard boundary).",
		"want=" + shellQuote("refs/heads/"+workBranch),
		`while read -r local_ref local_sha remote_ref remote_sha; do`,
		`  if [ "$remote_ref" != "$want" ]; then echo "vesper: this chat may only push $want, not $remote_ref" >&2; exit 1; fi`,
		"done",
		"exit 0",
	}
	quoted := make([]string, len(hook))
	for i, l := range hook {
		quoted[i] = shellQuote(l)
	}
	b.WriteString("mkdir -p .git/hooks\n")
	b.WriteString("printf '%s\\n' " + strings.Join(quoted, " ") + " > .git/hooks/pre-push\n")
	b.WriteString("chmod +x .git/hooks/pre-push\n")
	b.WriteString(keepOurFilesOutOfGit)
	b.WriteString("mkdir -p .lazyagents/outbox\n")
	b.WriteString(dependencySetupScript())
	b.WriteString("echo '→ workspace ready on branch " + strings.ReplaceAll(workBranch, "'", "") + "'\n")
	return b.String()
}

// TurnWrapper git-excludes skill files (in the repository's shared exclude
// file, which a delegated run's worktree uses too), writes AGENTS.md (our section above the repo's own) and then
// execs the harness, so every harness reads the same instructions file.
var TurnWrapper = []string{"sh", "-c",
	`if [ -f .lazyagents/skills-exclude.txt ] && [ -e .git ]; then ex=$(git rev-parse --git-path info/exclude); mkdir -p "$(dirname "$ex")"; while read -r p; do grep -qxF "$p" "$ex" 2>/dev/null || echo "$p" >> "$ex"; done < .lazyagents/skills-exclude.txt; fi; cat .lazyagents/agents-section.md > AGENTS.md && if [ -f .lazyagents/AGENTS.repo.md ]; then printf '\n' >> AGENTS.md && cat .lazyagents/AGENTS.repo.md >> AGENTS.md; fi && exec "$@"`,
	"lazyagents-turn"}

var fileSlug = regexp.MustCompile(`[^a-z0-9]+`)

func contextName(typ, title string) string {
	s := strings.Trim(fileSlug.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	ext := ".md"
	if typ == "mockup" {
		ext = ".svg"
	}
	return ".lazyagents/context/" + typ + "-" + s + ext
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
		ext := filepath.Ext(path)
		base := strings.TrimSuffix(path, ext)
		for n := 2; seen[path]; n++ {
			path = base + fmt.Sprintf("-%d", n) + ext
		}
		seen[path] = true
		out = append(out, ContextFile{Path: path, Body: []byte(body), ArtifactID: id})
	}
	docs, err := q.ListTicketArtifacts(ctx, store.ListTicketArtifactsParams{TicketID: t.ID, AssigneeAgentID: &agent.ID})
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(docs))
	for _, d := range docs {
		ids = append(ids, d.ID)
	}
	// Epic-level mockups gate this ticket too, so deliver them with the attached ones.
	if gate, err := artifacts.MockupGate(ctx, q, t); err == nil {
		for _, m := range gate.Mockups {
			if !slices.Contains(ids, m.ID) {
				ids = append(ids, m.ID)
			}
		}
	}
	for _, id := range ids {
		a, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: id, WorkspaceID: t.WorkspaceID})
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
		if slices.Contains(ids, *p.Target.ID) {
			continue
		} // attached artifacts are already staged
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
	agentdef.HarnessClaudeCode: {chatmodels.KeySecret[agentdef.HarnessClaudeCode], []string{"ANTHROPIC_API_KEY"}},
	agentdef.HarnessCodex:      {chatmodels.KeySecret[agentdef.HarnessCodex], []string{"CODEX_API_KEY", "OPENAI_API_KEY"}},
}

// Subscription logins. A stored harness key can be a plan login rather than an
// API key, recognized by its shape (chatmodels.ClaudeOAuthToken, ParseCodexAuth).
const (
	ClaudeOAuthEnv = "CLAUDE_CODE_OAUTH_TOKEN"
	// CodexAuthEnv and CodexAuthScopeEnv only travel from SecretEnv to the
	// backend: Docker turns them into ~/.codex/auth.json.
	CodexAuthEnv      = "LAZYAGENTS_CODEX_AUTH"
	CodexAuthScopeEnv = "LAZYAGENTS_CODEX_AUTH_SCOPE"
)

// HarnessKey is the API-key secret and env vars a resolved spec authenticates
// with. A harness with no mapping is an error: there is no fallback to another
// harness's key.
func HarnessKey(spec agentdef.Resolved) (struct {
	Secret string
	Env    []string
}, error) {
	var out struct {
		Secret string
		Env    []string
	}
	k, ok := HarnessKeys[spec.Harness]
	if !ok {
		return out, fmt.Errorf("runner: no credential mapping for harness %q", spec.Harness)
	}
	return k, nil
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
	key, err := HarnessKey(spec)
	if err != nil {
		return nil, err
	}
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
				d.Reason = d.Scope + " requires approval: Vesper uses it server-side after a person approves"
			default:
				d.Reason = d.Scope + " is denied"
			}
		}
		out = append(out, d)
	}
	return out, nil
}

// GitHubHTTPS reports a repository URL the workspace's git token may be used
// for; SSHKeyEnv's key is likewise only for GitHubSSH URLs (only GitHub's host
// keys are pinned).
func GitHubHTTPS(repoURL string) bool { return strings.HasPrefix(repoURL, "https://github.com/") }

// GitHubSSH reports a github.com SSH repository URL (scp or ssh:// form).
func GitHubSSH(repoURL string) bool {
	return strings.HasPrefix(repoURL, "git@github.com:") || strings.HasPrefix(repoURL, "ssh://git@github.com/") || strings.HasPrefix(repoURL, "ssh://git@github.com:22/")
}

// ChatSecretPlan narrows a chat's secret plan to the harness key and the git
// credentials: no other repo:* or tests:run-scoped secret ever reaches a chat,
// whose repository (and so the host its git credentials go to) is a
// persona owner's choice.
func ChatSecretPlan(plan []SecretDecision, spec agentdef.Resolved) []SecretDecision {
	key, _ := HarnessKey(spec)
	return slices.DeleteFunc(slices.Clone(plan), func(d SecretDecision) bool {
		return d.Name != key.Secret && d.Name != "git:pat" && d.Name != "git:ssh"
	})
}

// ScrubChatGitEnv removes the git credentials a chat's repository can't use:
// the token unless it is a github.com https repo, the SSH key unless it is a
// github.com ssh repo. A chat without a repository gets neither.
func ScrubChatGitEnv(env map[string]string, repoURL string) {
	if !GitHubHTTPS(repoURL) {
		for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN", envName("git:pat")} {
			delete(env, k)
		}
	}
	if !GitHubSSH(repoURL) {
		delete(env, SSHKeyEnv)
	}
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
		switch {
		case d.Name == chatmodels.KeySecret[agentdef.HarnessClaudeCode] && chatmodels.ClaudeOAuthToken(v):
			env[ClaudeOAuthEnv] = v
			continue
		case d.Name == chatmodels.KeySecret[agentdef.HarnessCodex]:
			if _, ok := chatmodels.ParseCodexAuth(v); ok {
				env[CodexAuthEnv] = v
				env[CodexAuthScopeEnv] = CodexAuthScope(ws, project)
				continue
			}
		}
		for _, e := range d.Env {
			env[e] = v
		}
	}
	return env, nil
}

// CodexAuthScope says which secret a refreshed Codex login belongs to.
func CodexAuthScope(ws uuid.UUID, project *uuid.UUID) string {
	if project == nil {
		return ws.String()
	}
	return ws.String() + "/" + project.String()
}

// ParseCodexAuthScope reverses CodexAuthScope.
func ParseCodexAuthScope(scope string) (uuid.UUID, *uuid.UUID, error) {
	wsID, projectID, hasProject := strings.Cut(scope, "/")
	ws, err := uuid.Parse(wsID)
	if err != nil || !hasProject {
		return ws, nil, err
	}
	project, err := uuid.Parse(projectID)
	return ws, &project, err
}

// sensitiveEnv is what gets scrubbed from events: the env values, plus each
// token inside a Codex login, since an agent would print those on their own.
func sensitiveEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env)+3)
	for k, v := range env {
		out[k] = v
	}
	if auth, ok := chatmodels.ParseCodexAuth(env[CodexAuthEnv]); ok {
		out["codex_access"], out["codex_refresh"], out["codex_id"] = auth.Tokens.AccessToken, auth.Tokens.RefreshToken, auth.Tokens.IDToken
	}
	return out
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

// PlanningRefFiles delivers what the person attached when starting a plan, at
// the paths its prompt already lists. A reference that no longer resolves
// (deleted since the plan was queued) is skipped; the planner sees it missing.
func PlanningRefFiles(ctx context.Context, q *store.Queries, run store.Run) ([]ContextFile, error) {
	p, err := q.PlanningSessionForRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	var refs []runs.ChatContextRef
	if err = json.Unmarshal(p.ContextRefs, &refs); err != nil || len(refs) == 0 {
		return nil, err
	}
	var files []ContextFile
	for i, ref := range refs {
		d, err := runs.ResolveChatRefs(ctx, q, run.WorkspaceID, p.ProjectID, uuid.Nil, []runs.ChatContextRef{ref})
		if errors.As(err, new(*apperr.ValidationError)) {
			continue
		}
		if err != nil {
			return nil, err
		}
		files = append(files, ContextFile{Path: runs.PlanningRefPath(i), Body: []byte(d[0].Body), ArtifactID: derefID(d[0].ArtifactID)})
	}
	return files, nil
}

// ChatTurnContext delivers the references attached to a chat turn's message.
// File names are generated from the turn and position, never from the ref
// itself, so a title or path can't escape .lazyagents/context/. prompt is the
// text appended to the turn's prompt to list the files. A reference that no
// longer resolves (deleted since it was queued) is named in the prompt but
// not delivered.
func ChatTurnContext(ctx context.Context, q *store.Queries, run store.Run, project uuid.UUID, turn store.RunTurn) (files []ContextFile, prompt string, err error) {
	msg, err := q.ChatMessageForTurn(ctx, &turn.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	var refs []runs.ChatContextRef
	if err = json.Unmarshal(msg.ContextRefs, &refs); err != nil || len(refs) == 0 {
		return nil, "", err
	}
	var b strings.Builder
	b.WriteString("\n\nThe person attached these references to this message. Read them first:\n")
	for i, ref := range refs {
		d, err := runs.ResolveChatRefs(ctx, q, run.WorkspaceID, project, msg.SessionID, []runs.ChatContextRef{ref})
		if err != nil {
			if errors.As(err, new(*apperr.ValidationError)) {
				fmt.Fprintf(&b, "- %s (no longer available)\n", ref)
				continue
			}
			return nil, "", err
		}
		path := fmt.Sprintf(".lazyagents/context/chat-turn-%d-ref-%d.md", turn.Seq, i+1)
		files = append(files, ContextFile{Path: path, Body: []byte(d[0].Body), ArtifactID: derefID(d[0].ArtifactID)})
		fmt.Fprintf(&b, "- `%s`: %s %q\n", path, ref.Kind, d[0].Title)
	}
	return files, b.String(), nil
}

// maxChatAttachmentTurnBytes bounds what one turn loads into memory to deliver.
const maxChatAttachmentTurnBytes = runs.MaxChatAttachments * runs.MaxChatAttachmentBytes

// chatTurnAttachments delivers the files uploaded with a chat turn's message
// under .lazyagents/attachments/. Paths come from the turn and position plus
// the sanitized upload name, so a name can't escape that directory. A file
// whose bytes are gone (or with no attachment store) is named as unavailable.
func (r *Runner) chatTurnAttachments(ctx context.Context, q *store.Queries, turn store.RunTurn) (files []ContextFile, prompt string, err error) {
	msg, err := q.ChatMessageForTurn(ctx, &turn.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	atts, err := q.ListChatAttachmentsForMessages(ctx, []uuid.UUID{msg.ID})
	if err != nil || len(atts) == 0 {
		return nil, "", err
	}
	var b strings.Builder
	b.WriteString("\n\nThe person attached these files to this message. Open each with your file-reading tool before answering:\n")
	var total int64
	for i, a := range atts {
		name := runs.ChatAttachmentDiskName(runs.ChatAttachmentName(a.Filename))
		if name == "" || strings.HasPrefix(name, ".") {
			name = "file" + name
		}
		p := fmt.Sprintf(".lazyagents/attachments/turn-%d/%d-%s", turn.Seq, i+1, name)
		total += a.SizeBytes
		if r.Attachments == nil || total > maxChatAttachmentTurnBytes {
			fmt.Fprintf(&b, "- %q (%s): not available\n", a.Filename, a.ContentType)
			continue
		}
		body, err := r.readAttachment(ctx, a.BlobKey)
		if errors.Is(err, blob.ErrNotFound) {
			fmt.Fprintf(&b, "- %q (%s): not available\n", a.Filename, a.ContentType)
			continue
		}
		if err != nil {
			return nil, "", err
		}
		files = append(files, ContextFile{Path: p, Body: body})
		fmt.Fprintf(&b, "- `%s`: %q (%s)\n", p, a.Filename, a.ContentType)
	}
	return files, b.String(), nil
}

func (r *Runner) readAttachment(ctx context.Context, key string) ([]byte, error) {
	o, err := r.Attachments.Get(ctx, r.Cfg.AttachmentBucket, key)
	if err != nil {
		return nil, err
	}
	defer o.Body.Close()
	return io.ReadAll(io.LimitReader(o.Body, runs.MaxChatAttachmentBytes+1))
}

func derefID(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}

// chatMemoryIndex renders the always-loaded index of the memory of the person
// the chat belongs to: titles and summaries only, never another person's.
func chatMemoryIndex(ctx context.Context, q *store.Queries, run store.Run) (string, error) {
	chat, err := q.ChatSessionForRun(ctx, &run.ID)
	if err != nil {
		return "", err
	}
	space, err := runs.ChatMemorySpace(ctx, q, chat)
	if err != nil {
		return "", err
	}
	o := memory.Owner{WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy, SpaceID: space}
	entries, total, err := memory.New(q).Index(ctx, o)
	if err != nil {
		return "", err
	}
	return memory.IndexMarkdown(entries, total), nil
}

// agentsSection renders the turn's instructions for the run's purpose. Only
// chat turns read the person's memory index and persona; ticket, delegated and
// planning runs never load a persona. Errors only come from chat turns, with
// the outcome to show in the conversation.
func agentsSection(ctx context.Context, q *store.Queries, c runCtx, paths []string) ([]byte, string, error) {
	switch c.run.Purpose {
	case "chat":
		// Both are refreshed every turn, so memory saves and persona edits show
		// on the next turn.
		idx, err := chatMemoryIndex(ctx, q, c.run)
		if err != nil {
			return nil, "Could not load the assistant's memory index.", fmt.Errorf("chat memory index: %w", err)
		}
		persona, err := chatPersona(ctx, q, c.run)
		if err != nil {
			return nil, "Could not load the conversation's persona.", fmt.Errorf("chat persona: %w", err)
		}
		group, err := chatGroup(ctx, q, c.run)
		if err != nil {
			return nil, "Could not load the conversation's personas.", fmt.Errorf("chat group: %w", err)
		}
		chat, err := q.ChatSessionForRun(ctx, &c.run.ID)
		if err != nil {
			return nil, "Could not load the conversation.", fmt.Errorf("chat session: %w", err)
		}
		space, spaceFiles, err := chatSpaceFiles(ctx, q, chat)
		if err != nil {
			return nil, "Could not load the conversation's space.", fmt.Errorf("chat space: %w", err)
		}
		svg, err := runs.ChatCanDrawSVGs(ctx, q, chat)
		if err != nil {
			return nil, "Could not load the conversation's persona.", fmt.Errorf("chat svg capability: %w", err)
		}
		spec := c.spec
		spec.Instructions += agents.AssistantRecommendSection + agents.AssistantRichRepliesSection + agents.AssistantCanvasSection + agents.AssistantWorkflowSection
		buildWorkflows, err := runs.SpeakerCanCreateWorkflows(ctx, q, chat)
		if err != nil {
			return nil, "Could not load the conversation's persona.", fmt.Errorf("chat workflow capability: %w", err)
		}
		if buildWorkflows {
			spec.Instructions += agents.AssistantWorkflowBuildSection
		}
		workflows, err := runs.WorkflowNote(ctx, q, chat)
		if err != nil {
			return nil, "Could not load the person's workflows.", fmt.Errorf("chat workflows: %w", err)
		}
		spec.Instructions += workflows
		tutor, err := runs.LearningTutorNote(ctx, q, chat)
		if err != nil {
			return nil, "Could not load the course.", fmt.Errorf("course note: %w", err)
		}
		spec.Instructions += tutor
		if svg {
			spec.Instructions += agents.AssistantSVGSection
		}
		return harness.ChatAgentsMDInSpace(spec, paths, c.project.Key, idx, persona, group, runs.SpaceSection(space, spaceFiles)), "", nil
	case runs.PurposeResearch, runs.PurposeWorkflow, runs.PurposeLearning:
		return []byte(c.spec.Instructions), "", nil
	case "planning":
		return []byte(c.spec.Instructions + "\nRead these context files first:\n" + strings.Join(paths, "\n")), "", nil
	}
	if c.run.ParentRunID != nil {
		return harness.SubagentAgentsMD("", c.spec, paths, c.ticket.Key), "", nil
	}
	return harness.AgentsMD("", c.spec, paths, c.ticket.Key), "", nil
}

// chatGroup is the turn's "Other personas" section (runs.GroupNote).
func chatGroup(ctx context.Context, q *store.Queries, run store.Run) (string, error) {
	chat, err := q.ChatSessionForRun(ctx, &run.ID)
	if err != nil {
		return "", err
	}
	return runs.GroupNote(ctx, q, chat)
}

// chatPersona loads the persona answering this turn (the chat's speaker, else
// its host) and that persona's own memory fresh each turn, so edits show on
// the next turn, a new segment (harness switch, idle cleanup) keeps the same
// voice, and each persona in a shared chat answers in its own. It must belong to the chat's creator; nil
// is the built-in Assistant.
func chatPersona(ctx context.Context, q *store.Queries, run store.Run) (*harness.Persona, error) {
	chat, err := q.ChatSessionForRun(ctx, &run.ID)
	if err != nil {
		return nil, err
	}
	id := chatSpeaker(chat)
	if id == nil {
		return nil, nil
	}
	p, err := q.GetPersona(ctx, store.GetPersonaParams{ID: *id, WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// The persona's own memory, loaded in full (within a budget) every turn.
	o := memory.Owner{WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy}.Persona(p.ID)
	entries, total, err := memory.New(q).PersonaContext(ctx, o)
	if err != nil {
		return nil, fmt.Errorf("persona memory: %w", err)
	}
	return &harness.Persona{Name: p.Name, Tone: p.Tone, Instructions: p.CustomInstructions,
		Memory: memory.PersonaMarkdown(entries, total)}, nil
}

// chatSpaceFiles is the chat's space (nil outside one) and its sources laid
// out as files, without their contents.
func chatSpaceFiles(ctx context.Context, q *store.Queries, chat store.ChatSession) (*store.ChatSpace, []runs.SpaceFile, error) {
	sp, err := runs.ChatSpaceOf(ctx, q, chat)
	if err != nil || sp == nil {
		return nil, nil, err
	}
	sources, err := q.ListChatSpaceSources(ctx, sp.ID)
	if err != nil {
		return nil, nil, err
	}
	return sp, runs.SpaceFiles(sources), nil
}

// spaceSourceFiles writes a chat's space sources into the turn's workspace,
// at the paths its instructions list. A file whose bytes are gone (or with no
// attachment store) is left out.
func (r *Runner) spaceSourceFiles(ctx context.Context, q *store.Queries, run store.Run) (map[string][]byte, error) {
	chat, err := q.ChatSessionForRun(ctx, &run.ID)
	if err != nil {
		return nil, err
	}
	_, files, err := chatSpaceFiles(ctx, q, chat)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, f := range files {
		if f.Source.BlobKey == nil {
			out[f.Path] = []byte(f.Source.Body)
			continue
		}
		if r.Attachments == nil {
			continue
		}
		body, err := r.readAttachment(ctx, *f.Source.BlobKey)
		if errors.Is(err, blob.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[f.Path] = body
	}
	return out, nil
}
