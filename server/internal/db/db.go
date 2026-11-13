// Package db owns the Postgres connection pool and schema migrations.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/liamphmurphy/vesper/server/migrations"
)

// Open connects to Postgres, retrying until ctx is done so the API can start
// before the database container is ready.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("db: parse url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	for {
		pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = pool.Ping(pctx)
		cancel()
		if err == nil {
			return pool, nil
		}
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, fmt.Errorf("db: not reachable: %w", err)
		case <-time.After(time.Second):
		}
	}
}

// Migrate applies all pending goose migrations embedded in the binary.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	sqldb := stdlib.OpenDBFromPool(pool)
	defer sqldb.Close()
	if err := migrate(ctx, sqldb); err != nil {
		return err
	}
	if err := backfillKnowledge(ctx, pool); err != nil {
		return fmt.Errorf("knowledge backfill: %w", err)
	}
	// River owns its migration history; serialize API/runner startup migrations.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(71933712)"); err != nil {
		return err
	}
	m, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	if _, err = m.MigrateTx(ctx, tx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("river migrate: %w", err)
	}
	return tx.Commit(ctx)
}

func migrate(ctx context.Context, sqldb *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, sqldb, "."); err != nil {
		return fmt.Errorf("db: migrate: %w", err)
	}
	return nil
}

// Version is the newest goose migration applied, or 0 on a database that has
// none. It creates goose's bookkeeping table when it is missing.
func Version(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	sqldb := stdlib.OpenDBFromPool(pool)
	defer sqldb.Close()
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return 0, err
	}
	v, err := goose.GetDBVersionContext(ctx, sqldb)
	if err != nil {
		return 0, fmt.Errorf("db: version: %w", err)
	}
	return v, nil
}

// MigrateTo applies goose migrations up to and including version, so a data
// import can load rows into the schema they were exported from before newer
// migrations reshape them.
func MigrateTo(ctx context.Context, pool *pgxpool.Pool, version int64) error {
	sqldb := stdlib.OpenDBFromPool(pool)
	defer sqldb.Close()
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpToContext(ctx, sqldb, ".", version); err != nil {
		return fmt.Errorf("db: migrate to %d: %w", version, err)
	}
	return nil
}

// LatestVersion is the newest migration embedded in this binary.
func LatestVersion() (int64, error) {
	goose.SetBaseFS(migrations.FS)
	all, err := goose.CollectMigrations(".", 0, goose.MaxVersion)
	if err != nil {
		return 0, fmt.Errorf("db: collect migrations: %w", err)
	}
	last, err := all.Last()
	if err != nil {
		return 0, err
	}
	return last.Version, nil
}
