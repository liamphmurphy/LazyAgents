---
name: api-contract-first
description: "Use when a change adds or alters an API (endpoint, request/response field, event, tool schema) that another component, the frontend or an external client depends on."
---

# API contract first

1. **Write the contract before the code.** Update the OpenAPI/schema file (or protobuf/GraphQL schema) with: path, method, operation id, request and response schemas, all error responses, required vs optional fields, enums, formats (`uuid`, `date-time`), and a one-line description of semantics.
2. **Make it additive.** New optional fields and new endpoints are safe. Renaming, removing, tightening a type or making an optional field required is breaking: add a new field/version, support both for a release, then remove.
3. **Regenerate** server stubs and client types, commit the generated output, and make sure CI's generated-code check passes.
4. **Document behaviour that the schema can't express**: ordering, pagination, idempotency, rate limits, who is authorized, what 404 vs 403 means, side effects.
5. **Give the consumer something to build against.** Post the contract on the ticket (or link the schema diff) so `@frontend-engineer` or other agents can depend on it; mark the backend ticket as a blocker of dependent UI tickets.
6. **Implement, then verify against the contract:** tests that exercise the real router and assert the response shape for success *and* error cases. Examples in the spec should be real, copy-pasteable responses.
7. **PR description lists** the contract diff, whether it is breaking, and migration/rollout notes.
