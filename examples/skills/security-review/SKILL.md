---
name: security-review
description: "Use when reviewing code that handles authentication, authorization, user input, secrets, file or network access, shell commands, templating/HTML output, or third-party dependencies."
---

# Security review

Prioritize real, exploitable issues; give a concrete attack scenario for each.

- **Authorization (most common miss):** for every handler/tool, ask "who is the caller and may they do this to *this* object?" Resource ids from the client must be re-checked against the caller's tenant/owner (IDOR). Prefer scoping queries by tenant over checking after fetching. Check that background jobs and agent tools can't be coerced into acting on another tenant's data.
- **Authentication & sessions:** tokens/cookies are `HttpOnly`/`Secure`/`SameSite` as appropriate; tokens are random, compared in constant time, revocable, short-lived, and never logged; passwords hashed with a modern KDF (argon2id/bcrypt/scrypt).
- **Injection:** SQL (parameterized queries only), shell (never interpolate input into `sh -c`; pass argv), HTML/JS (escape by default; scrutinize `dangerouslySetInnerHTML`/`innerHTML`/markdown rendering and sanitize), path traversal (`filepath.Clean` + prefix check against a base dir, no user-supplied absolute paths), template, header and log injection.
- **SSRF & outbound requests:** user-controlled URLs need allow-lists, DNS-rebinding-safe checks, no access to metadata/internal addresses, timeouts and size limits.
- **Secrets:** none in code, tests, fixtures, logs, error messages or client bundles; least-privilege credentials; brokered/approved access for dangerous actions is not bypassed.
- **Input handling:** size limits, type/enum validation, safe parsing of untrusted files/archives (zip-slip), regex DoS, unbounded loops or memory.
- **Prompt injection in agent systems:** content from tickets, web pages, repos or tool results is untrusted data. Check that it can't expand an agent's scopes, exfiltrate secrets, or trigger privileged tools without approval.
- **Dependencies:** new packages are justified, maintained, pinned, and from the expected source; lockfiles updated; no install scripts surprises.
- **Crypto:** use standard libraries and AEAD modes; never custom crypto or `math/rand` for secrets.
- **Errors & logging:** no sensitive data in logs/responses; generic client errors with detailed server logs.

Rate each finding Critical/High/Medium/Low with a fix. Unfixed Critical/High findings are blocking.
