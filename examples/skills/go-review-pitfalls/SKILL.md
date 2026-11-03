---
name: go-review-pitfalls
description: "Use when reviewing Go code to spot common bugs: nil/typed-nil errors, goroutine and context leaks, ignored errors, slice aliasing, defer in loops, time handling and database misuse."
---

# Go review pitfalls

- **Errors:** ignored (`_ =`) or overwritten errors; `%v` where `%w` was meant; string comparison of errors; returning a nil-valued concrete pointer as `error` (non-nil interface); `if err != nil { return nil }`; `sc.Err()` unchecked after `bufio.Scanner`; unchecked `Close()` on writers.
- **Concurrency:** goroutine with no exit path or no `ctx`; unbuffered channel send with no receiver; closing a channel from the receiver or twice; map access from several goroutines; lock held across I/O; `WaitGroup.Add` inside the goroutine; missing `defer cancel()`; `time.After` in loops; data races in tests (run `-race`).
- **`defer` in a loop** holds resources until the function returns; `defer` evaluates arguments immediately; deferred `Close` on a file being written loses the error.
- **Slices & maps:** appending to a sub-slice clobbers the original's backing array; retaining a slice of a huge buffer; modifying a slice while ranging; writing to a nil map panics; range copies values (mutating the copy has no effect).
- **Loop variable capture** in closures/goroutines (depends on `go` version in `go.mod`).
- **HTTP:** response body not closed (`defer resp.Body.Close()`) / not drained; no client timeout; handler writes after error without `return`; reading unbounded bodies; setting headers after `WriteHeader`; missing `Content-Type`.
- **Database:** `rows.Close()`/`rows.Err()` missing; transaction without rollback on error path; queries inside a tx using the pool instead of the tx; read-then-write races; unbounded `SELECT`; N+1 loops; string-built SQL.
- **Time:** comparing `time.Time` with `==` (use `Equal`); using local time instead of UTC for storage; `time.Duration` unit mistakes (`time.Sleep(5)` = 5ns); monotonic vs wall clock.
- **Context:** `context.Background()` deep in a request path (drops cancellation); storing contexts in structs; ignoring `ctx.Err()` after a select.
- **API design:** exported identifiers without docs; interfaces defined at the producer with many methods; `init()` side effects; global mutable state; leaking internal types in public API; naked `interface{}` where generics or a type fit.
- **Tests:** `t.Fatal` from a goroutine; shared state between parallel subtests; sleeps; assertions that always pass.
