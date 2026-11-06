# FIXES: parallel delegation + repair resume (review findings)

Uncommitted work on `main` adds parallel delegation (up to 3 live sub-agent runs per parent, each in its own git worktree, merged back on the parent's resume) and `continue_run_id` (repair runs reuse the previous child's worktree and harness session). A review found the issues below. Fix all of them, in order.

## Ground rules

- **Never execute `claude` or `codex` CLIs**, never start the server, and never send anything but GET to `localhost:8000`. Real logged-in CLIs are on PATH. Runner tests must keep their guards: `runner.Local{OnlyBins: true}` and the fake-harness PATH (`server/cmd/fake-harness`).
- Keep changes minimal and in the surrounding style. Don't touch unrelated uncommitted work in the tree.
- Regenerate after any SQL or OpenAPI change: `cd server && /opt/homebrew/bin/sqlc generate && go generate ./...`, and `cd web && npm run gen` if `api/openapi.yaml` changes.
- Verify with:
  ```sh
  cd server && go build ./... && go vet ./... && gofmt -l .
  TEST_DATABASE_URL="postgres://lazyagents:lazyagents@127.0.0.1:5432/postgres?sslmode=disable" go test ./... -count=1
  ```
  The DB tests create throwaway databases. All packages must pass.
- Don't commit.

## Key files

- `server/internal/runs/delegation.go`: `DelegateWithContract`, `delegating()`, `continuedRun()`, `checkParallel()`, `WakeParent`, `CancelChildren`, `MaxParallelDelegations`.
- `server/internal/runs/approvals.go`: `SetRunControl` (pause/resume/cancel), `OnHumanComment`.
- `server/internal/runner/runner.go`: `startChild`, `turn` (the `turn.Trigger == "delegation"` branch, about line 746), `afterTurn`, `cleanup`, `wtMu`.
- `server/internal/runner/worktree.go`: `waitParentIdle`, `prepareWorktree`, `mergeChild`, `removeWorktrees`.
- `server/internal/runner/checks.go`: `delegationReport`, `checkChild`.
- `server/internal/store/queries/runner.sql`: `ClaimPendingTurn`, about lines 22-36.
- `server/internal/store/queries/runs.sql`: `ActiveChildRun`, `ChildRuns`.
- `server/internal/agentapi/tools.go`: the `delegate` tool description.
- `server/internal/harness/harness.go`: `delegationSection`.
- Tests: `server/internal/runs/parallel_delegation_test.go`, `server/internal/runner/delegation_worktree_test.go`, `server/internal/runner/runner_test.go`.

---

## 1. MAJOR: a paused parent resumed after its children ended never gets their checks or merge

**Where:** `approvals.go`, `SetRunControl` case `"resume"` (about lines 409-428), plus `runner.go`, where `delegationReport` only runs for `turn.Trigger == "delegation"`.

**Bug:** the parent is paused while children run, and the children then end. `WakeParent` does nothing because the parent is `paused`. When a person later resumes it, no child is live, so the parent is resumed with trigger `"resume"` and the text "You were paused; continue." Its children are never checked or merged and never reported. When the parent completes, `cleanup` deletes the workspace and the unmerged branches with it.

**Fix:**
1. In `delegation.go`, pull the wake-message building out of `WakeParent` into a helper, e.g. `func delegationWakeMessage(ctx, q, parent store.Run) (msg string, ok bool, err error)`. It loads `ChildRuns(parent.ID)` and builds the message from every terminal child whose `Checks` is empty. It returns `ok=false` when there are none.
2. Have `WakeParent` use that helper.
3. In the `"resume"` branch of `SetRunControl`:
   - If a child is still live (see fix 2b), keep today's behavior: `waiting`/`delegation`.
   - Else, if `delegationWakeMessage` returns `ok`, call `resume(ctx, q, run, "delegation", msg)` so the runner runs `delegationReport`.
   - Else, fall back to the generic `resume(..., "resume", "You were paused; continue.")`.
4. In `continuedRun`, also reject (422) when `len(prev.Checks) == 0`, with a message like "that run hasn't been checked and merged yet; wait for your delegation report".

**Test (runs or runner):** start a parent, delegate one child, pause the parent, complete the child, resume the parent. Assert:
- the parent's next pending turn has trigger `delegation`;
- after the runner processes it, the child has `checks` stored and its branch is merged.

## 2. MAJOR: `OnHumanComment` can resume the wrong child or silently drop a reply

**Where:** `approvals.go`, `OnHumanComment`, about lines 473-478. It uses `q.ActiveChildRun`, a `:one` query with no `ORDER BY`, so with several live children it returns an arbitrary one.

**Fix:**
- **a. Pick the child the reply is for.** When the ticket's active run is `waiting` with reason `delegation`:
  - Loop over `q.ChildRuns(ctx, &run.ID)`.
  - For each child with `State == "waiting"` and `WaitReason == "human_reply"`, load its agent with `GetAgent`.
  - If `Mentions(body, agent.Handle)`, `LockRun(child.ID)`, re-check that it's still waiting on `human_reply`, and `resume(ctx, q, child, "comment", body)`.
  - Resume every matching child, and document that a mention reaches every waiting child of that agent.
  - If no child matches, return nil.
- **b. Replace other `ActiveChildRun` uses.** Add a query `HasLiveChild :one` returning `SELECT EXISTS(SELECT 1 FROM runs WHERE parent_run_id = $1 AND state NOT IN ('completed','failed','cancelled'))`. Use it wherever code only needs to know whether any child is live: the `"resume"` branch above, and anywhere else `ActiveChildRun` is used for that (grep for it). Keep `ActiveChildRun` only where one specific child is really needed, and add `ORDER BY started_at DESC` to it.

**Test:** a parent with two live children. Child A waits on `human_reply`; child B is running. Post a comment mentioning A's handle. Assert A resumes (it gets a pending `comment` turn) and B is unchanged. Then a comment that mentions no waiting child's handle changes nothing.

## 3. MINOR: the parent's delegation turn can start while a child's process is still running

**Where:** `runner.sql`, `ClaimPendingTurn`, and `checks.go`, `delegationReport` / `mergeChild`.

**Bug:** `WakeParent` fires when the child calls `complete_run`, before its CLI process exits. Checks and the merge can then run while the child is still writing.

**Fix:**
- In `ClaimPendingTurn`, don't claim a parent's turn while any of its children has a running turn:
  ```sql
  AND NOT EXISTS (
    SELECT 1 FROM run_turns b JOIN runs c ON c.id = b.run_id
    WHERE c.parent_run_id = r.id AND b.status = 'running')
  ```
  Adapt the alias to the query.
- Also, in `checkChild`, record `git rev-parse HEAD` of the child worktree into `Checks`, e.g. a new field `HeadSHA`. Have `mergeChild` merge that SHA, not the branch name, so the merged commit is exactly the one that was checked.

**Test:** in the fake harness, a child that sleeps after `complete_run` (an env knob such as `FAKE_HARNESS_LINGER`). Assert the parent's delegation turn doesn't start until the child's turn has finished.

## 4. MINOR: the parent can be woken while its own turn is still running

**Where:** `WakeParent` in `delegation.go`, and `DelegateWithContract`.

**Bug:** the only live child fails before the parent's process exits, for example on a missing secret or a person's cancel. The parent becomes `running` with a pending `delegation` turn while turn N is still alive. N can then delegate child D, which is never reported.

**Fix:**
- In `DelegateWithContract`, under the parent lock, refuse with 409 if the parent already has a pending turn with trigger `delegation`. Message: "your delegated runs already finished; end your turn to read their report".
- Also make `delegationReport` defer when the parent has any live child: return an empty report, then requeue or park the parent as `waiting`/`delegation` without running the turn.

**Test:** the child fails immediately (fake harness exit 1 before `complete_run`). The parent's same turn calls delegate again and gets 409. The parent then gets exactly one delegation report.

## 5. MINOR: run state isn't re-checked after `LockRun` in `DelegateWithContract`

**Where:** `delegation.go`, about lines 240-243.

**Fix:** after `LockRun(parent)`, require `run.State == "running" || (run.State == "waiting" && deref(run.WaitReason) == "delegation")`. Otherwise return `apperr.Conflict("your run is no longer running")`. Do this before inserting the child or writing any event.

**Test:** cancel the parent between lookup and lock, simulated by cancelling it and then calling delegate with a caller that still passes `delegating()` if possible. Otherwise unit-test the post-lock check directly.

## 6. MINOR: a crash mid-report loses already-checked children's results

**Where:** `checks.go`, `delegationReport`.

**Bug:** `delegationReport` selects only children whose `Checks` is empty. If the runner crashes after storing some checks, the requeued turn omits them.

**Fix:**
- Add a field `ReportedTurn int` (the parent turn seq or id) to `runs.Checks`.
- `delegationReport` reports every terminal child whose `Checks` is empty, plus those whose `Checks.ReportedTurn` equals the current parent turn. Re-render the stored `Checks.Report()` for those without re-running checks or merges.
- Never re-run a merge for a child whose `Checks.Merged` is set.

**Test:** store checks for one of two children with the current turn's id, then run `delegationReport`. Both children must appear, and no second merge happens.

## 7. MINOR: `wtMu` is a single global mutex

**Where:** `runner.go`, about line 64, and its uses in `worktree.go`.

**Fix:** replace it with a per-workspace lock, e.g. `wtLocks sync.Map` of `string → *sync.Mutex` keyed by the parent workspace id, through a helper `r.wtLock(ws) func()`. Update the comment to say it serializes within one runner process only; git's own locks and the run state machine cover cross-process cases.

## 8. MINOR: wrong wording in the delegate tool description

**Where:** `tools.go` (the `delegate` description) and `harness.go` (`delegationSection`).

**Fix:**
- Replace "delegating is refused while your checkout has uncommitted changes" with "commit your work first: a delegated run fails to start if your checkout has uncommitted changes".
- Build the parallel limit in both texts from `runs.MaxParallelDelegations` with `fmt.Sprintf`, not a literal "3".

## 9. MINOR: the 5-minute `waitParentIdle` deadline fails children and holds a runner slot

**Where:** `worktree.go`, `waitParentIdle`, called from `startChild`.

**Fix:** if the parent's turn is still running when the child is claimed, release the child (set it back to `queued`, or leave it unclaimed) and return, so the runner slot is freed. Don't fail it after 5 minutes. Simplest: add to `ClaimPendingTurn`, or to the queued-run claim query for child runs, the condition that the parent has no `run_turns` row with `status = 'running'`. Then `waitParentIdle` becomes a short safety wait or can be removed.

**Test:** the parent keeps running for longer than the old deadline (use a short test override). The child must start after the parent's turn exits, not fail.

## 10. Test gaps

Add to `server/internal/runs/parallel_delegation_test.go`:
- **Concurrent wake:** two live children completed concurrently, in two goroutines behind a barrier calling the service's complete-child path. Assert exactly one `delegation` turn for the parent and that the parent ends `running`.
- **Pause and resume:** pause, children end, then resume, as in fix 1.
- **Two-child reply:** a comment routed to the right one of two children, as in fix 2.
- **Realistic conflict (optional):** two children whose merges conflict. For example, child B modifies a file outside its `allowed_paths` but within child A's, with B's checks forced to pass or no contract. This complements the existing `TestMergeConflictIsAbortedAndReported`.

## Done when

- All 10 items are implemented, with the tests above.
- `go build ./... && go vet ./... && gofmt -l .` is clean.
- The full suite passes with `TEST_DATABASE_URL` and `-count=1`.
- No real CLI was executed.

---

## Remaining planned work (separate tasks, after the fixes above)

1. **Clone cache.** `server/internal/runner/prepare.go` does a full `git clone --no-checkout` for every new run workspace. Keep a per-project bare mirror on the runner host, e.g. `git clone --mirror` once, then `git fetch --prune` before use. Clone with `--reference <mirror> --dissociate`, or clone from the mirror and reset `origin` to the real URL. It must work for the local and docker backends, with the mirror mounted read-only into containers, and keep the GH_TOKEN credential-helper behavior. Record setup time in the existing `setup_ms` timing.
2. **Fewer bookkeeping round trips** (`server/internal/agentapi/tools.go`, `server/internal/runs`):
   - `claim_ticket` also returns the ticket payload `get_ticket` returns.
   - `update_step` and `check_criterion` accept a list of items in one call. Keep the single-item form working.
   - Update tool descriptions and the run-start `next` hint to say batching is preferred.
   - Then remove "Start with lazyagents.whoami" from the live agents' instructions through the API's `PUT /agents/{handle}/definition`, with a dry run first.
3. **Smaller tool sets per role.**
   - Sub agents shouldn't be shown `delegate`, `list_subagents`, `propose_tickets` or the pattern curation tools (`list_proposed_patterns`, `publish_pattern`, `reject_pattern`).
   - Filter the MCP tool list a run is offered by agent kind and scopes, the same way `chatTools` restricts chat runs (`server/internal/agentapi/chat.go`).
   - Add a test that iterates all tools.
4. **Skip the policy hook for safe tools** (`server/internal/harness/claude.go`). The PreToolUse hook (`tool_hook.py`) runs for every tool call with matcher `"*"`, at about 90ms per call. Narrow the matcher so it doesn't fire for `mcp__lazyagents__*`, which the server enforces with the run token, or for `Read`, `Grep`, `Glob`, `TodoWrite` and `Skill`. Do the same for Codex (`codex.go` / `codex_launch.py`) if it applies. Update `tool_hook_test.go`.
5. **Wiki additions** (via the LazyAgents propose → librarian → tech-lead flow or `POST /wiki/pages`):
   - Patterns:
     - "Isolate parallel agent work in git worktrees with disjoint paths"
     - "Resume the same session for repairs"
     - "Measure phase timings before optimizing agent runs"
     - "Never exec real harness CLIs in tests"
     - "Batch bookkeeping tool calls"
     - "Soft-delete records that history points at"
   - ADRs in `Architecture/`: the new agent set and tiered Codex fallbacks, effort via `harness_options`, parallel delegation semantics, and the Patterns curation flow.
