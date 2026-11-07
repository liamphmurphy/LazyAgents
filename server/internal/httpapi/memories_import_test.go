package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/seed"
)

type importResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Failed  []struct {
		Index int    `json:"index"`
		Title string `json:"title"`
		Error string `json:"error"`
	} `json:"failed"`
}

// Importing saves each entry as the signed-in person with source "import",
// folds near-duplicate titles into the existing entry, reports bad entries by
// index without failing the batch, and bounds the batch size.
func TestImportMemories(t *testing.T) {
	ctx := context.Background()
	s, h := newServer(t)
	if _, err := seed.Run(ctx, s.Pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	doImport := func(body string, want int) importResult {
		t.Helper()
		rec := sendJSON(h, http.MethodPost, "/api/v1/memories/import", body)
		if rec.Code != want {
			t.Fatalf("code = %d, want %d: %s", rec.Code, want, rec.Body)
		}
		var res importResult
		if want == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
				t.Fatal(err)
			}
		}
		return res
	}

	res := doImport(`{"memories":[
		{"title":"Prefers dark mode","summary":"Likes dark themes everywhere","tags":["#UI"," Theme "],"type":"preference"},
		{"title":"Dog is called Biscuit"},
		{"title":"Quarterly planning ritual","body":"Plan on the first Monday of each quarter."}]}`, http.StatusOK)
	if res.Created != 3 || res.Updated != 0 || len(res.Failed) != 0 {
		t.Fatalf("first import = %+v, want 3 created", res)
	}

	var list struct {
		Memories []struct {
			Title  string   `json:"title"`
			Type   string   `json:"type"`
			Source string   `json:"source"`
			Tags   []string `json:"tags"`
		} `json:"memories"`
	}
	list1 := func() {
		t.Helper()
		rec := get(h, "/api/v1/memories")
		if rec.Code != http.StatusOK {
			t.Fatalf("list code = %d: %s", rec.Code, rec.Body)
		}
		list.Memories = nil
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
	}
	list1()
	if len(list.Memories) != 3 {
		t.Fatalf("listed %d memories, want 3: %s", len(list.Memories), get(h, "/api/v1/memories").Body)
	}
	for _, m := range list.Memories {
		if m.Source != "import" {
			t.Errorf("%q source = %q, want import", m.Title, m.Source)
		}
		switch m.Title {
		case "Prefers dark mode":
			if m.Type != "preference" || !slices.Equal(m.Tags, []string{"ui", "theme"}) {
				t.Errorf("dark mode = type %q tags %v, want preference [ui theme]", m.Type, m.Tags)
			}
		case "Dog is called Biscuit":
			if m.Type != "other" {
				t.Errorf("untyped entry type = %q, want other", m.Type)
			}
		}
	}

	res = doImport(fmt.Sprintf(`{"memories":[
		{"title":"Prefers dark mode!"},
		{"title":"   "},
		{"title":"Deploy notes","body":"key is %s"},
		{"title":"Favourite editor is Neovim"}]}`, "AKIAABCDEFGHIJKLMNOP"), http.StatusOK)
	if res.Created != 1 || res.Updated != 1 || len(res.Failed) != 2 {
		t.Fatalf("second import = %+v, want 1 created, 1 updated, 2 failed", res)
	}
	if f := res.Failed[0]; f.Index != 1 || f.Error == "" {
		t.Errorf("failed[0] = %+v, want index 1 (empty title)", f)
	}
	if f := res.Failed[1]; f.Index != 2 || f.Title != "Deploy notes" || !strings.Contains(strings.ToLower(f.Error), "aws") {
		t.Errorf("failed[1] = %+v, want index 2 refused as an AWS key", f)
	}
	list1()
	if len(list.Memories) != 4 {
		t.Fatalf("listed %d memories after second import, want 4", len(list.Memories))
	}

	doImport(`{"memories":[]}`, http.StatusUnprocessableEntity)
	over := strings.TrimSuffix(strings.Repeat(`{"title":"x"},`, 201), ",")
	doImport(`{"memories":[`+over+`]}`, http.StatusUnprocessableEntity)
}
