# Integration inventory at 96110d1

Paths are relative to repository root. These are future changes, not implemented support.

| Surface | Files | Required integration / compatibility |
| --- | --- | --- |
| Definition, override and fallback validation | server/internal/agentdef/{spec,resolve,enforce}.go | Add confirmed `opencode` ID to Harnesses; capability defaults false until verified; validate options and fallback scopes |
| API source/generated consumers | api/openapi.yaml; server/internal/api/{gen.go,oapi.yaml,api.gen.go}; web/lib/api/schema.d.ts | Harness enum currently two values; regenerate both consumers in coordinated LAZYAGENTS-22 rollout after adapter gates |
| Agent catalog/resolved UI metadata | server/internal/httpapi/agents.go; server/internal/agentdef/enforce.go | Advertise accurate tool gating/approval/native-cost/resume and enforcement strength |
| Agent defaults and editable definitions | server/internal/seed/agents.go; web/app/(app)/agents/[handle]/definition-editor.tsx | No new default; remove two-way fallback assumption when adding third harness |
| Adapter boundary | server/internal/harness/{harness,claude,codex,tool_hook}.go; server/internal/harness/codex_launch.py | New Invocation/Parser; reuse purpose renderers; provide authenticated policy bridge or reject capability |
| Dispatch and frozen specs | server/internal/httpapi/dispatch.go; server/internal/runner/{runner,fallback}.go | Validate availability before setup; replace Codex-specific parser/baseline handling with explicit approved branch; preserve frozen spec and fallback behavior |
| Backends and binary config | server/internal/runner/{backend,docker}.go; server/internal/config/config.go; server/cmd/lazyagents | Add explicit binary mapping/OnlyBins protection and version pin; local host execution is not isolation |
| Image and lifecycle wrapper | deploy/agent-base/Dockerfile; server/internal/runner/prepare.go | Pin verified package; preserve cwd/instruction merge and terminate private-server descendants |
| Credential injection | server/internal/runner/prepare.go HarnessKeys/PlanSecrets; server/internal/runner/fallback.go; server/internal/secrets | Map anthropic/openai to existing provider secrets narrowly; deny unsupported auth modes; never accidentally use a missing map entry or inject other provider keys |
| Model catalog | server/internal/chatmodels/{chatmodels,discovery,local}.go | Live discovery already supersedes baseline static catalog; validate provider-qualified IDs under runtime identity; bounded timeout/output and sanitized unavailable reason |
| Planning | server/internal/runs/planning.go; server/internal/runner/{runner,fallback}.go | Preserve PlanningSpec no setup/repo/external servers and readonly fallback constraints; propose_tickets workflow |
| Ticket and delegation | server/internal/runs/{service,delegation}.go; server/internal/harness/harness.go; server/internal/runner/runner.go | Preserve parent ownership, generated instructions and frozen resolution; no claim in delegated execution |
| Chat segment/resume/handoff | server/internal/runs/{chat,handoff}.go; server/internal/runner/{chat,handoff,runner}.go; server/docs/chat-switching.md | Keep segment-per-harness, same-harness model changes, idle cleanup, bounded handoff and one resume retry |
| MCP authorization | server/internal/agentapi/{transport,chat,tools}.go; server/internal/runs/approvals.go; server/internal/httpapi/chat_tools_test.go | Per-run purpose/project authorization remains server enforced; preserve nine-tool project-scoped chat policy; reject memory writes and cross-project mutations |
| Scopes and policy endpoint | server/internal/agentdef/enforce.go; server/internal/harness/tool_hook.go; server/internal/httpapi | Existing hooks cannot be assumed portable; test authenticated policy transport and fail closed |
| Usage and pricing | server/internal/harness/{harness,codex,claude}.go; server/internal/config/config.go; server/internal/runner/{runner,chat}.go | Exact provider/model pricing; explicit unknown/partial, deltas and terminal deduplication; no unknown-as-free |
| Persistence | server/migrations; server/internal/store/queries; server/internal/store | Existing text harness/model/session and JSON usage suffice; no migration in this scope |
| UI chat and activity | web/app/(app)/chat/_components/{model-picker,conversation,chat-screen}.tsx; web/components/domain/start-chat.tsx; web/app/(app)/runs/[id]/run-log.tsx | Use live catalog labels/availability and provider IDs; preserve switching warning, cancellation and unknown/partial cost display |
| Manual onboarding/docs | web/app/(app)/agents/[handle]/tokens-tab.tsx; README.md; docs/guide.md; PLAN.md | Document executable/pin/provider auth with pinned 2.2.1 rollout; no new command promised now |
| Fixtures and verification | server/internal/harness/testdata; server/internal/harness/{harness,parity,fallback}_test.go; server/internal/runner/*test.go; server/internal/httpapi/chats_test.go; web/e2e/{chat.spec.ts,mock-chat-api.ts,run-log.spec.ts} | Capture actual approved CLI envelopes; extend two-harness assumptions without rewriting existing evidence |
| Generation gate | Makefile gen-check; .github/workflows/ci.yml | Go oapi-codegen + sqlc + openapi-typescript; no hand edits to generated files |

Search method: rg for claude-code, codex, Harnesses, HarnessKeys, Prices and chat/planning purpose branches across server, web, api and deploy. Review the approved implementation diff against this inventory before enabling any catalog entry.

Owner resolution (2026-11-03): official @opencode/cli; public ID opencode;
verified installed baseline 2.2.1. Target full current Claude/Codex ticket, planning,
and chat behavior. ADR.md defines the coordinated enum rollout and provider-aware
credential mapping; no runtime or generated schema change belongs in this commit.
