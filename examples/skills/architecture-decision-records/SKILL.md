---
name: architecture-decision-records
description: "Use when a meaningful technical trade-off must be decided or recorded (new dependency, data model, API style, boundary between components, build vs buy) and when documenting a decision as a wiki ADR."
---

# Architecture decision records

Write an ADR when the decision is hard to reverse, affects more than one component or agent, or someone will ask "why did we do it this way?" later. Not for routine choices that follow existing conventions.

**Process**: read the code and prior ADRs (`search_knowledge`) → state the problem → list real options → compare against explicit criteria → decide → record with `write_note` (title `ADR-NNN <decision>`; link tickets and related pages with `[[…]]`) → link it from the affected tickets.

**Template**
- **Status:** proposed | accepted | superseded by [[ADR-…]]
- **Context:** the forces at play: requirements, constraints, current architecture, what is painful now. Facts, not advocacy.
- **Options considered:** 2–4 genuinely viable options, including "do nothing / keep current" and the simplest thing. For each: how it works, pros, cons, cost to build and to reverse.
- **Decision:** one sentence, active voice ("We will …"), then the key reasons tied to the criteria.
- **Consequences:** what becomes easier and harder, new risks, follow-up work (create tickets), migration/rollback plan, and how we'll know it was wrong (a trigger to revisit).

**Principles to apply**: prefer boring, proven technology; simplest design that meets today's requirements with a clear path to the next order of magnitude; make state and ownership explicit (one writer per datum); design for failure (timeouts, retries, idempotency); keep interfaces narrow and stable, implementations replaceable; consider security, operability (logs, metrics, rollbacks) and cost; avoid speculative generality.

If the decision needs a human (cost, vendor, product trade-off), ask via `await_human` with your recommendation and the options rather than deciding alone.
