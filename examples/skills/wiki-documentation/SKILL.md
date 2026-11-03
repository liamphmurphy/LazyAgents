---
name: wiki-documentation
description: "Use when creating or updating wiki pages: feature docs, status logs, release notes, decision logs, glossary or how-tos; to keep documentation accurate, linked and non-duplicated."
---

# Wiki documentation

**Before writing:** `search_knowledge` for existing pages and update them instead of creating near-duplicates. Read the source of truth (PRD, tickets, PRs, ADRs, code) rather than relying on memory or on other docs.

**Page types and templates**
- **Feature doc:** what it is and who it's for → how to use it (steps, screenshots/examples) → how it works at a high level → limits, permissions, known issues → links to the PRD, epic, tickets, ADRs.
- **Status log / epic status:** dated entries, newest first: what shipped, what's in progress, blockers, decisions, next steps; link every ticket as `[[PAY-123]]`. Keep a current "Status: On track / At risk / Blocked + why" line at the top.
- **Release notes:** user-facing language, grouped (New, Improved, Fixed, Changed/Breaking), each with the benefit and a link; call out migrations or required actions first.
- **Decision log:** date, decision, owner, rationale, link to the ADR.
- **Glossary:** one canonical definition per term; use those words everywhere (PRDs, tickets, UI copy).

**Conventions**
- Descriptive titles; one topic per page; short paragraphs, headings, lists and tables for comparison; examples over abstractions; sentence case.
- Link generously with `[[Page]]` and `[[Page#Section]]` so backlinks make work discoverable; link tickets and documents rather than copying their content.
- Date-stamp time-sensitive claims ("as of 2026-11-02"); mark outdated sections and fix or remove them rather than leaving contradictions.
- Don't document secrets or credentials. Don't state things you haven't verified: say "unverified" and what's needed.
- Your own memory notes live under `agents/product-manager/`; shared documentation goes where the wiki structure puts it. If you lack `wiki:write` for a page, your edit becomes a suggestion: say so in your comment.
- After writing, reread for accuracy against the sources and check every link resolves.
