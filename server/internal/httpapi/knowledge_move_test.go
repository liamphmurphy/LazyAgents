package httpapi

import (
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Save to Knowledge moves an ordinary chat canvas into a private page with
// its history, authors, comments and chat, and leaves no canvas behind.
func TestSaveCanvasToKnowledgeMovesWithoutACopy(t *testing.T) {
	e := newChatEnv(t)
	ctx := t.Context()
	chat := e.newChat(nil)
	chatID := uuid.MustParse(chat["id"].(string))
	session, err := e.q.GetChatSession(ctx, store.GetChatSessionParams{ID: chatID, WorkspaceID: e.me.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	cv, err := e.q.CreateChatCanvas(ctx, store.CreateChatCanvasParams{SessionID: chatID, Kind: "text", Title: "Harbour notes"})
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range []struct{ body, typ string }{{"# Harbour\n\nFirst draft.\n", "user"}, {"# Harbour\n\nPersona rewrite.\n", "agent"}} {
		author := e.me.ID
		if v.typ == "agent" {
			author = session.AgentID
		}
		if _, err = e.q.CreateChatCanvasVersion(ctx, store.CreateChatCanvasVersionParams{CanvasID: cv.ID, Seq: int32(i + 1), Body: v.body, AuthorType: v.typ, AuthorID: author}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = e.q.CreateChatCanvasComment(ctx, store.CreateChatCanvasCommentParams{CanvasID: cv.ID, Seq: 1, AuthorType: "user", AuthorID: e.me.ID, Body: "Check the ferry times", Anchor: []byte(`{}`), Status: "open"}); err != nil {
		t.Fatal(err)
	}

	page := e.want("POST", "/knowledge/pages/from-canvas", map[string]any{"chat_id": chatID, "canvas_id": cv.ID, "folder": "Notes"}, 200)
	if page["audience"] != "private" || page["chat_id"] != chatID.String() || !strings.Contains(page["body_md"].(string), "Persona rewrite.") {
		t.Fatalf("moved page %v", page)
	}
	versions := page["versions"].([]any)
	authors := map[string]bool{}
	for _, v := range versions {
		authors[v.(map[string]any)["author_type"].(string)] = true
	}
	if len(versions) != 2 || !authors["user"] || !authors["agent"] {
		t.Fatalf("history not kept with authors: %v", versions)
	}
	var comments int
	if err = e.pool.QueryRow(ctx, `SELECT count(*) FROM page_comments WHERE artifact_id=$1`, page["id"]).Scan(&comments); err != nil || comments != 1 {
		t.Fatalf("comments %d %v", comments, err)
	}
	if _, err = e.q.GetChatCanvas(ctx, store.GetChatCanvasParams{ID: cv.ID, SessionID: chatID}); err == nil {
		t.Fatal("canvas still exists after the move")
	}
	e.want("POST", "/knowledge/pages/from-canvas", map[string]any{"chat_id": chatID, "canvas_id": cv.ID}, 404)
}

// Addresses from before the merge resolve to the page that holds them now.
func TestLegacyDocsAndWikiAddressesResolveToPages(t *testing.T) {
	e := newChatEnv(t)
	ctx := t.Context()
	wikiPage := e.want("POST", "/knowledge/pages", map[string]any{"title": "Rollback runbook", "path": "Payments/Rollback runbook", "body_md": "Steps.", "audience": "shared"}, 200)
	got := e.want("GET", "/knowledge/legacy?path="+url.QueryEscape("/wiki/Payments/Rollback%20runbook"), nil, 200)
	if got["id"] != wikiPage["id"] || !strings.HasPrefix(got["url"].(string), "/knowledge/"+wikiPage["id"].(string)) {
		t.Fatalf("wiki address resolved to %v", got)
	}

	chat := e.newChat(nil)
	cv, err := e.q.CreateChatCanvas(ctx, store.CreateChatCanvasParams{SessionID: uuid.MustParse(chat["id"].(string)), Kind: "text", Title: "Old doc"})
	if err != nil {
		t.Fatal(err)
	}
	docPage := e.want("POST", "/knowledge/pages", map[string]any{"title": "Old doc", "body_md": "Moved."}, 200)
	if _, err = e.pool.Exec(ctx, `INSERT INTO doc_page_redirects(canvas_id,artifact_id,merged,migration_seq) VALUES($1,$2,false,1)`, cv.ID, docPage["id"]); err != nil {
		t.Fatal(err)
	}
	got = e.want("GET", "/knowledge/legacy?path="+url.QueryEscape("/apps/docs/"+cv.ID.String()), nil, 200)
	if got["id"] != docPage["id"] {
		t.Fatalf("docs address resolved to %v", got)
	}

	e.want("GET", "/knowledge/legacy?path="+url.QueryEscape("/apps/docs/"+uuid.NewString()), nil, 404)
	e.want("GET", "/knowledge/legacy?path="+url.QueryEscape("/wiki/Nowhere"), nil, 404)
	e.want("GET", "/knowledge/legacy?path="+url.QueryEscape("/elsewhere"), nil, 422)
}

// The whole-vault graph lists the viewer's pages, the links between them and
// links to pages that don't exist yet; agent pages only when asked for.
func TestKnowledgeNetworkFollowsAudience(t *testing.T) {
	e := newChatEnv(t)
	ctx := t.Context()
	a := e.want("POST", "/knowledge/pages", map[string]any{"title": "Harbour", "path": "Net/Harbour", "body_md": "See [[Net/Tower]] and [[Nowhere yet]]."}, 200)
	b := e.want("POST", "/knowledge/pages", map[string]any{"title": "Tower", "path": "Net/Tower", "body_md": "Back to [[Net/Harbour]]."}, 200)
	if _, err := e.pool.Exec(ctx, `UPDATE artifacts SET audience='agent', owner_user_id=NULL WHERE id=$1`, b["id"]); err != nil {
		t.Fatal(err)
	}
	other, err := e.q.CreateUser(ctx, store.CreateUserParams{WorkspaceID: e.me.WorkspaceID, Email: "other@example.com", Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.pool.Exec(ctx, `UPDATE artifacts SET owner_user_id=$2 WHERE id=$1`, a["id"], other.ID); err != nil {
		t.Fatal(err)
	}
	if net := e.want("GET", "/knowledge/network", nil, 200); strings.Contains(fmtJSON(net), a["id"].(string)) || strings.Contains(fmtJSON(net), b["id"].(string)) {
		t.Fatalf("network leaked another person's page or an agent page: %v", net)
	}
	if _, err = e.pool.Exec(ctx, `UPDATE artifacts SET owner_user_id=$2 WHERE id=$1`, a["id"], e.me.ID); err != nil {
		t.Fatal(err)
	}
	net := e.want("GET", "/knowledge/network", nil, 200)
	if s := fmtJSON(net); !strings.Contains(s, a["id"].(string)) || strings.Contains(s, b["id"].(string)) || !strings.Contains(s, "Nowhere yet") {
		t.Fatalf("default network %v", net)
	}
	withAgent := fmtJSON(e.want("GET", "/knowledge/network?include_agent=true", nil, 200))
	if !strings.Contains(withAgent, `"source":"`+a["id"].(string)+`","target":"`+b["id"].(string)) {
		t.Fatalf("agent link missing: %s", withAgent)
	}
}
