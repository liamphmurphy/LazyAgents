# LAZYAGENTS-36 Kanban requirements matrix

Evidence is for this verification branch, which includes the LAZYAGENTS-32,
LAZYAGENTS-34 and LAZYAGENTS-35 branches. It is **not product sign-off** or proof
that those branches reached main. Checked 2026-11-06.

**VERIFIED** means the named local or CI checks passed. Mocked browser tests prove UI
behavior only; Go integration tests use real disposable Postgres databases.
The real-stack browser suite uses the real API, MCP, Postgres, SSE and Next.
Its chat tests use fake CLIs; the board flow makes no model/provider call.
**UNMET** is a known delivery gap; **UNVERIFIED** needs further evidence.

## Goal coverage

| Requirement | Implementation | Evidence | Status |
| --- | --- | --- | --- |
| Kanban statuses: Backlog, To do, In progress, In review, Done; opt-in Canceled; blocked badge and canceled blockers | `web/app/(app)/board/board-screen.tsx`, `server/internal/httpapi/board.go`, `server/internal/tracker/service.go` | `web/e2e/board.spec.ts`: columns/card anatomy, canceled dependencies, keyboard move, native drag, stale revision rollback, SSE refetch; `TestBoardAndEpicCriteriaHTTP`, `TestBoardErrorContractsAndWorkspaceIsolation`, `TestCanceledTicketCannotBeClaimedOrDispatched` | VERIFIED locally |
| Agent point estimates: 1, 2, 3, 5, 8, 13; attribution/rationale; human precedence; point rollups | `server/internal/agentapi/estimates.go`, `server/internal/tracker/service.go`, board and ticket modal | `TestMCPEstimateTicket`, `TestTicketRevisionAndEstimate`, `TestHumanConfirmsAgentEstimate`, `TestPlanningProposalPoints`; `board-editor.spec.ts`: estimate change/clear and attribution | VERIFIED locally |
| Epic final acceptance criteria: ordered editing/checking, progress, gate on all criteria plus all tickets done/canceled | `server/internal/tracker/epics.go`, `web/components/domain/epic-completion.tsx`, board and epics screens | `TestEpicCriteriaAndGate`, `TestEpicCloseSerializesWithTicketCreate`, `TestProjectLockSerializesEpicChangesAndStalePatch`; all four `web/e2e/epic-criteria.spec.ts` tests | VERIFIED locally |
| Human edit modal: prefilled fields, deep link, atomic criteria draft, changed fields only, revision conflicts, keyboard/focus behavior | `web/components/domain/ticket-editor-dialog.tsx`, `web/lib/ticket-draft.ts`, `server/internal/tracker/criteria.go` | `web/e2e/board-editor.spec.ts` (12 tests); `TestTicketCriteriaDraftHTTP`, `TestCriteriaDraftAtomicSave`, `TestCriteriaDraftRejections`, `TestOmittedCriteriaLeavesPatchUnchanged`, `TestLegacyCriterionWritersTakeTicketLock` | VERIFIED locally |
| Agents as primary users through MCP: estimate siblings, inspect epic criteria/rollups, check criteria with explicit scope, estimate ticket creation/planning proposals | `server/internal/agentapi/{estimates,tools}.go`, `server/internal/runs/planning.go` | `TestMCPEstimateTicket`, `TestMCPEpicCriteria`, `TestPlanningProposalPoints` cover permissions, project isolation, validation, human precedence and attribution; real `/mcp` calls in `board-flow.spec.ts` | VERIFIED locally |
| Combined estimate → live board → human modal → done → epic gate | `web/e2e-stack/board-flow.spec.ts`, `web/e2e-stack/run.sh` | Prior session's `npm run e2e:stack`: 3/3 passed, including board flow (914ms), no REST/SSE mocks. Fresh-session rerun built/started the stack but Chromium launch was denied by the macOS sandbox before test execution | VERIFIED in prior session; fresh browser rerun blocked |
| CI executes the real-stack board flow green | `.github/workflows/ci.yml`: `e2e-stack` runs `npm run e2e:stack`; `playwright.stack.config.ts` discovers all `e2e-stack` specs | [Linux CI run 42330861294, e2e-stack](https://github.com/liamphmurphy/LazyAgents/actions/runs/42330861294/job/128276812650): 3 passed (12.7s), including the real MCP board flow, at branch head `21d509b` | VERIFIED in CI |
| User and agent documentation | `docs/board.md`, linked from `docs/guide.md`; `web/e2e-stack/README.md` | Documents statuses, Fibonacci scale, epic criteria/gate, modal editing, MCP tools/permissions and evidence limits | VERIFIED by source review |
| Every dependency commit delivered to main | See ancestry table below | All seven `git merge-base --is-ancestor` checks returned 1 after fetching origin | UNMET |
| Product-manager sign-off | LAZYAGENTS-36 ticket | No sign-off recorded at verification time | UNVERIFIED |

## Regression checks

- Prior session: `cd web && npm run test:unit` — 47 passed.
- Prior session: `cd web && npx playwright test e2e/board.spec.ts e2e/board-editor.spec.ts e2e/epic-criteria.spec.ts e2e/epic-dispatch.spec.ts` — 31 passed (mocked API/SSE).
- Prior session: full `go test ./...` log has no completion result; it is **not** claimed as a pass.
- Linux CI run `42330861294` at branch head `21d509b`: server (including `go test ./...`), web, contract and e2e-stack jobs all completed successfully. Independent review approved `2319006..21d509b` with no blocking defects; the UI-only empty-criteria restriction is clarified in the board guide.
- Fresh session: `cd server && TEST_DATABASE_URL=... go test -count=1 ./internal/httpapi ./internal/tracker ./internal/runs` — passed with Postgres (30.053s, 11.824s, 19.873s respectively).

## Dependency delivery

After `git fetch origin`, `origin/main` was `eff2329`. Each commit exists and is
reachable from this branch, but each command below returned **1**, so criterion
3 remains unchecked. Merge order on this branch: LAZYAGENTS-32 → LAZYAGENTS-34
→ LAZYAGENTS-35. Open dependency PRs: [27](https://github.com/liamphmurphy/LazyAgents/pull/27),
[33](https://github.com/liamphmurphy/LazyAgents/pull/33),
[28](https://github.com/liamphmurphy/LazyAgents/pull/28).

| Ticket | Commits checked with `git merge-base --is-ancestor <sha> origin/main` | Result |
| --- | --- | --- |
| LAZYAGENTS-32 | `c7a822e`, `e1ce444` | UNMET: not on main |
| LAZYAGENTS-34 | `262a1ec`, `1819db5`, `748c114`, `76ffa3b` | UNMET: not on main |
| LAZYAGENTS-35 | `5f8c6f4` | UNMET: not on main |

Re-fetch and repeat these exact ancestry checks after the dependency PRs merge.
Do not substitute ticket status or a branch merge for proof of delivery to main.
