package runs_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// passChecks does what the runner does when a delegated run that waits on its
// checks passes them: completes it and wakes its parent. Runs that don't wait
// on checks are left alone.
func passChecks(t *testing.T, pool *pgxpool.Pool, ids ...uuid.UUID) {
	t.Helper()
	for _, id := range ids {
		if err := finishChecked(pool, id); err != nil {
			t.Fatal(err)
		}
	}
}

func finishChecked(pool *pgxpool.Pool, id uuid.UUID) error {
	ctx := context.Background()
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		_, err := runs.FinishCheckedChild(ctx, store.New(tx), id, nil)
		return err
	})
}
