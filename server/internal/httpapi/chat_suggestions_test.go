package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/seed"
)

func TestListChatSuggestions(t *testing.T) {
	s, h := newServer(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, s.Pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	list := func() []api.ChatSuggestion {
		t.Helper()
		rec := get(h, "/api/v1/chat-suggestions")
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d: %s", rec.Code, rec.Body)
		}
		var body api.ChatSuggestionList
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Suggestions == nil {
			t.Fatal("suggestions is null")
		}
		return body.Suggestions
	}
	if got := list(); len(got) != 0 {
		t.Fatalf("before any batch: %+v", got)
	}
	me, err := s.Q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	insert := func(age string) {
		t.Helper()
		if _, err := s.Pool.Exec(ctx, `INSERT INTO chat_suggestions (workspace_id, user_id, items, generated_at) VALUES ($1, $2, '[{"title":"T","prompt":"P","icon":"dice"}]', now() - $3::interval)
			ON CONFLICT (workspace_id, user_id) DO UPDATE SET items = excluded.items, generated_at = excluded.generated_at`, me.WorkspaceID, me.ID, age); err != nil {
			t.Fatal(err)
		}
	}
	insert("1 hour")
	if got := list(); len(got) != 1 || got[0].Title != "T" || got[0].Icon != api.ChatSuggestionIconDice {
		t.Fatalf("batch: %+v", got)
	}
	insert("15 days")
	if got := list(); len(got) != 0 {
		t.Fatalf("stale batch: %+v", got)
	}
}
