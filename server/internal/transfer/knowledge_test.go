package transfer_test

import (
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/db"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/transfer"
)

// preKnowledgeSchema is the last migration on main before the Knowledge epic
// (00113_designs). An export taken there has Docs as chat canvases only.
const preKnowledgeSchema = 113

// An export from before Docs moved into Knowledge imports into the new schema,
// and the newer migrations turn its Doc into a private page with its history.
func TestPreKnowledgeExportImportsDocsAsPages(t *testing.T) {
	ctx := t.Context()
	src := testdb.NewUnmigrated(t)
	if err := db.MigrateTo(ctx, src, preKnowledgeSchema); err != nil {
		t.Fatal(err)
	}
	seed(t, src)
	var ws, user, agent, space, chat, canvas string
	scan := func(dst *string, sql string, args ...any) {
		t.Helper()
		if err := src.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	scan(&user, `SELECT id FROM users LIMIT 1`)
	scan(&ws, `SELECT workspace_id FROM users WHERE id=$1`, user)
	scan(&agent, `SELECT id FROM agents WHERE workspace_id=$1 LIMIT 1`, ws)
	scan(&space, `INSERT INTO chat_spaces(workspace_id,user_id,name,color,app) VALUES($1,$2,'Docs','blue','docs') RETURNING id`, ws, user)
	scan(&chat, `INSERT INTO chat_sessions(workspace_id,title,created_by,agent_id,harness,model,effort,space_id) VALUES($1,'Campaign doc',$2,$3,'codex','m','high',$4) RETURNING id`, ws, user, agent, space)
	scan(&canvas, `INSERT INTO chat_canvases(session_id,kind,title) VALUES($1,'text','Campaign') RETURNING id`, chat)
	if _, err := src.Exec(ctx, `INSERT INTO chat_canvas_versions(canvas_id,seq,body,author_type,author_id) VALUES($1,1,'# Campaign','user',$2),($1,2,'# Campaign'||E'\n\nThe harbour.','agent',$3)`, canvas, user, agent); err != nil {
		t.Fatal(err)
	}

	zr, m := exportToZip(t, src, newMemBlobs(), "")
	if m.SchemaVersion != preKnowledgeSchema {
		t.Fatalf("export schema %d, want %d", m.SchemaVersion, preKnowledgeSchema)
	}
	dst := testdb.NewUnmigrated(t)
	if err := (&transfer.Importer{Pool: dst, Blobs: newMemBlobs()}).Import(ctx, zr); err != nil {
		t.Fatalf("import: %v", err)
	}

	var audience, pageChat string
	var versions int
	if err := dst.QueryRow(ctx, `SELECT a.audience, a.chat_session_id::text, (SELECT count(*) FROM artifact_versions v WHERE v.artifact_id=a.id)
		FROM doc_page_redirects d JOIN artifacts a ON a.id=d.artifact_id WHERE d.canvas_id=$1`, canvas).Scan(&audience, &pageChat, &versions); err != nil {
		t.Fatalf("imported Doc has no page: %v", err)
	}
	if audience != "private" || pageChat != chat || versions != 2 {
		t.Fatalf("imported Doc page: audience %s chat %s versions %d", audience, pageChat, versions)
	}
}
