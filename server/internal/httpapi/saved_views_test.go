package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/seed"
)

func sendJSON(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	h.ServeHTTP(rec, req)
	return rec
}

// Saved views are per person: create, rename, replace the query and delete,
// with case-insensitive unique names, and another person's views stay hidden.
func TestSavedViewsCRUD(t *testing.T) {
	ctx := context.Background()
	s, h := newServer(t)
	if _, err := seed.Run(ctx, s.Pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}

	type view struct {
		ID    uuid.UUID `json:"id"`
		Name  string    `json:"name"`
		Query string    `json:"query"`
	}
	decode := func(rec *httptest.ResponseRecorder, want int, v any) {
		t.Helper()
		if rec.Code != want {
			t.Fatalf("code = %d, want %d: %s", rec.Code, want, rec.Body)
		}
		if v != nil {
			if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
				t.Fatal(err)
			}
		}
	}

	var a view
	decode(sendJSON(h, http.MethodPost, "/api/v1/saved-views", `{"name":"  UX epic triage ","query":"?view=active&group=epic"}`), http.StatusCreated, &a)
	if a.Name != "UX epic triage" || a.Query != "view=active&group=epic" {
		t.Fatalf("created = %+v, want trimmed name and query", a)
	}
	var b view
	decode(sendJSON(h, http.MethodPost, "/api/v1/saved-views", `{"name":"Blocked","query":"blocked=true"}`), http.StatusCreated, &b)

	// Duplicate names (ignoring case) are 409, on create and on rename.
	decode(sendJSON(h, http.MethodPost, "/api/v1/saved-views", `{"name":"blocked","query":""}`), http.StatusConflict, nil)
	decode(sendJSON(h, http.MethodPatch, "/api/v1/saved-views/"+a.ID.String(), `{"name":"BLOCKED"}`), http.StatusConflict, nil)
	decode(sendJSON(h, http.MethodPost, "/api/v1/saved-views", `{"name":"   ","query":""}`), http.StatusUnprocessableEntity, nil)
	decode(sendJSON(h, http.MethodPost, "/api/v1/saved-views", `{"name":"a\u0000b","query":""}`), http.StatusUnprocessableEntity, nil)
	decode(sendJSON(h, http.MethodPost, "/api/v1/saved-views", `{"name":"ok","query":"q=\u0000"}`), http.StatusUnprocessableEntity, nil)

	// Rename keeps the query; a query-only patch keeps the name.
	var renamed view
	decode(sendJSON(h, http.MethodPatch, "/api/v1/saved-views/"+a.ID.String(), `{"name":"UX triage"}`), http.StatusOK, &renamed)
	if renamed.Name != "UX triage" || renamed.Query != a.Query {
		t.Fatalf("renamed = %+v", renamed)
	}
	decode(sendJSON(h, http.MethodPatch, "/api/v1/saved-views/"+a.ID.String(), `{"query":"group=priority"}`), http.StatusOK, &renamed)
	if renamed.Name != "UX triage" || renamed.Query != "group=priority" {
		t.Fatalf("query patch = %+v", renamed)
	}

	// Another person's view is invisible and untouchable.
	var other uuid.UUID
	if err := s.Pool.QueryRow(ctx, `
		WITH u AS (
			INSERT INTO users(workspace_id,name,email,initials,password_hash)
			SELECT workspace_id,'Other Person','other@example.com','OP','x' FROM users LIMIT 1
			RETURNING id, workspace_id
		)
		INSERT INTO saved_views(workspace_id,user_id,name,query) SELECT workspace_id,id,'Blocked','' FROM u RETURNING id`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	decode(sendJSON(h, http.MethodPatch, "/api/v1/saved-views/"+other.String(), `{"name":"Mine now"}`), http.StatusNotFound, nil)
	decode(sendJSON(h, http.MethodDelete, "/api/v1/saved-views/"+other.String(), ""), http.StatusNotFound, nil)

	var list struct{ Views []view }
	decode(get(h, "/api/v1/saved-views"), http.StatusOK, &list)
	if len(list.Views) != 2 || list.Views[0].Name != "Blocked" || list.Views[1].Name != "UX triage" {
		t.Fatalf("list = %+v, want Blocked then UX triage", list.Views)
	}

	decode(sendJSON(h, http.MethodDelete, "/api/v1/saved-views/"+b.ID.String(), ""), http.StatusNoContent, nil)
	decode(sendJSON(h, http.MethodDelete, "/api/v1/saved-views/"+b.ID.String(), ""), http.StatusNotFound, nil)
	decode(get(h, "/api/v1/saved-views"), http.StatusOK, &list)
	if len(list.Views) != 1 || list.Views[0].ID != a.ID {
		t.Fatalf("after delete = %+v", list.Views)
	}
}
