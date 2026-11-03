---
name: go-code-style
description: "Use when writing or refactoring Go code to keep it idiomatic: package layout, naming, interfaces, struct design, generics, logging, dependencies and formatting."
---

# Idiomatic Go

- **Follow the repo first**, then Effective Go / Go Code Review Comments. `gofmt` (or `goimports`) clean and `go vet` clean are table stakes; run the project's linter config if present (`golangci-lint run`).
- **Packages:** short, lowercase, singular, no `util`/`common`/`helpers`. Name by what it provides. Keep `internal/` for non-public code. Avoid import cycles by depending "inward" (handlers → services → store), never the reverse.
- **Naming:** no stutter (`agents.Service`, not `agents.AgentsService`); initialisms are capitalized (`ID`, `URL`, `HTTP`); receivers are 1–2 letters and consistent; getters omit `Get`.
- **Interfaces:** small (1–3 methods), defined by the *consumer*, only when there are ≥2 implementations or a test seam is needed. Return concrete types. Don't export an interface just to mock it.
- **Structs & zero values:** make the zero value useful; use functional options or a config struct for constructors with many parameters. Constructors validate and return `(T, error)` rather than a half-built value.
- **Control flow:** early returns, minimal nesting, the happy path unindented. `switch` over long `if/else` chains. Avoid naked returns except in tiny functions.
- **Slices/maps:** preallocate when size is known (`make([]T, 0, n)`); remember slices alias their backing arrays (copy before mutating or retaining). Use the `slices`/`maps` packages (Go ≥1.21) instead of hand-written loops.
- **Generics:** use for genuinely type-agnostic containers/helpers, not to avoid writing two small functions.
- **Logging:** `log/slog` with structured key/values; no `fmt.Println` in library code; don't log and return the same error.
- **Comments:** every exported identifier has a doc comment starting with its name; comments explain *why*, not what.
- **Dependencies:** prefer the standard library; adding a module needs a reason. Run `go mod tidy`. Pin tool versions via `go.mod` `tool` directives when available.
- **Configuration & secrets** come from environment/config structs validated at startup; never hard-code or log secrets.
- **Don't refactor unrelated code** in a feature PR; mention follow-ups in the PR description or a new ticket.
