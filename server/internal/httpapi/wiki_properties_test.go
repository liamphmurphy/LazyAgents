package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
)

func TestWikiPropertiesAPI(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	wk := &wiki.Service{Pool: pool, Artifacts: arts}
	handler := (&Server{AnonymousAPI: true, Pool: pool, Q: q, Artifacts: arts, Wiki: wk}).Handler()
	request := func(method, path, body string, status int, out any) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(rec, req)
		if rec.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
		}
		if out != nil {
			if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
	}
	body := `{"title":"Display [ ] # | ^", "body_md":"body #inline", "properties":{"text":"before\n---\nafter\n","number":1.5,"date":"2026-11-13","checkbox":true,"list":["one","two"],"link":"[[Target]]"}, "tags":["Explicit"],"aliases":["An alias"]}`
	var a, b api.Artifact
	request("POST", "/wiki/pages", body, 201, &a)
	request("POST", "/wiki/pages", `{"title":"Display [ ] # | ^","body_md":"other"}`, 201, &b)
	if a.Title != b.Title || *a.Path == *b.Path {
		t.Fatal("title/path separation failed")
	}
	if a.Properties == nil || (*a.Properties)["number"] != 1.5 || (*a.Properties)["date"] != "2026-11-13" || (*a.Properties)["checkbox"] != true || (*a.Properties)["link"] != "[[Target]]" || !reflect.DeepEqual((*a.Properties)["list"], []any{"one", "two"}) {
		t.Fatalf("properties response: %+v", a.Properties)
	}
	if a.Tags == nil || !reflect.DeepEqual(*a.Tags, []string{"explicit", "inline"}) || a.Aliases == nil || !reflect.DeepEqual(*a.Aliases, []string{"An alias"}) {
		t.Fatalf("metadata response: %+v", a)
	}
	var got api.Artifact
	request("GET", "/wiki/page?path=An%20alias", "", 200, &got)
	if got.Id != a.Id {
		t.Fatal("alias read")
	}
	// The actual API property payload must survive the HTTP export unchanged,
	// including an indented delimiter in YAML's literal scalar representation.
	exported := httptest.NewRecorder()
	handler.ServeHTTP(exported, httptest.NewRequest("GET", "/api/v1/wiki/export", nil))
	if exported.Code != 200 {
		t.Fatalf("export: %d %s", exported.Code, exported.Body.String())
	}
	me, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := wk.Import(ctx, me.WorkspaceID, me.ID, exported.Body.Bytes())
	if err != nil || imported.Updated != 0 || imported.Created != 0 {
		t.Fatalf("API-property YAML roundtrip: %+v %v", imported, err)
	}
	var roundtrip api.Artifact
	request("GET", "/wiki/page?path=An%20alias", "", 200, &roundtrip)
	if roundtrip.BodyMd.MustGet() != a.BodyMd.MustGet() || roundtrip.CurrentVersion.Id != a.CurrentVersion.Id || (*roundtrip.Properties)["text"] != "before\n---\nafter\n" {
		t.Fatal("YAML separator corrupted API property/body")
	}
	request("POST", "/wiki/pages", `{"path":"An alias","body_md":"cannot steal"}`, 422, nil)
	var bodyEdited api.Artifact
	request("PUT", "/wiki/pages/"+a.Id.String(), `{"body_md":"body #inline edited"}`, 200, &bodyEdited)
	a = bodyEdited
	var tree api.WikiTree
	request("GET", "/wiki/pages?tag=INLINE", "", 200, &tree)
	if len(tree.Pages) != 1 || tree.Pages[0].Id != a.Id {
		t.Fatalf("tag filter: %+v", tree)
	}
	// Metadata-only edits retain the body and form a new version.
	var edited api.Artifact
	request("PUT", "/wiki/pages/"+a.Id.String(), `{"properties":{"number":2,"checkbox":false},"tags":[],"aliases":[]}`, 200, &edited)
	if edited.BodyMd.MustGet() != a.BodyMd.MustGet() || edited.CurrentVersion.Id == a.CurrentVersion.Id || (*edited.Properties)["checkbox"] != false {
		t.Fatalf("metadata edit: %+v", edited)
	}
	got = api.Artifact{}
	request("PUT", "/wiki/pages/"+a.Id.String(), `{"body_md":"edited #next"}`, 200, &got)
	if !reflect.DeepEqual(*got.Properties, *edited.Properties) {
		t.Fatal("body-only save lost properties")
	}
	var invalid api.Error
	request("PUT", "/wiki/pages/"+a.Id.String(), `{"path":"Should roll back","title":"Should roll back","properties":{"bad":[1,2]}}`, 422, &invalid)
	request("GET", "/wiki/page?path=Should%20roll%20back", "", 404, nil)
	if invalid.Code != api.Invalid {
		t.Fatal(invalid)
	}
	request("POST", "/wiki/pages", `{"path":"Collision","body_md":"bad","aliases":["Unique collision alias"]}`, 201, nil)
	request("POST", "/wiki/pages", `{"path":"Another collision","body_md":"bad","aliases":["Unique collision alias"]}`, 422, &invalid)
	request("PUT", "/wiki/pages/"+a.Id.String(), `{"properties":{"number":3},"base_version_id":"`+a.CurrentVersion.Id.String()+`"}`, 409, &invalid)
	request("GET", "/wiki/page?path=Missing", "", 404, &invalid)
	var renamed api.Artifact
	request("PUT", "/wiki/pages/"+a.Id.String(), `{"title":"Updated [title]"}`, 200, &renamed)
	if *renamed.Path != "Updated title" || renamed.Title != "Updated [title]" {
		t.Fatalf("HTTP title rename: %+v", renamed)
	}
	var oldPath api.Artifact
	request("GET", "/wiki/page?path=Display", "", 200, &oldPath)
	if oldPath.Id != a.Id {
		t.Fatal("old path lost on title rename")
	}
	longTitle, _ := json.Marshal(map[string]any{"title": strings.Repeat("x", 201)})
	request("PUT", "/wiki/pages/"+a.Id.String(), string(longTitle), 422, nil)

}
