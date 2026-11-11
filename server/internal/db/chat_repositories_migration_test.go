package db_test

import (
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"testing"
)

func TestChatRepositoryMigrationPreservesLegacyWork(t *testing.T) {
	f := newConstraintFixture(t)
	ctx := t.Context()
	sqlDB := stdlib.OpenDBFromPool(f.pool)
	defer sqlDB.Close()
	if err := goose.DownToContext(ctx, sqlDB, ".", 100); err != nil {
		t.Fatal(err)
	}
	// Two legacy personas sharing a repository become one personal source.
	for _, name := range []string{"Coder", "Reviewer"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO personas(workspace_id,user_id,name,can_code,repos) VALUES($1,$2,$3,true,ARRAY['https://github.com/acme/app'])`, f.ws, f.user, name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `UPDATE chat_sessions SET repo_url='https://github.com/acme/app',work_branches=ARRAY['feat/earlier-work'] WHERE id=$1`, f.chat); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM repository_sources WHERE workspace_id=$1 AND user_id=$2`, f.ws, f.user).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	var directory string
	var branches []string
	if err := f.pool.QueryRow(ctx, `SELECT directory,work_branches FROM chat_repositories WHERE chat_id=$1 AND repo_url='https://github.com/acme/app'`, f.chat).Scan(&directory, &branches); err != nil || directory != "." || len(branches) != 1 || branches[0] != "feat/earlier-work" {
		t.Fatal(directory, branches, err)
	}
	if err := goose.DownToContext(ctx, sqlDB, ".", 100); err != nil {
		t.Fatal(err)
	}
	var url string
	if err := f.pool.QueryRow(ctx, `SELECT repo_url FROM chat_sessions WHERE id=$1`, f.chat).Scan(&url); err != nil || url != "https://github.com/acme/app" {
		t.Fatal(url, err)
	}
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		t.Fatal(err)
	}
}
