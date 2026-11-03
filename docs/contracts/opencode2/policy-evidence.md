# OpenCode 2.2.1 policy correction

Verified 2026-11-03 for LAZYAGENTS-20 / PR #15 review of 98d58b6.
Decision: retain official @opencode/cli **2.2.1**, executable/public ID `opencode`,
and use `integration.use` with prefixed resources. No newer version is selected.
The existing cli-help.txt remains installed Nix 2.2.1 help evidence, not an
npm integrity check or proof of effective runtime enforcement.

## Immutable upstream evidence

`git ls-remote https://github.com/anomalyco/opencode.git refs/tags/v2.2.1`
resolved to `3ebcf9a32c7b28f658cfee80775448929361727a`.
Read these files directly at that commit, without installing or executing upstream:

- [Policy schema](https://github.com/anomalyco/opencode/blob/3ebcf9a32c7b28f658cfee80775448929361727a/packages/schema/src/config/policy.ts): action enum is exactly `provider.use`, `tool.use`, `integration.use`; effects are `allow`/`deny`, resource is a string.
- [Configuration schema](https://github.com/anomalyco/opencode/blob/3ebcf9a32c7b28f658cfee80775448929361727a/packages/schema/src/config.ts): top-level `policies` accepts an array of those statements.
- [Policy implementation](https://github.com/anomalyco/opencode/blob/3ebcf9a32c7b28f658cfee80775448929361727a/packages/core/src/config/plugin/policy.ts): MCP transform checks `integration.use` against `mcp:${name}` and removes denied servers; skill transform checks `skill:${skill.id}`. Provider filtering uses `provider.use`; the permission hook matches `tool.use` against action/resource and forces denial.
- [Policy evaluation](https://github.com/anomalyco/opencode/blob/3ebcf9a32c7b28f658cfee80775448929361727a/packages/core/src/managed-policy.ts): last matching statement wins, unmatched resources default allow, broader configuration takes precedence through reversed document order.
- [Versioned V2 docs](https://github.com/anomalyco/opencode/blob/3ebcf9a32c7b28f658cfee80775448929361727a/services/www/src/docs/content/policies.mdx#integrations): documents `integration.use` resources `mcp:<name>`, `skill:<id>`, `plugin:<target>`, including deny `mcp:*` followed by an exact server allow. Use this V2 page; the same tag's older `packages/web` page describes the earlier experimental provider-only interface.
- [Upstream tests](https://github.com/anomalyco/opencode/blob/3ebcf9a32c7b28f658cfee80775448929361727a/packages/core/test/config/policy.test.ts): cover tool last-match precedence, deny overriding ask, broad denies, and global policy precedence. These were inspected, not executed here.

The [current migration documentation](https://opencode.ai/v2/docs/policies/#upgrading-from-integrationuse)
explains the 2.2.3 action split. It corroborates the review finding: older clients
ignore the new actions. It does not justify using them on 2.2.1.

## Selected syntax and limits

ADR.md now denies `integration.use` / `mcp:*`, then allows only `mcp:lazyagents`.
It separately denies `skill:*` and `plugin:*`; native skill discovery may add exact
resolved skill exceptions only after verification. MCP tool permissions and
`tool.use` policy exceptions remain separate, purpose-specific requirements.
Never treat a server allow as authorization to every server tool.

This is source-backed syntax and intended filtering behavior, not a live enforcement
capture. No model call, host install, credential access or runtime change was made.
Schema validation and effective-denial probes under the exact deployed artifact,
managed config isolation, required built-in policy loading, authenticated approval
bridge, filesystem sandboxing and all ticket/planning/nine-tool chat parity gates
remain mandatory before advertising runtime support. Optional plugin denial does
not sandbox built-in or arbitrary plugin code. Reject unsupported actions, unknown
options, policy load failures and unenforceable modes before setup/credentials.
