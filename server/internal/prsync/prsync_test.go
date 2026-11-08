package prsync_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/prsync"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func TestMergedAndClosed(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	for key, n := range map[string]int{"PAY-214": 491, "PAY-224": 492} {
		tk, _ := tracker.Get(ctx, q, ws, key)
		url := "https://github.com/northwind/payments/pull/" + map[int]string{491: "491", 492: "492"}[n]
		num := int32(n)
		open := "open"
		if err := q.SetTicketPR(ctx, store.SetTicketPRParams{ID: tk.ID, PrNumber: &num, PrUrl: &url, PrState: &open}); err != nil {
			t.Fatal(err)
		}
	}
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/repos/northwind/payments/pulls/491"):
			_, _ = io.WriteString(w, `{"state":"closed","merged":true,"draft":false,"additions":120,"deletions":14}`)
		case strings.HasSuffix(r.URL.Path, "/pulls/492"):
			_, _ = io.WriteString(w, `{"state":"closed","merged":false,"additions":3,"deletions":1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer gh.Close()
	s := &prsync.Syncer{Pool: pool, Tracker: &tracker.Service{Pool: pool}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), BaseURL: gh.URL}
	if n, err := s.Once(ctx); err != nil || n < 2 {
		t.Fatalf("once: %d %v", n, err)
	}
	merged, _ := tracker.Get(ctx, q, ws, "PAY-214")
	if merged.Status != "done" || *merged.PrState != "merged" || *merged.PrAdditions != 120 {
		t.Fatalf("merged ticket: %s %v", merged.Status, *merged.PrState)
	}
	closed, _ := tracker.Get(ctx, q, ws, "PAY-224")
	if closed.Status == "done" || *closed.PrState != "closed" || !closed.PrClosedUnmerged {
		t.Fatalf("closed ticket: %s %v %v", closed.Status, *closed.PrState, closed.PrClosedUnmerged)
	}
	evs, _ := q.ListTicketActivity(ctx, &merged.ID)
	verbs := []string{}
	for _, e := range evs {
		verbs = append(verbs, e.Verb)
	}
	if !strings.Contains(strings.Join(verbs, ","), runs.VerbPRMerged+",status_changed") {
		t.Fatalf("verbs %v", verbs)
	}
	// Merged and closed PRs aren't polled again.
	if n, _ := s.Once(ctx); n != 0 {
		t.Fatalf("re-polled %d", n)
	}
}
