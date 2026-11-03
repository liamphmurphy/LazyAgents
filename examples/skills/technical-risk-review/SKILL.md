---
name: technical-risk-review
description: "Use when reviewing a plan, design or epic for technical risk before work starts, or when a ticket touches migrations, auth, concurrency, external services, data integrity or performance-sensitive paths."
---

# Technical risk review

Walk the plan against this checklist and write the findings as a short comment or wiki note (Risk → Likelihood/Impact → Mitigation → Owner):

1. **Data & migrations:** is the schema change backward compatible (expand → migrate → contract)? Backfill size and locking? Rollback story? Does anything need human approval?
2. **Security:** authn/authz on every new path, tenant isolation (every query scoped), input validation, secrets handling, injection and SSRF, least privilege for tokens and agent scopes, audit trail.
3. **Correctness under concurrency & failure:** retries, duplicates, ordering, partial failure, idempotency, timeouts, cancellation, crash mid-operation.
4. **Compatibility:** API/contract changes, generated code, existing clients, feature flags or staged rollout.
5. **Performance & scale:** unbounded queries, N+1, hot paths, payload sizes, background work, cost of LLM/API calls.
6. **Operability:** logging, metrics, alerts, config, runbook; how do we notice it is broken in production?
7. **Testability:** can each acceptance criterion be verified automatically? Where do we need integration or e2e coverage?
8. **Dependencies & sequencing:** hidden coupling between tickets, shared files likely to conflict, unknowns that need a spike first (timebox it and make it a ticket).
9. **Scope:** anything gold-plated or speculative that can be cut; anything missing that the PRD requires.

Resolve risks by changing the plan (reorder, split, add a spike or test ticket, add acceptance criteria) rather than only noting them.
