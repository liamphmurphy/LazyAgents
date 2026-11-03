---
name: accessibility-checklist
description: "Use before finishing any UI change, and when building interactive components (menus, dialogs, tabs, comboboxes, tables, custom controls), to meet WCAG 2.2 AA."
---

# Accessibility checklist (WCAG 2.2 AA)

**Semantics first**
- Use native elements: `<button>` for actions, `<a href>` for navigation, `<input>/<select>/<textarea>`, `<table>` with `<th scope>`, lists for lists, one `<h1>` and a logical heading order, landmarks (`header`, `nav`, `main`, `aside`). A clickable `<div>` is a bug.
- Reach for ARIA only when no native element fits, and then implement the full pattern (roles, states, keyboard) from the WAI-ARIA Authoring Practices. Prefer a vetted headless library (e.g. Radix) for menus, dialogs, tabs, popovers and comboboxes over hand-rolling.

**Names and labels**
- Every control has an accessible name: visible `<label>`, `aria-label`/`aria-labelledby` for icon-only buttons ("Remove scope", not "X"). Images: meaningful `alt`, or `alt=""` if decorative. Don't repeat visible text in `aria-label` with different wording.

**Keyboard**
- All functionality works with keyboard alone; no keyboard traps; visible focus indicator (≥3:1 contrast, never `outline: none` without a replacement); focus moves into opened dialogs and returns on close; skip link on pages with heavy navigation; `tabindex` only `0`/`-1`.

**Color and visuals**
- Text contrast ≥4.5:1 (≥3:1 for large text and for UI component boundaries/icons conveying info); information never conveyed by color alone; works at 200% zoom and 320px width without two-dimensional scrolling; respects `prefers-reduced-motion` and doesn't flash.

**Dynamic content**
- Announce async changes: `role="status"`/`aria-live="polite"` for toasts and results counts, `role="alert"` for errors. Mark current page/tab/selection (`aria-current`, `aria-selected`, `aria-expanded`, `aria-pressed`). Error messages are associated with inputs (`aria-describedby`, `aria-invalid`).

**Targets and timing**
- Touch targets ≥24×24 CSS px (aim for 40+); no hover-only information; don't time out without warning.

**Verify**: tab through the screen; run an automated check (axe / eslint-plugin-jsx-a11y / Playwright + axe) and fix findings; spot-check with a screen reader (VoiceOver) for new custom widgets. Note what you tested in the PR.
