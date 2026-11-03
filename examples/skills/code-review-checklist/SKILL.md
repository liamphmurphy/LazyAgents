---
name: code-review-checklist
description: "Use when reviewing any pull request or diff for correctness, design, tests and maintainability, in priority order, and when deciding between approve and request-changes."
---

# Code review checklist

**Before reading the diff:** read the ticket, acceptance criteria and linked spec/ADR. Know what "done" means. Check the PR is focused (one concern, reasonable size) and has a description with test evidence; if it's too big to review well, say so.

**Pass 1: does it do the right thing?** Map each acceptance criterion to code/tests. Look for missing criteria, unrequested behaviour (scope creep), and unhandled edge cases: empty/nil, zero/one/many, boundaries, duplicates, ordering, unicode, time zones, large inputs, concurrent access, partial failure and retries.

**Pass 2: correctness & robustness.** Trace the main path and each error path by hand. Check error handling (none swallowed; context preserved), resource cleanup (files, rows, connections, goroutines, subscriptions), cancellation/timeouts, idempotency, off-by-one and nil dereferences, race conditions, transaction boundaries, and that state transitions are valid.

**Pass 3: security & data.** AuthN/authZ on every entry point and every resource lookup (tenant/ownership scoping), input validation, injection (SQL, shell, HTML/XSS, path traversal, SSRF), secrets in code/logs, PII exposure, unsafe deserialization, dependency additions. Migrations: reversible/compatible, locking, backfill, and approval used for shared environments.

**Pass 4: design & maintainability.** Fits existing architecture and conventions; right layer for the logic; naming; duplication vs premature abstraction; public API/contract changes are intentional and documented; no dead code, stray debug output or commented-out blocks; comments explain *why*.

**Pass 5: tests.** Tests fail without the change and pass with it; cover error paths and edge cases, not only the happy path; assert behaviour rather than implementation; deterministic (no sleeps/real network); the suite actually ran (run it yourself). Missing tests for new behaviour are blocking.

**Verdict rules**
- **Request changes** if any: acceptance criterion unmet, correctness/security/data-loss bug, breaking change without migration path, missing tests for new behaviour, failing CI/tests.
- **Approve** if only non-blocking suggestions remain. Don't withhold approval for style that tooling should enforce or for personal preference.
- State confidence and what you did *not* verify (e.g. couldn't run e2e).
