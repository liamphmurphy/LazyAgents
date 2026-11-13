package seed

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

type demoDoc struct {
	project, typ, title, label, epic string
	number                           int32  // ADRs: keep the mockup's ADR-012
	author                           string // user key or agent handle
	agent                            bool
	summary, body                    string
	versions                         []string // earlier labels, oldest first
}

// Documents mirror docs/design-system/screens/artifacts.html.
var demoDocs = []demoDoc{
	{
		project: "PAY", typ: "prd", title: "Ledger v2 — Product requirements", label: "v3", epic: "Ledger v2", author: "maya",
		versions: []string{"v1", "v2"},
		summary:  "Goals, constraints and success metrics for the multi-currency ledger rebuild. Source of truth for the Ledger v2 epic.",
		body: `# Ledger v2 — Product requirements

## Problem & goals

The ledger is a single 18M-row table. Month-end reconciliation takes over 9s at p95 and blocks finance for hours. Ledger v2 makes reconciliation fast, auditable and multi-currency.

- Reconcile a month in under a second at p95.
- Every balance change is traceable to a source event.
- No downtime during the migration.

## Users and merchant impact

Finance runs reconciliation daily and at month end. Merchants see balances and payouts in the dashboard; they must never see a stale or negative balance during the migration.

## Money precision requirements

Store amounts as integer minor units with an explicit currency. JPY and other zero-decimal currencies must round-trip exactly. Legacy rows with float amounts are backfilled (see PAY-187).

## Reconciliation performance

### 4.1 Targets

p95 under 800ms for a single-month reconciliation on a production-sized snapshot.

### 4.2 Partition by posting month

Partition ledger_entries by posted_at month so reconciliation prunes to one partition. Use Postgres declarative partitioning (ADR-012).

## Rollout and rollback

Dual-write during backfill, verify with checksums, then swap tables in one transaction. Keep the legacy index for 72 hours as a rollback window.

## Out of scope

Changing the payout schedule, and any merchant-facing UI beyond balance accuracy.
`,
	},
	{
		project: "PAY", typ: "spec", title: "Ledger partitioning — Technical spec", label: "v1.2", epic: "Ledger v2", author: "dev",
		versions: []string{"v1.0", "v1.1"},
		body: `# Ledger partitioning — Technical spec

How we move ledger_entries to monthly range partitions without downtime.

## Partition layout

One partition per posted_at month from 2019-01 through 2027-12, plus a default partition. New partitions are created 3 months ahead by a scheduled job.

## Dual-write trigger

A trigger on the legacy table writes every insert and update to the partitioned parent during backfill.

## Backfill

Copy in 50k-row batches ordered by id. After each batch, compare per-batch checksums between old and new tables; stop on mismatch.

## Cutover

Inside one transaction: lock both tables, verify the final checksum, rename tables, drop the trigger. Target lock time under 200ms.

## Rollback

Rename back within the 72h window; the legacy index stays until then.
`,
	},
	{
		project: "PAY", typ: "adr", title: "Postgres declarative partitioning", label: "Accepted", epic: "Ledger v2", author: "dev", number: 12,
		body: `# Postgres declarative partitioning

## Context

ledger_entries is too large for month-end queries. We considered partitioning, a separate archive table, and moving reconciliation to the warehouse.

## Decision

Use Postgres declarative range partitioning on posted_at, monthly.

## Consequences

Queries must include posted_at to prune. Unique constraints must include the partition key. Partition maintenance becomes a scheduled job.
`,
	},
	{
		project: "PAY", typ: "report", title: "Migration dry-run report — PAY-214", label: "v1", epic: "Ledger v2", author: "atlas", agent: true,
		body: `# Migration dry-run report — PAY-214

## Summary

Dry run against the staging snapshot (18.2M rows) succeeded. Reconciliation p95 dropped from 9.4s to 610ms.

## Backfill

- 365 batches of 50k rows, 41 minutes total.
- 0 checksum mismatches.

## Cutover

Table swap held an exclusive lock for 140ms.

## Risks

Two reporting queries don't filter on posted_at and will scan every partition. Opened a follow-up to add the filter.
`,
	},
	{
		project: "PAY", typ: "prd", title: "Refunds API — Product requirements", label: "v2", epic: "Refunds API", author: "maya",
		versions: []string{"v1"},
		body: `# Refunds API — Product requirements

## Goals

Let merchants issue full and partial refunds through the API, safely retried.

## Idempotency

Every refund request carries an idempotency key; a repeated key returns the original result.

## Rate limits

Limit refunds per merchant to protect against runaway integrations.
`,
	},
	{
		project: "PAY", typ: "report", title: "Weekly test-coverage summary", label: "v1", author: "quill", agent: true,
		body: `# Weekly test-coverage summary

## Coverage

Payments service line coverage is 81% (+2 this week). The webhook dispatcher is the least covered package at 54%.

## Suggested tests

- Contract tests for webhook payloads.
- Retry jitter bounds in the dispatcher.
`,
	},
	{
		project: "MOB", typ: "spec", title: "Offline sync — Technical spec", label: "v1.4", epic: "Offline mode", author: "lena",
		body: `# Offline sync — Technical spec

## Write queue

Mutations made offline are queued locally with a client-generated id and replayed in order on reconnect.

## Conflict handling

The server wins on conflicting fields; the client shows a non-blocking notice.

## Session expiry

If the session expired while offline, the queue waits for re-authentication before replaying.
`,
	},
	{
		project: "DOC", typ: "report", title: "Spec conflict analysis — rate limits", label: "v1", epic: "API reference v2", author: "sentry", agent: true,
		body: `# Spec conflict analysis — rate limits

## Finding

The API reference says refunds are limited to 100/min per merchant; the Refunds API PRD says 30/min. One of them is wrong.

## Recommendation

Confirm the limit with the payments team and update the reference.
`,
	},
	{
		project: "BIL", typ: "spec", title: "Stripe cutover — Dual-run plan", label: "v0.9 draft", epic: "Stripe → Ledger cutover", author: "maya",
		body: `# Stripe cutover — Dual-run plan

## Approach

Run legacy billing and the new ledger in parallel for one full billing cycle and compare invoices line by line.

## Exit criteria

Zero unexplained invoice differences for 30 days.
`,
	},
}

// demoReads gives the demo its "Read by" chips and citation counts. In a
// real workspace these come from agents calling read_artifact (Phase 4).
var demoReads = []struct {
	title   string
	agent   string
	anchors map[string]int // anchor → reads; "" = whole document
}{
	{"Ledger v2 — Product requirements", "atlas", map[string]int{"": 2, "reconciliation-performance": 8, "4-2-partition-by-posting-month": 9, "rollout-and-rollback": 6, "money-precision-requirements": 3}},
	{"Ledger v2 — Product requirements", "relay", map[string]int{"problem-goals": 5, "users-and-merchant-impact": 2}},
	{"Ledger v2 — Product requirements", "pike", map[string]int{"money-precision-requirements": 14, "problem-goals": 4}},
	{"Ledger v2 — Product requirements", "sentry", map[string]int{"problem-goals": 3, "out-of-scope": 4, "money-precision-requirements": 10, "users-and-merchant-impact": 1}},
	{"Ledger partitioning — Technical spec", "atlas", map[string]int{"": 1, "backfill": 4, "cutover": 3}},
	{"Postgres declarative partitioning", "atlas", map[string]int{"": 1, "decision": 2}},
	{"Refunds API — Product requirements", "quill", map[string]int{"idempotency": 5}},
	{"Refunds API — Product requirements", "sentry", map[string]int{"rate-limits": 3}},
	{"Offline sync — Technical spec", "juno", map[string]int{"write-queue": 4, "session-expiry": 2}},
}

// designSVG is a small "screens" export for the demo design artifact.
const designSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="480" height="300" viewBox="0 0 480 300">
<rect width="480" height="300" fill="#111114"/>
<g fill="#16161B" stroke="#2A2A31"><rect x="40" y="30" width="120" height="240" rx="16"/><rect x="180" y="30" width="120" height="240" rx="16"/><rect x="320" y="30" width="120" height="240" rx="16"/></g>
<g fill="#8B8FFF"><rect x="56" y="214" width="88" height="24" rx="6"/><rect x="196" y="214" width="88" height="24" rx="6"/></g>
<g fill="#F59E5B"><rect x="336" y="214" width="88" height="24" rx="6"/></g>
<g fill="#A1A1AA"><rect x="56" y="60" width="60" height="8" rx="4"/><rect x="196" y="60" width="72" height="8" rx="4"/><rect x="336" y="60" width="50" height="8" rx="4"/></g>
</svg>`

func seedArtifacts(ctx context.Context, svc *artifacts.Service, q *store.Queries, tr *tracker.Service, ws uuid.UUID,
	users map[string]store.User, agents map[string]store.Agent, epics map[string]store.Epic) error {
	byTitle := map[string]store.Artifact{}
	for _, d := range demoDocs {
		by := artifacts.Actor{Type: "user"}
		if d.agent {
			by = artifacts.Actor{Type: "agent", ID: agents[d.author].ID}
		} else {
			by.ID = users[d.author].ID
		}
		// Earlier versions first, so the history panel has something to show.
		labels := append(append([]string{}, d.versions...), d.label)
		body := d.body
		first := strings.Replace(body, "\n## ", "\n_Early draft._\n\n## ", 1)
		if len(labels) == 1 {
			first = body
		}
		in := artifacts.NewArtifact{ProjectKey: d.project, Type: d.typ, Title: d.title, Summary: d.summary,
			VersionLabel: labels[0], Content: artifacts.Content{BodyMD: &first}}
		if d.number > 0 {
			in.Number = &d.number
		}
		if d.epic != "" {
			e := epics[d.epic]
			in.EpicID = &e.ID
		}
		a, err := svc.Create(ctx, ws, by, in)
		if err != nil {
			return fmt.Errorf("seed artifact %q: %w", d.title, err)
		}
		for i, l := range labels[1:] {
			b := body
			if i < len(labels)-2 {
				b = first
			}
			if a, err = svc.AddVersion(ctx, ws, by, a.ID, artifacts.NewVersion{Label: l, Note: "Revised after review", Content: artifacts.Content{BodyMD: &b}}); err != nil {
				return err
			}
		}
		byTitle[d.title] = a
	}

	// The design export needs object storage; skipped when there is none (tests).
	if svc.Blobs != nil {
		svg := designSVG
		e := epics["Offline mode"]
		a, err := svc.Create(ctx, ws, artifacts.Actor{Type: "user", ID: users["lena"].ID}, artifacts.NewArtifact{
			ProjectKey: "MOB", Type: "design", Title: "Session refresh flow — screens", EpicID: &e.ID, VersionLabel: "Figma export",
			Summary: "Three screens for re-authenticating after a session expires offline.",
			Content: artifacts.Content{File: &artifacts.File{Name: "session-refresh.svg", ContentType: "image/svg+xml",
				Size: int64(len(svg)), Body: strings.NewReader(svg)}},
		})
		if err != nil {
			return fmt.Errorf("seed design: %w", err)
		}
		byTitle[a.Title] = a
	}

	// Reload so CurrentVersionID reflects the last version written.
	for title, a := range byTitle {
		fresh, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: a.ID, WorkspaceID: ws, Viewer: pageview.From(ctx).UserID})
		if err != nil {
			return err
		}
		byTitle[title] = fresh
	}
	readAt := time.Now().Add(-26 * time.Hour)
	for _, r := range demoReads {
		a := byTitle[r.title]
		for anchor, n := range r.anchors {
			for range n {
				if err := artifacts.RecordRead(ctx, q, a, agents[r.agent].ID, nil, anchor, &readAt); err != nil {
					return fmt.Errorf("seed read %q#%s: %w", r.title, anchor, err)
				}
			}
		}
	}

	// PAY-214's context artifacts and the report Atlas generated (ticket-detail.html).
	maya := tracker.UserActor(users["maya"].ID)
	atlas := tracker.Actor{Type: "agent", ID: ptr(agents["atlas"].ID)}
	for _, t := range []string{"Ledger v2 — Product requirements", "Ledger partitioning — Technical spec", "Postgres declarative partitioning"} {
		if err := tr.AttachArtifact(ctx, ws, maya, "PAY-214", byTitle[t].ID, "context"); err != nil {
			return err
		}
	}
	if err := tr.AttachArtifact(ctx, ws, atlas, "PAY-214", byTitle["Migration dry-run report — PAY-214"].ID, "generated"); err != nil {
		return err
	}
	for _, k := range []string{"PAY-187", "PAY-221", "PAY-224", "PAY-226"} {
		if err := tr.AttachArtifact(ctx, ws, maya, k, byTitle["Ledger v2 — Product requirements"].ID, "context"); err != nil {
			return err
		}
	}
	for _, k := range []string{"PAY-198", "PAY-209", "PAY-219"} {
		if err := tr.AttachArtifact(ctx, ws, maya, k, byTitle["Refunds API — Product requirements"].ID, "context"); err != nil {
			return err
		}
	}
	return nil
}
