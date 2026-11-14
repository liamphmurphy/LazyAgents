package httpapi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
)

func TestKnowledgeImageSuggestionsFilterTypeAndVisibility(t *testing.T) {
	e := newChatEnv(t)
	ctx := t.Context()
	other, err := e.q.CreateUser(ctx, store.CreateUserParams{WorkspaceID: e.me.WorkspaceID, Name: "Other", Email: "other@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	put := func(owner uuid.UUID, name, mime string) store.KnowledgeAsset {
		t.Helper()
		asset, err := e.q.PutKnowledgeAsset(ctx, store.PutKnowledgeAssetParams{WorkspaceID: e.me.WorkspaceID, OwnerUserID: owner, ContentHash: uuid.NewString(), BlobKey: uuid.NewString(), Filename: name, ContentType: mime, SizeBytes: 1})
		if err != nil {
			t.Fatal(err)
		}
		return asset
	}
	own := put(e.me.ID, "Map.PNG", "image/png")
	shared := put(other.ID, "shared-map.svg", "image/svg+xml")
	hidden := put(other.ID, "private-map.jpg", "image/jpeg")
	disguised := put(e.me.ID, "fake-map.png", "application/pdf")
	foreignWorkspace, err := e.q.CreateWorkspace(ctx, store.CreateWorkspaceParams{Name: "Other", Slug: "other-images"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := e.q.PutKnowledgeAsset(ctx, store.PutKnowledgeAssetParams{WorkspaceID: foreignWorkspace.ID, OwnerUserID: e.me.ID, ContentHash: "foreign", BlobKey: "foreign", Filename: "foreign-map.png", ContentType: "image/png", SizeBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Referenced assets follow the same visibility as downloading or embedding them.
	for _, item := range []struct {
		asset    store.KnowledgeAsset
		audience string
	}{{shared, "shared"}, {hidden, "private"}} {
		page := e.want("POST", "/knowledge/pages", map[string]any{"title": item.audience, "audience": item.audience}, 200)
		if _, err := e.pool.Exec(ctx, `INSERT INTO page_asset_refs(version_id,asset_id) VALUES ($1,$2)`, page["version_id"], item.asset.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := e.pool.Exec(ctx, `UPDATE artifacts SET owner_user_id=$2 WHERE id=$1`, page["id"], other.ID); err != nil {
			t.Fatal(err)
		}
	}
	out := e.want("GET", "/knowledge/assets?q=MAP", nil, 200)
	images := out["images"].([]any)
	if len(images) != 2 {
		t.Fatalf("image suggestions: %v", out)
	}
	raw := fmtJSON(out)
	for _, id := range []uuid.UUID{own.ID, shared.ID} {
		if !strings.Contains(raw, id.String()) {
			t.Fatalf("missing visible image %s: %s", id, raw)
		}
	}
	for _, id := range []uuid.UUID{hidden.ID, disguised.ID, foreign.ID} {
		if strings.Contains(raw, id.String()) {
			t.Fatalf("leaked private or non-image asset %s: %s", id, raw)
		}
	}
	if !strings.Contains(raw, wiki.AttachmentRef(own.ID, own.Filename)) {
		t.Fatalf("missing usable embed ref: %s", raw)
	}
	if got := e.want("GET", "/knowledge/assets?q=missing", nil, 200); len(got["images"].([]any)) != 0 {
		t.Fatalf("unrelated images: %v", got)
	}

	// The list is capped even when many images match.
	for i := 0; i < 15; i++ {
		put(e.me.ID, fmt.Sprintf("extra-map-%d.png", i), "image/png")
	}
	if got := e.want("GET", "/knowledge/assets", nil, 200); len(got["images"].([]any)) != 12 {
		t.Fatalf("uncapped suggestions: %v", got)
	}
}
