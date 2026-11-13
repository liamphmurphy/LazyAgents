# Page suggestions and review API

Pages keep suggestion snapshots in `artifact_versions` with a numbered `changes`
array. Body changes contain their before/after text and neighboring blocks;
property changes contain a key and before/after values (null means absent).
Every change carries `why`, its review state, and the deciding person's id,
time and reason. `persona_id` identifies the persona alongside the existing
agent author fields. An originating chat/run is retained for feedback.

`GET /wiki/suggestions?artifact_id=<page>` includes these changes. It also lists
direct persona edits awaiting review, with `applied: true`. Page detail exposes
`persona_direct`, `persona_paused`, `chat_session_id`, and each published version's
persona and changes.

## Resolve changes

`POST /pages/{id}/suggestions/{versionId}/resolve` accepts, for example:

```json
{"decisions":[{"i":0,"accept":true},{"i":2,"accept":false,"reason":"Keep my wording"}],"rest":"accept"}
```

Explicit decisions take precedence over `rest` (`accept` or `reject`). Without
`rest`, unselected changes remain pending. The response is the persisted changes
array, including decision states and reasons. Invalid/duplicate indices return
422; deciding an already resolved change/version returns 409. Missing pages,
versions, and versions of another page return 404. These endpoints use the
existing authenticated workspace boundary; the audience ticket adds private
page authorization to this boundary with the other note queries.

Resolution takes the workspace namespace lock, then the page lock. It matches
accepted changes against the current head and creates at most one human-authored
version per request. Missing or ambiguous body targets, changed context, and
properties whose old values changed become stale. Other changes still apply.
Original suggestion snapshots remain decision records, outside published history;
the human result is the history entry. Old whole-suggestion accept/reject routes
resolve all pending changes through the same service. Old suggestion rows without
`changes` are diffed against their recorded base when resolved.

Body review groups contiguous changed blocks using LCS, preserving source
whitespace and keeping fenced code together. Work is bounded; very large block
comparisons fall back to one conservative body change. Context matching can mark
a change stale when a neighboring block changed, even if its target still exists.

Rejected/stale changes, including their before/after values, why and reasons, are
appended as a system message to the originating persona chat. Run suggestions
return feedback as a ticket comment. Version, decisions, index updates and
feedback commit together; failed feedback rolls them all back. Duplicate requests
return conflicts without duplicating versions or feedback.

## Persona controls and history

`PUT /pages/{id}/persona-edits` takes optional `direct` and `paused` booleans.
Pausing takes precedence over direct editing and returns a clear conflict on a
persona write; human writes and ticket run scope rules are unchanged. With direct
editing on, a persona version publishes immediately with `applied` changes and
`review_state=pending`. Accepting those changes records review; rejecting them
writes an inverse human edit against the current head, preserving newer edits.
Existing shared pages default to suggestions. The private page creation/migration
path must set `persona_direct=true`, as specified in the Knowledge ADR.

`POST /pages/{id}/versions/{versionId}/restore` copies a published version's body
and properties into a new version authored by the signed-in person, even when its
content equals the current head. Original authors and snapshots remain intact;
tags, aliases, links and search are rebuilt through the normal publish path.

Migration 00116 is additive. It adds version changes/origins and page controls/chat
linkage; it does not apply migrations to any deployed instance. Its down migration
removes those additions and their review metadata.
