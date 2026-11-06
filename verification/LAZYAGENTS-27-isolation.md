# LAZYAGENTS-27: OpenCode configuration isolation

## Scope and contract

The prerequisite OpenCode runtime/UI/verification stack from PR #20 (including
#18/#19) is integrated in merge commit ffb4e76, preserving current main's
delegated worktrees, session retry and fallback effort controls. Ticket changes
follow that commit.

The adapter requires a backend-owned absolute state directory outside the
checkout. Generated config travels in Invocation.StateFiles and
ExecSpec.StateFiles, independently of checkout files. Local state lives under
the run directory's .lazyagents/opencode/<namespace> sibling of workspace.
Docker uses a separate named volume at /.lazyagents/opencode; worktrees have
separate stable namespaces. Both are removed with their parent workspace.

The backend selects its generated config with OPENCODE_CONFIG, replaces all
HOME/XDG paths and discards ambient OPENCODE_* switches. Each launch checks
repository and ancestor configs, known HOME/XDG configs and old checkout-local
state. State loss or legacy resume fails explicitly. Staging rejects symlink
destinations. Docker uploads a payload to a fresh image-owned directory and
stages into persistent volumes using no-follow checks, avoiding unsafe archive
extraction into an agent-controlled tree and large argv payloads.

## Requirement evidence

| Requirement from ticket | Evidence |
| --- | --- |
| State/config outside checkout | TestOpenCodeExternalStateAndResume, adapter external-state validation, Docker volume/payload test |
| Prevent workspace-ancestor config | TestOpenCodeRejectsConfigLayersEveryTurn: repository JSON/JSONC, .opencode, ancestor, XDG, HOME, dangling symlink, legacy state; checked on the next turn |
| Denies follow allows | TestOpenCodeExternalStateRequiredAndProtectedRulesOrdered: absolute write:*/.lazyagents/*, edit:*/.lazyagents/* plus relative variants |
| Real opt-in self-escalation regression | TestRealOpenCodeSelfEscalation: pinned 2.2.1, localhost provider, fake key, successful ordinary write, blocked XDG/metadata poison writes, same-session resume with history, unavailable shell |
| Protect generated destinations | TestOpenCodeStagingRejectsSymlinks, TestDockerOpenCodeNoFollowStaging |
| Resume/lifecycle | External-state persistence/cleanup, missing-resume-state rejection, database-backed ticket and chat resume tests |

The real probe corrects the known LAZYAGENTS-26 tool allows and permits external
directory access only within its test config, so those unrelated restrictions
cannot accidentally make the security regression pass. Both absolute and
relative metadata rules are needed: the real CLI matches in-checkout resources
relative to the checkout root. A mutation removing all four protected rules
makes the real regression fail because a protected file is written.

## Validation

- From server/: GOCACHE=/private/tmp/lazyagents-27-go-cache go test ./...:
  PASS (database-dependent tests skipped in this invocation).
- With the local dev Postgres URL as TEST_DATABASE_URL:
  go test ./internal/harness ./internal/runner ./internal/httpapi ./internal/agentdef:
  all passed except the chat test's old checkout-config path assertion. After
  updating that assertion, go test ./internal/runner -run TestOpenCodeChatTurnsResume -v:
  PASS. Tests create and drop their own uniquely named disposable databases.
- LAZYAGENTS_REAL_OPENCODE=1 go test ./internal/runner -run TestRealOpenCodeSelfEscalation -v:
  PASS against real OpenCode 2.2.1, no database and no paid provider.
- Focused new isolation and Docker staging/Engine-contract tests: PASS.
- Web npm run typecheck and npm run lint: PASS after prerequisite conflict resolution.
- git diff --check: PASS.
- Docker staging is tested through a simulated Engine and the actual Python
  staging helper. A rebuilt real Docker image has not been exercised here.

## Remaining boundary

This change does not create an OS sandbox. The existing isolation attestation
stays off by default and now explicitly requires that granted shell/MCP tools
cannot alter protected config/state or race the checks. An ordinary same-user
local process or writable container state volume cannot provide that guarantee
by itself. Shared-runner enablement remains blocked without that boundary and
the separate LAZYAGENTS-26 policy fix. No paid-provider parity is claimed.

Old checkout-local sessions are not imported. Start a new run after upgrading.
