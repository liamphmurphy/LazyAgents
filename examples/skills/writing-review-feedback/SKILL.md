---
name: writing-review-feedback
description: "Use when writing the review comment or verdict on a ticket or PR, to make findings clear, prioritized, actionable and kind."
---

# Writing review feedback

**Shape of the comment**
1. **Verdict line first:** `Verdict: Approve` or `Verdict: Request changes (N blocking)`, plus one sentence of overall assessment and what you ran (e.g. "ran `go test -race ./...`, all pass; did not run e2e").
2. **Blocking** findings (must fix before merge), then **Non-blocking** (suggestions, nits, follow-ups). Number them so the author can reply "fixed #2".
3. Each finding: `file:line` → what's wrong → *why it matters* (the failure scenario: inputs/state → wrong outcome) → a concrete fix or example. If unsure, say what you checked and ask a question rather than asserting.
4. **Acceptance criteria:** list each criterion as met / not met / not verifiable with a pointer to the evidence. Tick `check_criterion` only for those you verified.

**Style**
- Critique the code, not the author. Be specific and brief; skip praise padding but do call out something genuinely well done when it teaches a pattern.
- Don't repeat what linters/formatters catch; don't bikeshed naming unless it misleads.
- Prefer "Consider X because Y" for preferences, "This must change because Y" only for real defects. Label nits as `nit:`.
- Rank by impact. A review with 25 nits and a buried data-loss bug has failed.
- Don't rewrite the PR in your head: if the approach is sound but different from what you'd do, approve it.
- Escalate design-level concerns to `@tech-lead` via comment instead of litigating them in the PR.
- On re-review, verify each previously raised item and say which are resolved; don't raise new non-blocking issues late unless they're newly introduced.
