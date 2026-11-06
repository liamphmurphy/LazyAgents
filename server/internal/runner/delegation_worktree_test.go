package runner_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/runner"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// spyBackend looks at the parent's workspace just before it's removed, i.e.
// after the runner has cleaned up the delegated runs' worktrees.
type spyBackend struct {
	runner.Backend
	worktrees []string // directories left under .lazyagents/worktrees
	branches  string   // git branch --list
}

func (s *spyBackend) RemoveWorkspace(ctx context.Context, runID string) error {
	root := s.WorkspaceDir(runID)
	if ents, err := os.ReadDir(filepath.Join(root, ".lazyagents", "worktrees")); err == nil {
		for _, e := range ents {
			s.worktrees = append(s.worktrees, e.Name())
		}
	}
	cmd := exec.Command("git", "branch", "--list")
	cmd.Dir = root
	out, _ := cmd.CombinedOutput()
	s.branches = string(out)
	return s.Backend.RemoveWorkspace(ctx, runID)
}

func (e env) subagent(handle string) store.Agent {
	e.t.Helper()
	a, err := (&agents.Service{Pool: e.pool}).Create(e.ctx, e.ws, e.me.ID, handle, agents.Identity{Name: handle, Role: "Dev", Kind: agents.KindSub}, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return a
}

func script(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type args = map[string]any

func (e env) children(parent store.Run) []store.Run {
	e.t.Helper()
	kids, err := e.q.ChildRuns(e.ctx, &parent.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return kids
}

func (e env) checksOf(k store.Run) runs.Checks {
	e.t.Helper()
	var c runs.Checks
	if err := json.Unmarshal(k.Checks, &c); err != nil {
		e.t.Fatalf("checks %s: %v", k.Checks, err)
	}
	return c
}

func (e env) contractOf(k store.Run) runs.Contract {
	e.t.Helper()
	var c runs.Contract
	if err := json.Unmarshal(k.Contract, &c); err != nil {
		e.t.Fatalf("contract %s: %v", k.Contract, err)
	}
	return c
}

func readLog(t *testing.T, path string) []map[string]string {
	t.Helper()
	var out []map[string]string
	raw, _ := os.ReadFile(path)
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var m map[string]string
		if json.Unmarshal([]byte(l), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func spy(e env) *spyBackend {
	s := &spyBackend{Backend: e.runner.Backend}
	e.runner.Backend = s
	return s
}

func TestParallelDelegationMergesBothChildren(t *testing.T) {
	e := setup(t)
	sp := spy(e)
	e.subagent("scribe")
	e.subagent("tester")
	syncDir, logPath := t.TempDir(), filepath.Join(t.TempDir(), "log.jsonl")
	t.Setenv("FAKE_HARNESS_MODE", "delegate_script")
	t.Setenv("FAKE_HARNESS_SYNC", syncDir) // each child waits until both are running at once
	t.Setenv("FAKE_HARNESS_LOG", logPath)
	t.Setenv("FAKE_HARNESS_DELEGATIONS", script(t, []args{
		{"agent": "scribe", "task": "Write FILE=docs/a.txt", "allowed_paths": []string{"docs/"}, "test_command": "test -f docs/a.txt"},
		{"agent": "tester", "task": "Write FILE=src/b.txt", "allowed_paths": []string{"src/"}},
	}))
	e.dispatch("PAY-214")
	e.until("completed", e.state("PAY-214=completed"))
	parent := e.run("PAY-214")
	outcome := deref(parent.Outcome)
	kids := e.children(parent)
	if len(kids) != 2 {
		t.Fatalf("children %d\n%s", len(kids), outcome)
	}
	if ents, _ := os.ReadDir(syncDir); len(ents) != 2 {
		t.Fatalf("children didn't run at the same time: %d started", len(ents))
	}
	// Resumed exactly once, with a report covering both.
	turns, _ := e.q.ListTurns(e.ctx, parent.ID)
	if len(turns) != 2 || turns[1].Trigger != "delegation" || !strings.Contains(turns[1].InputMessage, "2 delegated runs have ended") {
		t.Fatalf("parent turns %+v", turns)
	}
	if !strings.HasPrefix(outcome, "RESUMED") || strings.Count(outcome, "**MERGED**") != 2 || !strings.Contains(outcome, "@scribe") || !strings.Contains(outcome, "@tester") {
		t.Fatalf("resume report:\n%s", outcome)
	}
	for _, want := range []string{"docs/a.txt", "src/b.txt", "Merge commit"} {
		if !strings.Contains(outcome, want) {
			t.Fatalf("parent checkout lacks %q:\n%s", want, outcome)
		}
	}
	branches := map[string]bool{}
	for _, k := range kids {
		c, ct := e.checksOf(k), e.contractOf(k)
		if k.State != "completed" || !c.Merged || !c.Passed || c.Branch != ct.Branch || ct.Worktree != ".lazyagents/worktrees/"+k.ID.String() || ct.BaseSHA == "" || len(ct.BaseUncommitted) != 0 {
			t.Fatalf("child %s: checks %s contract %s", k.State, k.Checks, k.Contract)
		}
		if !strings.Contains(ct.Branch, "--") || !strings.HasSuffix(ct.Branch, "-"+k.ID.String()[:8]) {
			t.Fatalf("branch %q", ct.Branch)
		}
		branches[ct.Branch] = true
		if k.ChecksMs == nil || k.SetupMs == nil || k.HarnessMs == nil {
			t.Fatalf("timings setup=%v harness=%v checks=%v", k.SetupMs, k.HarnessMs, k.ChecksMs)
		}
	}
	if len(branches) != 2 {
		t.Fatalf("branches %v", branches)
	}
	// Each child ran with its own worktree as cwd.
	cwds := map[string]bool{}
	for _, l := range readLog(t, logPath) {
		if strings.Contains(l["prompt"], "delegated part of") {
			if !strings.Contains(l["cwd"], filepath.Join(".lazyagents", "worktrees")) {
				t.Fatalf("child cwd %q", l["cwd"])
			}
			cwds[l["cwd"]] = true
		}
	}
	if len(cwds) != 2 {
		t.Fatalf("child cwds %v", cwds)
	}
	// Worktrees are gone and merged branches deleted once the parent ended.
	if len(sp.worktrees) != 0 || strings.Contains(sp.branches, "--") {
		t.Fatalf("left behind: worktrees %v branches %q", sp.worktrees, sp.branches)
	}
}

func TestFailedChecksLeaveBranchUnmerged(t *testing.T) {
	e := setup(t)
	sp := spy(e)
	e.subagent("scribe")
	t.Setenv("FAKE_HARNESS_MODE", "delegate_script")
	t.Setenv("FAKE_HARNESS_DELEGATIONS", script(t, []args{
		{"agent": "scribe", "task": "Write FILE=src/oops.txt", "allowed_paths": []string{"docs/"}},
	}))
	e.dispatch("PAY-214")
	e.until("completed", e.state("PAY-214=completed"))
	parent := e.run("PAY-214")
	kid := e.children(parent)[0]
	c := e.checksOf(kid)
	if c.Merged || c.Passed || c.Branch == "" || len(c.OutOfBounds) != 1 {
		t.Fatalf("checks %s", kid.Checks)
	}
	out := deref(parent.Outcome)
	if !strings.Contains(out, "**NOT MERGED**") || !strings.Contains(out, c.Branch) || !strings.Contains(out, "[git branch]") {
		t.Fatalf("report:\n%s", out)
	}
	if after := out[strings.Index(out, "[git ls-files]"):]; strings.Contains(after, "src/oops.txt") {
		t.Fatalf("unmerged work reached the parent branch:\n%s", out)
	}
	if !strings.Contains(sp.branches, c.Branch) || len(sp.worktrees) != 0 {
		t.Fatalf("worktrees %v, branches %q (want %s kept)", sp.worktrees, sp.branches, c.Branch)
	}
}

func TestMergeConflictIsAbortedAndReported(t *testing.T) {
	e := setup(t)
	e.subagent("scribe")
	hold := filepath.Join(t.TempDir(), "release")
	t.Setenv("FAKE_HARNESS_MODE", "delegate_script")
	t.Setenv("FAKE_HARNESS_HOLD", hold)
	t.Setenv("FAKE_HARNESS_DELEGATIONS", script(t, []args{{"agent": "scribe", "task": "Write FILE=shared.txt"}}))
	e.dispatch("PAY-214")
	// Tick without waiting: the child blocks until released.
	for i := 0; ; i++ {
		e.runner.Tick(e.ctx)
		if _, err := os.Stat(hold + ".started"); err == nil {
			break
		}
		if i > 300 {
			t.Fatal("child never started")
		}
		time.Sleep(30 * time.Millisecond)
	}
	// Meanwhile the parent's branch moves on with a conflicting change.
	parent := e.run("PAY-214")
	root := e.runner.Backend.WorkspaceDir(parent.ID.String())
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("the parent's version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, a := range [][]string{{"add", "shared.txt"}, {"commit", "-q", "-m", "Parent adds shared.txt"}} {
		cmd := exec.Command("git", a...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	e.until("completed", e.state("PAY-214=completed"))
	parent = e.run("PAY-214")
	kid := e.children(parent)[0]
	c := e.checksOf(kid)
	if c.Merged || len(c.MergeConflicts) != 1 || c.MergeConflicts[0] != "shared.txt" || c.Branch == "" {
		t.Fatalf("checks %s", kid.Checks)
	}
	out := deref(parent.Outcome)
	if !strings.Contains(out, "**NOT MERGED**") || !strings.Contains(out, "conflicted") || !strings.Contains(out, "shared.txt") || !strings.Contains(out, c.Branch) {
		t.Fatalf("report:\n%s", out)
	}
	state := out[strings.Index(out, "=== checkout"):]
	if strings.Contains(state, "Merge commit") || strings.Contains(state, "UU ") || strings.Contains(state[strings.Index(state, "[git status]"):], "shared.txt") {
		t.Fatalf("merge wasn't aborted cleanly:\n%s", state)
	}
}

func TestDirtyParentCheckoutRejectsDelegation(t *testing.T) {
	e := setup(t)
	e.subagent("scribe")
	t.Setenv("FAKE_HARNESS_MODE", "delegate_script")
	t.Setenv("FAKE_HARNESS_DIRTY", "1")
	t.Setenv("FAKE_HARNESS_DELEGATIONS", script(t, []args{{"agent": "scribe", "task": "Write FILE=docs/a.txt"}}))
	e.dispatch("PAY-214")
	e.until("completed", e.state("PAY-214=completed"))
	parent := e.run("PAY-214")
	kid := e.children(parent)[0]
	if kid.State != "failed" || !strings.Contains(deref(kid.Outcome), "commit your work before delegating") || strings.Contains(string(kid.Contract), "worktree") {
		t.Fatalf("child %s %q contract %s", kid.State, deref(kid.Outcome), kid.Contract)
	}
	if out := deref(parent.Outcome); !strings.Contains(out, "commit your work before delegating") || !strings.Contains(out, "dirty.txt") {
		t.Fatalf("parent told:\n%s", out)
	}
}

func TestContinueRunReusesWorktreeAndSession(t *testing.T) {
	for name, reject := range map[string]bool{"resumes the session": false, "falls back to a fresh session": true} {
		t.Run(name, func(t *testing.T) {
			e := setup(t)
			e.subagent("scribe")
			logPath := filepath.Join(t.TempDir(), "log.jsonl")
			t.Setenv("FAKE_HARNESS_MODE", "delegate_script")
			t.Setenv("FAKE_HARNESS_LOG", logPath)
			if reject {
				t.Setenv("FAKE_HARNESS_REJECT_RESUME", "1")
			}
			t.Setenv("FAKE_HARNESS_DELEGATIONS", script(t, []args{{"agent": "scribe", "task": "Write FILE=docs/a.txt", "allowed_paths": []string{"docs/"}}}))
			t.Setenv("FAKE_HARNESS_ROUND2", script(t, []args{{"agent": "scribe", "task": "Fix it: FILE=docs/b.txt", "allowed_paths": []string{"docs/"}, "continue_run_id": "$1"}}))
			e.dispatch("PAY-214")
			e.until("completed", e.state("PAY-214=completed"))
			parent := e.run("PAY-214")
			kids := e.children(parent)
			if len(kids) != 2 || kids[0].State != "completed" || kids[1].State != "completed" {
				t.Fatalf("children %+v\n%s", kids, deref(parent.Outcome))
			}
			first, second := e.contractOf(kids[0]), e.contractOf(kids[1])
			if first.Worktree == "" || second.Worktree != first.Worktree || second.Branch != first.Branch || second.ContinueRunID != kids[0].ID.String() {
				t.Fatalf("contracts\n%s\n%s", kids[0].Contract, kids[1].Contract)
			}
			if !e.checksOf(kids[0]).Merged || !e.checksOf(kids[1]).Merged {
				t.Fatalf("checks\n%s\n%s", kids[0].Checks, kids[1].Checks)
			}
			session := deref(kids[0].HarnessSessionID)
			if session == "" {
				t.Fatal("first child recorded no harness session")
			}
			var resumes []string
			var cwds []string
			for _, l := range readLog(t, logPath) {
				if strings.Contains(l["prompt"], "delegated part of") {
					resumes = append(resumes, l["resume"])
					cwds = append(cwds, l["cwd"])
				}
			}
			if reject {
				// The resume was refused, so the turn ran again without a session.
				if len(resumes) != 3 || resumes[0] != "" || resumes[1] != session || resumes[2] != "" {
					t.Fatalf("child resumes %q", resumes)
				}
				if ss := deref(kids[1].HarnessSessionID); ss == session {
					t.Fatalf("second child kept the dead session")
				}
			} else if len(resumes) != 2 || resumes[0] != "" || resumes[1] != session {
				t.Fatalf("child resumes %q, want [\"\" %q]", resumes, session)
			}
			if cwds[0] != cwds[len(cwds)-1] {
				t.Fatalf("worktree cwds differ: %q", cwds)
			}
			turns, _ := e.q.ListTurns(e.ctx, parent.ID)
			if len(turns) != 3 {
				t.Fatalf("parent turns %d", len(turns))
			}
			// The repair started from the merged state and was merged in turn.
			if out := deref(parent.Outcome); !strings.Contains(out, "docs/a.txt") || !strings.Contains(out, "docs/b.txt") || strings.Count(out, "**MERGED**") != 1 {
				t.Fatalf("outcome:\n%s", out)
			}
		})
	}
}

// tickUntil doesn't wait for held fake processes to exit.
func (e env) tickUntil(what string, ready func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		e.runner.Tick(e.ctx)
		if ready() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.t.Fatalf("timed out waiting for %s", what)
}

func releaseFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPausedParentChecksAndMergesAfterResume(t *testing.T) {
	e := setup(t)
	e.subagent("scribe")
	hold := filepath.Join(t.TempDir(), "release")
	t.Setenv("FAKE_HARNESS_MODE", "delegate_script")
	t.Setenv("FAKE_HARNESS_HOLD", hold)
	t.Setenv("FAKE_HARNESS_DELEGATIONS", script(t, []args{{"agent": "scribe", "task": "Write FILE=docs/a.txt"}}))
	e.dispatch("PAY-214")
	e.tickUntil("held child", func() bool { _, err := os.Stat(hold + ".started"); return err == nil })
	parent := e.run("PAY-214")
	if _, err := e.runs.SetRunControl(e.ctx, e.ws, e.me.ID, parent.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	releaseFile(t, hold)
	e.runner.Wait()
	if got := e.run("PAY-214"); got.State != "paused" {
		t.Fatalf("parent %s", got.State)
	}
	if _, err := e.runs.SetRunControl(e.ctx, e.ws, e.me.ID, parent.ID, "resume"); err != nil {
		t.Fatal(err)
	}
	turns, _ := e.q.ListTurns(e.ctx, parent.ID)
	if len(turns) != 2 || turns[1].Trigger != "delegation" || turns[1].Status != "pending" {
		t.Fatalf("resume turns %+v", turns)
	}
	e.until("completed", e.state("PAY-214=completed"))
	kid := e.children(parent)[0]
	checks := e.checksOf(kid)
	if !checks.Merged || checks.HeadSHA == "" || !strings.Contains(deref(e.run("PAY-214").Outcome), "docs/a.txt") {
		t.Fatalf("checks %s", kid.Checks)
	}
}

func TestDelegationWaitsForChildProcessExit(t *testing.T) {
	e := setup(t)
	e.subagent("scribe")
	hold := filepath.Join(t.TempDir(), "linger")
	t.Setenv("FAKE_HARNESS_MODE", "delegate_script")
	t.Setenv("FAKE_HARNESS_LINGER", hold)
	t.Setenv("FAKE_HARNESS_DELEGATIONS", script(t, []args{{"agent": "scribe", "task": "Write FILE=docs/a.txt"}}))
	e.dispatch("PAY-214")
	e.tickUntil("completed child still running", func() bool { _, err := os.Stat(hold + ".started"); return err == nil })
	parent := e.run("PAY-214")
	for i := 0; i < 5; i++ {
		e.runner.Tick(e.ctx)
		time.Sleep(20 * time.Millisecond)
	}
	turns, _ := e.q.ListTurns(e.ctx, parent.ID)
	if len(turns) != 2 || turns[1].Status != "pending" {
		t.Fatalf("parent ran before child exited: %+v", turns)
	}
	child := e.children(parent)[0]
	if len(child.Checks) != 0 {
		t.Fatalf("checks before process exited: %s", child.Checks)
	}
	releaseFile(t, hold)
	e.until("completed", e.state("PAY-214=completed"))
	turns, _ = e.q.ListTurns(e.ctx, parent.ID)
	ct, _ := e.q.ListTurns(e.ctx, child.ID)
	if turns[1].ProcessStartedAt == nil || ct[0].EndedAt == nil || turns[1].ProcessStartedAt.Before(*ct[0].EndedAt) {
		t.Fatalf("turn timestamps: parent %+v child %+v", turns[1], ct[0])
	}
}

func TestQueuedChildDoesNotOccupyRunnerSlotWhileParentRuns(t *testing.T) {
	e := setup(t)
	e.subagent("scribe")
	hold := filepath.Join(t.TempDir(), "parent")
	t.Setenv("FAKE_HARNESS_MODE", "delegate_script")
	t.Setenv("FAKE_HARNESS_PARENT_HOLD", hold)
	t.Setenv("FAKE_HARNESS_DELEGATIONS", script(t, []args{{"agent": "scribe", "task": "Write FILE=docs/a.txt"}}))
	e.dispatch("PAY-214")
	e.tickUntil("held parent", func() bool { _, err := os.Stat(hold + ".started"); return err == nil })
	parent := e.run("PAY-214")
	// Even an arbitrarily old running parent keeps its child queued, without a timeout.
	if _, err := e.pool.Exec(e.ctx, "UPDATE run_turns SET process_started_at=now()-interval '10 minutes' WHERE run_id=$1", parent.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		e.runner.Tick(e.ctx)
		time.Sleep(20 * time.Millisecond)
	}
	child := e.children(parent)[0]
	if child.State != "queued" || child.SetupMs != nil {
		t.Fatalf("child was claimed: %+v", child)
	}
	// Restore bookkeeping so the artificial age doesn't consume the parent's runtime budget.
	if _, err := e.pool.Exec(e.ctx, "UPDATE run_turns SET process_started_at=now() WHERE run_id=$1", parent.ID); err != nil {
		t.Fatal(err)
	}
	releaseFile(t, hold)
	e.until("completed", e.state("PAY-214=completed"))
	if child = e.children(parent)[0]; child.State != "completed" || !e.checksOf(child).Merged {
		t.Fatalf("child %s checks %s", child.State, child.Checks)
	}
}

func TestDelegationReportReplaysStoredChecks(t *testing.T) {
	e := setup(t)
	e.subagent("scribe")
	e.subagent("tester")
	hold := filepath.Join(t.TempDir(), "release")
	t.Setenv("FAKE_HARNESS_MODE", "delegate_script")
	t.Setenv("FAKE_HARNESS_HOLD", hold)
	t.Setenv("FAKE_HARNESS_DELEGATIONS", script(t, []args{
		{"agent": "scribe", "task": "Write FILE=docs/a.txt", "allowed_paths": []string{"docs/"}},
		{"agent": "tester", "task": "Write FILE=src/b.txt", "allowed_paths": []string{"src/"}},
	}))
	e.dispatch("PAY-214")
	e.tickUntil("two held children", func() bool {
		kids := e.children(e.run("PAY-214"))
		if len(kids) != 2 {
			return false
		}
		for _, k := range kids {
			turns, _ := e.q.ListTurns(e.ctx, k.ID)
			if len(turns) != 1 || turns[0].Status != "running" {
				return false
			}
		}
		return true
	})
	releaseFile(t, hold)
	e.runner.Wait()
	parent := e.run("PAY-214")
	kids := e.children(parent)
	turns, _ := e.q.ListTurns(e.ctx, parent.ID)
	if len(turns) != 2 || turns[1].Status != "pending" {
		t.Fatalf("turns %+v", turns)
	}
	// Simulate a crash after the first child's merge and checks were stored.
	contract := e.contractOf(kids[0])
	root := e.runner.Backend.WorkspaceDir(parent.ID.String())
	git := func(a ...string) string {
		t.Helper()
		cmd := exec.Command("git", a...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", a, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	sha := git("rev-parse", contract.Branch)
	git("merge", "--no-ff", "--no-edit", sha)
	stored := runs.Checks{Ran: true, Passed: true, Merged: true, ChildState: "completed", Branch: contract.Branch, HeadSHA: sha, ReportedTurn: int(turns[1].Seq), DiffStat: "stored report marker"}
	raw, _ := json.Marshal(stored)
	if err := e.q.SetRunChecks(e.ctx, store.SetRunChecksParams{ID: kids[0].ID, Checks: raw}); err != nil {
		t.Fatal(err)
	}
	// Removing the branch proves replay doesn't try to check or merge it again.
	git("worktree", "remove", "--force", contract.Worktree)
	git("branch", "-D", contract.Branch)
	e.until("completed", e.state("PAY-214=completed"))
	outcome := deref(e.run("PAY-214").Outcome)
	if strings.Count(outcome, "**MERGED**") != 2 || !strings.Contains(outcome, "stored report marker") {
		t.Fatalf("report %s", outcome)
	}
	got := e.children(parent)
	check := e.checksOf(got[0])
	if check.HeadSHA != sha || check.DiffStat != stored.DiffStat || check.ReportedTurn != stored.ReportedTurn {
		t.Fatalf("stored result changed: %s", got[0].Checks)
	}
}

func TestMergeUsesCheckedCommit(t *testing.T) {
	e := setup(t)
	e.subagent("scribe")
	t.Setenv("FAKE_HARNESS_MODE", "delegate_script")
	t.Setenv("FAKE_HARNESS_DELEGATIONS", script(t, []args{{"agent": "scribe", "task": "Write FILE=docs/a.txt", "test_command": "git commit --allow-empty -q -m 'Unchecked commit'"}}))
	e.dispatch("PAY-214")
	e.until("completed", e.state("PAY-214=completed"))
	parent := e.run("PAY-214")
	child := e.children(parent)[0]
	checks := e.checksOf(child)
	if !checks.Merged || checks.HeadSHA == "" {
		t.Fatalf("checks %s", child.Checks)
	}
	out := deref(parent.Outcome)
	if checkout := out[strings.Index(out, "=== checkout"):]; strings.Contains(checkout, "Unchecked commit") || !strings.Contains(checkout, "Add docs/a.txt") {
		t.Fatalf("merged unchecked head: %s", out)
	}
}

func TestChildCancelledBeforeParentProcessExits(t *testing.T) {
	e := setup(t)
	e.subagent("scribe")
	hold := filepath.Join(t.TempDir(), "parent")
	t.Setenv("FAKE_HARNESS_MODE", "delegate_script")
	t.Setenv("FAKE_HARNESS_PARENT_HOLD", hold)
	t.Setenv("FAKE_HARNESS_DELEGATIONS", script(t, []args{{"agent": "scribe", "task": "Write FILE=docs/a.txt"}}))
	e.dispatch("PAY-214")
	e.tickUntil("held parent", func() bool { _, err := os.Stat(hold + ".started"); return err == nil })
	parent := e.run("PAY-214")
	child := e.children(parent)[0]
	if _, err := e.runs.SetRunControl(e.ctx, e.ws, e.me.ID, child.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	_, err := e.runs.Delegate(e.ctx, agents.Caller{Agent: e.atlas, Kind: "run", RunID: &parent.ID}, "", "scribe", "another task")
	if err == nil || !strings.Contains(err.Error(), "end your turn to read their report") {
		t.Fatalf("delegate after child ended: %v", err)
	}
	for i := 0; i < 5; i++ {
		e.runner.Tick(e.ctx)
		time.Sleep(20 * time.Millisecond)
	}
	turns, _ := e.q.ListTurns(e.ctx, parent.ID)
	if len(turns) != 2 || turns[1].Status != "pending" {
		t.Fatalf("report started while parent running: %+v", turns)
	}
	releaseFile(t, hold)
	e.until("completed", e.state("PAY-214=completed"))
	turns, _ = e.q.ListTurns(e.ctx, parent.ID)
	if len(turns) != 2 || len(e.children(parent)) != 1 {
		t.Fatalf("duplicate report or delegation: %+v", turns)
	}
	if len(e.children(parent)[0].Checks) == 0 {
		t.Fatal("cancelled child was never reported")
	}
}
