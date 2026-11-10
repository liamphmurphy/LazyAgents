package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

// Rolling back 00082 drops chat videos and the Library's videos, keeps its
// other items, and the migration applies again on top.
func TestChatVideosMigrationRollsBack(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	var ws, user, image uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'U','u@example.com','U','x') RETURNING id`, ws).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO library_items(workspace_id,user_id,kind,title,filename,content_type,size_bytes,blob_key) VALUES($1,$2,'image','I','i.png','image/png',1,'k1') RETURNING id`, ws, user).Scan(&image); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO library_items(workspace_id,user_id,kind,title,filename,content_type,size_bytes,blob_key,duration_ms,poster_key) VALUES($1,$2,'video','V','v.mp4','video/mp4',1,'k2',1000,'p')`, ws, user); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO chat_video_jobs(workspace_id,user_id,title,width,height,fps,duration_ms) VALUES($1,$2,'V',64,64,1,1000)`, ws, user); err != nil {
		t.Fatal(err)
	}

	sqldb := stdlib.OpenDBFromPool(pool)
	defer sqldb.Close()
	if err := goose.DownToContext(ctx, sqldb, ".", 81); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	if err := pool.QueryRow(ctx, `SELECT array_agg(kind) FROM library_items`).Scan(&kinds); err != nil || len(kinds) != 1 || kinds[0] != "image" {
		t.Fatalf("library after rollback %v %v", kinds, err)
	}
	if err := goose.UpContext(ctx, sqldb, "."); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO chat_video_jobs(workspace_id,user_id,title,width,height,fps,duration_ms) VALUES($1,$2,'Again',64,64,1,1000)`, ws, user); err != nil {
		t.Fatalf("after re-applying: %v", err)
	}
}
