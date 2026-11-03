---
name: go-http-api
description: "Use when adding or changing HTTP handlers, routes, middleware, request validation, pagination or error responses in a Go service (net/http, chi, generated OpenAPI servers)."
---

# Go HTTP API

1. **Contract first.** If the repo has an OpenAPI spec (`openapi.yaml`), change it first, regenerate server/client code (`go generate`/`make gen`), then implement the generated interface. Never hand-edit generated files; commit regenerated output.
2. **Handlers stay thin:** decode → validate → call a service/domain function → encode. Business rules live in services that don't know about `http.Request`, so they can be unit tested and reused by other transports.
3. **Validate at the boundary:** required fields, enums, lengths, ID formats, sizes. Cap request bodies with `http.MaxBytesReader`. Reject unknown fields when strictness matters (`dec.DisallowUnknownFields()`).
4. **Consistent errors:** one error shape (code, message, optional field), one place that maps domain errors to statuses (see go-error-handling). Use 201 + resource for creates, 204 for empty success, 409 for conflicts, 422 for semantic validation errors, 404 for not found *and* for resources the caller may not know exist.
5. **Authn/authz:** derive the caller from the request context (set by middleware), and check authorization against the *resource's* workspace/owner on every handler, not only at the route level. Scope every query by tenant/workspace id so IDOR is impossible by construction.
6. **Context:** pass `r.Context()` into everything downstream so cancelled requests stop work. Set server `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout`; never use `http.DefaultClient` without a timeout for outbound calls.
7. **Pagination:** cursor-based (opaque cursor from `(sort_key, id)`) for large or mutable lists, with a max `limit`; offset only for small, stable sets. Always return a deterministic order.
8. **Idempotency & safety:** GET never mutates; make PUT/DELETE idempotent; for POSTs that can be retried (webhooks, agent tool calls) accept an idempotency key or dedupe on a natural key.
9. **Compatibility:** additive changes only (new optional fields, new endpoints). Removing/renaming a field or changing a type is breaking: version it or coordinate with the frontend ticket.
10. **Observability:** a request-id middleware, structured logs (`log/slog`) with method/path/status/duration, no secrets or PII in logs.
11. **Tests:** `httptest` through the real router including middleware; cover 401/403/404/422 paths, not only 200.
