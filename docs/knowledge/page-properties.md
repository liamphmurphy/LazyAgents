# Page identity and frontmatter contract

Implemented by LAZYAGENTS-176, following [ADR §3.3–3.4](ADR-knowledge.md).

`POST /api/v1/wiki/pages` accepts the existing `path` and `body_md` fields plus
optional `title`, `properties`, `tags`, and `aliases`. When `path` is omitted,
`title` is required and a unique link-safe path is derived, with numeric suffixes
for collisions. Display titles are 1–200 characters, may repeat, and may contain
link punctuation. An explicit path still follows `ValidNotePath` and conflicts return 422.

```json
{
  "title": "Release [plan] #1",
  "body_md": "Follow up #work/releases",
  "properties": {
    "status": "draft",
    "estimate": 2.5,
    "due": "2026-11-13",
    "ready": false,
    "reviewers": ["Liam"],
    "source": "[[Release source]]"
  },
  "tags": ["work"],
  "aliases": ["Release plan"]
}
```

Properties use native JSON/YAML values: strings, numbers, booleans and string
lists. Dates use `YYYY-MM-DD` strings and links use `[[path]]` strings, preserving
both their syntax and values through export. `tags` and `aliases` are reserved
properties with first-class API fields; those fields take precedence when both
forms are supplied. Explicit tags are normalized to lowercase without `#`.
`vesper_id`, `lazyagents_id`, `created`, `updated`, `project` and `title` are
reserved export envelope fields and cannot be custom properties.

`PUT /api/v1/wiki/pages/{id}` accepts these same optional metadata fields.
Omitted metadata is inherited. A supplied properties map replaces the custom
properties, and supplied tag/alias arrays replace those fields (use `[]` to
clear them). A body-only edit retains metadata. Changing the body or properties
creates a version. A title edit derives a unique last path segment in the current
folder, with numeric suffixes for collisions, and retains the old path as a moved
alias. A folder move changes the path while keeping the display title. When both
path and title change, the title determines the last segment in the destination
folder. Supplying the unchanged title preserves the current path. Combined edits
are transactional; invalid metadata (422) or a stale `base_version_id` (409) rolls them back.

The returned `Artifact` includes the current version's `properties`, explicit
`aliases`, and the union of explicit and inline `tags`. `GET /api/v1/wiki/pages?tag=work/releases`
filters by an exact, case-insensitive indexed tag. Explicit and inline tags
also participate in knowledge full-text search. Inline tags preserve nested
paths; code blocks, code spans and link targets do not supply tags.

Suggestions store their own properties; only publication rebuilds `page_tags`
and frontmatter aliases. Alias changes cannot steal another page's path or
alias. Explicit path creation also rejects another page's frontmatter alias,
including when the new page is only a suggestion; historical moved paths may
still be explicitly reused. Removed frontmatter aliases leave moved-path aliases
intact. Metadata has no independent audience or read endpoint: it is returned with its parent
page under that page's read policy. The audience implementation is LAZYAGENTS-175.

Vault export writes stable YAML frontmatter, including the display title and
native property types. Import accepts native YAML properties, keeps unknown
nested maps as YAML text, and rejects malformed frontmatter transactionally.
Importing an unchanged export creates no version.

Migration `00114_page_properties.sql` adds version properties, the alias kind,
and the derived tag table. It leaves existing bodies and versions intact;
legacy aliases are classified as moved paths. It ships as SQL for the normal
migration rollout; this ticket does not apply it to an application database.
Page writes take a workspace namespace transaction lock before row locks to
serialize derived paths and alias updates consistently. Reads remain unlocked.
