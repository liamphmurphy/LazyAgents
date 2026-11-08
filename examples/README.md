# Connecting agents (unmanaged sessions)

Until the runner lands (Phase 5), agents connect to Vesper themselves. A
person mints a token on the agent's **Tokens** tab (Agents → Atlas → Tokens),
and the session acts as that agent through the Agent API at `/mcp`.

## Claude Code

```sh
claude mcp add --transport http lazyagents http://localhost:8080/mcp \
  --header "Authorization: Bearer la_…"
```

Then, in the repo, ask it to work a ticket:

> Use the lazyagents tools. Call whoami, claim PAY-214, read the memory it
> returns and its context artifacts, post a plan, and work through it. Ask for
> approval before anything that needs it. Before complete_run, write_note what
> you learned under agents/atlas/, linking the ticket and documents with [[…]].

## Codex

```sh
export LAZYAGENTS_TOKEN=la_…
codex mcp add lazyagents --url http://localhost:8080/mcp --bearer-token-env-var LAZYAGENTS_TOKEN
```

## The scripted agent

`server/cmd/fake-agent` plays Atlas over the same API with no LLM, which is
handy for demos and for checking a deployment end to end:

```sh
cd server
LAZYAGENTS_TOKEN=la_… go run ./cmd/fake-agent -url http://localhost:8080/mcp -ticket PAY-214
```

It stops at `request_approval`: approve the `db:migrate` request on PAY-214 in
the UI and the migration runs through the broker. That needs a `db:staging`
secret (Settings → Connections & secrets) pointing at a database it may change.

It also exercises agent memory (docs/dev/plan.md §12): `claim_ticket` returns the wiki
pages the run starts with, and before finishing it appends to
`agents/atlas/Partition rollback gotchas`, linking `[[ADR-012]]` and the
ticket. Claim PAY-224 afterwards (it has ADR-012 attached) and that note is in
the `memory` the claim returns:

```sh
curl -s -X POST http://localhost:8080/api/v1/agent/tools/claim_ticket \
  -H "Authorization: Bearer la_…" -d '{"ticket":"PAY-224"}' | jq '.memory[] | {ref, reason}'
```

With `make dev`, use `http://localhost:8000/mcp`, the API directly.
