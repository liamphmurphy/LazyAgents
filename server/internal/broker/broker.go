// Package broker performs approval-gated actions on an agent's behalf
// (docs/dev/plan.md §6.3, run_gated). The agent never holds the credential: the
// broker reads it server-side and runs exactly the action a person approved.
package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Result is stored on the approval and returned to the agent.
type Result struct {
	OK           bool   `json:"ok"`
	Summary      string `json:"summary"`
	DurationMS   int64  `json:"duration_ms"`
	RowsAffected int64  `json:"rows_affected,omitempty"`
	Error        string `json:"error,omitempty"`
}

// Action is one brokered capability.
type Action interface {
	// Secret is the credential name the action needs (e.g. db:staging).
	Secret() string
	// Validate checks an approval request's detail before a person sees it.
	Validate(detail map[string]any) error
	// Describe renders the detail for the approval banner.
	Describe(detail map[string]any) string
	// Run executes the approved detail with the revealed secret.
	Run(ctx context.Context, secret string, detail map[string]any) Result
}

// Registry maps brokered scopes to actions. Scopes marked Brokered in
// agentdef.Catalog must appear here to be executable.
var Registry = map[string]Action{
	"db:migrate": SQLMigration{},
}

func For(scope string) (Action, bool) {
	a, ok := Registry[scope]
	return a, ok
}

// SQLMigration applies SQL to the database in the db:staging secret, in one
// transaction with statement and lock timeouts.
type SQLMigration struct{}

func (SQLMigration) Secret() string { return "db:staging" }

const maxSQL = 200 << 10

func sqlOf(detail map[string]any) string {
	s, _ := detail["sql"].(string)
	return strings.TrimSpace(s)
}

func (SQLMigration) Validate(detail map[string]any) error {
	s := sqlOf(detail)
	if s == "" {
		return errors.New(`detail.sql is required: the exact SQL to apply`)
	}
	if len(s) > maxSQL {
		return fmt.Errorf("detail.sql is limited to %d KB", maxSQL>>10)
	}
	return nil
}

func (SQLMigration) Describe(detail map[string]any) string {
	return sqlOf(detail)
}

func (SQLMigration) Run(ctx context.Context, dsn string, detail map[string]any) Result {
	start := time.Now()
	res := func(ok bool, summary string, rows int64, err error) Result {
		r := Result{OK: ok, Summary: summary, DurationMS: time.Since(start).Milliseconds(), RowsAffected: rows}
		if err != nil {
			r.Error = err.Error()
		}
		return r
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	conn, err := pgx.Connect(cctx, dsn)
	if err != nil {
		return res(false, "Couldn't connect to the staging database", 0, err)
	}
	defer conn.Close(context.Background())
	tx, err := conn.Begin(cctx)
	if err != nil {
		return res(false, "Couldn't start a transaction", 0, err)
	}
	defer tx.Rollback(context.Background())
	for _, stmt := range []string{"SET LOCAL statement_timeout = '60s'", "SET LOCAL lock_timeout = '5s'"} {
		if _, err := tx.Exec(cctx, stmt); err != nil {
			return res(false, "Couldn't set timeouts", 0, err)
		}
	}
	// No arguments → simple protocol, so multi-statement migrations work.
	tag, err := tx.Exec(cctx, sqlOf(detail))
	if err != nil {
		return res(false, "Migration failed and was rolled back", 0, err)
	}
	if err := tx.Commit(cctx); err != nil {
		return res(false, "Commit failed", 0, err)
	}
	return res(true, "Migration applied", tag.RowsAffected(), nil)
}

// DetailMap decodes stored jsonb detail.
func DetailMap(raw []byte) map[string]any {
	m := map[string]any{}
	_ = json.Unmarshal(raw, &m)
	return m
}
