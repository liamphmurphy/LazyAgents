---
name: lazyagents-wiki
description: Look up the LazyAgents team wiki (shared Patterns, ADRs, agent notes) through the local API. Use when starting non-trivial work in this repo, before designing or reviewing a change, or when unsure of a convention (Go style, testing, API, Postgres, React, architecture, process).
---

# LazyAgents wiki lookup

The team wiki lives in the running LazyAgents server, not in the repo. Read it over the local REST API with `curl`. Read-only: never POST/PUT/DELETE here; proposing and publishing patterns happens inside LazyAgents (propose_pattern → librarian → tech-lead).

API base: `${LAZYAGENTS_API:-http://localhost:8000/api/v1}`. If it doesn't answer, say "wiki unavailable" and continue without it. Don't start the server.

## 1. Patterns first (curated conventions)

Fuzzy search; typos and partial words are fine. Empty `q` lists everything. Optional `&topic=`: Go, React, Postgres, Testing, API, Architecture, Process.

```sh
API=${LAZYAGENTS_API:-http://localhost:8000/api/v1}
q() { python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$1"; }
curl -s "$API/wiki/patterns?limit=5&q=$(q 'handler tests')" | python3 -c "
import json, sys
for p in json.load(sys.stdin)['patterns']:
    print('%.2f  %s  [%s]' % (p['score'], p['path'], ', '.join(p['tags'])))
    print('      ' + p['summary'])"
```

## 2. Wider search (ADRs, notes, tickets)

```sh
curl -s "$API/search?q=$(q 'parallel delegation')" | python3 -c "
import json, sys
for h in json.load(sys.stdin)['hits'][:8]:
    t = h['target']
    print('%.2f  %s' % (h['score'], t.get('path') or t.get('title')))"
```

## 3. Open a page

```sh
curl -s "$API/wiki/page?path=$(q 'Patterns/Testing/Run DB tests against a real Postgres 17')" \
  | python3 -c "import json, sys; print(json.load(sys.stdin)['body_md'])"
```

## How to use what you find

- Follow a pattern that applies, and say which one you followed.
- Treat `agents/<handle>/…` notes as context, not rules; they are run logs and can be stale.
- `Architecture/ADR - …` pages are project decisions: don't contradict one without flagging it.
- If a pattern conflicts with the code, trust the code and mention the conflict so the pattern can be fixed.
- Keep lookups short: 1-3 queries, open at most a few pages.
