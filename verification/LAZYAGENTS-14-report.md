# Chat feature verification — LAZYAGENTS-14

## Verdict

Local-runner correction (2026-11-03): application provider secrets are required
only for Docker mode. The local retry failed before provider startup: an older
runner binary logged `load run` / `no rows in result set`; the API independently
used Docker catalog gating and rejected Codex. Local CLI authentication was never
tested by that attempt. Commit 21702bd aligns Makefile API/runner defaults to
local while retaining explicit Docker mode. Owners must coordinate an idle
refresh of stale workers and the API before retrying; no active process was
interrupted. See the runtime evidence's local continuation and coordinated refresh
procedure. Earlier missing-secret prerequisite language below describes the first
attempt and is superseded for local execution.

Partial verification; criterion 2 remains unverified. Do not treat this report as release or QA approval. Verification date: 2026-11-03. Reviewed checkout baseline: 4176f64; delegated evidence commit: 3f5de6c. No feature implementation, shared database migration or deployment was performed.

## Requirement traceability

| Goal requirement | Passing evidence and ticket criteria | Remaining coverage / follow-up |
| --- | --- | --- |
| Persisted chat history | LAZYAGENTS-7 criteria 2–5; fresh database/query suite and TestChatMessagesPostCancelAndHistory, TestChatTurnsResumeWithoutTools, recovery/cancel tests pass | LAZYAGENTS-12 criteria 1–3/5 were previously left unchecked because mock UI tests do not prove integration. [[LAZYAGENTS-16]] supplies real-stack browser coverage. |
| Harness/model toggle | LAZYAGENTS-10 criteria 1/2/4: fresh TestChatHarnessSwitchStartsFreshSegmentWithHandoff, TestChatModelSwitchKeepsSession and handoff boundary tests pass. LAZYAGENTS-11 catalog/selection tests pass | Real-model recall remains criterion 2 below. Handoff metadata/truncation gaps: [[LAZYAGENTS-17]]. |
| Artifacts/wiki/ticket project context through MCP | LAZYAGENTS-9 tool policy/project isolation; LAZYAGENTS-11 context resolution/citation fields; LAZYAGENTS-13 criteria 1–5 recorded checked. Fresh TestChatRunToolAuthorization, TestChatContextRefsAreDeliveredPerTurn and TestChatTurnCitations pass | Actual provider MCP read and citation pending real smoke; browser Sources/context integration in [[LAZYAGENTS-16]]. |
| Model-independent behavior | Fake binaries exercise both Claude Code and Codex adapters and preserve model attribution/session behavior | No real-provider parity claim: neither provider completed a turn in the smoke. Same-harness missing-session fallback and idle cleanup: [[LAZYAGENTS-18]]. |
| Chat usage documentation | README Chat section describes start/history, picker, context chips, Sources, read-only scratch workspace, provider prerequisites and transfer limits | Reviewed against current UI strings and context/switch code. |

## Executed checks

Delegated QA freshly ran full server `go test -race -count=1 ./...` with TEST_DATABASE_URL on disposable Postgres 17 port 55441: PASS. Web lint, Next type generation, typecheck and all 7 unit tests: PASS. Playwright production build succeeded, but all 10 cases failed before assertions at sandbox-denied Chromium launch. Every existing chat Playwright case installs request mocks; E2E_BASE_URL does not remove them.

Detailed commands, sanitized IDs, observations and evidence limits are in [runtime evidence](LAZYAGENTS-14-runtime-evidence.md). Lead reviewed that commit and verified the run-detail defect in server/internal/httpapi/runs.go and unconditional mock setup in web/e2e/chat.spec.ts. No repeated full suite was needed for subsequent documentation-only edits; git diff --check was used.

## Real-project smoke: blocked

Existing live app on localhost:8000/3000 advertised both harnesses unavailable: missing harness:anthropic and harness:openai application secrets (metadata-only check, no values accessed). A newly created LAZYAGENTS project chat accepted its first message, asking to remember amber-otter-14 and read/cite the Project chat sessions ADR. Run 01a1a227-f1ba-7b14-8c44-5e82797c7a46 remained starting; no assistant response was produced. No Claude → Codex switch or recall/citation success occurred. The live binary's provenance was not established.

QA cancelled and deleted only its own smoke chats; runs ended cancelled. No shared migration or deployment occurred. For local execution, owners must align the API mode and refresh all eligible stale workers when idle; existing host CLI logins supply authentication. Docker execution requires configured provider secrets. Preserve the resulting two assistant replies, correct recall, wiki citation/version and segment IDs as evidence. A fake test cannot substitute for this criterion.

## Filed follow-ups

- [[LAZYAGENTS-15]] — fix chat run GET returning 500: toRun treats all ticketless runs as planning; add purpose-aware HTTP regression.
- [[LAZYAGENTS-16]] — real-stack UI/fake-harness coverage without route mocks, in a Chromium-capable environment.
- [[LAZYAGENTS-17]] — include title/context refs in bounded handoff and explicitly mark SQL-preclipped message bodies.
- [[LAZYAGENTS-18]] — missing-session fresh-handoff retry and seven-day idle segment cleanup per accepted ADR.
- [[LAZYAGENTS-19]] — reproduce unavailable-key startup and cancellation state mismatch on current binary; establish root cause and regression tests.

Each follow-up includes an intended owner and numbered acceptance criteria. Token streaming, shared chats and repository-editing chats remain deliberate v1 non-goals in [[Architecture/ADR - Project chat sessions]]; they were not reported as defects.

## Acceptance and next action

Criterion 1: this REPORT maps every goal to passing test/ticket evidence or filed follow-up.
Criterion 2: UNVERIFIED; needs configured providers/runner and successful real-project smoke.
Criterion 3: README Chat section added and reviewed.
Criterion 4: confirmed gaps filed above.
Overall done/QA approval must wait for criterion 2 and independent final review.

