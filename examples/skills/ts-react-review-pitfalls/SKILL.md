---
name: ts-react-review-pitfalls
description: "Use when reviewing TypeScript, React or Next.js changes to catch stale closures, effect bugs, key and state mistakes, accessibility regressions, type holes and client/server boundary leaks."
---

# TypeScript / React review pitfalls

- **Effects:** `useEffect` used to derive state (should be computed in render); missing or wrongly suppressed dependencies; no cleanup for subscriptions/timers/fetch (use `AbortController`); effects that fire twice in Strict Mode and break; fetch-in-effect without race handling.
- **State:** state that duplicates props or other state; mutating state/props in place; setState from stale closures (use the functional form); state stored where the URL should be; global state for local concerns.
- **Lists & keys:** array index as key on reorderable lists; non-unique keys; components defined inside components (remount every render).
- **Types:** `any`, `as` casts and non-null `!` hiding real nullability; hand-written API types instead of generated ones; unchecked `JSON.parse`/`fetch` results; optional chaining that silently swallows missing data that should be an error; unions without exhaustive checks.
- **Async UI:** no loading/error/empty state; unhandled promise rejections; double submit; optimistic updates with no rollback; stale data after mutation (missing invalidation).
- **Server/client boundary (Next.js):** `"use client"` on a large tree; server-only imports or secrets reaching client bundles; non-serializable props across the boundary; data fetched on the client that could be fetched on the server.
- **Accessibility regressions:** clickable `div`s, icon buttons without names, removed focus outlines, missing labels, color-only meaning, focus not managed in dialogs, no keyboard path.
- **Security:** `dangerouslySetInnerHTML` / unsanitized markdown or URLs (`javascript:`), tokens in `localStorage`, secrets in `NEXT_PUBLIC_*`, trusting client-side checks for authorization.
- **Performance:** new large dependency for a trivial task; expensive work in render; unvirtualized huge lists; images without dimensions causing layout shift; premature `memo` clutter.
- **Hygiene:** leftover `console.log`, commented-out code, unused exports, copy-pasted components instead of reuse, hard-coded colors/spacing instead of tokens, missing tests for new behaviour.
