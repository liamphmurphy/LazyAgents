---
name: requirements-verification
description: "Use when tickets or an epic reach In Review/Done and you must check whether the delivered work satisfies the PRD and acceptance criteria, and report gaps."
---

# Requirements verification

**Inputs:** the PRD/epic requirements (`R1…`), each ticket's acceptance criteria, PR descriptions and diffs summaries, test results/run events, artifacts, and the running behaviour where you can observe it. You judge outcomes; defer code quality to `@code-reviewer` and technical choices to `@tech-lead`.

**Method**
1. Build a traceability table: requirement → tickets → acceptance criteria → evidence.
2. For each requirement decide: **Met** (evidence directly shows the behaviour, including the edge/error cases required), **Partially met** (works but missing a case, state or NFR: say exactly what), **Not met**, or **Unverified** (no evidence available: say what evidence would settle it). Evidence = test names and results, PR/ticket links, behaviour observed, screenshots. "The ticket is Done" is not evidence.
3. Also check: requirements with no ticket (planning gap); tickets whose work isn't traced to any requirement (scope creep); non-goals accidentally delivered; docs/release notes updated; success metrics measurable as planned.
4. Look for user-facing regressions and inconsistencies in wording, flows and states versus the PRD and design.

**Report** (publish with `create_artifact` as a REPORT, linked from the epic and the wiki status page):
- **Summary:** overall status (Ready / Ready with gaps / Not ready), a one-line reason, counts of Met / Partial / Not met / Unverified.
- **Table:** requirement | status | evidence | gap.
- **Gaps & recommendations:** each gap with severity (blocks release / should fix / follow-up) and a proposed ticket.
- **Open questions** needing a human decision.

**Follow-through:** open a ticket per gap with `create_ticket` (user-facing title, what's wrong vs which requirement, acceptance criteria) and re-verify after fixes, noting the outcome in the report. Don't mark an epic complete while a Must requirement is Partial or Not met.
