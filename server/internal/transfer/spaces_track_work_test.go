package transfer_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/db"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/transfer"
)

// preSpacesWorkSchema is the last migration before spaces and projects became one thing
// (00132_spaces_track_work, LAZYAGENTS-213).
const preSpacesWorkSchema = 131

type spacesFixture struct {
	project, personal, loose, inPersonal, hidden string
}

// spacesIn adds, beside seed's project PRJ: a person's own space that files tickets under PRJ, a
// chat with PRJ and no space, a chat with PRJ in that space, and a hidden (app) chat with PRJ.
func spacesIn(t *testing.T, pool *pgxpool.Pool) spacesFixture {
	t.Helper()
	ctx := t.Context()
	var f spacesFixture
	var ws, user, agent string
	scan := func(dst *string, sql string, args ...any) {
		t.Helper()
		if err := pool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	scan(&user, `SELECT id FROM users ORDER BY created_at, id LIMIT 1`)
	scan(&ws, `SELECT workspace_id FROM users WHERE id=$1`, user)
	scan(&agent, `SELECT id FROM agents WHERE workspace_id=$1 LIMIT 1`, ws)
	scan(&f.project, `SELECT id FROM projects WHERE key='PRJ'`)
	scan(&f.personal, `INSERT INTO chat_spaces(workspace_id,user_id,name,color,project_id,instructions) VALUES($1,$2,'Health','green',$3,'Be gentle') RETURNING id`, ws, user, f.project)
	chat := `INSERT INTO chat_sessions(workspace_id,title,created_by,agent_id,harness,model,effort,project_id,space_id,hidden) VALUES($1,$2,$3,$4,'codex','m','high',$5,$6,$7) RETURNING id`
	scan(&f.loose, chat, ws, "Loose", user, agent, f.project, nil, false)
	scan(&f.inPersonal, chat, ws, "In Health", user, agent, f.project, f.personal, false)
	scan(&f.hidden, chat, ws, "Deck chat", user, agent, f.project, nil, true)
	return f
}

// An export from before spaces tracked work imports into the new schema, and the migration
// gives each project its own space: its name and nearest color, no instructions and space
// memory off (so the chats moved in see what they saw before). Chats with the project and no
// space move in; chats in another space, and app chats, stay where they were.
func TestPreSpacesWorkExportGivesEachProjectASpace(t *testing.T) {
	ctx := t.Context()
	src := testdb.NewUnmigrated(t)
	if err := db.MigrateTo(ctx, src, preSpacesWorkSchema); err != nil {
		t.Fatal(err)
	}
	seed(t, src)
	f := spacesIn(t, src)

	zr, m := exportToZip(t, src, newMemBlobs(), "")
	if m.SchemaVersion != preSpacesWorkSchema {
		t.Fatalf("export schema %d, want %d", m.SchemaVersion, preSpacesWorkSchema)
	}
	dst := testdb.NewUnmigrated(t)
	if err := (&transfer.Importer{Pool: dst, Blobs: newMemBlobs()}).Import(ctx, zr); err != nil {
		t.Fatalf("import: %v", err)
	}

	var space, name, color, instructions string
	var memory bool
	var filesUnder *string
	if err := dst.QueryRow(ctx, `SELECT s.id, s.name, s.color, s.instructions, s.space_memory, s.project_id::text
		FROM projects p JOIN chat_spaces s ON s.id = p.space_id WHERE p.id = $1`, f.project).Scan(&space, &name, &color, &instructions, &memory, &filesUnder); err != nil {
		t.Fatalf("PRJ's space: %v", err)
	}
	// seed's PRJ is #123456: nearest of the tiles is blue (#60A5FA).
	if name != `Project, "quoted"` || color != "blue" || instructions != "" || memory || filesUnder == nil || *filesUnder != f.project {
		t.Errorf("PRJ's space = %q %q %q memory=%v files under %v", name, color, instructions, memory, filesUnder)
	}

	spaceOf := func(chat string) *string {
		t.Helper()
		var s *string
		if err := dst.QueryRow(ctx, `SELECT space_id::text FROM chat_sessions WHERE id=$1`, chat).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if s := spaceOf(f.loose); s == nil || *s != space {
		t.Errorf("loose chat's space = %v, want PRJ's %s", s, space)
	}
	if s := spaceOf(f.inPersonal); s == nil || *s != f.personal {
		t.Errorf("chat in Health moved to %v", s)
	}
	if s := spaceOf(f.hidden); s != nil {
		t.Errorf("app chat moved into %s", *s)
	}
	// Health keeps filing tickets under PRJ; it doesn't become PRJ's.
	var healthFiles string
	if err := dst.QueryRow(ctx, `SELECT project_id::text FROM chat_spaces WHERE id=$1`, f.personal).Scan(&healthFiles); err != nil || healthFiles != f.project {
		t.Errorf("Health files under %q (%v), want PRJ", healthFiles, err)
	}
}

// A space's work comes back from an export as that space's.
func TestSpaceWorkRoundTrips(t *testing.T) {
	ctx := t.Context()
	src := testdb.New(t)
	seed(t, src)
	var ws, user, project, space string
	if err := src.QueryRow(ctx, `SELECT u.workspace_id, u.id, p.id FROM users u JOIN projects p ON p.workspace_id = u.workspace_id WHERE p.key='PRJ'`).Scan(&ws, &user, &project); err != nil {
		t.Fatal(err)
	}
	if err := src.QueryRow(ctx, `INSERT INTO chat_spaces(workspace_id,user_id,name,color,project_id) VALUES($1,$2,'Work','teal',$3) RETURNING id`, ws, user, project).Scan(&space); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(ctx, `UPDATE projects SET space_id=$2 WHERE id=$1`, project, space); err != nil {
		t.Fatal(err)
	}
	zr, _ := exportToZip(t, src, newMemBlobs(), "")
	dst := testdb.NewUnmigrated(t)
	if err := (&transfer.Importer{Pool: dst, Blobs: newMemBlobs()}).Import(ctx, zr); err != nil {
		t.Fatalf("import: %v", err)
	}
	var got string
	if err := dst.QueryRow(ctx, `SELECT space_id::text FROM projects WHERE id=$1`, project).Scan(&got); err != nil || got != space {
		t.Errorf("PRJ's space after import = %q (%v), want %s", got, err, space)
	}
}
