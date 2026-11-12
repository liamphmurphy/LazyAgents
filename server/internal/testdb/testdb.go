// Package testdb gives integration tests a migrated, throwaway database.
//
// Tests are skipped unless TEST_DATABASE_URL points at a Postgres server the
// tests may create databases on (CI provides one; locally use dev compose).
// Inside a Vesper run or on CI they fail instead: a skip there reads as a
// pass, and agents reported "tests pass" without ever reaching a database.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/db"
)

// requiredBy names what requires database tests to run rather than skip, or
// "" when skipping is fine (a person running go test locally).
func requiredBy() string {
	switch {
	case os.Getenv("LAZYAGENTS_RUN_ID") != "":
		return "a Vesper run"
	case os.Getenv("CI") != "":
		return "CI"
	}
	return ""
}

// New creates a uniquely named database, migrates it and drops it when the test ends.
func New(t testing.TB) *pgxpool.Pool { return create(t, true) }

// NewUnmigrated is New without the migrations: a database as a fresh machine
// has it, for tests of code that migrates it itself.
func NewUnmigrated(t testing.TB) *pgxpool.Pool { return create(t, false) }

func create(t testing.TB, migrate bool) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		if reason := requiredBy(); reason != "" {
			t.Fatalf("testdb: TEST_DATABASE_URL is not set, and %s requires database tests to run. "+
				"Start Postgres 17 (e.g. `docker run -d --name pg -e POSTGRES_PASSWORD=pg -p 5432 postgres:17-alpine`) "+
				"and point TEST_DATABASE_URL at it", reason)
		}
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("testdb: connect: %v", err)
	}
	defer admin.Close(ctx)

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "test_" + hex.EncodeToString(b)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("testdb: create: %v", err)
	}

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	pool, err := db.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	if migrate {
		if err := db.Migrate(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}

	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if c, err := pgx.Connect(ctx, base); err == nil {
			_, _ = c.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			_ = c.Close(ctx)
		}
	})
	return pool
}
