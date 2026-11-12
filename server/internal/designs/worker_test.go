package designs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestWorkerClient(t *testing.T) {
	var busy atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/render":
			if busy.Add(-1) >= 0 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("X-Design-Width", "10")
			w.Header().Set("X-Design-Height", "20")
			_, _ = w.Write([]byte("png"))
		case "/layout":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"pages":[{"id":"p_a","boxes":{"n_a":{"x":1,"y":2,"w":3,"h":4}}}]}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"nope"}`))
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	w := &Worker{URL: srv.URL, Token: "tok", Backoff: 1}

	busy.Store(1) // one 503, then it renders: the client retries once
	r, err := w.Render(ctx, RenderRequest{Body: "{}"})
	if err != nil || string(r.PNG) != "png" || r.W != 10 || r.H != 20 {
		t.Fatalf("render: %+v %v", r, err)
	}
	busy.Store(2) // still busy after the retry: unavailable
	if _, err := w.Render(ctx, RenderRequest{Body: "{}"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("busy twice: %v", err)
	}
	pages, err := w.Layout(ctx, "{}", "")
	if err != nil || pages[0].Boxes["n_a"].W != 3 {
		t.Fatalf("layout: %+v %v", pages, err)
	}

	if _, err := (&Worker{URL: srv.URL, Token: "wrong"}).Layout(ctx, "{}", ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("a refused token is an outage, not the design's fault: %v", err)
	}
	if _, err := (&Worker{URL: srv.URL}).Layout(ctx, "{}", ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no token: %v", err)
	}
	var nilWorker *Worker
	if nilWorker.Available() {
		t.Fatal("a nil worker is available")
	}
	if _, err := (&Worker{URL: "http://127.0.0.1:1", Token: "tok"}).Layout(ctx, "{}", ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unreachable: %v", err)
	}
	bad := &Worker{URL: srv.URL + "/x", Token: "tok"}
	var rej *RejectedError
	if _, err := bad.Layout(ctx, "{}", ""); !errors.As(err, &rej) || rej.Message != "nope" {
		t.Fatalf("a 400 carries the worker's message: %v", err)
	}
}
