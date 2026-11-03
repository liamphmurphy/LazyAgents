---
name: go-error-handling
description: "Use when writing or changing Go code that returns, wraps, checks or logs errors, defines sentinel/typed errors, or maps errors to HTTP responses."
---

# Go error handling

1. **Return errors, don't log-and-return.** Handle an error exactly once: either return it (with context) or deal with it (log, retry, fall back). Logging at every layer produces duplicate noise. Log at the boundary (HTTP handler, worker loop, `main`).
2. **Wrap with context using `%w`:** `fmt.Errorf("load agent %s: %w", id, err)`. Context says what *this* function was doing, not what the callee did. Don't write "failed to" prefixes; they pile up ("failed to X: failed to Y: ...").
3. **Inspect with `errors.Is` / `errors.As`, never string matching or `==` on wrapped errors.**
4. **Sentinel errors** (`var ErrNotFound = errors.New("not found")`) for conditions callers branch on; **typed errors** (a struct implementing `Error()`) when callers need fields (e.g. which field failed validation). Export them only if callers outside the package need them. Prefer translating storage errors (`pgx.ErrNoRows`, `sql.ErrNoRows`) into your domain sentinel at the repository boundary so handlers don't import the driver.
5. **Don't panic for expected failures.** `panic` is for programmer errors and impossible states. Never let a panic cross a goroutine boundary unrecovered in a server.
6. **Never ignore an error silently.** If you truly must (e.g. `defer rows.Close()` on read paths), say why in a short comment. Check `Close()`/`Flush()` errors on writers, and `sc.Err()` after `bufio.Scanner` loops.
7. **Errors in goroutines:** collect with `errgroup.Group` (cancels siblings on first error) rather than a channel of errors you might forget to drain.
8. **At the HTTP boundary** map domain errors to status codes in one place (a helper/middleware): not found → 404, validation → 422 with the field, conflict → 409, unauthenticated → 401, forbidden → 403, everything else → 500 with a generic message. Never leak internal error text, SQL, or file paths to clients; log them with a request id instead.
9. **Test error paths:** assert with `errors.Is`/`errors.As`, and test the wrapped message only when it is part of the contract.

Red flags in review: `if err != nil { return nil }`, `_ = f()` on something that can fail meaningfully, `err.Error() == "..."`, `fmt.Errorf("...: %v", err)` where `%w` was intended, returning a typed nil pointer as `error`.
