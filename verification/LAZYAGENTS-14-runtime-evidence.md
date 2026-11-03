# LAZYAGENTS-14 delegated runtime verification

Verified 2026-11-03 in the existing checkout at `4176f64`. Initial working tree was clean; no previous feature changes or commits were replaced. Read the ticket, attached local ADR and three tech-lead memory pages. Ticket criteria were all unchecked. Posted a three-step delegated verification plan before writing this evidence. No feature code, push, deployment, or shared database migration performed.

**QA verdict: changes requested / acceptance criterion 2 unverified.** This is supporting evidence for the lead's REPORT, not the final requirement-to-ticket report or acceptance sign-off.

## Checks executed

| Command | Result and scope |
| --- | --- |
| `docker ps --format '{{.Names}} {{.Ports}}'` | Existing `la14-pg`, Postgres 17 disposable test container, available at 127.0.0.1:55441. Shared development Postgres also running at 5432. |
| In `server`: `TEST_DATABASE_URL='postgres://pg:pg@127.0.0.1:55441/postgres?sslmode=disable' GOCACHE=/private/tmp/la14-gocache go test -race -count=1 ./...` | PASS, exit 0, including runner (76.999s), HTTP API (38.238s), runs, database, MCP authorization, and other packages. Confirms the historical pass supplied in the delegation with a fresh run. Database setup/migrations by tests target disposable test databases, not shared development data. |
| In `web`: `npm ci` | PASS; installed dependencies without changing tracked lockfile. Audit reported 20 moderate vulnerabilities; no dependency edits made. |
| In `web`: `npm run lint && npx next typegen && npm run typecheck && npm run test:unit` | PASS, exit 0. Seven unit tests passed. |
| In `web`: `npm run e2e` | Next production build/start succeeded. All 10 tests failed at Chromium launch, before UI assertions: `bootstrap_check_in ... Permission denied (1100)`, SIGTRAP. Sandbox prevents browser launch; no passing UI evidence claimed. Local ignored traces remain under `web/test-results/`. |

## Real-project smoke attempt through the running application

Application: API `http://localhost:8000`, web listener at 3000. Bootstrap identifies active project `LAZYAGENTS`, id `01a19ef1-194f-7e0e-a4ec-ea6adaf97f9c`. Running binary provenance was not independently established; these observations describe the existing live development stack, while tests above describe this checkout.

1. `GET /api/v1/chat-models` returned HTTP 200. Claude Code: `available=false`, reason `no harness:anthropic secret`; Codex: `available=false`, reason `no harness:openai secret`. Catalog availability is workspace-wide; adding a project query did not change it. A metadata-only SQL query selecting secret names and project-scope booleans for these two names returned zero rows. No secret values or process environments were read or printed. Existing CLI login/provider subscription state is not evidence of configured application secrets.
2. `POST /api/v1/chats` with `{"project":"LAZYAGENTS","harness":"claude-code","model":"sonnet-5.5","title":"LAZYAGENTS-14 provider smoke"}` returned HTTP 201. Chat id: `01a1a227-954f-76a5-a324-b2eb65fb96ed`. The app allowed creation despite the unavailable catalog.
3. An initial request using the response field `body_md` was correctly rejected with HTTP 422 (`body: provide 1–100000 characters`). Retried using the documented request field `body`.
4. `POST /api/v1/chats/<chat id>/messages` with `{"body":"LAZYAGENTS-14 verification: remember the marker amber-otter-14. Read the project wiki ADR on Project chat sessions and cite that page in a brief reply."}` returned HTTP 202. User message id: `01a1a227-f1c0-70c1-92e8-8a81459ba02c`; segment run id: `01a1a227-f1ba-7b14-8c44-5e82797c7a46`. Created at 08:04:49 -08:00. Repeated reads returned only that user message, no assistant response or citations. Read-only database status showed the run in `starting`.
5. `GET /api/v1/runs/01a1a227-f1ba-7b14-8c44-5e82797c7a46` repeatedly returned HTTP 500, `{"code":"internal","message":"internal error"}`. Checkout code in `server/internal/httpapi/runs.go`, `toRun`, unconditionally calls `PlanningSessionForRun` whenever `TicketID == nil`; chat runs are ticketless but have no planning session. This is a concrete code defect consistent with the runtime result and merits a follow-up/regression test.
6. Cleanup: `POST /api/v1/chats/<id>/cancel` returned HTTP 200; the run became `paused`, wait reason `pause`, while chat still showed `running`. Archive with revision 2 returned HTTP 409 (`a turn is still running`). Deleted only this newly created smoke chat via normal `DELETE /api/v1/chats/<id>`: HTTP 204. Read-only SQL confirmed the run is `cancelled`, outcome `The chat was deleted.` The transcript is recorded above; chat history itself was removed by cleanup.

No Claude assistant turn completed. A Claude-to-Codex switch with context recall and wiki citation therefore could not be demonstrated. No success, model equivalence, provider authentication, or criterion 2 check is claimed. Missing secrets are a confirmed prerequisite blocker; the exact cause of the run remaining in `starting` was not established.

## Traceability and evidence limits

| Goal behavior | Existing passing backend coverage | Remaining integration limit |
| --- | --- | --- |
| Persisted history and resume | `runner/chat_test.go`: `TestChatTurnsResumeWithoutTools`, recovery/concurrent/cancel cases; `httpapi/chats_test.go`: `TestChatMessagesPostCancelAndHistory`; `db/chat_queries_test.go` | Backend runner uses guarded fake harness binaries, not real providers. UI reload evidence requires browser execution. |
| Harness/model switching | `runner/chat_switch_test.go`: `TestChatHarnessSwitchStartsFreshSegmentWithHandoff`, `TestChatModelSwitchKeepsSession`; `runs/handoff_test.go` checks budget/order/clipping | Fakes echo prompt/session/model data; this proves handoff transport and attribution, not an actual model's comprehension. |
| Wiki/artifact/ticket context and sources | `runner/chat_context_test.go`: `TestChatContextRefsAreDeliveredPerTurn`; `runner/chat_test.go`: `TestChatTurnCitations`; `httpapi/chats_test.go`: context resolution, citation fields; `httpapi/chat_tools_test.go`: read-only tool authorization | Real-provider MCP discovery/read/citation has not passed. |
| History/UI controls/context chips/save reply | All ten `web/e2e/chat.spec.ts` cases cover the intended UI flows using `installChatApi(page)` | Every case installs request mocks unconditionally. `E2E_BASE_URL` only disables the Next webServer; it does not disable mocks. No real-stack fake-harness UI test exists in this suite. Browser launch blocked even mocked tests here. |
| Model independence | Runner tests cover both harness adapters with fake executables; same-harness model changes preserve session | Real Claude Code and Codex parity remains unverified. |

## Follow-ups for lead to file/link in REPORT

- Supply both application provider secrets through normal configuration, restore a working runner, then rerun the exact criterion 2 flow and retain assistant text, citation target/version, segment ids, and a successful recall of the marker. Do not mark criterion 2 based on fake tests.
- Fix run-detail rendering to distinguish `purpose=chat` from planning and add an HTTP regression test for a chat run. Current 500 blocks live activity/run inspection.
- Add or perform a real-stack UI flow with a fake harness without `page.route` mocks; rerun Playwright in an environment permitting Chromium launch. Current checked-in mock suite cannot validate REST/runner/SSE/UI integration.
- Investigate unavailable-catalog creation and a first turn lingering in `starting`; cancellation returned success while archive remained blocked. These are observed runtime behaviors, not a proven common root cause or a newly passing requirement.
- ADR implementation gaps confirmed by inspection: handoff entries carry only message role/model/body (no title/attached context refs), SQL pre-clipping at 8000 characters can hide the clipping marker, and `Runner.Sweep` only cleans ended workspaces (no seven-day idle segmentation). Same-harness resume-failure handoff fallback remains unverified and was already recorded as open in tech-lead memory.

Token-level streaming, shared chats, and repository-editing chats are ADR non-goals; absence is not a runtime regression. README Chat instructions, final REPORT artifact, and follow-up ticket filing remain owned by the lead. No acceptance criteria were checked by this delegation.
