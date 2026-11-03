---
name: go-testing
description: "Use when writing, fixing or reviewing Go tests: table-driven tests, test helpers, fakes vs mocks, httptest, database-backed tests, golden files, fuzzing, or flaky tests."
---

# Go testing

- **Table-driven tests with named subtests:** a slice of `{name, in, want, wantErr}` and `t.Run(tc.name, ...)`. Keep the table readable; if cases need heavy setup, split into separate tests.
- **Test behaviour through the public API** of the package; use `package foo_test` for black-box tests, `package foo` only when you must reach internals.
- **Helpers call `t.Helper()`**, take `*testing.T` first, and fail with `t.Fatalf` (not panic). Use `t.Cleanup` for teardown instead of `defer` in helpers; `t.TempDir()` for files; `t.Setenv` for env.
- **Fakes over mocks.** Prefer small hand-written fakes of narrow interfaces (defined where they're *used*, "accept interfaces, return structs"). Avoid asserting call sequences; assert outcomes.
- **Compare with `cmp.Diff`** (`github.com/google/go-cmp`) for structs/slices if it is already a dependency; otherwise `reflect.DeepEqual` with a clear message: `t.Errorf("got %v, want %v", got, want)` ("got" before "want").
- **HTTP:** `httptest.NewRecorder` for handlers, `httptest.NewServer` for clients. Assert status, headers *and* body shape, including error responses.
- **Database tests:** run against a real Postgres (container or the dev DB in a throwaway schema) rather than mocking SQL; each test creates its own data and cleans up, and must be safe to run in parallel or be explicitly serial. Test migrations up (and down if supported) on a fresh DB.
- **Time and randomness:** inject a clock / seed. No `time.Sleep` for synchronization.
- **Parallelism:** `t.Parallel()` only for tests with no shared mutable state; remember loop variable semantics for your Go version.
- **Run `go test -race -count=1 ./...`** before finishing; use `-run` to iterate and `-count=20` to smoke out flakes. A flaky test is a bug in the test or the code; never `t.Skip` it to get green.
- **Cover error paths and edge cases** (empty, nil, max size, unicode, duplicates, cancelled context), not only the happy path.
- **Never let a test exec real external tools or spend real money** (cloud APIs, LLM CLIs). Use fakes and make the test fail fast if a real binary is reachable.
- Add a **fuzz test** (`func FuzzX(f *testing.F)`) for parsers and validators.
