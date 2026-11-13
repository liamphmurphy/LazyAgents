package db_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/pressly/goose/v3"
	"testing"
)

func TestPageAudienceMigration(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	if err := goose.DownToContext(ctx, sqlDB, ".", 113); err != nil {
		t.Fatal(err)
	}
	var ws, user uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('Audience','audience') RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'U','u@example.com','U','x') RETURNING id`, ws).Scan(&user); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{"agents/atlas/Memory": "agent", "Agents/Echo/Gotcha": "agent", "Patterns/Proposed/Rule": "agent", "Patterns/Go/Rule": "shared", "Architecture/ADR - Knowledge": "shared", "Normal": "shared"}
	for path := range paths {
		if _, err := pool.Exec(ctx, `INSERT INTO artifacts(workspace_id,type,title,path,author_type,author_id) VALUES($1,'note',$2,$2,'user',$3)`, ws, path, user); err != nil {
			t.Fatal(err)
		}
	}
	var project uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO projects(workspace_id,key,name,color) VALUES($1,'AU','Audience','#fff') RETURNING id`, ws).Scan(&project); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO artifacts(workspace_id,project_id,type,title,number,author_type,author_id) VALUES($1,$3,'adr','Decision',1,'user',$2)`, ws, user, project); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		t.Fatal(err)
	}
	for path, want := range paths {
		var got string
		if err := pool.QueryRow(ctx, `SELECT audience FROM artifacts WHERE workspace_id=$1 AND path=$2`, ws, path).Scan(&got); err != nil || got != want {
			t.Fatalf("%s: %q %v, want %s", path, got, err, want)
		}
	}
	var got string
	if err := pool.QueryRow(ctx, `SELECT audience FROM artifacts WHERE workspace_id=$1 AND type='adr'`, ws).Scan(&got); err != nil || got != "shared" {
		t.Fatalf("ADR %s %v", got, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE artifacts SET audience='private' WHERE path='Normal'`); err == nil {
		t.Fatal("private page without owner accepted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO artifacts(workspace_id,type,title,path,author_type,author_id,audience,owner_user_id) VALUES($1,'note','Normal','Normal','user',$2,'private',$2)`, ws, user); err != nil {
		t.Fatalf("private/shared paths must coexist: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO artifacts(workspace_id,type,title,path,author_type,author_id,audience,owner_user_id) VALUES($1,'note','Normal','Normal','user',$2,'private',$2)`, ws, user); err == nil {
		t.Fatal("duplicate private path accepted")
	}
}
