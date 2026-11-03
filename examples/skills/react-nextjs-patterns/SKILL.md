---
name: react-nextjs-patterns
description: "Use when writing React or Next.js (App Router) code: components, hooks, state, data fetching, server vs client components, forms, performance and TypeScript typing."
---

# React & Next.js patterns

**Structure**
- Default to **Server Components**; add `"use client"` only for interactivity, browser APIs, or client state, and push it to the smallest leaf. Fetch data on the server where possible and pass serializable props down. Never import server-only code (secrets, DB) into a client module.
- Co-locate: a route's components, hooks and tests live near it; truly shared UI goes in the shared components folder following the existing structure.

**State**
- Keep state minimal and **derive** everything else during render (no `useEffect` to copy props into state). Lift state only as high as needed; use URL search params for shareable state (filters, tabs, pagination). Use a server-state library (TanStack Query/SWR) or framework data APIs for remote data instead of `useEffect` + `fetch` + `useState`. Reach for context/global stores sparingly.
- `useEffect` is for synchronizing with external systems (subscriptions, timers, DOM APIs), with a cleanup function and a complete dependency array. Don't suppress `react-hooks/exhaustive-deps` without a comment explaining why.

**Rendering & performance**
- Stable, unique `key`s (ids, not indexes, for dynamic lists). Don't create components inside components. Measure before using `useMemo`/`useCallback`/`memo`; use them for expensive computations and to stabilize props of memoized children. Virtualize or paginate very long lists. Lazy-load heavy, below-the-fold or rarely used components (`next/dynamic`); use `next/image` and `next/font`; avoid large dependencies for small tasks.

**TypeScript**
- `strict` types, no `any` (use `unknown` and narrow). Use the **generated API types** from the OpenAPI schema; never hand-write request/response shapes. Model UI state as discriminated unions (`{status:"loading"} | {status:"error", error} | {status:"ready", data}`) so impossible states don't compile. Type event handlers and props explicitly; prefer `type Props = {...}` colocated with the component.

**Forms & mutations**
- Controlled inputs for validated forms, or Server Actions/route handlers with progressive enhancement; validate with a schema (e.g. zod) shared between client and server where possible; surface server validation errors per field.

**Quality bar**
- Follow existing lint/format config; run `typecheck`, `lint` and tests. No `console.log` left behind; no dead code; error boundaries around risky subtrees; handle `loading.tsx`/`error.tsx`/`not-found.tsx` for routes. Don't fetch secrets or tokens into the browser.
