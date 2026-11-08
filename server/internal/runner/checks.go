package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

const (
	checkTimeout     = 30 * time.Second
	testCheckTimeout = 10 * time.Minute
)

// execOnce runs a shell script in a run's workspace and returns its combined
// output and exit code. The process is killed when ctx ends.
func (r *Runner) execOnce(ctx context.Context, ws, name, script string, env map[string]string) (string, int, error) {
	return r.execIn(ctx, ws, "", name, script, env)
}

// execIn is execOnce in a directory of the workspace (a delegated run's worktree).
func (r *Runner) execIn(ctx context.Context, ws, dir, name, script string, env map[string]string) (string, int, error) {
	proc, err := r.Backend.Start(ctx, ws, ExecSpec{Name: name, Argv: []string{"sh", "-c", script}, Env: env, Dir: dir})
	if err != nil {
		return "", -1, err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = proc.Kill()
		case <-done:
		}
	}()
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
	return out.String(), code, err
}

// parseStatus lists the files `git status --porcelain` reports, minus
// Vesper's own.
func parseStatus(out string) []string {
	var files []string
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if len(l) < 4 {
			continue
		}
		f := l[3:]
		if i := strings.LastIndex(f, " -> "); i >= 0 {
			f = f[i+4:]
		}
		if f = strings.Trim(f, `"`); !strings.HasPrefix(f, ".lazyagents/") {
			files = append(files, f)
		}
	}
	return files
}

// parkDelegation holds the parent lock while checking children and parking it,
// so a child's concurrent WakeParent cannot be lost between those operations.
func (r *Runner) parkDelegation(ctx context.Context, parent store.Run, turn store.RunTurn) (bool, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)
	locked, err := q.LockRun(ctx, parent.ID)
	if err != nil {
		return false, err
	}
	if locked.State != "running" {
		if err := q.FinishTurn(ctx, store.FinishTurnParams{ID: turn.ID, Usage: []byte("{}")}); err != nil {
			return false, err
		}
		return true, tx.Commit(ctx)
	}
	live, err := q.HasLiveChild(ctx, &parent.ID)
	if err != nil {
		return false, err
	}
	if !live {
		return false, nil
	}
	wait := "delegation"
	if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: parent.ID, State: "waiting", WaitReason: &wait}); err != nil {
		return false, err
	}
	if err := q.FinishTurn(ctx, store.FinishTurnParams{ID: turn.ID, Usage: []byte("{}")}); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// delegationReport checks every delegated run that ended since the parent was
// parked, in its worktree, merges the ones that passed into the parent's
// branch, stores the results on the runs and returns the text to append to the
// parent's resume message. It returns "" when no finished child is waiting to be
// checked.
func (r *Runner) delegationReport(ctx context.Context, parent runCtx, env map[string]string, reportedTurn int) string {
	kids, err := r.q().ChildRuns(ctx, &parent.run.ID)
	if err != nil {
		return ""
	}
	var ended []store.Run
	for _, k := range kids {
		if !isDone(k.State) {
			return ""
		}
		var checks runs.Checks
		if len(k.Checks) == 0 || (json.Unmarshal(k.Checks, &checks) == nil && checks.ReportedTurn == reportedTurn) {
			ended = append(ended, k)
		}
	}
	if len(ended) == 0 {
		return ""
	}
	ws := wsID(parent.run)
	parentBranch := r.gitOut(ctx, ws, "", "lazyagents-branch-"+parent.run.ID.String(), "git symbolic-ref --short HEAD", env)
	var report strings.Builder
	for _, child := range ended {
		var contract runs.Contract
		if len(child.Contract) > 0 {
			_ = json.Unmarshal(child.Contract, &contract)
		}
		var k runs.Checks
		if len(child.Checks) > 0 {
			_ = json.Unmarshal(child.Checks, &k)
		} else {
			checksStart := time.Now()
			k = r.checkChild(ctx, child, contract, parentBranch, env)
			k.ReportedTurn = reportedTurn
			r.mergeChild(ctx, ws, parentBranch, child, contract, &k, env)
			r.addTiming(child.ID, 0, 0, time.Since(checksStart))
			if b, err := json.Marshal(k); err == nil {
				if err := r.q().SetRunChecks(ctx, store.SetRunChecksParams{ID: child.ID, Checks: b}); err != nil {
					r.Log.Warn("store checks", "run", child.ID, "err", err)
				}
			}
			r.event(ctx, child.ID, "lifecycle", map[string]any{"state": "checks", "passed": k.Passed, "ran": k.Ran, "merged": k.Merged, "branch": k.Branch})
		}
		report.WriteString("\n\n")
		if len(ended) > 1 {
			handle := child.AgentID.String()
			if a, err := r.q().GetAgent(ctx, store.GetAgentParams{ID: child.AgentID, WorkspaceID: child.WorkspaceID}); err == nil {
				handle = "@" + a.Handle
			}
			report.WriteString("---\n\n# " + handle + " (run " + child.ID.String()[:8] + ", " + child.State + ")\n\n")
		}
		report.WriteString(k.Report())
	}
	return report.String()
}

func isDone(state string) bool {
	return state == "completed" || state == "failed" || state == "cancelled"
}

// gitOut runs a one-line git query and returns its trimmed output, "" on failure.
func (r *Runner) gitOut(ctx context.Context, ws, dir, name, script string, env map[string]string) string {
	cctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	out, code, err := r.execIn(cctx, ws, dir, name, script, env)
	if err != nil || code != 0 {
		return ""
	}
	return strings.TrimSpace(out)
}

// checkChild measures a delegated run's result in its worktree: uncommitted
// files, what its branch changed against the parent's branch, allowed paths
// and the test command.
func (r *Runner) checkChild(ctx context.Context, child store.Run, contract runs.Contract, parentBranch string, env map[string]string) runs.Checks {
	k := runs.Checks{ChildState: child.State, TestCommand: contract.TestCommand, Branch: contract.Branch}
	if contract.Worktree == "" {
		k.Error = "the delegated run never got a worktree (it didn't start), so there is nothing to check"
		k.Evaluate(contract)
		return k
	}
	ws, dir := wsID(child), contract.Worktree
	git := func(script string) (string, bool) {
		cctx, cancel := context.WithTimeout(ctx, checkTimeout)
		defer cancel()
		out, code, err := r.execIn(cctx, ws, dir, "lazyagents-check-"+child.ID.String(), script, env)
		return out, err == nil && code == 0
	}
	head, ok := git("git rev-parse HEAD")
	if !ok {
		k.Error = "git HEAD could not be determined"
		k.Evaluate(contract)
		return k
	}
	k.HeadSHA = strings.TrimSpace(head)
	status, ok := git("git status --porcelain")
	if !ok {
		k.Error = "git status failed in the worktree"
		k.Evaluate(contract)
		return k
	}
	k.Uncommitted = parseStatus(status)
	// What the branch would add to the parent's: everything since it forked from it.
	base := ""
	if parentBranch != "" {
		base, _ = git("git merge-base " + shellQuote(parentBranch) + " " + shellQuote(k.HeadSHA))
		base = strings.TrimSpace(base)
	}
	if base == "" {
		k.Error = "the branch's fork point from the main agent's branch couldn't be found, so committed changes can't be listed"
	} else {
		k.BaseSHA = base
		names, ok := git("git diff --name-only " + shellQuote(base) + " " + shellQuote(k.HeadSHA))
		if !ok {
			k.Error = "git diff against the fork point failed"
		} else {
			for _, f := range strings.Split(strings.TrimSpace(names), "\n") {
				if f != "" {
					k.ChangedFiles = append(k.ChangedFiles, f)
				}
			}
			k.DiffStat, _ = git("git diff --stat " + shellQuote(base) + " " + shellQuote(k.HeadSHA))
			k.DiffStat = lastLines(strings.TrimSpace(k.DiffStat), 40)
		}
	}
	k.Ran = true
	if contract.TestCommand != "" {
		tctx, cancel := context.WithTimeout(ctx, testCheckTimeout)
		out, code, err := r.execIn(tctx, ws, dir, "lazyagents-test-"+child.ID.String(), contract.TestCommand, env)
		timedOut := tctx.Err() != nil
		cancel()
		if err == nil && !timedOut {
			k.TestExit = &code
		}
		k.TestTail = lastLines(redact(out, env), 40)
	}
	k.Evaluate(contract)
	return k
}
