package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

// 00099 swaps chat_ticket_runs' one-run-per-reply rule for one run per ticket
// per reply; it rolls back to the old constraint and applies again.
func TestChatTicketRunsPerTicketMigrationRollsBack(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	sqldb := stdlib.OpenDBFromPool(pool)
	defer sqldb.Close()
	unique := func(name string) bool {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE tablename = 'chat_ticket_runs' AND indexname = $1`, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	if !unique("chat_ticket_runs_message_ticket") || unique("chat_ticket_runs_message_id_key") {
		t.Fatal("after 00099: want the (message_id, ticket_id) index only")
	}
	if err := goose.DownToContext(ctx, sqldb, ".", 98); err != nil {
		t.Fatal(err)
	}
	if unique("chat_ticket_runs_message_ticket") || !unique("chat_ticket_runs_message_id_key") {
		t.Fatal("after rollback: want the message_id constraint back")
	}
	if err := goose.UpContext(ctx, sqldb, "."); err != nil {
		t.Fatal(err)
	}
	if !unique("chat_ticket_runs_message_ticket") {
		t.Fatal("after re-applying: the per-ticket index is missing")
	}
}
