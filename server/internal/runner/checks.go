package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// noCommits is the base "commit" of a checkout that had no commits yet.
const noCommits = "none"

const (
	checkTimeout     = 30 * time.Second
	testCheckTimeout = 10 * time.Minute
)

// execOnce runs a shell script in a run's workspace and returns its combined
// output and exit code. The process is killed when ctx ends.
func (r *Runner) execOnce(ctx context.Context, ws, name, script string, env map[string]string) (string, int, error) {
	proc, err := r.Backend.Start(ctx, ws, ExecSpec{Name: name, Argv: []string{"sh", "-c", script}, Env: env})
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
// LazyAgents' own.
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

// recordBaseline notes the checkout's HEAD and dirty files when a delegated
// run starts, so the checks can tell what it changed.
func (r *Runner) recordBaseline(ctx context.Context, c runCtx, env map[string]string) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	out, code, err := r.execOnce(ctx, wsID(c.run), "lazyagents-base-"+c.run.ID.String(), "git rev-parse HEAD", env)
	sha := strings.TrimSpace(out)
	if err != nil {
		return
	}
	if code != 0 || sha == "" {
		sha = noCommits // an unborn branch: everything committed later is new
	}
	var contract runs.Contract
	if len(c.run.Contract) > 0 {
		_ = json.Unmarshal(c.run.Contract, &contract)
	}
	contract.BaseSHA = sha
	if st, code, err := r.execOnce(ctx, wsID(c.run), "lazyagents-base-status-"+c.run.ID.String(), "git status --porcelain", env); err == nil && code == 0 {
		contract.BaseUncommitted = parseStatus(st)
	}
	raw, _ := json.Marshal(contract)
	if _, err := r.q().SetRunContract(ctx, store.SetRunContractParams{ID: c.run.ID, Contract: raw}); err != nil {
		r.Log.Warn("record baseline", "run", c.run.ID, "err", err)
	}
}

// delegationReport runs the deterministic checks on the latest delegated run
// under a parent that is about to resume, stores them on that run and returns
// the text to append to the parent's resume message. It returns "" when
// there's no finished child to check.
func (r *Runner) delegationReport(ctx context.Context, parent runCtx, env map[string]string) string {
	kids, err := r.q().ChildRuns(ctx, &parent.run.ID)
	if err != nil || len(kids) == 0 {
		return ""
	}
	child := kids[len(kids)-1]
	if !isDone(child.State) {
		return ""
	}
	var contract runs.Contract
	if len(child.Contract) > 0 {
		_ = json.Unmarshal(child.Contract, &contract)
	}
	k := r.checkChild(ctx, parent, child, contract, env)
	if b, err := json.Marshal(k); err == nil {
		if err := r.q().SetRunChecks(ctx, store.SetRunChecksParams{ID: child.ID, Checks: b}); err != nil {
			r.Log.Warn("store checks", "run", child.ID, "err", err)
		}
	}
	r.event(ctx, child.ID, "lifecycle", map[string]any{"state": "checks", "passed": k.Passed, "ran": k.Ran})
	return "\n\n" + k.Report()
}

func isDone(state string) bool {
	return state == "completed" || state == "failed" || state == "cancelled"
}

func (r *Runner) checkChild(ctx context.Context, parent runCtx, child store.Run, contract runs.Contract, env map[string]string) runs.Checks {
	k := runs.Checks{ChildState: child.State, BaseSHA: contract.BaseSHA, TestCommand: contract.TestCommand}
	ws := wsID(child)
	git := func(script string) (string, bool) {
		cctx, cancel := context.WithTimeout(ctx, checkTimeout)
		defer cancel()
		out, code, err := r.execOnce(cctx, ws, "lazyagents-check-"+child.ID.String(), script, env)
		return out, err == nil && code == 0
	}
	status, ok := git("git status --porcelain")
	if !ok {
		k.Error = "git status failed in the workspace"
		k.Ran = false
		k.Evaluate(contract)
		return k
	}
	for _, f := range parseStatus(status) {
		if !slices.Contains(contract.BaseUncommitted, f) {
			k.Uncommitted = append(k.Uncommitted, f)
		}
	}
	if contract.BaseSHA == "" {
		k.Error = "the checkout's starting commit wasn't recorded, so committed changes can't be listed"
	} else {
		base := shellQuote(contract.BaseSHA)
		diff, stat := "git diff --name-only "+base+" HEAD", "git diff --stat "+base+" HEAD"
		if contract.BaseSHA == noCommits {
			diff, stat = "git ls-tree -r --name-only HEAD 2>/dev/null || true", "git show --stat --format= HEAD 2>/dev/null || true"
		}
		names, ok := git(diff)
		if !ok {
			k.Error = "git diff against the starting commit failed"
		} else {
			for _, f := range strings.Split(strings.TrimSpace(names), "\n") {
				if f != "" {
					k.ChangedFiles = append(k.ChangedFiles, f)
				}
			}
			k.DiffStat, _ = git(stat)
			k.DiffStat = lastLines(strings.TrimSpace(k.DiffStat), 40)
		}
	}
	k.Ran = true
	if contract.TestCommand != "" {
		tctx, cancel := context.WithTimeout(ctx, testCheckTimeout)
		out, code, err := r.execOnce(tctx, ws, "lazyagents-test-"+child.ID.String(), contract.TestCommand, env)
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
