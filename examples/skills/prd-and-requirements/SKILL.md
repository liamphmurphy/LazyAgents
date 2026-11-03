---
name: prd-and-requirements
description: "Use when writing or improving a PRD, feature spec or epic description, defining requirements, user stories, goals/non-goals and success criteria, or when a request is ambiguous."
---

# PRD and requirements

**Template** (a wiki page via `write_note`, linked to its epic/tickets):
1. **Problem & context:** who has the problem, what it costs them, evidence (quotes, data, tickets). One paragraph.
2. **Goals & success metrics:** outcomes, not outputs, each measurable ("First-run users create a ticket within 5 minutes"). Say how and where it will be measured.
3. **Non-goals:** explicitly what we are *not* doing now, to protect scope.
4. **Users & scenarios:** personas and the key flows end to end.
5. **Requirements:** numbered and testable (`R1`, `R2`, …), each with priority (Must / Should / Could) and rationale. Use user-story form where it helps: *As a <user>, I want <capability> so that <benefit>.*
6. **Acceptance criteria** per requirement in Given/When/Then or checklist form: observable, unambiguous, covering the edge and error cases (empty, invalid, permissions, failure).
7. **Non-functional requirements:** performance, security/privacy, accessibility, reliability, compatibility, localization: only those that matter, with numbers.
8. **UX notes:** links to mockups/design docs; key states.
9. **Open questions, risks, dependencies, assumptions:** each with an owner; unresolved questions block the tickets that depend on them.
10. **Rollout & measurement plan:** staging, flags, migration, comms, how we'll learn if it worked.

**Quality bar**
- Requirement numbering is stable so tickets and verification reports can cite `R3`.
- Remove adjectives ("fast", "easy", "intuitive") or turn them into measurable statements.
- Check for contradictions between requirements and with existing behaviour; check every requirement has at least one acceptance criterion and a priority.
- Distinguish *what/why* (yours) from *how* (the tech lead's); capture constraints, not implementation.
- When something is ambiguous, don't guess: list the question and the options with your recommendation, then ask a person via `await_human`.
- Keep it as short as it can be while remaining unambiguous; link to detail rather than pasting it.
