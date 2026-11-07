---
name: ticket-breakdown
description: "Use when turning a goal, PRD or epic into tickets with propose_tickets: sizing, slicing, acceptance criteria, dependencies and assignment."
---

# Breaking work into tickets

**Slice vertically, size for one run.** Each ticket should be completable by one agent in one sitting (roughly ≤ a few hundred lines changed, one PR) and deliver something verifiable. Slice by user-visible behaviour or by a cleanly bounded technical step, not by layer-for-layer busywork. Split when a ticket has "and", touches unrelated modules, or can't be tested on its own.

**Order and dependencies**
- Put contracts first: schema/API/ADR tickets block the implementations that use them. Backend contract → backend implementation ∥ frontend implementation → integration/e2e → docs.
- Use `blocked_by` only for real blockers; independent tickets should be parallelizable. Avoid cycles. Call out any migration or deploy ticket explicitly (it needs human approval).
- Include a final "verify against requirements" step or ticket for `@product-manager`.

**Each ticket contains**
1. **Title:** imperative, specific ("Add `POST /v1/widgets` with validation"), not "Backend work".
2. **Description:** the *why* (link the PRD/ADR with `[[…]]`), the scope, explicit non-goals, and pointers to the relevant code, files and patterns to follow.
3. **Acceptance criteria:** 3–7 numbered, observable, binary-testable statements ("Returns 422 with `field=name` when name is empty"; "Keyboard: Esc closes the dialog and returns focus"). No vague criteria like "works well". Include error/edge cases and non-functional criteria (performance, accessibility, security) that matter.
4. **Estimate:** `points` on the Fibonacci scale (1, 2, 3, 5, 8, 13) plus a one-line `estimate_rationale` (what drives the size: files touched, unknowns, test setup). Anything above 8 is a sign to split; anything else off the scale is rejected. Accepted drafts keep the estimate, and implementers see it in `get_ticket`.
5. **Test expectation:** what tests must exist or change.
6. **Assignee:** backend work → `@backend-engineer`; UI → `@frontend-engineer`; docs/requirements verification → `@product-manager`; review happens via `@code-reviewer` on each PR.
7. **Risks/unknowns:** anything that needs a human decision; make it a question on the ticket instead of guessing.

**Check before submitting:** every PRD requirement maps to ≥1 ticket criterion (traceability); no ticket is larger than one PR; dependencies form a DAG; names and criteria use the PRD's vocabulary.
