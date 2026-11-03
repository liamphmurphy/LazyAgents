---
name: go-concurrency
description: "Use when adding goroutines, channels, mutexes, worker pools, background loops, timeouts or cancellation to Go code, or when debugging races, leaks and deadlocks."
---

# Go concurrency

- **Don't start a goroutine without knowing how it ends.** Every goroutine needs an owner, a way to be told to stop (`context.Context` or a closed channel), and a way for the owner to wait for it (`sync.WaitGroup` or `errgroup`). "Fire and forget" in a server is a leak.
- **Pass `context.Context` as the first parameter** of anything that does I/O or may block; never store it in a struct. Derive with `context.WithTimeout`/`WithCancel` and **always `defer cancel()`**. Respect `ctx.Done()` in loops and selects.
- **Prefer `errgroup.WithContext`** for fan-out/fan-in: the first error cancels the rest. Bound parallelism with `g.SetLimit(n)`.
- **Channels:** the sender closes, never the receiver; never close from multiple senders. Use buffered channels deliberately (to decouple a known burst), not to "fix" a deadlock. `select` with a `ctx.Done()` case on every blocking send/receive.
- **Mutexes:** keep the critical section tiny, don't hold a lock across I/O or a callback, document which fields a mutex guards, and put the mutex next to them. Use `sync.RWMutex` only when reads dominate and you've measured. Never copy a struct containing a mutex (`go vet` catches this).
- **Loop variable capture:** since Go 1.22 each iteration has its own variable, but check `go.mod`'s `go` version before relying on that.
- **Shared maps need protection**: concurrent map writes panic. Use a mutex or `sync.Map` (only for its narrow use cases: write-once/read-many or disjoint keys).
- **Timers/tickers:** `defer ticker.Stop()`; prefer `time.NewTimer` + `Reset` over `time.After` inside hot loops (leaks until fired in older Go).
- **Graceful shutdown:** `signal.NotifyContext`, stop accepting work, `server.Shutdown(ctx)` with a deadline, then wait for workers.
- **Test with `go test -race ./...`** and make tests deterministic: inject clocks and channels, never `time.Sleep` to "wait for" a goroutine; use sync primitives or `Eventually`-style polling with a hard timeout.
- When in doubt, don't add concurrency: a sequential version that is correct beats a fast one that races.
