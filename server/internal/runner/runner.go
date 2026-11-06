package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/harness"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

// Config tunes a runner.
type Config struct {
	ID            string        // unique per runner process
	MaxConcurrent int           // processes at once (RUNNER_MAX_CONCURRENT)
	MCPURL        string        // LazyAgents' /mcp as turns reach it
	Poll          time.Duration // queue poll interval
	Grace         time.Duration // after SIGINT, before SIGKILL (pause, cancel)
	LogBucket     string        // raw harness logs (run-logs)
	Version       string
	// RequireHarnessKey refuses to start a run without the harness's API
	// key secret. Docker runs need it; local runs can use the host's login.
	RequireHarnessKey bool
	ModelPrices       map[string]harness.Price
}

// Blobs stores raw logs (blob.Store).
type Blobs interface {
	Put(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) error
}

type Runner struct {
	Cfg     Config
	Pool    *pgxpool.Pool
	Backend Backend
	Secrets Revealer
	Tracker *tracker.Service
	Blobs   Blobs // nil: raw logs aren't kept
	Log     *slog.Logger

	mu     sync.Mutex
	active int
	wg     sync.WaitGroup
	// wtMu serialises git operations on a parent's repository that create,
	// merge or remove delegated runs' worktrees.
	wtMu sync.Mutex
}

const tokenEnv = "LAZYAGENTS_TOKEN"

func (r *Runner) q() *store.Queries { return store.New(r.Pool) }

func (r *Runner) system() tracker.Actor { return tracker.Actor{Type: "system"} }

// Run polls until ctx is cancelled. In-flight processes are killed on exit;
// their turns run again when a runner starts (the harness resumes).
func (r *Runner) Run(ctx context.Context) error {
	if r.Cfg.Poll <= 0 {
		r.Cfg.Poll = time.Second
	}
	if err := r.Recover(ctx); err != nil {
		return err
	}
	beat := time.NewTicker(10 * time.Second)
	defer beat.Stop()
	sweep := time.NewTicker(30 * time.Second)
	defer sweep.Stop()
	poll := time.NewTicker(r.Cfg.Poll)
	defer poll.Stop()
	r.heartbeat(ctx)
	for {
		select {
		case <-ctx.Done():
			r.wg.Wait()
			_ = r.q().DeleteRunner(context.Background(), r.Cfg.ID)
			return nil
		case <-beat.C:
			r.heartbeat(ctx)
		case <-sweep.C:
			r.Sweep(ctx)
		case <-poll.C:
			r.Tick(ctx)
		}
	}
}

// Recover requeues work a dead runner left half-done.
func (r *Runner) Recover(ctx context.Context) error {
	if err := r.Backend.Cleanup(ctx); err != nil {
		r.Log.Warn("runner cleanup", "err", err)
	}
	q := r.q()
	if err := q.RequeueStarting(ctx); err != nil {
		return err
	}
	if err := q.RequeueInterrupted(ctx, &r.Cfg.ID); err != nil {
		return err
	}
	return q.RecoverChatCancellations(ctx, &r.Cfg.ID)
}

func (r *Runner) heartbeat(ctx context.Context) {
	r.mu.Lock()
	n := r.active
	r.mu.Unlock()
	if err := r.q().Heartbeat(ctx, store.HeartbeatParams{ID: r.Cfg.ID, Backend: r.Backend.Name(),
		MaxConcurrent: int32(r.Cfg.MaxConcurrent), Running: int32(n), Version: r.Cfg.Version}); err != nil && ctx.Err() == nil {
		r.Log.Warn("heartbeat", "err", err)
	}
}

func (r *Runner) slot() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active >= r.Cfg.MaxConcurrent {
		return false
	}
	r.active++
	return true
}

func (r *Runner) release() {
	r.mu.Lock()
	r.active--
	r.mu.Unlock()
	r.wg.Done()
}

// Tick starts what it can: queued runs first (workspace setup), then
// pending turns of running runs. Work runs in goroutines; Wait waits.
func (r *Runner) Tick(ctx context.Context) {
	q := r.q()
	for r.slot() {
		run, err := q.ClaimQueuedRun(ctx, &r.Cfg.ID)
		if err != nil {
			r.mu.Lock()
			r.active--
			r.mu.Unlock()
			if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
				r.Log.Error("claim run", "err", err)
			}
			break
		}
		r.wg.Add(1)
		go func() {
			defer r.release()
			r.start(ctx, run)
		}()
	}
	for r.slot() {
		turn, err := q.ClaimPendingTurn(ctx)
		if err != nil {
			r.mu.Lock()
			r.active--
			r.mu.Unlock()
			if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
				r.Log.Error("claim turn", "err", err)
			}
			break
		}
		r.wg.Add(1)
		go func() {
			defer r.release()
			r.turn(ctx, turn)
		}()
	}
}

// Wait blocks until in-flight work finishes (tests).
func (r *Runner) Wait() { r.wg.Wait() }

// ---------------------------------------------------------------- context

type runCtx struct {
	run     store.Run
	ticket  store.Ticket
	agent   store.Agent
	project store.Project
	spec    agentdef.Resolved
}

func (r *Runner) load(ctx context.Context, runID uuid.UUID) (runCtx, error) {
	q := r.q()
	var c runCtx
	var err error
	if c.run, err = q.LockRunNoWait(ctx, runID); err != nil {
		return c, err
	}
	if c.run.Purpose == "ticket" && c.run.TicketID != nil {
		if c.ticket, err = q.GetTicketByID(ctx, *c.run.TicketID); err != nil {
			return c, err
		}
	} else if c.run.Purpose == "chat" {
		chat, err := q.ChatSessionForRun(ctx, &c.run.ID)
		if err != nil {
			return c, err
		}
		c.ticket = store.Ticket{WorkspaceID: chat.WorkspaceID, ProjectID: chat.ProjectID, Key: "CHAT-" + chat.ID.String()[:8], Title: chat.Title}
	} else {
		plan, err := q.PlanningSessionForRun(ctx, c.run.ID)
		if err != nil {
			return c, err
		}
		c.ticket = store.Ticket{WorkspaceID: plan.WorkspaceID, ProjectID: plan.ProjectID, EpicID: plan.EpicID, Key: "PLAN-" + plan.ID.String()[:8], Title: plan.Goal}
	}
	if c.agent, err = q.GetAgent(ctx, store.GetAgentParams{ID: c.run.AgentID, WorkspaceID: c.run.WorkspaceID}); err != nil {
		return c, err
	}
	if c.project, err = q.GetProject(ctx, store.GetProjectParams{ID: c.ticket.ProjectID, WorkspaceID: c.run.WorkspaceID}); err != nil {
		return c, err
	}
	c.spec, err = runs.Spec(c.run)
	if c.spec.Repository != nil {
		c.project.RepoUrl = c.spec.Repository.URL
		c.project.DefaultBranch = c.spec.Repository.DefaultBranch
	}
	return c, err
}

// addTiming adds to a run's cumulative phase timings. It is best-effort and
// uses its own context so a shutting-down runner still records the phase.
func (r *Runner) addTiming(runID uuid.UUID, setup, harness, checks time.Duration) {
	err := r.q().AddRunTiming(context.Background(), store.AddRunTimingParams{ID: runID,
		SetupMs: setup.Milliseconds(), HarnessMs: harness.Milliseconds(), ChecksMs: checks.Milliseconds()})
	if err != nil {
		r.Log.Warn("record timing", "run", runID, "err", err)
	}
}

func (r *Runner) event(ctx context.Context, runID uuid.UUID, typ string, payload map[string]any) {
	b, _ := json.Marshal(payload)
	if err := r.q().AddRunEvent(ctx, store.AddRunEventParams{RunID: runID, Type: typ, Payload: b}); err != nil && ctx.Err() == nil {
		r.Log.Warn("run event", "run", runID, "err", err)
	}
}

// end finishes a run the runner had to stop (failure or a guard), with a
// timeline entry and the workspace removed.
func (r *Runner) end(ctx context.Context, c runCtx, state, outcome, verb string) {
	if c.run.Purpose == "chat" {
		r.failChat(ctx, c, outcome)
		return
	}
	err := pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		run, err := q.LockRun(ctx, c.run.ID)
		if err != nil {
			return err
		}
		if run.State == "completed" || run.State == "failed" || run.State == "cancelled" {
			return nil
		}
		if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: state, Outcome: &outcome}); err != nil {
			return err
		}
		if err := q.DropPendingTurns(ctx, run.ID); err != nil {
			return err
		}
		if err := q.ExpireRunApprovals(ctx, run.ID); err != nil {
			return err
		}
		b, _ := json.Marshal(map[string]any{"state": state, "reason": outcome})
		if err := q.AddRunEvent(ctx, store.AddRunEventParams{RunID: run.ID, Type: "lifecycle", Payload: b}); err != nil {
			return err
		}
		if err := runnerRecord(ctx, q, c.run.WorkspaceID, c.ticket.ID, c.ticket.EpicID, r.system(), verb,
			map[string]any{"run_id": run.ID, "reason": outcome}, nil); err != nil {
			return err
		}
		// A delegated run reports to its parent; a parent takes its delegated runs down with it.
		if err := runs.CancelChildren(ctx, q, run.ID, "The main agent's run ended: "+outcome); err != nil {
			return err
		}
		if err := runs.RecomputeAgentStatus(ctx, q, run.AgentID); err != nil {
			return err
		}
		return runs.WakeParent(ctx, q, run, state, outcome)
	})
	if err != nil {
		r.Log.Error("end run", "run", c.run.ID, "err", err)
	}
	if c.run.ParentRunID == nil {
		r.cleanup(context.Background(), c.run.ID) // a delegated run has no workspace of its own
	}
}

func (r *Runner) cleanup(ctx context.Context, runID uuid.UUID) {
	r.removeWorktrees(ctx, runID)
	if err := r.Backend.RemoveWorkspace(ctx, runID.String()); err != nil {
		r.Log.Warn("remove workspace", "run", runID, "err", err)
		return
	}
	_ = r.q().SetRunWorkspace(ctx, store.SetRunWorkspaceParams{ID: runID, WorkspaceVolume: nil})
}

// Sweep removes workspaces of runs that ended between turns.
func (r *Runner) Sweep(ctx context.Context) {
	ids, err := r.q().EndedRunsWithWorkspace(ctx)
	if err != nil {
		return
	}
	for _, id := range ids {
		r.cleanup(ctx, id)
	}
}

// wsID names the workspace a run executes in: a delegated run uses its
// parent's checkout.
func wsID(run store.Run) string {
	if run.ParentRunID != nil {
		return run.ParentRunID.String()
	}
	return run.ID.String()
}

// childDir is the directory of the parent's workspace a delegated run works
// in (its git worktree); "" for any other run.
func childDir(run store.Run) string {
	if run.ParentRunID == nil {
		return ""
	}
	var contract runs.Contract
	if len(run.Contract) > 0 {
		_ = json.Unmarshal(run.Contract, &contract)
	}
	return contract.Worktree
}

// startChild readies a delegated run. There's no clone or setup: it gets its
// own git worktree and branch in the parent's workspace, cut from the branch
// the parent left committed (or the worktree of the run it continues).
func (r *Runner) startChild(ctx context.Context, c runCtx) {
	run := c.run
	fail := func(msg string) {
		r.Log.Warn("delegated run failed to start", "run", run.ID, "reason", msg)
		r.end(ctx, c, "failed", msg, runs.VerbRunFailed)
	}
	parent, err := r.q().GetRun(ctx, store.GetRunParams{ID: *run.ParentRunID, WorkspaceID: run.WorkspaceID})
	if err != nil || parent.WorkspaceVolume == nil || slices.Contains(runs.Terminal, parent.State) {
		fail("The main agent's workspace is gone, so the delegated run can't start.")
		return
	}
	plan, err := PlanSecrets(ctx, r.q(), run.WorkspaceID, &c.project.ID, c.spec)
	if err != nil {
		fail("Couldn't read secrets: " + err.Error())
		return
	}
	if key := HarnessKeys[c.spec.Harness]; r.Cfg.RequireHarnessKey && !hasSecret(plan, key.Secret) {
		fail(fmt.Sprintf("No %s secret: add an API key in Settings → Connections & secrets.", key.Secret))
		return
	}
	setupStart := time.Now()
	if err := r.waitParentIdle(ctx, parent.ID); err != nil {
		if ctx.Err() == nil {
			fail("The main agent's turn didn't finish, so the delegated run can't start: " + err.Error())
		}
		return
	}
	contract, err := r.prepareWorktree(ctx, c, map[string]string{})
	if err != nil {
		fail("Couldn't start the delegated run: " + err.Error())
		return
	}
	rawContract, _ := json.Marshal(contract)
	r.addTiming(run.ID, time.Since(setupStart), 0, 0)
	err = pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		cur, err := q.LockRun(ctx, run.ID)
		if err != nil {
			return err
		}
		if cur.State != "starting" {
			return nil // cancelled while queued
		}
		if _, err := q.SetRunContract(ctx, store.SetRunContractParams{ID: run.ID, Contract: rawContract}); err != nil {
			return err
		}
		if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "running"}); err != nil {
			return err
		}
		b, _ := json.Marshal(map[string]any{"state": "running", "mode": "managed", "parent_run_id": parent.ID, "branch": contract.Branch, "worktree": contract.Worktree})
		if err := q.AddRunEvent(ctx, store.AddRunEventParams{RunID: run.ID, Type: "lifecycle", Payload: b}); err != nil {
			return err
		}
		if err := runnerRecord(ctx, q, run.WorkspaceID, c.ticket.ID, c.ticket.EpicID, tracker.AgentActor(c.agent.ID), runs.VerbRunStarted,
			map[string]any{"run_id": run.ID, "mode": "managed", "harness": c.spec.Harness, "delegated": true}, nil); err != nil {
			return err
		}
		return runs.RecomputeAgentStatus(ctx, q, c.agent.ID)
	})
	if err != nil {
		r.Log.Error("mark delegated run running", "run", run.ID, "err", err)
	}
}

// ---------------------------------------------------------------- start

// start prepares a queued run's workspace and marks it running; its first
// turn (created at dispatch) is picked up by the next Tick.
func (r *Runner) start(ctx context.Context, run store.Run) {
	c, err := r.load(ctx, run.ID)
	if err != nil {
		r.Log.Error("load run", "run", run.ID, "err", err)
		return
	}
	if c.run.ParentRunID != nil {
		r.startChild(ctx, c)
		return
	}
	log := r.Log.With("run", run.ID, "ticket", c.ticket.Key)
	fail := func(msg string) {
		log.Warn("run failed to start", "reason", msg)
		r.end(ctx, c, "failed", msg, runs.VerbRunFailed)
	}
	if c.run.Purpose == "ticket" && c.run.TicketID != nil {
		blockers, err := tracker.DispatchBlockers(ctx, r.q(), c.ticket)
		if err != nil {
			fail("Could not check dependencies: " + err.Error())
			return
		}
		if len(blockers) > 0 {
			fail(strings.Join(blockers, "; "))
			return
		}
	}
	plan, err := PlanSecrets(ctx, r.q(), run.WorkspaceID, &c.project.ID, c.spec)
	if err != nil {
		fail("Couldn't read secrets: " + err.Error())
		return
	}
	key := HarnessKeys[c.spec.Harness]
	if r.Cfg.RequireHarnessKey && !hasSecret(plan, key.Secret) {
		fail(fmt.Sprintf("No %s secret: add an API key in Settings → Connections & secrets, then dispatch again.", key.Secret))
		return
	}
	env, err := SecretEnv(ctx, r.Secrets, run.WorkspaceID, &c.project.ID, plan)
	if err != nil {
		fail("Couldn't decrypt secrets: " + err.Error())
		return
	}
	// Setup may clone a private repo even when the agent can only read it.
	if gh := env["GH_TOKEN"]; gh == "" && (allowedScope(c.spec, "repo:read") || allowedScope(c.spec, "repo:write")) {
		if v, err := r.Secrets.Reveal(ctx, run.WorkspaceID, &c.project.ID, "git:pat"); err == nil {
			env["GH_TOKEN"] = v
		}
	}
	// Same for the SSH key: setup clones over SSH even when the agent only reads.
	if env[SSHKeyEnv] == "" && (allowedScope(c.spec, "repo:read") || allowedScope(c.spec, "repo:write")) {
		if v, err := r.Secrets.Reveal(ctx, run.WorkspaceID, &c.project.ID, "git:ssh"); err == nil {
			env[SSHKeyEnv] = v
		}
	}
	setupStart := time.Now()
	if err := r.Backend.EnsureWorkspace(ctx, run.ID.String()); err != nil {
		fail("Couldn't create the workspace: " + err.Error())
		return
	}
	branch := BranchName(c.agent.Handle, c.ticket.Key, c.ticket.Title)
	if c.ticket.Branch != nil && *c.ticket.Branch != "" {
		branch = *c.ticket.Branch
	}
	repo := ""
	if c.run.Purpose == "ticket" && c.run.TicketID != nil && c.project.RepoUrl != nil {
		repo = *c.project.RepoUrl
	}
	base := ""
	if c.run.Purpose == "ticket" && c.run.TicketID != nil {
		base = c.project.DefaultBranch
	}
	script := SetupScriptForBranch(repo, base, branch, c.agent.Name+" (LazyAgents)", c.agent.Handle+"@agents.lazyagents.local", c.spec.Setup)
	if c.run.Purpose == "chat" {
		script = "mkdir -p .lazyagents/context"
	}
	r.event(ctx, run.ID, "lifecycle", map[string]any{"state": "starting", "runner": r.Cfg.ID, "backend": r.Backend.Name(), "branch": branch})
	proc, err := r.Backend.Start(ctx, run.ID.String(), ExecSpec{
		Name: "lazyagents-setup-" + run.ID.String(), Argv: []string{"sh", "-c", script}, Env: env,
	})
	if err != nil {
		fail("Couldn't start the workspace setup: " + err.Error())
		return
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
	r.addTiming(run.ID, time.Since(setupStart), 0, 0)
	tail := lastLines(redact(out.String(), env), 30)
	if err != nil || code != 0 {
		fail(fmt.Sprintf("Workspace setup failed (exit %d):\n%s", code, tail))
		return
	}
	r.event(ctx, run.ID, "lifecycle", map[string]any{"state": "workspace_ready", "branch": branch, "log": tail})

	err = pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		var chat store.ChatSession
		if c.run.Purpose == "chat" {
			var err error
			chat, err = lockRunChat(ctx, q, c.run)
			if err != nil {
				return err
			}
		}
		cur, err := q.LockRun(ctx, run.ID)
		if err != nil {
			return err
		}
		if c.run.Purpose == "chat" && cur.State == "paused" {
			vol := run.ID.String()
			if err := q.SetRunWorkspace(ctx, store.SetRunWorkspaceParams{ID: run.ID, WorkspaceVolume: &vol}); err != nil {
				return err
			}
			return idleChat(ctx, q, chat, cur, "idle")
		}
		if cur.State != "starting" {
			return nil // cancelled while setting up
		}
		vol := run.ID.String()
		if err := q.SetRunWorkspace(ctx, store.SetRunWorkspaceParams{ID: run.ID, WorkspaceVolume: &vol}); err != nil {
			return err
		}
		if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "running"}); err != nil {
			return err
		}
		if err := q.SetTicketBranch(ctx, store.SetTicketBranchParams{ID: c.ticket.ID, Branch: &branch}); err != nil {
			return err
		}
		b, _ := json.Marshal(map[string]any{"state": "running", "mode": "managed"})
		if err := q.AddRunEvent(ctx, store.AddRunEventParams{RunID: run.ID, Type: "lifecycle", Payload: b}); err != nil {
			return err
		}
		if err := runnerRecord(ctx, q, run.WorkspaceID, c.ticket.ID, c.ticket.EpicID, tracker.AgentActor(c.agent.ID), runs.VerbRunStarted,
			map[string]any{"run_id": run.ID, "mode": "managed", "harness": c.spec.Harness}, nil); err != nil {
			return err
		}
		return runs.RecomputeAgentStatus(ctx, q, c.agent.ID)
	})
	if err != nil {
		log.Error("mark running", "err", err)
		return
	}
	if c.ticket.Status == "backlog" || c.ticket.Status == "todo" {
		st := "in_progress"
		if _, err := r.Tracker.Update(ctx, run.WorkspaceID, tracker.AgentActor(c.agent.ID), c.ticket.Key, tracker.Patch{Status: &st}); err != nil {
			log.Warn("move ticket", "err", err)
		}
	}
	log.Info("run started", "branch", branch)
}

func hasSecret(plan []SecretDecision, name string) bool {
	for _, d := range plan {
		if d.Name == name && d.Injected {
			return true
		}
	}
	return false
}

func allowedScope(spec agentdef.Resolved, scope string) bool {
	for _, s := range spec.Scopes {
		if s.Scope == scope {
			return s.Mode == agentdef.Allowed
		}
	}
	return false
}

// redact removes secret values from text shown to people.
func redact(s string, env map[string]string) string {
	for _, v := range env {
		if len(v) >= 6 {
			s = strings.ReplaceAll(s, v, "[redacted]")
		}
	}
	return s
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------- turns

// IdleNudge resumes a turn that ended without the agent saying what's next.
const IdleNudge = "You ended your turn without calling complete_run, await_human or request_approval. " +
	"If the work is done, call complete_run. If you need a person, call await_human or request_approval. Otherwise, continue."

func (r *Runner) turn(ctx context.Context, turn store.RunTurn) {
	c, err := r.load(ctx, turn.RunID)
	if err != nil {
		r.Log.Error("load run", "run", turn.RunID, "err", err)
		return
	}
	q := r.q()
	log := r.Log.With("run", c.run.ID, "ticket", c.ticket.Key, "turn", turn.Seq)
	if c.run.State != "running" {
		if c.run.Purpose == "chat" && c.run.State == "paused" {
			r.finishChatTurn(ctx, c, turn, harness.Result{}, 130, nil, nil)
			return
		}
		_ = q.FinishTurn(ctx, store.FinishTurnParams{ID: turn.ID, Usage: []byte("{}")})
		return
	}

	// Ticket/planning budgets accumulate harness time. Chat runtime is per
	// invocation, and long conversations have no cumulative turn limit.
	var maxRuntime time.Duration
	if c.spec.Budget.MaxRuntime != "" {
		maxRuntime, _ = time.ParseDuration(c.spec.Budget.MaxRuntime)
	}
	var used time.Duration
	if c.run.Purpose != "chat" {
		act, err := q.RunActiveSeconds(ctx, c.run.ID)
		if err != nil {
			log.Error("budget", "err", err)
			return
		}
		if mt := c.spec.Budget.MaxTurns; mt > 0 && int(act.Turns) > mt {
			_ = q.FinishTurn(ctx, store.FinishTurnParams{ID: turn.ID, Usage: []byte("{}")})
			r.end(ctx, c, "cancelled", fmt.Sprintf("Stopped: reached the %d-turn limit.", mt), runs.VerbRunCancelled)
			return
		}
		used = time.Duration(act.Seconds * float64(time.Second))
		if maxRuntime > 0 && used >= maxRuntime {
			_ = q.FinishTurn(ctx, store.FinishTurnParams{ID: turn.ID, Usage: []byte("{}")})
			r.end(ctx, c, "cancelled", "Stopped: reached the "+c.spec.Budget.MaxRuntime+" runtime limit.", runs.VerbRunCancelled)
			return
		}
	}

	h, err := harness.For(c.spec.Harness)
	if c.spec.Harness == agentdef.HarnessCodex {
		baseline := harness.Usage{}
		if c.run.HarnessSessionID != nil {
			turns, err := q.ListTurns(ctx, c.run.ID)
			if err != nil {
				log.Error("load usage baseline", "err", err)
				return
			}
			for i := len(turns) - 1; i >= 0; i-- {
				if turns[i].Seq >= turn.Seq || len(turns[i].Usage) == 0 {
					continue
				}
				var usage harness.Usage
				if json.Unmarshal(turns[i].Usage, &usage) != nil {
					continue
				}
				if usage.SessionID == *c.run.HarnessSessionID {
					baseline = usage
					break
				}
				// Older Codex turns stored cumulative usage without a session marker.
				if usage.SessionID == "" && usage.CostSource != "harness" && usage.InputTokens > 0 {
					baseline = harness.Usage{SessionID: *c.run.HarnessSessionID, SessionInputTokens: usage.InputTokens, SessionOutputTokens: usage.OutputTokens, SessionCachedTokens: usage.CachedTokens}
					break
				}
			}
		}
		h = harness.Codex{Prices: r.Cfg.ModelPrices, Baseline: baseline}
	}
	if err != nil {
		r.end(ctx, c, "failed", err.Error(), runs.VerbRunFailed)
		return
	}
	plan, err := PlanSecrets(ctx, q, c.run.WorkspaceID, &c.project.ID, c.spec)
	if err != nil {
		if c.run.Purpose == "chat" {
			r.failChat(ctx, c, "Could not load chat credentials.")
			return
		}
		log.Error("secrets", "err", err)
		return
	}
	env, err := SecretEnv(ctx, r.Secrets, c.run.WorkspaceID, &c.project.ID, plan)
	if err != nil {
		r.end(ctx, c, "failed", "Couldn't decrypt secrets: "+err.Error(), runs.VerbRunFailed)
		return
	}
	// A token for this turn only; revoked when the process exits.
	expires := time.Now().Add(24 * time.Hour)
	token, tok, err := agents.IssueToken(ctx, q, c.agent.ID, "run", "run "+c.run.ID.String()[:8]+" turn "+fmt.Sprint(turn.Seq), &c.run.ID, nil, &expires)
	if err != nil {
		if c.run.Purpose == "chat" {
			r.failChat(ctx, c, "Could not issue a chat turn token.")
			return
		}
		log.Error("token", "err", err)
		return
	}
	defer func() { _ = agents.RevokeToken(context.Background(), q, c.agent.ID, tok.ID) }()

	ctxFiles, err := ContextFiles(ctx, q, c.agent, c.ticket, c.spec.Memory)
	if err != nil {
		log.Warn("context files", "err", err)
	}
	if c.run.Purpose == "planning" {
		source, err := PlanningContextFile(ctx, q, c.run)
		if err != nil {
			r.end(ctx, c, "failed", "Could not load planning source: "+err.Error(), runs.VerbRunFailed)
			return
		}
		if source != nil {
			ctxFiles = append(ctxFiles, *source)
		}
	}
	prompt := turn.InputMessage
	if turn.Trigger == "delegation" {
		prompt += r.delegationReport(ctx, c, env)
	}
	if c.run.Purpose == "chat" {
		// The files this turn's message attached, listed in its prompt.
		refFiles, refPrompt, err := ChatTurnContext(ctx, q, c.run, c.project.ID, turn)
		if err != nil {
			r.failChat(ctx, c, "Could not load the message's attached context.")
			return
		}
		ctxFiles = append(ctxFiles, refFiles...)
		prompt += refPrompt
	}
	paths := make([]string, len(ctxFiles))
	files := map[string][]byte{}
	for i, f := range ctxFiles {
		paths[i] = f.Path
		files[f.Path] = f.Body
	}
	session := ""
	if c.run.HarnessSessionID != nil {
		session = *c.run.HarnessSessionID
	}
	dir := childDir(c.run)
	wsDir := r.Backend.WorkspaceDir(wsID(c.run))
	if dir != "" {
		wsDir = path.Join(wsDir, dir)
	}
	inv, err := h.Invocation(harness.Workspace{
		Dir: wsDir, MCPURL: r.Cfg.MCPURL, Token: token, TokenEnv: tokenEnv,
	}, harness.Turn{Spec: c.spec, Prompt: prompt, SessionID: session})
	if err != nil {
		r.end(ctx, c, "failed", err.Error(), runs.VerbRunFailed)
		return
	}
	for k, v := range inv.Files {
		files[k] = v
	}
	files[".lazyagents/agents-section.md"] = harness.AgentsMD("", c.spec, paths, c.ticket.Key)
	if c.run.ParentRunID != nil {
		files[".lazyagents/agents-section.md"] = harness.SubagentAgentsMD("", c.spec, paths, c.ticket.Key)
	}
	if c.run.Purpose == "chat" {
		files[".lazyagents/agents-section.md"] = harness.ChatAgentsMD(c.spec, paths, c.project.Key)
	}
	if c.run.Purpose == "planning" {
		files[".lazyagents/agents-section.md"] = []byte(c.spec.Instructions + "\nRead these context files first:\n" + strings.Join(paths, "\n"))
	}
	for k, v := range inv.Env {
		env[k] = v
	}
	// Only secrets and the token are scrubbed from events, not every env
	// value (the ticket key is in env too).
	sensitive := make(map[string]string, len(env))
	for k, v := range env {
		sensitive[k] = v
	}
	env["LAZYAGENTS_RUN_ID"] = c.run.ID.String()
	env["LAZYAGENTS_TICKET"] = c.ticket.Key
	argv := append(append([]string{}, TurnWrapper...), inv.Argv...)

	r.event(ctx, c.run.ID, "lifecycle", map[string]any{"state": "turn_started", "turn": turn.Seq, "trigger": turn.Trigger,
		"harness": c.spec.Harness, "resume": session != ""})
	proc, err := r.Backend.Start(ctx, wsID(c.run), ExecSpec{
		Name: fmt.Sprintf("lazyagents-turn-%s-%d", c.run.ID, turn.Seq), Argv: argv, Env: env, Dir: dir, Files: files,
	})
	if err != nil {
		_ = q.FinishTurn(ctx, store.FinishTurnParams{ID: turn.ID, Usage: []byte("{}")})
		r.end(ctx, c, "failed", "Couldn't start the harness: "+err.Error(), runs.VerbRunFailed)
		return
	}
	// The first turn delivers context files: each counts as one read (§6.5).
	if turn.Trigger == "start" || (c.run.Purpose == "chat" && turn.Seq == 1) {
		now := time.Now()
		if c.run.Purpose == "chat" {
			// Delivered context still counts as a read, but sits just before the turn so
			// it isn't cited as something the assistant looked up (see runs.ChatCitations).
			now = turn.StartedAt.Add(-time.Millisecond)
		}
		for _, f := range ctxFiles {
			if a, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: f.ArtifactID, WorkspaceID: c.run.WorkspaceID}); err == nil {
				if f.VersionID != nil {
					a.CurrentVersionID = f.VersionID
				}
				_ = artifacts.RecordRead(ctx, q, a, c.agent.ID, &c.run.ID, "", &now)
			}
		}
	}

	harnessStart := time.Now()
	res, raw, code, waitErr := r.watch(ctx, c, proc, h.NewParser(c.spec.Model), sensitive, maxRuntime-used)
	r.addTiming(c.run.ID, 0, time.Since(harnessStart), 0)
	// Some CLI failures only reach stderr, without a JSON error event.
	if code != 0 && !res.UsageLimited {
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "[stderr] ") && harness.UsageLimit(line) {
				res.UsageLimited = true
				break
			}
		}
	}
	if code != 0 && res.Error == "" && c.run.ParentRunID != nil {
		// A session the CLI can't resume is reported on stderr only.
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "[stderr] ") && sessionGone.MatchString(line) {
				res.Error = strings.TrimPrefix(line, "[stderr] ")
				break
			}
		}
	}
	logKey := r.saveLog(c.run.ID, turn.Seq, raw)
	if c.run.Purpose == "chat" {
		if ctx.Err() == nil {
			r.finishChatTurn(ctx, c, turn, res, code, waitErr, logKey)
		}
		return
	}
	usage, _ := json.Marshal(res.Usage)
	exit := int32(code)
	if err := q.FinishTurn(context.Background(), store.FinishTurnParams{ID: turn.ID, ExitCode: &exit, LogKey: logKey, Usage: usage}); err != nil {
		log.Error("finish turn", "err", err)
	}
	if res.Usage != (harness.Usage{}) {
		src := res.Usage.CostSource
		var srcp *string
		if src != "" {
			srcp = &src
		}
		_, usageErr := q.AddRunUsage(context.Background(), store.AddRunUsageParams{ID: c.run.ID, ToolCalls: int32(res.Usage.ToolCalls),
			InputTokens: res.Usage.InputTokens, OutputTokens: res.Usage.OutputTokens, CostUsdMicros: int64(res.Usage.CostUSD * 1e6), CostSource: srcp})
		if usageErr != nil {
			log.Error("record usage", "err", usageErr)
		}
	}
	r.event(context.Background(), c.run.ID, "lifecycle", map[string]any{"state": "turn_ended", "turn": turn.Seq, "exit_code": code})
	if ctx.Err() != nil {
		return // shutting down: the turn re-runs on the next runner start
	}
	r.afterTurn(ctx, c.run.ID, res, code, waitErr)
}

// watch streams a turn's output into events, enforces pause/cancel and the
// runtime guard, and returns what the parser learned.
func (r *Runner) watch(ctx context.Context, c runCtx, proc Process, p harness.Parser, env map[string]string, remaining time.Duration) (harness.Result, []byte, int, error) {
	var raw bytes.Buffer
	var rawMu sync.Mutex
	appendRaw := func(prefix string, b []byte) {
		rawMu.Lock()
		defer rawMu.Unlock()
		if raw.Len() < 64<<20 {
			raw.WriteString(prefix)
			raw.Write(b)
			raw.WriteByte('\n')
		}
	}
	done := make(chan struct{})
	var readers sync.WaitGroup
	readers.Add(2)
	go func() {
		defer readers.Done()
		sc := bufio.NewScanner(proc.Stderr())
		sc.Buffer(make([]byte, 64<<10), 4<<20)
		for sc.Scan() {
			appendRaw("[stderr] ", sc.Bytes())
		}
	}()
	go func() {
		defer readers.Done()
		sc := bufio.NewScanner(proc.Stdout())
		sc.Buffer(make([]byte, 256<<10), 32<<20)
		session := ""
		for sc.Scan() {
			line := sc.Bytes()
			appendRaw("", line)
			for _, e := range p.Line(line) {
				r.event(context.Background(), c.run.ID, e.Type, redactPayload(e.Payload, env))
			}
			if s := p.Result().SessionID; s != "" && s != session {
				session = s
				_ = r.q().SetRunSession(context.Background(), store.SetRunSessionParams{ID: c.run.ID, HarnessSessionID: &s})
			}
		}
	}()

	// Watchdog: people pause or cancel through the API; we act on it here.
	var watchdog sync.WaitGroup
	watchdog.Add(1)
	go func() {
		defer watchdog.Done()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		var deadline <-chan time.Time
		if remaining > 0 {
			deadline = time.After(remaining)
		}
		interrupted := time.Time{}
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				_ = proc.Kill()
				return
			case <-deadline:
				if c.run.Purpose == "chat" {
					_ = proc.Kill()
					return
				}
				r.end(context.Background(), c, "cancelled", "Stopped: reached the "+c.spec.Budget.MaxRuntime+" runtime limit.", runs.VerbRunCancelled)
				_ = proc.Kill()
				return
			case <-t.C:
				run, err := r.q().GetRun(context.Background(), store.GetRunParams{ID: c.run.ID, WorkspaceID: c.run.WorkspaceID})
				if err != nil {
					continue
				}
				switch {
				case run.State == "cancelled" || run.State == "failed":
					_ = proc.Kill()
				case run.State == "paused" && c.run.Purpose == "chat":
					// Chat Stop cancels this reply; do not wait through the
					// ticket shutdown grace period for an unresponsive CLI.
					_ = proc.Kill()
				case run.State == "paused" && interrupted.IsZero():
					// Let the harness close the turn cleanly (§6.4).
					interrupted = time.Now()
					_ = proc.Interrupt()
				case !interrupted.IsZero() && time.Since(interrupted) > r.Cfg.Grace:
					_ = proc.Kill()
				}
			}
		}
	}()
	code, err := proc.Wait()
	readers.Wait()
	if c, ok := proc.(io.Closer); ok {
		_ = c.Close()
	}
	close(done)
	watchdog.Wait()
	return p.Result(), raw.Bytes(), code, err
}

func redactPayload(p map[string]any, env map[string]string) map[string]any {
	b, _ := json.Marshal(p)
	s := redact(string(b), env)
	if s == string(b) {
		return p
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func (r *Runner) saveLog(runID uuid.UUID, seq int32, raw []byte) *string {
	if r.Blobs == nil || len(raw) == 0 {
		return nil
	}
	key := fmt.Sprintf("runs/%s/turn-%03d.jsonl", runID, seq)
	if err := r.Blobs.Put(context.Background(), r.Cfg.LogBucket, key, bytes.NewReader(raw), int64(len(raw)), "application/x-ndjson"); err != nil {
		r.Log.Warn("save raw log", "run", runID, "err", err)
		return nil
	}
	return &key
}

// afterTurn decides what follows a turn (§6.4):
//   - completed, failed or cancelled: the workspace goes;
//   - waiting or paused: nothing until a person acts (that queues a turn);
//   - still running: the harness failed, or the agent stopped without
//     saying what's next. That gets one nudge, then a person.
func (r *Runner) afterTurn(ctx context.Context, runID uuid.UUID, res harness.Result, code int, waitErr error) {
	c, err := r.load(ctx, runID)
	if err != nil {
		r.Log.Error("load run", "run", runID, "err", err)
		return
	}
	q := r.q()
	switch c.run.State {
	case "completed", "failed", "cancelled":
		if c.run.ParentRunID == nil {
			r.cleanup(ctx, runID)
		}
		return
	case "waiting", "paused":
		_ = q.SetIdleTurns(ctx, store.SetIdleTurnsParams{ID: runID, IdleTurns: 0})
		return
	case "running":
	default:
		return
	}
	pending, err := q.HasPendingTurn(ctx, runID)
	if err != nil || pending {
		return // a person already resumed it (a reply arrived mid-turn)
	}
	if res.UsageLimited && len(c.spec.Fallbacks) > 0 {
		if err := r.fallback(ctx, c, res); err != nil {
			r.end(ctx, c, "failed", "Could not switch to fallback: "+err.Error(), runs.VerbRunFailed)
		}
		return
	}
	if (waitErr != nil || code != 0 || res.IsError) && r.retryFreshSession(ctx, c, res) {
		return
	}
	if waitErr != nil || code != 0 || res.IsError || res.UsageLimited {
		msg := res.Error
		if msg == "" && res.UsageLimited {
			msg = "Provider usage exhausted; no fallback targets remain"
		}
		if msg == "" && waitErr != nil {
			msg = waitErr.Error()
		}
		if msg == "" {
			msg = fmt.Sprintf("the harness exited with code %d", code)
		}
		r.end(ctx, c, "failed", "Harness error: "+msg, runs.VerbRunFailed)
		return
	}
	if c.run.IdleTurns < 1 {
		err := pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
			q := store.New(tx)
			if err := q.SetIdleTurns(ctx, store.SetIdleTurnsParams{ID: runID, IdleTurns: c.run.IdleTurns + 1}); err != nil {
				return err
			}
			_, err := q.StartTurn(ctx, store.StartTurnParams{RunID: runID, Trigger: "resume", InputMessage: func() string {
				if c.run.Purpose == "planning" {
					return "You stopped without submitting a draft. Finish this planning run with propose_tickets; do not use complete_run or claim_ticket."
				}
				return IdleNudge
			}()})
			return err
		})
		if err != nil {
			r.Log.Error("nudge", "run", runID, "err", err)
		}
		return
	}
	if c.run.Purpose == "planning" {
		r.end(ctx, c, "failed", "The planner stopped without submitting proposals. Start a new plan to retry.", runs.VerbRunFailed)
		return
	}
	if c.run.ParentRunID != nil {
		// Nobody to hand it to but the main agent, which gets told it didn't finish.
		r.end(ctx, c, "failed", "The sub agent stopped without calling complete_run. Last message: "+lastLines(res.Final, 8), runs.VerbRunFailed)
		return
	}
	// Second idle turn: hand it to a person; an @mention resumes it.
	reason := "The agent stopped without finishing. Reply with @" + c.agent.Handle + " to continue."
	err = pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		wait := "human_reply"
		if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: runID, State: "waiting", WaitReason: &wait}); err != nil {
			return err
		}
		if err := q.SetIdleTurns(ctx, store.SetIdleTurnsParams{ID: runID, IdleTurns: 0}); err != nil {
			return err
		}
		if err := runnerRecord(ctx, q, c.run.WorkspaceID, c.ticket.ID, c.ticket.EpicID, r.system(), runs.VerbWaitingForHuman,
			map[string]any{"run_id": runID, "reason": reason, "last_message": lastLines(res.Final, 8)}, nil); err != nil {
			return err
		}
		return runs.RecomputeAgentStatus(ctx, q, c.agent.ID)
	})
	if err != nil {
		r.Log.Error("hand to person", "run", runID, "err", err)
	}
}

func runnerRecord(ctx context.Context, q *store.Queries, ws, ticket uuid.UUID, epic *uuid.UUID, actor tracker.Actor, verb string, payload map[string]any, at *time.Time) error {
	if ticket == uuid.Nil {
		return nil
	}
	return tracker.Record(ctx, q, ws, ticket, epic, actor, verb, payload, at)
}

// sessionGone matches the CLIs' errors for a session id they can't resume.
var sessionGone = regexp.MustCompile(`(?i)no conversation found|session[^\n]{0,40}(not found|expired|invalid|unknown)|(cannot|could not|couldn't|unable to) (resume|find)[^\n]{0,40}(session|thread|conversation)|thread[^\n]{0,20}not found`)

// retryFreshSession handles a delegated run that continues an earlier run and
// whose first turn failed because the harness could not resume that run's
// session (expired, or lost with a volume): it forgets the session and starts
// the turn again fresh, once. The worktree still holds the earlier work.
// Detection is by error text, so an unrecognised message just fails the run.
func (r *Runner) retryFreshSession(ctx context.Context, c runCtx, res harness.Result) bool {
	var contract runs.Contract
	if c.run.ParentRunID == nil || json.Unmarshal(c.run.Contract, &contract) != nil || !contract.ResumeSession || !sessionGone.MatchString(res.Error) {
		return false
	}
	turns, err := r.q().ListTurns(ctx, c.run.ID)
	if err != nil || len(turns) != 1 {
		return false
	}
	contract.ResumeSession = false
	raw, _ := json.Marshal(contract)
	err = pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.SetRunSession(ctx, store.SetRunSessionParams{ID: c.run.ID, HarnessSessionID: nil}); err != nil {
			return err
		}
		if _, err := q.SetRunContract(ctx, store.SetRunContractParams{ID: c.run.ID, Contract: raw}); err != nil {
			return err
		}
		_, err := q.StartTurn(ctx, store.StartTurnParams{RunID: c.run.ID, Trigger: "resume", InputMessage: turns[0].InputMessage +
			"\n\n(Your earlier conversation could not be resumed, so this is a fresh session. Your earlier work is still in this worktree: check git log and git diff before you start.)"})
		return err
	})
	if err != nil {
		r.Log.Error("fresh session retry", "run", c.run.ID, "err", err)
		return false
	}
	r.event(ctx, c.run.ID, "lifecycle", map[string]any{"state": "session_reset", "reason": res.Error})
	return true
}
