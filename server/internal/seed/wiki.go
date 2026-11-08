package seed

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
)

// Demo wiki (docs/dev/plan.md §12): a few team pages, Atlas's memory from PAY-214,
// and a suggestion from Sentry waiting for a person.
var demoPages = []struct{ path, body string }{
	{"Payments/Ledger/Rollback runbook", `# Rollback runbook

How to back out a ledger partition migration. Owned by the Payments team; agents may suggest edits.

## When to roll back

- Reconciliation p95 regresses past the target in [[Ledger v2 — Product requirements#Reconciliation performance]].
- Writes to ` + "`ledger_entries`" + ` fail after the cutover in [[PAY-214]].

## Steps

1. Pause the dual-write job ([[Ledger dual-write]]).
2. Swap the view back to ` + "`ledger_entries_legacy`" + `. The legacy index is kept for 72h (see [[ADR-012#Consequences]]).
3. Tell #payments-oncall and link the incident here.

Questions go to [[@atlas]] or the [[Ledger v2]] epic owner.
`},
	{"Payments/Glossary", `# Payments glossary

**Posting**: writing a ledger entry. Entries are immutable; corrections are new entries.

**Reconciliation**: matching ledger totals against the processor's settlement files, nightly. See [[Ledger v2 — Product requirements]].

**Partition**: one month of ` + "`ledger_entries`" + `, per ADR-012.
`},
	{"Engineering/Working with agents", `# Working with agents

- Assign a ticket, then dispatch it. Agents post a plan before editing.
- Attach the PRD, spec and any ADRs as context. Agents read them through Vesper, so citations show up on each section.
- Agents keep notes under ` + "`agents/<handle>/`" + `; each has a Memory page. Read [[agents/atlas/Memory]] for an example.
- Agent edits to team pages arrive as suggestions on the page. Accept or reject them there.

## Main agents and sub agents

- **Main agents** are assigned tickets and dispatched. **Sub agents** can't be assigned tickets; they only run when a main agent delegates part of its ticket to them (implementation, docs, tests).
- A delegated sub agent works in the main agent's checkout on the same branch. The main agent waits, then gets the sub agent's summary and reviews the diff before continuing.
- Delegation is one sub agent at a time for now.

## Planned: parallel delegation

Letting a main agent hand work to several sub agents at once is planned but not built. It needs a worktree or branch per sub agent, a merge step, and a way for the main agent to collect results instead of waiting on each one. Until then, split work across tickets if you need it done in parallel.
`},
}

const atlasGotchas = `# Partition rollback gotchas

Learned on [[PAY-214]] while applying [[ADR-012]] to staging.

- ` + "`DETACH PARTITION`" + ` takes an ACCESS EXCLUSIVE lock on the parent. Use ` + "`DETACH PARTITION … CONCURRENTLY`" + ` (Postgres 14+) outside a transaction.
- The legacy index must outlive the cutover by 72h, or rollback means a full rebuild (Dev asked for this in review).
- The staging snapshot lags prod by a day; re-run the dry-run before asking for ` + "`db:migrate`" + `.
`

func seedWiki(ctx context.Context, pool *pgxpool.Pool, arts *artifacts.Service, tr *tracker.Service, ws uuid.UUID,
	users map[string]store.User, agents map[string]store.Agent) error {
	svc := &wiki.Service{Pool: pool, Artifacts: arts, Tracker: tr}
	for _, p := range demoPages {
		if _, err := svc.SaveAsUser(ctx, ws, users["dev"].ID, wiki.Save{Path: p.path, Body: p.body, Mode: wiki.ModeCreate}); err != nil {
			return err
		}
	}
	// Atlas's memory from PAY-214. PAY-224 (the rollback runbook ticket)
	// has ADR-012 attached, so its next run starts with this note.
	if _, err := svc.AgentWrite(ctx, agents["atlas"], nil, wiki.Save{Path: wiki.MemoryRoot("atlas") + "Partition rollback gotchas", Body: atlasGotchas}); err != nil {
		return err
	}
	adr := int32(12)
	q := store.New(pool)
	a, err := q.GetADRByNumber(ctx, store.GetADRByNumberParams{WorkspaceID: ws, Number: &adr})
	if err != nil {
		return err
	}
	if err := tr.AttachArtifact(ctx, ws, tracker.UserActor(users["maya"].ID), "PAY-224", a.ID, "context"); err != nil {
		return err
	}
	// Sentry (wiki:write requires approval) suggests a fix to the runbook.
	sentry := map[string]agentdef.Mode{"wiki:write": agentdef.RequiresApproval}
	_, err = svc.AgentWrite(ctx, agents["sentry"], sentry, wiki.Save{
		Path: "Payments/Ledger/Rollback runbook", Mode: wiki.ModeAppend,
		Body: "## Verify\n\nAfter the swap, compare row counts between `ledger_entries` and `ledger_entries_legacy` for the last 7 days before resuming writes.\n",
		Note: "Add a verification step",
	})
	return err
}
