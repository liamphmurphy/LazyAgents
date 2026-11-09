package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// checkpointPrefix namespaces the branches Vesper pushes on its own behalf.
const checkpointPrefix = "vesper-checkpoint/"

const checkpointPushed = "vesper-checkpoint: pushed"

// CheckpointBranch is per run, so only that run ever writes it and a retried
// push may safely replace an earlier attempt.
func CheckpointBranch(key string, runID uuid.UUID) string {
	return checkpointPrefix + strings.ToLower(key) + "/" + runID.String()
}

// CheckpointScript pushes the workspace's unsaved work to branch: a commit of
// the working tree on top of HEAD, built with a scratch copy of the index so
// the checkout is left as it was. The copy keeps AGENTS.md's skip-worktree bit,
// so Vesper's own section is not saved. Work already on a real remote branch
// needs no checkpoint; earlier checkpoints don't count as saved, so work
// recovered from one survives another run. It prints nothing when there is
// nothing to save, and checkpointPushed after a push.
func CheckpointScript(branch string) string {
	var b strings.Builder
	b.WriteString("set -eu\n")
	b.WriteString("[ -d .git ] || exit 0\n")
	b.WriteString("git remote get-url origin >/dev/null 2>&1 || exit 0\n")
	b.WriteString("git rev-parse -q --verify HEAD >/dev/null || exit 0\n")
	b.WriteString(`idx=$(git rev-parse --git-path vesper-checkpoint-index)` + "\n")
	b.WriteString(`trap 'rm -f "$idx"' EXIT` + "\n")
	b.WriteString(`cp "$(git rev-parse --git-path index)" "$idx"` + "\n")
	b.WriteString(`GIT_INDEX_FILE="$idx" git add -A -- .` + "\n")
	b.WriteString(`tree=$(GIT_INDEX_FILE="$idx" git write-tree)` + "\n")
	b.WriteString(`commit=$(git rev-parse HEAD)` + "\n")
	b.WriteString(`if [ "$tree" != "$(git rev-parse 'HEAD^{tree}')" ]; then` + "\n")
	b.WriteString(`  commit=$(git commit-tree "$tree" -p HEAD -m 'Vesper checkpoint: uncommitted work from an earlier run')` + "\n")
	b.WriteString(`elif [ -z "$(git rev-list -1 HEAD --not --exclude='origin/` + checkpointPrefix + `*' --remotes)" ]; then` + "\n")
	b.WriteString("  exit 0\nfi\n")
	b.WriteString(`git push --quiet --no-verify origin "+$commit:refs/heads/"` + shellQuote(branch) + "\n")
	b.WriteString("echo '" + checkpointPushed + "'\n")
	return b.String()
}

// runScript runs a script in a run's workspace and returns its combined
// output, redacted of env's values.
func (r *Runner) runScript(ctx context.Context, runID uuid.UUID, name, script string, env map[string]string) (string, error) {
	if err := r.Backend.EnsureWorkspace(ctx, runID.String()); err != nil {
		return "", err
	}
	proc, err := r.Backend.Start(ctx, runID.String(), ExecSpec{Name: name + "-" + runID.String(), Argv: []string{"sh", "-c", script}, Env: env})
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	var mu sync.Mutex
	var cp sync.WaitGroup
	for _, rd := range []io.Reader{proc.Stdout(), proc.Stderr()} {
		cp.Add(1)
		go func() {
			defer cp.Done()
			b, _ := io.ReadAll(io.LimitReader(rd, 1<<20))
			mu.Lock()
			out.Write(b)
			mu.Unlock()
		}()
	}
	code, err := proc.Wait()
	cp.Wait()
	if c, ok := proc.(io.Closer); ok {
		_ = c.Close()
	}
	output := redact(out.String(), env)
	if err != nil {
		return output, err
	}
	if code != 0 {
		return output, fmt.Errorf("exit %d: %s", code, lastLines(output, 10))
	}
	return output, nil
}

// checkpoint saves a ticket run's unsaved work before its workspace is given
// up. Runs with no repository or no repo:write have nowhere they may push, and
// are left alone. An error means the work is not safe: keep the workspace.
func (r *Runner) checkpoint(ctx context.Context, runID uuid.UUID) error {
	if has, err := r.Backend.HasWorkspace(ctx, runID.String()); err == nil && !has {
		return nil // on the other backend, which saves it
	}
	if run, err := r.q().LockRunNoWait(ctx, runID); err == nil && run.Purpose == "chat" {
		return r.checkpointChat(ctx, run)
	}
	c, err := r.load(ctx, runID)
	if c.run.Purpose != "ticket" || c.run.ParentRunID != nil || c.run.TicketID == nil {
		return nil
	}
	if err != nil {
		return err
	}
	if c.run.CheckpointAt != nil || c.project.RepoUrl == nil || *c.project.RepoUrl == "" || !allowedScope(c.spec, "repo:write") {
		return nil
	}
	env := map[string]string{}
	v, err := r.Secrets.Reveal(ctx, c.run.WorkspaceID, &c.project.ID, "git:pat")
	switch {
	case err == nil:
		env["GH_TOKEN"] = v
	case !errors.Is(err, apperr.ErrNotFound) && !errors.Is(err, secrets.ErrNoKey):
		return r.checkpointFailed(ctx, c, err)
	}
	branch := CheckpointBranch(c.ticket.Key, c.run.ID)
	out, err := r.runScript(ctx, c.run.ID, "lazyagents-checkpoint", CheckpointScript(branch), env)
	if err != nil {
		return r.checkpointFailed(ctx, c, err)
	}
	var saved *string
	if strings.Contains(out, checkpointPushed) {
		saved = &branch
		r.event(ctx, c.run.ID, "lifecycle", map[string]any{"state": "checkpoint_pushed", "branch": branch})
	}
	return r.q().SetRunCheckpoint(ctx, store.SetRunCheckpointParams{ID: c.run.ID, Branch: saved})
}

// chatStrayFilesScript lists what a chat left that no checkpoint can save: any
// file beyond Vesper's own, when the workspace isn't a checkout with a remote.
const chatStrayFilesScript = `if [ -d .git ] && git remote get-url origin >/dev/null 2>&1 && git rev-parse -q --verify HEAD >/dev/null; then exit 0; fi
for f in * .[!.]* ..?*; do
  [ -e "$f" ] || continue
  case "$f" in AGENTS.md|CLAUDE.md|.lazyagents|.claude|.codex|.agents) ;; *) printf '%s\n' "$f" ;; esac
done`

// checkpointChat saves a chat's work before its workspace is given up: a
// coding chat's to a checkpoint branch. Files with nowhere to go stay on disk,
// and the run stops tracking them so the sweep doesn't keep retrying. An error
// means the workspace must not be removed. A deleted chat's goes with it.
func (r *Runner) checkpointChat(ctx context.Context, run store.Run) error {
	if run.CheckpointAt != nil {
		return nil
	}
	chat, err := r.q().ChatSessionForEndedRun(ctx, &run.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	spec, err := runs.Spec(run)
	if err != nil {
		return err
	}
	c := runCtx{run: run, spec: spec}
	c.project.ID = runs.ProjectIDOrNil(chat.ProjectID)
	var saved *string
	if allowedScope(c.spec, "repo:write") && chat.RepoUrl != nil {
		env := map[string]string{}
		for name, key := range map[string]string{"git:pat": "GH_TOKEN", "git:ssh": SSHKeyEnv} {
			v, err := r.Secrets.Reveal(ctx, c.run.WorkspaceID, &c.project.ID, name)
			switch {
			case err == nil:
				env[key] = v
			case !errors.Is(err, apperr.ErrNotFound) && !errors.Is(err, secrets.ErrNoKey):
				return r.checkpointFailed(ctx, c, err)
			}
		}
		ScrubChatGitEnv(env, *chat.RepoUrl)
		branch := CheckpointBranch("chat", c.run.ID)
		out, err := r.runScript(ctx, c.run.ID, "lazyagents-checkpoint", CheckpointScript(branch), env)
		if err != nil {
			return r.checkpointFailed(ctx, c, err)
		}
		if strings.Contains(out, checkpointPushed) {
			saved = &branch
			r.event(ctx, c.run.ID, "lifecycle", map[string]any{"state": "checkpoint_pushed", "branch": branch})
		}
	}
	out, err := r.runScript(ctx, c.run.ID, "lazyagents-chat-files", chatStrayFilesScript, nil)
	if err != nil {
		return r.checkpointFailed(ctx, c, err)
	}
	if files := strings.Fields(out); len(files) > 0 {
		r.event(ctx, c.run.ID, "lifecycle", map[string]any{"state": "workspace_kept", "files": files, "path": r.Backend.WorkspaceDir(c.run.ID.String())})
		if err := r.q().SetRunWorkspace(ctx, store.SetRunWorkspaceParams{ID: c.run.ID, WorkspaceVolume: nil}); err != nil {
			return err
		}
		return fmt.Errorf("kept the workspace: %d file(s) can't be checkpointed", len(files))
	}
	return r.q().SetRunCheckpoint(ctx, store.SetRunCheckpointParams{ID: c.run.ID, Branch: saved})
}

// checkpointFailed records why, with one timeline entry per distinct failure
// since the sweep retries.
func (r *Runner) checkpointFailed(ctx context.Context, c runCtx, cause error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	msg := cause.Error()
	if c.run.CheckpointError == nil || *c.run.CheckpointError != msg {
		r.event(ctx, c.run.ID, "lifecycle", map[string]any{"state": "checkpoint_failed", "reason": msg})
		if err := r.q().SetRunCheckpointError(ctx, store.SetRunCheckpointErrorParams{ID: c.run.ID, Error: &msg}); err != nil {
			r.Log.Warn("record checkpoint failure", "run", c.run.ID, "err", err)
		}
	}
	return cause
}

// recoverTicketWork checkpoints earlier runs of the ticket that still hold
// work, and returns the branch this run should start from ("" for none).
func (r *Runner) recoverTicketWork(ctx context.Context, c runCtx) (string, error) {
	ids, err := r.q().RunsToCheckpoint(ctx, store.RunsToCheckpointParams{TicketID: c.run.TicketID, ID: c.run.ID})
	if err != nil {
		return "", err
	}
	for _, id := range ids {
		if err := r.checkpoint(ctx, id); err != nil {
			return "", fmt.Errorf("run %s: %w", id, err)
		}
	}
	branch, err := r.q().LatestTicketCheckpoint(ctx, store.LatestTicketCheckpointParams{TicketID: c.run.TicketID, ID: c.run.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return branch, err
}
