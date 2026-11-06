package db_test

import (
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

// Migration 24 rolls back and re-applies cleanly.
func TestMockupMigrationRoundTrip(t *testing.T) {
	pool := testdb.New(t)
	gooseTo(t, pool, false, 23)
	gooseTo(t, pool, true, 24)
	mustExec(t, pool, `ALTER TABLE tickets ALTER COLUMN requires_mockup SET DEFAULT false`)
}
