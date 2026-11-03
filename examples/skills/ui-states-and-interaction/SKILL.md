---
name: ui-states-and-interaction
description: "Use when implementing forms, data fetching views, lists, dialogs, async actions or any interactive UI, to cover loading, empty, error, success and edge states and good interaction behaviour."
---

# UI states and interaction

Every data-driven view needs **all** of these. Check each before calling it done:

- **Loading:** skeletons shaped like the final content (no layout shift) for first load; keep stale data visible with a subtle indicator on refetch. Avoid spinners that flash for <200ms.
- **Empty:** says why it's empty and offers the next action ("No tickets yet. Create one"). Distinguish "nothing exists" from "no results match your filter" (with a clear-filters action).
- **Error:** inline, near the cause, with a retry; the message says what failed in user terms. Keep user input on failure. Never show raw error objects or stack traces. Handle 401 (re-auth), 403, 404 (resource gone) and network offline separately where it matters.
- **Partial/long content:** very long names, many items (pagination or virtualization), zero/one/many pluralization, unusual characters/RTL/emoji, missing optional fields.
- **Success feedback:** toast or inline confirmation for non-obvious actions; offer **Undo** for destructive-but-reversible actions instead of a confirm dialog when feasible.

**Forms**
- Visible `<label>` for every control, linked via `htmlFor`/`id`; helper text for constraints before the user errs; validate on blur/submit, show errors next to the field and in an error summary for long forms, and move focus to the first invalid field.
- Disable submit only while submitting (and show progress), not as a hidden validation gate; prevent double-submit. Preserve drafts on accidental navigation for long inputs. Use the right input `type`/`inputmode`/`autocomplete`.

**Async actions**
- Optimistic update for cheap, likely-to-succeed actions with rollback on failure; otherwise show pending state on the control itself. Make mutations idempotent-safe on retry. Invalidate/refresh dependent queries after a mutation so no view shows stale data.

**Keyboard & focus**
- Everything operable by keyboard in a logical tab order; visible focus ring; `Esc` closes dialogs/menus and returns focus to the trigger; dialogs trap focus and restore it; menus support arrow keys. Offer shortcuts only for frequent actions and don't override browser ones.

**Real-time/streamed data**: show connection state, don't reorder what the user is interacting with, and be tolerant of out-of-order or duplicate events.
