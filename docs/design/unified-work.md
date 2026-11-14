# Unified Work

Source: **Unified Work: activity, tickets and epics**, canvas
`01a1db46-2a21-76d8-b0ad-b7d76ebf7ac2`, version 2, approved for implementation
in the chat. The five desktop frames are 1440 × 900; the two compact frames
are 390 × 844.

The implementation uses the existing theme tokens (the source uses Gruvbox),
a 248px shell navigator, a 42px top row, and a 352px context/run companion.

| Design frame | Work state |
| --- | --- |
| 01 Activity, selected run | `/work?run=<id>` |
| 02 Ticket beside run | `&ticket=<key>&context=ticket` |
| 03 Epic replaces ticket | `&epic=<id>&context=epic` |
| 04 Focused ticket with live run | `&view=tickets&ticket=<key>` |
| 05 Focused epic with live run | `&view=epics&epic=<id>` |
| 06 Compact run with draft | Same Activity URL below 768px |
| 07 Compact ticket and return | Same ticket context URL below 768px |

One mounted run thread moves between the primary area and the companion using
CSS ordering. Compact context and the browse list hide the thread without
unmounting it. The composer and scroll container consequently keep their state.
Switching attempts mounts the newly selected run. Drafts are not persisted across
a full page reload or switching attempts.

Selection, view and filters use native history, supported by Next's search-param
integration. Updates read the live URL at the event boundary to preserve preceding
navigation that React has not rendered yet. Back/Forward restore the same run,
context and filters. Modified link clicks retain normal browser behavior.

Legacy Runs, Tickets and Epics URLs enter Work. Advanced run log/graph/files URLs
remain available. Explicit `?full=1` ticket and epic detail links retain the
existing activity, criteria management and other detailed tools. No API or database
schema changes are needed.

`e2e/unified-work.spec.ts` exercises the desktop and compact journeys and captures
screenshots for all seven frames, plus saved filters, previous attempts, errors,
and deep links. It uses the existing REST fixtures; it is not a real-backend test.
