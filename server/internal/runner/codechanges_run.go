package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/harness"
	"github.com/liamphmurphy/vesper/server/internal/personas"
)

// What a coding chat turn changed is read with git run by the runner in the
// workspace, so the repository's own config is untrusted: nothing may run a
// program from it (external diff, textconv, fsmonitor, clean filters) and the
// output format is pinned.
const (
	// codeDiffReadLimit bounds the tracked files' patch; the numstat is small.
	codeDiffReadLimit = 4 << 20
	// codeUntrackedReadLimit bounds one untracked file's patch.
	codeUntrackedReadLimit = 1 << 20
	codeChangesTimeout     = time.Minute
)

var commitRe = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// codeChangePathspec leaves out what Vesper itself writes into the workspace.
var codeChangePathspec = []string{"--", ".", ":(exclude,top).lazyagents", ":(exclude,top)AGENTS.md"}

// gitCmd runs git in the run's workspace on any backend and returns stdout
// (read up to limit; the rest is drained) and the exit code.
func (r *Runner) gitCmd(ctx context.Context, runID string, env map[string]string, limit int64, args ...string) (out []byte, cut bool, code int, err error) {
	argv := append([]string{"git",
		"-c", "core.quotepath=off", "-c", "diff.external=", "-c", "core.pager=cat", "-c", "color.ui=false",
		"-c", "diff.noprefix=false", "-c", "core.fsmonitor=false"}, args...)
	e := map[string]string{"GIT_TERMINAL_PROMPT": "0", "GIT_OPTIONAL_LOCKS": "0"}
	for k, v := range env {
		e[k] = v
	}
	proc, err := r.Backend.Start(ctx, runID, ExecSpec{Name: "lazyagents-git-" + runID + "-" + uuid.NewString()[:8], Argv: argv, Env: e})
	if err != nil {
		return nil, false, -1, err
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		out, _ = io.ReadAll(io.LimitReader(proc.Stdout(), limit+1))
		if int64(len(out)) > limit {
			out, cut = out[:limit], true
		}
		_, _ = io.Copy(io.Discard, proc.Stdout())
	}()
	go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, proc.Stderr()) }()
	wg.Wait()
	code, err = proc.Wait()
	if c, ok := proc.(io.Closer); ok {
		_ = c.Close()
	}
	return out, cut, code, err
}

// workspaceBase is the commit a coding chat's turn starts from ("" when the
// repository has none yet or git fails). It is kept by the runner, never in a
// file the agent could edit.
func (r *Runner) workspaceBase(ctx context.Context, runID string) string {
	out, _, code, err := r.gitCmd(ctx, runID, nil, 1<<10, "rev-parse", "--verify", "-q", "HEAD")
	base := strings.TrimSpace(string(out))
	if err != nil || code != 0 || !commitRe.MatchString(base) {
		return ""
	}
	return base
}

// repoFilterEnv neutralizes the clean/process filters the repository config
// defines: `git diff` of the working tree would run them. Config read here is
// only listed, never executed; the result is passed as command-line config.
func (r *Runner) repoFilterEnv(ctx context.Context, runID string) (map[string]string, error) {
	out, _, code, err := r.gitCmd(ctx, runID, nil, 1<<20, "config", "--list", "-z")
	if err != nil || code != 0 {
		return nil, fmt.Errorf("git config --list: exit %d: %v", code, err)
	}
	var keys []string
	for _, rec := range bytes.Split(out, []byte{0}) {
		key, _, _ := strings.Cut(string(rec), "\n")
		if strings.HasPrefix(key, "filter.") && (strings.HasSuffix(key, ".clean") || strings.HasSuffix(key, ".process") || strings.HasSuffix(key, ".smudge")) {
			keys = append(keys, key)
		}
	}
	env := map[string]string{"GIT_CONFIG_COUNT": strconv.Itoa(len(keys))}
	for i, k := range keys {
		env["GIT_CONFIG_KEY_"+strconv.Itoa(i)] = k
		env["GIT_CONFIG_VALUE_"+strconv.Itoa(i)] = ""
	}
	return env, nil
}

// codeChanges lists what changed in the workspace since base: commits and
// uncommitted tracked changes (the working tree against base) plus new
// untracked files. Secrets in env are redacted from the result.
func (r *Runner) codeChanges(ctx context.Context, runID, base string, env map[string]string) ([]CodeChange, error) {
	filterEnv, err := r.repoFilterEnv(ctx, runID)
	if err != nil {
		return nil, err
	}
	diffArgs := func(mode ...string) []string {
		args := []string{"diff", "--no-ext-diff", "--no-textconv", "-M", "--src-prefix=a/", "--dst-prefix=b/"}
		return append(append(args, mode...), base)
	}
	tracked := func(limit int64, mode ...string) ([]byte, bool, error) {
		out, cut, code, err := r.gitCmd(ctx, runID, filterEnv, limit, append(diffArgs(mode...), codeChangePathspec...)...)
		if err != nil || code != 0 {
			return nil, false, fmt.Errorf("git diff %v: exit %d: %v", mode, code, err)
		}
		return out, cut, nil
	}
	numstat, cut, err := tracked(1<<20, "--numstat", "-z")
	if err != nil || cut {
		return nil, fmt.Errorf("git diff --numstat: %v (cut %v)", err, cut)
	}
	patch, patchCut, err := tracked(codeDiffReadLimit, "-p", "--unified=3")
	if err != nil {
		return nil, err
	}
	changes, err := ParseCodeChanges(numstat, string(patch), patchCut)
	if err != nil {
		return nil, err
	}
	// New files git doesn't track yet (not ignored ones, and never our own).
	list, _, code, err := r.gitCmd(ctx, runID, nil, 1<<20, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil || code != 0 {
		return nil, fmt.Errorf("git ls-files: exit %d: %v", code, err)
	}
	seen := 0
	for _, name := range bytes.Split(list, []byte{0}) {
		f := string(name)
		if f == "" || f == "AGENTS.md" || strings.HasPrefix(f, ".lazyagents/") {
			continue
		}
		if len(changes) >= CodeChangeMaxFiles || seen >= CodeChangeMaxFiles {
			break
		}
		seen++
		out, cut, code, err := r.gitCmd(ctx, runID, nil, codeUntrackedReadLimit, "diff", "--no-index", "--no-ext-diff", "--no-textconv",
			"--src-prefix=a/", "--dst-prefix=b/", "--unified=3", "--", "/dev/null", f)
		if err != nil || code > 1 { // exit 1 only says the files differ
			r.Log.Warn("diff untracked file", "run", runID, "err", err, "exit", code)
			continue
		}
		changes = append(changes, UntrackedChange(f, string(out), cut))
	}
	changes = ApplyCodeChangeBudget(changes)
	for i := range changes {
		c := &changes[i]
		c.Path = redact(c.Path, env)
		if c.OldPath != nil {
			old := redact(*c.OldPath, env)
			c.OldPath = &old
		}
		if c.Patch != nil {
			p := redact(*c.Patch, env)
			c.Patch = &p
		}
	}
	return changes, nil
}

// chatCodeChanges is what a finished coding chat turn changed, as the JSON
// stored on its assistant message: "[]" when nothing changed or the changes
// can't be read, which is logged and never fails the turn.
func (r *Runner) chatCodeChanges(ctx context.Context, c runCtx, base string, env map[string]string, res harness.Result, code int, waitErr error) []byte {
	none := []byte("[]")
	if base == "" || waitErr != nil || code != 0 || res.IsError || res.UsageLimited {
		return none
	}
	tctx, cancel := context.WithTimeout(ctx, codeChangesTimeout)
	defer cancel()
	changes, err := r.codeChanges(tctx, c.run.ID.String(), base, env)
	if err == nil {
		var raw []byte
		if raw, err = json.Marshal(changes); err == nil && len(changes) > 0 {
			return raw
		}
	}
	if err != nil {
		r.Log.Warn("code changes", "run", c.run.ID, "err", err)
		r.event(ctx, c.run.ID, "lifecycle", map[string]any{"state": "code_changes_failed", "detail": redact(err.Error(), env)})
	}
	return none
}

// chatPushedBranches are the branches a coding chat's pre-push hook recorded
// it creating that the remote now has (a push that failed after the hook ran
// leaves no remote-tracking ref), oldest first. The record is in a file the
// agent can edit, so only names a chat may create are kept. nil when there are
// none or they can't be read, which never fails the turn.
func (r *Runner) chatPushedBranches(ctx context.Context, c runCtx) []string {
	if c.run.Purpose != "chat" || !allowedScope(c.spec, "repo:write") {
		return nil
	}
	tctx, cancel := context.WithTimeout(ctx, codeChangesTimeout)
	defer cancel()
	id := c.run.ID.String()
	out, _, code, err := r.gitCmd(tctx, id, nil, 64<<10, "config", "--get-all", ChatBranchConfig)
	if err != nil || code != 0 { // 1: nothing recorded
		return nil
	}
	remote, _, code, err := r.gitCmd(tctx, id, nil, 1<<20, "for-each-ref", "--format=%(refname)", "refs/remotes/origin/")
	if err != nil || code != 0 {
		r.Log.Warn("list pushed branches", "run", c.run.ID, "err", err, "exit", code)
		return nil
	}
	pushed := map[string]bool{}
	for _, ref := range strings.Split(string(remote), "\n") {
		pushed[strings.TrimPrefix(strings.TrimSpace(ref), "refs/remotes/origin/")] = true
	}
	var names []string
	for _, name := range strings.Split(string(out), "\n") {
		name = strings.TrimSpace(name)
		if personas.ValidWorkBranch(name) && pushed[name] && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}
