package links

import (
	"strings"
	"testing"
)

// benchDoc is a ~60 KB wiki page: mostly prose and lists, a few links,
// inline code and fenced blocks.
func benchDoc() string {
	section := `## Rollout plan

The **ledger** service moves to monthly partitions; see [[ADR-012#Decision]] and [[Rollback runbook|the runbook]].
- Backfill ` + "`ledger_entries`" + ` in batches of 10k rows.
- Watch replication lag on the staging replica before each batch.
1. Pause writers, then swap the table.

| Step | Owner |
|------|-------|
| Backfill | [[PAY-214]] |

` + "```sql\nSELECT * FROM ledger_entries WHERE id = 1; -- [[not a link]]\n```\n" + `
Plain paragraph text that goes on for a while to look like an actual design document, with no links at all in it.
Another line of ordinary prose describing tradeoffs, risks and the reasons behind the chosen approach.

`
	return strings.Repeat(section, 80)
}

func BenchmarkParse(b *testing.B) {
	doc := benchDoc()
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	for b.Loop() {
		Parse(doc)
	}
}
