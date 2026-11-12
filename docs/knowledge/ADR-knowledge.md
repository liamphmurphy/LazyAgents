# ADR: Knowledge: one store for Docs and the wiki, audience, identity, properties, suggestions, migration and Obsidian import

- **Status:** Proposed. Implementation tickets start once Liam approves it.
- **Ticket:** [[LAZYAGENTS-174]], the first ticket of the epic "Knowledge: one store for Docs and the wiki, a Zettelkasten, Obsidian import".
- **Plan:** canvas `01a1d1ee-bd4f-7c2d-9ace-42812e262274`, sections 1, 2 and 7 (requirement ids R-A, R-I, R-P, R-V, R-M below are the plan's).
- **Evidence:** `server/internal/wiki/resolve_nested_test.go` (§5), run against `TEST_DATABASE_URL`.
- **Modelled on:** `docs/design/ADR-designs.md` (its §3.7 proposals are one of the inputs to §3.5 here).

## 1. Context

Vesper has two places for long-form text:

| | Docs app | Wiki |
|---|---|---|
| Table | `chat_canvases` (`kind='text'`) + `chat_canvas_versions` (00059) | `artifacts` (`type='note'`, `path`) + `artifact_versions` |
| Owner / audience | One person: the canvas belongs to a chat session, which belongs to its creator. Docs are the canvases in the person's `chat_spaces.app='docs'` space (`server/internal/docs/docs.go`). | Everyone in the workspace, every persona chat and every ticket or epic run. No audience column exists. |
| Persona edits | Applied directly with `canvas_patch`, reviewed afterwards per change against `reviewed_seq` (client-side diff in `web/lib/doc-review`, `doc-review.tsx`); a rejection writes the document back as the person's edit. | Run writes follow `wiki:write`: direct, or a whole-version **suggestion** (`artifact_versions.state='suggested'`, `based_on_version_id`), accepted or rejected as a whole (`wiki.DecideSuggestion`). |
| Comments | `chat_canvas_comments` (anchored, with agent suggestions) | none |
| Links | none | `[[links]]` in `knowledge_links`, backlinks, graph, outline in `artifact_sections`, old paths in `artifact_aliases` |
| Promotion | `PromoteDocToWiki` copies the current text into a wiki page (version note "Promoted from the Doc “…”", a footer line). Nothing links the two afterwards. | |

The epic makes one store, keeps people's private writing out of runs, gives pages Obsidian-style properties, tags, aliases and links, and imports an Obsidian vault. Three facts from the code shape every decision:

1. **No note query knows who is asking.** Every query in the audit (§4) filters by `workspace_id` only. Adding private pages is therefore a change to every reader, not a new feature beside them.
2. **The MCP caller already says what it is.** `agents.Caller.RunID` → `runs.Purpose` (`chat`, research, learning, workflow, or a ticket run) is resolved in `agentapi.define` (`transport.go:43`) and `profileFor` (`transport.go:213`). A chat run's person is its `chat_sessions.created_by`.
3. **A note's title is its last path segment today** (`links.TitleOf`), so a bare `[[Name]]` already finds a nested page (§5). The display title and the path have to be separated before titles can hold any character.

## 2. Options considered

### 2.1 Storage

| Option | Verdict |
|---|---|
| **A. `artifacts` notes are the only substrate for documents; Docs-space text canvases migrate into them** | **Chosen.** Notes already have versions, suggestions, links, backlinks, sections, citations, search, export and run context. Canvases have none of these, and adding them twice is the cost we are removing. |
| B. Canvases become the substrate and the wiki moves onto them | Rejected. Canvases hang off a chat session (`session_id NOT NULL … ON DELETE CASCADE`), so a page would die with its chat; every wiki feature would need porting. |
| C. A new `pages` table both migrate into | Rejected. Same migration work as A plus re-pointing `knowledge_links`, `artifact_reads`, `ticket_artifacts` and citations, for no gain. |
| D. Keep both, sync on save | Rejected. Two histories that disagree, and audience would have to be enforced twice. |

### 2.2 Audience

| Option | Verdict |
|---|---|
| **A. `artifacts.audience` (`private` \| `shared` \| `agent`) + `owner_user_id`, enforced in SQL by one function every note query calls, with a guard test** | **Chosen.** One rule, checkable mechanically (epic criterion 2). |
| B. Filter in Go after the query | Rejected. Counts, ranks, snippets, link-count boosts and `LIMIT`s are computed before Go sees the rows, so they leak and paginate wrongly. |
| C. Postgres row-level security | Rejected for now. The API connects as one role; RLS would need a per-request `SET` on pooled connections, and sqlc tests could not see the rule. Revisit if a second service reads the tables. |
| D. A separate private store per person | Rejected. Breaks links between a person's private and shared pages and duplicates the store we just merged. |

### 2.3 Identity

| Option | Verdict |
|---|---|
| **A. `artifacts.id` is the page id; `title` is free display text; `path` stays the unique, link-safe key derived from it** | **Chosen.** Links are already stored by id (`knowledge_links.target_id`), so renames already keep backlinks. |
| B. Path is the identity (today) | Rejected. A title can't contain `/ [ ] # \| ^`, and two pages can't share a title in one folder. |
| C. Slug-only URLs (`/k/payments-rollback`) | Rejected as the identity; kept as decoration on the URL (§3.3). Slugs collide and change on rename. |

### 2.4 Properties, tags and aliases

| Option | Verdict |
|---|---|
| **A. Properties on the version (`artifact_versions.properties jsonb`), tags and aliases derived on publish into lookup tables** | **Chosen.** Properties are versioned and reviewable with the body, round-trip through frontmatter, and the derived tables make tag pages and alias resolution indexable. |
| B. Leave frontmatter inside `body_md` | Rejected. Every reader would parse YAML; search and the outline would index it; a suggestion couldn't show "tag added" as a change. |
| C. Properties on `artifacts` (unversioned) | Rejected. History and suggestions couldn't carry a property change. |

### 2.5 Suggestions

| | Docs review today | `design_proposals` (ADR-designs §3.7) | Wiki suggestions today |
|---|---|---|---|
| Where | client only: diff between `reviewed_seq` and head | own table, ops + `decisions jsonb` | `artifact_versions.state='suggested'` |
| Granularity | per run of changed blocks | per op, with dependencies | whole version |
| Before review | already applied | not applied | not applied |
| Applied on | n/a (rejecting rewrites the doc) | the current head; missing target → `stale` | replaces the head even if it moved on (UI warns) |
| Result | a person's version reverting rejected blocks | one version by the person, "Accepted n of m" | the suggested version becomes current |

| Option | Verdict |
|---|---|
| **A. One model on `artifact_versions`: a suggested version carries its individual changes (`changes jsonb`), accepted per change on the current head, stale when its context is gone** | **Chosen.** The version row already has state, author, base and decision columns; Designs proves per-change rebase-on-head with stale works. |
| B. A `page_proposals` table like `design_proposals` | Rejected. A second history beside `artifact_versions`; Markdown has no node ids for ops, so the table would hold text hunks anyway. |
| C. Keep whole-version suggestions | Rejected. Doesn't meet R-V2 (per change) and overwrites the person's newer edits. |
| D. Keep the Docs model (apply, then review) everywhere | Rejected as the default: a run's edit to a shared page would be live before anyone read it. Kept as the per-page "direct edit" switch (§3.5). |

### 2.6 Docs migration

| Option | Verdict |
|---|---|
| **A. One SQL migration converts each Docs-space text canvas into a private page with its full version history, authors, comments and chat; a Doc already promoted becomes the promoted page** | **Chosen.** |
| B. Lazy conversion on first open | Rejected. Two stores live for an unbounded time; search and runs would see half the data. |
| C. Export Docs and re-import as pages | Rejected. Loses authors, timestamps and the chat link (R-M2, R-M3). |

### 2.7 Obsidian import

| Option | Verdict |
|---|---|
| **A. An asynchronous import job (uploaded zip in S3, a job row, two passes, a per-file report), idempotent by `vesper_id` / path** | **Chosen.** |
| B. Today's synchronous `POST /wiki/import` (one transaction, 50 MB, frontmatter dropped, first bad file aborts all) | Rejected. A real vault times out, and one odd file would block it ("no manual fixes"). |
| C. A client-side importer calling the page API per file | Rejected. Links can only be resolved after every page exists, which wants the server's transaction and the second pass. |

## 3. Decision

### 3.1 Storage (R-S)

- **Pages are `artifacts` rows with `type='note'`.** "Page" in the UI and the tools means this. Other artifact types (ADR, spec, design, mockup, report) are unchanged and default to `shared`.
- **Docs-space text canvases** (a `chat_canvases` row with `kind='text'` whose `chat_sessions.space_id` is the person's `chat_spaces.app='docs'` space) **migrate to pages** (§3.6) and the Docs app becomes a view of the person's pages.
- **Canvases in ordinary chats stay canvases.** They are a chat's scratch output (text, SVG, slides, designs) and die with the chat. They gain "Save as page", which creates a private page and records `chat_canvases.promoted_artifact_id` (the column `PromoteChatCanvas` already uses).
- **Personas create and edit documents with the page tools** (§3.8), not `canvas_create`/`canvas_patch`. The canvas tools stay for SVG, slides and designs; `canvas_create kind=text` in a chat is refused with "use create_page" once the page tools ship.
- A page can have a chat: `artifacts.chat_session_id uuid NULL REFERENCES chat_sessions ON DELETE SET NULL`. Deleting the chat keeps the page.

### 3.2 Audience (R-A1..R-A7)

```sql
ALTER TABLE artifacts
  ADD COLUMN audience text NOT NULL DEFAULT 'shared' CHECK (audience IN ('private', 'shared', 'agent')),
  ADD COLUMN owner_user_id uuid REFERENCES users(id) ON DELETE CASCADE,
  ADD CONSTRAINT artifacts_private_owner CHECK (audience <> 'private' OR owner_user_id IS NOT NULL);
```

| Audience | Who sees it | Who writes it directly |
|---|---|---|
| `private` | Its owner, and persona chats **of that owner**. Never another person, never a ticket, epic, planning, workflow, research or delegated run. | The owner; the owner's persona chats per the page's edit switch (§3.5). |
| `shared` | Every person in the workspace, every persona chat, every run. | People; runs per `wiki:write` (direct or suggestion, as `wiki.Decide` today). |
| `agent` | Same readers as `shared`. Listed apart in the UI ("Agent memory", "Patterns"). | The owning agent (its `agents/<handle>/` namespace, `wiki.Decide` unchanged) and pattern curators. People can edit; it stays `agent`. |

- **R-A1 Defaults.** A person's new page: `private`. A page a persona creates in a person's chat: `private`, owned by that person. A page a ticket or epic run creates: `shared`, or `agent` under `agents/<handle>/` and `Patterns/`. Existing wiki pages: `agent` under `agents/` and `Patterns/`, else `shared`. Migrated Docs: `private` (§3.6). Imported pages: the importer's choice at upload, default `private`.
- **R-A2 Who may change it.** Only a person: the owner (private ↔ shared) or any person for a shared page (shared → private makes them the owner). `agent` is set by path and can't be changed by hand. No MCP tool changes audience; a persona may *ask* in chat.
- **R-A3 Sharing a page shares only that page**, never what it links to or embeds.
- **R-A4 Links across audiences.** The `[[text]]` in a shared page's body is the writer's text and is delivered as written. For a viewer who can't see the target, every resolver treats it as a **ghost**: `read_page`, `get_backlinks`, previews, embeds (`![[…]]` renders "Unavailable page") and autocomplete answer exactly as for a page that doesn't exist, so a run can't tell "private" from "missing". `knowledge_links` keeps the real `target_id`; hiding is at read time, so sharing the target later lights the link up with no reindex.
- **R-A5 Backlinks, graph and counts only include visible sources.** A private page linking to a shared one is not a backlink anyone but its owner sees, and doesn't raise the shared page's link-count boost in search for others.
- **R-A6 Attaching to a ticket.** Attaching a private page to a ticket (`tracker.AttachArtifact`) or a plan (`ResolveChatRefs` with `uuid.Nil` chat) is refused with "Share the page first", because ticket and planning runs get attachments as context files (`runner.ContextFiles`).
- **R-A7 Paths don't leak.** Uniqueness becomes two partial indexes: shared/agent pages unique on `(workspace_id, lower(path))`; private pages unique on `(workspace_id, owner_user_id, lower(path))`. A person's lookup prefers their own private page, then shared. Sharing a page whose path a shared page already has is refused with "Rename it first".

**Enforcement.** One SQL function and one Go type:

```sql
-- viewer NULL = a run with no person behind it (ticket, epic, planning, workflow, research, delegated).
CREATE FUNCTION artifact_visible(audience text, owner uuid, viewer uuid) RETURNS boolean
  LANGUAGE sql IMMUTABLE AS $$ SELECT audience <> 'private' OR (viewer IS NOT NULL AND owner = viewer) $$;
```

- `wiki.Viewer{UserID *uuid.UUID}` is built once per request: REST → the signed-in person; MCP → `RunID` → run purpose `chat` → `chat_sessions.created_by`; any other run → `nil`. Every note-reading function in §4 takes a `Viewer` and every query passes `viewer` to `artifact_visible(...)` for each artifact it returns **and** each link source/target it counts.
- **Guard test** (`internal/store/queries/audience_test.go`): parses every `-- name:` query in `internal/store/queries/*.sql`; any query that reads `artifacts`, `knowledge_links`, `artifact_sections`, `artifact_aliases`, `artifact_versions` or the new tag/alias tables must call `artifact_visible(` or appear in an allowlist with a reason (writes, locks by id after a visible lookup, admin export). A new unfiltered query fails CI.

### 3.3 Identity (R-I1..R-I3)

- **R-I1 Page id.** `artifacts.id`. The page address is `/knowledge/<id>/<slug>`; the slug is decoration derived from the title and is ignored on lookup. `/wiki/<path>` and `/apps/docs/<canvasId>` redirect to it (§3.6).
- **R-I2 Title vs path.** `title` is free text, 1–200 characters, any characters. `path` stays the link key and folder location; its last segment is derived from the title by replacing `/ [ ] # | ^` with a space and collapsing spaces. `ValidNotePath` (`artifacts/service.go:524`) keeps its checks on the path (no `[ ] # | ^`, ≤300 characters, ≤9 levels, no `.`/`..`). Renaming the title renames the path's last segment (old path → alias, as `Rename` does now); moving folders changes the path only.
- **R-I3 Duplicate titles.** Two pages with the same title in one folder (same audience scope, §3.2 R-A7) get distinct paths by suffix: `Plan`, `Plan 2`, `Plan 3` (Obsidian's convention). The title stays `Plan` for both; `[[Plan]]` resolves to the shortest path (today's rule, §5) and autocomplete inserts the full path when the name is ambiguous.
- **Resolution** (`links.Resolve`) becomes viewer-aware and closes the gaps §5 found: exact path → title/last segment → **trailing partial path** (`[[Ledger/Partition gotchas]]`) → **bare name with `.md`** → frontmatter alias → moved path → **old last segment of a moved page**. Ties: shortest path, then alphabetical, as now.

### 3.4 Properties, tags and aliases (R-P1..R-P3)

- **R-P1 Storage.** `artifact_versions.properties jsonb NOT NULL DEFAULT '{}'`: the frontmatter as a JSON object (values: string, number, bool, list of strings, date as `YYYY-MM-DD` string). The body is stored without frontmatter, as today. A version is a (body, properties) pair; a change to either is a new version, so history and suggestions show property edits.
- **R-P2 Tags and aliases.** On publish (where `artifact_sections` is rebuilt today), derive:
  - `page_tags(artifact_id, tag)`: frontmatter `tags` plus inline `#tags` outside code, lower-cased, nested tags kept as `a/b`.
  - Frontmatter `aliases` into `artifact_aliases` with a new `kind` column: `'moved'` (every existing row: an old path from `Rename`) or `'alias'` (a name the page answers to). Today's table only means "old path"; the column keeps both meanings apart.
- **R-P3 Round trip.** Export writes frontmatter from `properties` plus reserved keys `vesper_id`, `created`, `updated`, `project` (today's `lazyagents_id` is still read on import). Keys are written in a canonical order (reserved first, then the page's keys sorted), values normalised the same way on import, so import → export → import compares equal and produces no new version (epic criterion 5). Unknown YAML (nested maps) is kept verbatim as a string under its key and written back unchanged.

### 3.5 Suggestions (R-V1..R-V4)

```sql
ALTER TABLE artifact_versions ADD COLUMN changes jsonb;
-- [{i, kind: 'body'|'property', before, after, context_before, context_after, key?, why?,
--   state: 'pending'|'accepted'|'rejected'|'stale', reason?, decided_by?, decided_at?}]
```

- **R-V1 One model.** Every proposed edit, from a run (`wiki:write` requires approval or unset), from a persona chat on a page with direct edit off, or from an import onto a page the person changed, is a `state='suggested'` version with `based_on_version_id` and `changes`. `changes` is computed on the server when the suggestion is written: a block diff of base → suggested body (the algorithm in `web/lib/doc-review.ts`, ported to Go and shared with the client through the API) plus one change per property key.
- **R-V2 Per change on the current head.** `POST /pages/{id}/suggestions/{vid}/resolve {decisions}` locks the page, then applies each accepted change to the **current head**: a body change applies where its `before` text (with its context) is found in the head; a property change sets its key. The result is one new published version authored by the person, note "Accepted n of m changes from @handle". The suggestion becomes `published` if every change was accepted, else `rejected` with per-change states.
- **R-V3 Stale.** A change whose `before`/context can't be found in the head (the person edited that passage since) is marked `stale` and skipped; the person's edit always wins. Rejections with reasons and stale changes go back to the author: a system message in the persona's chat, or a ticket comment for a run.
- **R-V4 Direct edit switch.** Per page, `artifacts.persona_direct boolean`, default **on for private pages** (today's Docs behaviour) and **off for shared**. With it on, a persona's edit publishes immediately and stores its `changes` with `review_state='pending'`; the person reviews afterwards and a rejection writes a new version reverting that change (what Docs does now, but recorded per change on the server instead of by the client). Runs never get direct edit through this switch; for them `wiki:write` decides, as now.
- `design_proposals` stays for Designs: its ops are id-keyed JSON, not text. The review UI shares components (change list, pins, Accept/Reject/Accept rest) with Designs.

### 3.6 Migration of Docs (R-M1..R-M6)

One goose migration, in SQL, inside one transaction:

- **R-M1 Which rows.** Every `chat_canvases` row with `kind='text'` whose session's space has `app='docs'`. Text canvases in other chats are not migrated (§3.1).
- **R-M2 Page.** An `artifacts` row: `type='note'`, `audience='private'`, `owner_user_id` = the space's user, `title` = canvas title, `path` = `Docs/<title-derived segment>` with `2`, `3` suffixes per owner (R-I3), `chat_session_id` = the canvas's session, `persona_direct = true`, `created_at` = the canvas's.
- **R-M3 History.** Each `chat_canvas_versions` row becomes an `artifact_versions` row with the same `seq`, `label 'v<seq>'`, `body_md`, `note`, `created_by_type/id` = `author_type/id`, `created_at`, `state='published'`. `current_version_id` = the highest seq. `reviewed_seq` maps to `review_state`: versions after it by an agent are `pending`. Sections, search text and links are rebuilt by a Go backfill run once after the migration (`vesper migrate` hook), because link parsing lives in Go.
- **R-M4 Comments.** `chat_canvas_comments` move to a new `page_comments` table with the same columns, `artifact_id` instead of `canvas_id`; anchors are text ranges already and carry over.
- **R-M5 Docs already promoted to the wiki** become **one** page. A promoted Doc is matched to its wiki page by the version written by `PromoteDocToWiki`: author = the Doc's owner, note `Promoted from the Doc “<title>”`; the most recent match wins. The **wiki page keeps its id, path, audience `shared`, links and backlinks**; the Doc's versions are inserted before the page's versions (seqs renumbered, timestamps kept), the chat and comments attach to the page, and a final version by the system strips the "_Promoted from the Doc …_" footer. If the Doc changed after its promotion, its later text becomes one **suggestion** on the page for the owner to accept per change, never a silent overwrite. Ambiguous matches (two promotions to different pages) migrate as a private page and are listed in the migration report.
- **R-M6 Addresses and cleanup.** `doc_page_redirects(canvas_id PK, artifact_id)` keeps `/apps/docs/<canvasId>` working; `/wiki/<path>` resolves the path (and aliases) to `/knowledge/<id>/<slug>`. The migrated `chat_canvases` rows are deleted in the same migration after the copy (versions and comments cascade). `doc_page_redirects` is exported with the instance (AGENTS.md: new tables need no transfer change). The migration rewrites rows, so it also runs on imported older archives.

`db:migrate` is a gated scope: the migration ticket ships the SQL and a dry-run report query; a person applies it.

### 3.7 Obsidian import job (plan §6)

```sql
CREATE TABLE knowledge_imports (
  id uuid PRIMARY KEY DEFAULT uuid_v7(), workspace_id uuid NOT NULL, user_id uuid NOT NULL,
  audience text NOT NULL CHECK (audience IN ('private', 'shared')), folder text NOT NULL DEFAULT '',
  blob_key text NOT NULL, state text NOT NULL CHECK (state IN ('queued', 'running', 'done', 'failed')),
  totals jsonb NOT NULL DEFAULT '{}', report jsonb NOT NULL DEFAULT '[]',   -- per file: created|updated|unchanged|skipped|failed + reason
  created_at timestamptz NOT NULL DEFAULT now(), started_at timestamptz, finished_at timestamptz);
```

- **Upload.** `POST /knowledge/imports` streams the zip to S3 (limit 500 MB), inserts a `queued` row, returns `202` with the id. Progress and the report come over the existing realtime channel and `GET /knowledge/imports/{id}`.
- **Worker.** A loop in `vesper serve` claims `queued` rows with `FOR UPDATE SKIP LOCKED` (one import per workspace at a time).
- **Pass 1, pages.** For each `.md`: split frontmatter → `properties`; match an existing page by `vesper_id`/`lazyagents_id`, else by path under the chosen folder, else create. Paths a vault allows but `ValidNotePath` refuses (`# ^ [ ] |`, more than 9 levels, more than 300 characters) are mapped deterministically (characters replaced by a space; deeper levels joined into the 9th with ` - `) and the original path is stored as an `alias` so links written against it resolve. Unchanged body and properties → no version. A changed page the person edited since its last import → a suggestion (R-V1), not an overwrite. Each file is its own transaction; a failure is reported and the rest continue.
- **Attachments.** Non-Markdown files become `file` artifacts with the same audience, addressable by vault path for `![[image.png]]` embeds.
- **Pass 2, links.** After every page exists, re-index links of the imported pages so bare names resolve vault-wide with the §3.3 rules (Obsidian resolves by file name anywhere). Block ids (`^abc`) are indexed with sections so `[[Page#^abc]]` resolves.
- The synchronous `POST /wiki/import` and `GET /wiki/export` stay as thin wrappers until the UI moves; export includes only pages the requesting person can see.

### 3.8 Tools

Personas and runs use one set: `search_knowledge`, `read_page`, `get_backlinks`, `write_note` (kept as the name; `create_page`/`edit_page` are added for title, properties and anchored edits and share its service). All take the `Viewer` from §3.2. `canvas_*` tools stop handling text documents (§3.1). Chat tool lists in `agentapi/chat.go` and the exact lists in `httpapi/mcp_profiles_test.go` must be updated together.

## 4. Audit: every query and function that reads notes

Callers are named by who ultimately triggers them: **P** a person over REST/UI, **C** a persona (assistant) chat over MCP, **R** a ticket or epic run (lead or delegated) over MCP or the runner, **PL** a planning run, **S** the system. "Rule" is what §3.2 requires; *viewer* means `artifact_visible(audience, owner_user_id, viewer)` with the viewer of §3.2 (`NULL` for R/PL). Today every row filters by workspace only, unless stated.

### 4.1 SQL queries (`server/internal/store/queries/`)

| Query | Reads | Callers | Rule |
|---|---|---|---|
| `GetNoteByPath` (wiki.sql:3) | artifacts note by path | `links.Resolve` (resolve.go:131); `wiki.RunContext` memory index (memory.go:67) R; `wiki.write` (notes.go:100), `addToIndex` (notes.go:259), `Rename` (notes.go:356) P/C/R writes; patterns (patterns.go:400, 479, 546) R; `httpapi.GetWikiPage` (wiki.go:87) P; `PromoteDocToWiki` (docs.go:137) P; `runs.resolveChatRef` (chatrefs.go:124) C/PL; `artifacts` create (service.go:396) | viewer; with R-A7, prefer the viewer's own private path, then shared. Writes check uniqueness in the writer's scope. |
| `FindNotesByTitle` (wiki.sql:6) | notes by title, shortest path | `links.Resolve` (resolve.go:137); `wiki.Phrases` (backlinks.go:109); `publishedByTitle` (patterns.go:346) R | viewer |
| `FindArtifactByTitle` (wiki.sql:12) | non-note artifact by title | `links.Resolve` (resolve.go:144) | viewer (other types are shared by default, but filter uniformly) |
| `FindNoteByAlias` (wiki.sql:16) | artifact_aliases → artifacts | `links.Resolve` (resolve.go:152); `httpapi.GetWikiPage` (wiki.go:89) P; `wiki.write` (notes.go:104) | viewer on the joined artifact |
| `ListNotes` (wiki.sql:34) | all notes + suggestion counts | `httpapi.ListWikiPages` (wiki.go:39) P; `ListProposed` (patterns.go:425) R (curators) | viewer |
| `LinksFromSource` (wiki.sql:60) | knowledge_links by source | `readPage` outgoing links (tools.go:655) C/R; `RunContext` attached artifacts (memory.go:112) R | source already visible; targets' text is the body's own (R-A4), no extra rule |
| `LinksFromTicket` (wiki.sql:229) | knowledge_links from a ticket and its comments | `RunContext` (memory.go:94) R | targets: viewer (the caller's `note()` lookup must filter) |
| `Backlinks` (wiki.sql:63) | knowledge_links + source artifact/ticket/comment | `wiki.Backlinks` (backlinks.go:39) ← `getBacklinks` (tools.go:793) C/R, `httpapi.GetBacklinks` (wiki.go:255) P, `RunContext` incoming (memory.go:139) R | viewer on the **source** artifact (R-A5); the target was resolved for the viewer |
| `CountBacklinks` (wiki.sql:80) | knowledge_links count | none today (generated only) | viewer on sources, or delete |
| `GraphEdges` (wiki.sql:186) | knowledge_links pairs | `wiki.LocalGraph` (graph.go:39) ← `httpapi.GetGraph` (wiki.go:283) P | viewer on both ends |
| `ArtifactsByIDs` (wiki.sql:204) | artifacts by ids | `wiki.hydrate` (graph.go:102) ← graph P | viewer |
| `MentionCandidatesArtifacts` (wiki.sql:84) | artifact bodies containing a phrase | `wiki.UnlinkedMentions` (backlinks.go:211) ← `httpapi.GetBacklinks` (wiki.go:259) P | viewer |
| `SearchKnowledgeArtifacts` (wiki.sql:98) | full text over artifacts + link-count boost | `wiki.Search`/`SearchFuzzy` (graph.go:162,181) ← `searchKnowledge` (tools.go:595) C/R/PL, `RunContext` search (memory.go:165) R, `httpapi.SearchKnowledge` (wiki.go:299) P, global search `httpapi.Search` (attention.go:119) P | viewer on rows **and** on the boost's link sources (R-A5) |
| `SuggestTargets` (wiki.sql:132) | `[[` autocomplete over notes/artifacts | `wiki.Autocomplete` (graph.go:218) ← `httpapi.SuggestLinks` (wiki.go:241) P | viewer |
| `FindPatterns` (wiki.sql:233) | published `Patterns/` notes + boost | `wiki.FindPatterns` (patterns.go:254) ← `findPattern` (tools.go:693) C/R, `httpapi.FindPatterns`/`BrowsePatterns` (wiki.go:310,320) P | viewer (patterns are `agent`; filter anyway, boost sources too) |
| `RelevantPatterns` (wiki.sql:267) | patterns matching ticket words | `RunContext` (memory.go:78) R | viewer |
| `ListADRDocs` (wiki.sql:293) | ADR artifacts and notes titled "ADR …" | `wiki.ListADRs` (adrs.go:151) ← `httpapi.ListWikiADRs` (wiki.go:336) P | viewer |
| `ExportNotes` (wiki.sql:178) | every note's current body | `wiki.Export` (vault.go:24) ← `httpapi.ExportWiki` (wiki.go:175) P | viewer (a person exports what they can see). Instance export (`transfer`) reads tables directly and is allowlisted: it is the admin backup. |
| `ListSuggestions` (wiki.sql:158) | suggested versions + page | `httpapi.ListWikiSuggestions` (wiki.go:130) P; `httpapi/artifacts.go:179` P; `latestVersion` (notes.go:152), `DecideSuggestion` (notes.go:300) | viewer |
| `CountSuggestions` (wiki.sql:167) | count of suggested versions | `httpapi` bootstrap badge (bootstrap.go:95) P | viewer |
| `LockSuggestion` (wiki.sql:171) | a suggested version by id | `DecideSuggestion` (notes.go:282) P | viewer (a person can't decide on a page they can't see) |
| `NoteLinksTo` (wiki.sql:225) | link existence | `addToIndex` (notes.go:267) R (own memory) | allowlist: bookkeeping inside the agent's own namespace |
| `UnresolvedTargets` (wiki.sql:50) / `ResolveLinks` (wiki.sql:53) | ghost link texts; re-points them | `links.ResolvePending` (index.go:59) S after a page is created | allowlist: write path. Resolution stores the id; visibility applies at read time (R-A4). With R-A7, a private page must only resolve its owner's links and links in its own pages: `ResolveLinks` gets an audience condition on the link's source. |
| `GetArtifact` (artifacts.sql:16) | artifact by id | ~30 callers: `readArtifact` (tools.go:301) C/R, `readPage` (tools.go:618) C/R, `findPattern` (tools.go:701) C/R, `RunContext` `note()` (memory.go:63) R, `RecordContextReads` (memory.go:284) R, `runner.ContextFiles` (prepare.go:313) R, `httpapi` artifacts/wiki (artifacts.go:386,395,580; wiki.go:59,105,357) P, dispatch preview (dispatch.go:176) P, `chatrefs` (chatrefs.go:122) C/PL, `runs/planning.go:123` PL, `tracker` attach/detach (service.go:870,891) P/C/R, backlinks/LookupRef (backlinks.go:100,260,307), `DecideSuggestion`/`write` (notes.go:144,329), mockups (mockups.go:91,210) R, seed | Split: `GetArtifactFor(viewer)` for every reader above; the bare `GetArtifact` stays only for writes after a visible lookup and system code (allowlisted). |
| `LockArtifact` (artifacts.sql:19) | artifact FOR UPDATE | writes | allowlist (write path, after a visible lookup) |
| `GetArtifactVersion` (artifacts.sql:33) | a version's body | `readArtifact`/`readPage` (tools.go:323,625) C/R; `runner` (prepare.go:317,559) R/PL; `chatrefs` (chatrefs.go:143) C/PL; `planning.go:127` PL; `httpapi` (artifacts.go:185,530; wiki.go:142) P; `notes.go:83,150,157`; mockups | viewer (join artifacts) |
| `ListArtifactVersions` (artifacts.sql:37) | version list | `httpapi` history (artifacts.go:200) P | viewer (join) |
| `ListSectionsWithCitations` (artifacts.sql:48) | artifact_sections + reads | `httpapi` artifact detail (artifacts.go:189) P | viewer (join) |
| `GetSectionByAnchor` (artifacts.sql:60) | artifact_sections | `artifacts.RecordRead` (service.go:598); `chatrefs` section refs (chatrefs.go:152) C/PL | parent version already checked; allowlist with reason |
| `ListArtifacts` (artifacts.sql:75) | artifacts list (`type='note'` tab) | `httpapi` Library list (artifacts.go:296) P | viewer |
| `CountArtifactsByType` (artifacts.sql:98) | counts incl. notes | `httpapi` Library tabs (artifacts.go:300) P | viewer |
| `ListTicketArtifacts` (artifacts.sql:114) | artifacts attached to a ticket | `getTicket` (tools.go:227) C/R; `runner.ContextFiles` (prepare.go:296) R; `httpapi` ticket docs (artifacts.go:249) P | viewer; plus R-A6 stops private pages being attached |
| `OverviewArtifacts` (overview.sql:31) | recent artifacts incl. notes | `httpapi` overview (overview.go:177) P | viewer |
| `ChatTurnCitations` (chat.sql:100) | what a chat run read | chat turn citations C (shown to the chat's person) | allowlist: rows are reads the same viewer's chat made |
| attention `suggestion:` branch (attention.sql:63) | suggested/decided note versions | Needs-you feed P | viewer |
| `ListArtifactCitations` / `ListArtifactReaders` / `ListArtifactUnlinkedReaders` (wiki.sql:304,327; artifacts.sql:67) | reads of an artifact | `httpapi.GetWikiCitations` (wiki.go:355) P | viewer on the artifact (checked by the handler's lookup) |

### 4.2 Go functions

| Function | Reads (queries) | Callers | Rule |
|---|---|---|---|
| `wiki.RunContext` (memory.go:48) | GetNoteByPath, RelevantPatterns, GetArtifact, TicketArtifactIDs, LinksFromTicket, LinksFromSource, Backlinks, Search | `claim` (agentapi/tools.go:165) R (unmanaged claim); `runner.ContextFiles` (prepare.go:327) R, and its dispatch preview (httpapi/dispatch.go:170) P | viewer `NULL` always: only `shared`/`agent` pages. Its `note()` helper must use the viewer lookup. |
| `wiki.RecordContextReads` (memory.go:281) | GetArtifact | `claim` (tools.go:168) R | pages already filtered; no extra rule |
| `wiki.Backlinks` (backlinks.go:38) | Backlinks | `getBacklinks` C/R, `httpapi.GetBacklinks` P, `RunContext` R | viewer |
| `wiki.Phrases` / `wiki.UnlinkedMentions` (backlinks.go:97,200) | GetArtifact, FindNotesByTitle, MentionCandidatesArtifacts | `httpapi.GetBacklinks` P; `LinkMention` (backlinks.go:254) P | viewer |
| `wiki.LookupRef` (backlinks.go:304) | `links.Resolve`, GetArtifact | `readPage`, `getBacklinks` (tools.go:608,789) C/R | viewer; "nothing called X" for invisible targets (R-A4) |
| `links.Resolve` (resolve.go:98) | GetADRByNumber, GetTicketByKey, GetNoteByPath, FindNotesByTitle, FindArtifactByTitle, FindEpicByName, FindNoteByAlias | `LookupRef` C/R; `httpapi.ResolveLinks` (wiki.go:230) P (previews and hover cards); `links.Index` on save S | viewer for reads. **Indexing on save** resolves for the page's writer scope: a shared page's links resolve against shared pages plus, if the writer is a person, their private pages (stored id; hidden at read time per R-A4). |
| `wiki.LocalGraph` / `hydrate` (graph.go:32,95) | GraphEdges, ArtifactsByIDs, tickets, epics, agents | `httpapi.GetGraph` P | viewer |
| `wiki.Search` / `SearchFuzzy` (graph.go:152,158) | SearchKnowledgeArtifacts, SearchKnowledgeTickets | `searchKnowledge` C/R/PL; `RunContext` R; `httpapi.SearchKnowledge` P; global search P | viewer |
| `wiki.Autocomplete` (graph.go:217) | SuggestTargets | `httpapi.SuggestLinks` P (editor), later the page tools C | viewer |
| `wiki.FindPatterns` / `BrowsePatterns` (patterns.go:238,285) | FindPatterns | `findPattern` C/R; `httpapi` P | viewer |
| `wiki.ListProposed` / `loadProposal` / `publishedByTitle` (patterns.go:423,474,345) | ListNotes, GetNoteByPath, FindNotesByTitle | `listProposedPatterns`, `publishPattern`, `rejectPattern`, `proposePattern` R (curators) | viewer `NULL` (patterns are `agent`) |
| `wiki.ListADRs` (adrs.go:150) | ListADRDocs | `httpapi.ListWikiADRs` P | viewer |
| `wiki.Export` (vault.go:23) | ExportNotes | `httpapi.ExportWiki` P | viewer |
| `wiki.Service.write` / `latestVersion` / `DecideSuggestion` / `Rename` (notes.go) | GetNoteByPath, FindNoteByAlias, LockArtifact, ListSuggestions, LockSuggestion | `SaveAsUser` P, `AgentWrite` (`write_note`) C/R, `Import` P, `PromoteDocToWiki` P, patterns R | the lookup is the writer's viewer; a run can't write over (or learn of) a private page at that path: it gets a new shared page or "already exists" only for visible pages (R-A7 makes the paths independent) |
| `agentapi.readPage` (tools.go:606) | LookupRef, GetArtifact, GetArtifactVersion, LinksFromSource | C/R/PL | viewer from the run |
| `agentapi.readArtifact` (tools.go:296) | GetArtifact, GetADRByNumber, GetArtifactVersion | C/R | viewer (accepts a note's id today) |
| `agentapi.searchKnowledge` / `getBacklinks` / `findPattern` / `writeNote` (tools.go:587,787,691,675) | as above | C/R (`planningTools` and `subagentTools` include the readers) | viewer from the run |
| `runs.ResolveChatRefs` (chatrefs.go:54) | GetArtifact, GetNoteByPath, GetArtifactVersion, GetSectionByAnchor | chat turns `ChatTurnContext` (prepare.go:624) C; `runs/chat.go:493` C; planning `PlanningRefFiles` (prepare.go:583), `runs/planning.go:140` PL | chat: the chat's person; planning (`chat = uuid.Nil`): `NULL` → R-A6 refuses private refs when the plan starts |
| `runner.ContextFiles` (prepare.go) | ListTicketArtifacts, GetArtifact, GetArtifactVersion, RunContext | runner R; dispatch preview P | viewer `NULL` |
| `runner.PlanningContextFile` (prepare.go:551) | GetArtifactVersion of the plan source | PL | viewer `NULL`; starting a plan from a private page is refused (R-A6) |
| `tracker.AttachArtifact` (tracker/service.go:850) | GetArtifact | REST P; `create_artifact`/ticket tools C/R | R-A6: refuse private pages |

## 5. How a bare `[[Note]]` resolves today (measured)

`server/internal/wiki/resolve_nested_test.go` (`TestBareLinkToNestedPage`) ran against `TEST_DATABASE_URL` on 2026-11-12 and passes: `go test ./internal/wiki -run TestBareLinkToNestedPage` → `ok`. It shows:

| Link | Page(s) | Resolves to | vs Obsidian |
|---|---|---|---|
| `[[Partition gotchas]]` | `Payments/Ledger/Partition gotchas` | that page (title = last segment, `FindNotesByTitle`) | same |
| `[[partition GOTCHAS]]` | same | that page (case-insensitive) | same |
| `[[Ledger/Partition gotchas]]` | same | **ghost** | **gap**: Obsidian accepts a trailing partial path |
| `[[Payments/Ledger/Partition gotchas.md]]` | same | that page (`.md` stripped from paths) | same |
| `[[Partition gotchas.md]]` | same | **ghost** (the title lookup uses the raw text) | **gap** |
| `[[Partition gotchas]]` | also `Ops/…`, `Archive/…`, `Abc/…` | shortest path in characters, then alphabetical (`Abc/` before `Ops/`) | differs: Obsidian keeps the writer's choice by writing a qualified link when ambiguous |
| `[[Ledger notes]]` written before the page exists | later `Payments/Deep/Ledger notes`, then a shorter `Ledger notes` | the ghost resolves to the first page created and **stays** there (links are stored by id); a new link picks the shorter page | Obsidian re-resolves on every read |
| `[[Ops/Partition gotchas]]` after a rename to `Ops/Hot partitions` | | the renamed page (old path is an alias) | same |
| `[[Failover drill]]` after `Runbooks/Deep/Failover drill` was renamed | | **ghost**: the old full path is an alias, the old bare name isn't | **gap** (Obsidian rewrites links on rename instead) |
| `[[PAY-214]]` with a page `Glossary/PAY-214` | | the ticket (tickets and ADR ids resolve first) | n/a |

The import ticket closes the three gaps in `links.Resolve` (§3.3) and resolves imported links in pass 2 (§3.7) so the first-created-wins rule can't pick the wrong page during an import.

## 6. Decisions Liam made on 2026-11-12

> **To fill in before approval.** The epic's description lists these, but this run could not read the epic or the plan canvas (`get_epic` and `list_epics` need a chat; the canvas is not an artifact). Liam's reply on [[LAZYAGENTS-174]] is copied here verbatim, and any section above that disagrees is changed to match before the ticket closes.

## 7. Consequences

- Every note reader gains a `Viewer` parameter; the guard test makes forgetting one a CI failure. This is the bulk of the audience ticket and touches every row of §4.
- `title` stops being derived from the path: code that relies on `TitleOf(path) == title` (`FindNotesByTitle`, `Phrases`, autocomplete) moves to "title or last segment".
- Docs-space canvases, their versions and comments leave `chat_canvases`; the Docs app, the canvas tools and `PromoteDocToWiki` change in the same epic.
- `artifact_aliases` gains a `kind`; every existing row is `moved`.
- The synchronous wiki import stays only as a wrapper; vault imports are jobs.
- The Docs per-change review moves from the client to the server (`changes jsonb`), so runs, chats and imports share it.

## 8. Ticket order this ADR implies

1. Audience column, `artifact_visible`, `Viewer` through §4, the guard test (blocks everything that exposes pages to runs).
2. Identity: free titles, path derivation, duplicate suffixes, `/knowledge/<id>` and redirects; resolver gaps (§5).
3. Properties, tags, aliases (`kind`), frontmatter round trip.
4. Suggestions with per-change `changes` and the review API (Docs review UI moves onto it).
5. Docs migration (needs 1–4; `db:migrate` by a person).
6. Page tools for personas and runs; canvas tools stop handling text.
7. Obsidian import job and fixture vault (needs 2, 3, 5).
