package transcript

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

const testVideo = "dQw4w9WgXcQ"

// captionServer serves one video; release, when set, holds the watch page.
func captionServer(t *testing.T, watches *atomic.Int32, release chan struct{}, failFirst bool) *YouTube {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/watch":
			n := watches.Add(1)
			if release != nil {
				<-release
			}
			if failFirst && n == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
			}
		case "/youtubei/v1/player":
			_, _ = io.WriteString(w, `{"playabilityStatus":{"status":"OK"},"videoDetails":{"title":"x"},"captions":{"playerCaptionsTracklistRenderer":{"captionTracks":[{"baseUrl":"`+srv.URL+`/api/timedtext?v=1","languageCode":"en"}]}}}`)
		case "/api/timedtext":
			_, _ = io.WriteString(w, `<transcript><text start="1" dur="1">hello</text></transcript>`)
		}
	}))
	t.Cleanup(srv.Close)
	return &YouTube{HTTP: srv.Client(), BaseURL: srv.URL}
}

func TestPrefetchIsJoinedAndCached(t *testing.T) {
	var watches atomic.Int32
	release := make(chan struct{})
	y := captionServer(t, &watches, release, false)
	y.Prefetch("watch https://youtu.be/" + testVideo + ".")
	got := make(chan error, 1)
	go func() {
		// "en" is what an empty language means, so it shares the fetch.
		_, err := y.Fetch(context.Background(), testVideo, "en")
		got <- err
	}()
	time.Sleep(20 * time.Millisecond)
	close(release)
	if err := <-got; err != nil {
		t.Fatal(err)
	}
	if tr, err := y.Fetch(context.Background(), testVideo, ""); err != nil || len(tr.Lines) != 1 {
		t.Fatalf("cached fetch = %+v, %v", tr, err)
	}
	if n := watches.Load(); n != 1 {
		t.Fatalf("fetched %d times, want 1", n)
	}
}

func TestFailedPrefetchFallsBackToOwnFetch(t *testing.T) {
	var watches atomic.Int32
	release := make(chan struct{})
	y := captionServer(t, &watches, release, true)
	y.Prefetch("https://youtu.be/" + testVideo)
	got := make(chan error, 1)
	go func() {
		_, err := y.Fetch(context.Background(), testVideo, "")
		got <- err
	}()
	time.Sleep(20 * time.Millisecond)
	close(release)
	if err := <-got; err != nil {
		t.Fatalf("own fetch should have succeeded: %v", err)
	}
	if n := watches.Load(); n != 2 {
		t.Fatalf("fetched %d times, want 2", n)
	}
}

func TestFetchStopsWaitingWithItsContext(t *testing.T) {
	var watches atomic.Int32
	release := make(chan struct{})
	defer close(release)
	y := captionServer(t, &watches, release, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := y.Fetch(ctx, testVideo, ""); err == nil {
		t.Fatal("want a context error")
	}
}

func TestEvictKeepsCacheBounded(t *testing.T) {
	y := &YouTube{flights: map[string]*flight{}}
	base := time.Now().Add(time.Hour)
	for i := range cacheEntries {
		y.flights[string(rune('a'+i))] = &flight{until: base.Add(time.Duration(i) * time.Minute)}
	}
	y.evict()
	if _, ok := y.flights["a"]; ok || len(y.flights) != cacheEntries-1 {
		t.Fatalf("after evict: %d entries, oldest kept: %v", len(y.flights), ok)
	}
}

func TestVideoIDs(t *testing.T) {
	other := "aaaaaaaaaaa"
	text := "see (https://youtu.be/" + testVideo + "), https://youtube.com/watch?v=" + testVideo + " and https://example.com/x and https://youtu.be/" + other + " https://youtu.be/bbbbbbbbbbb"
	if got, want := VideoIDs(text, 2), []string{testVideo, other}; !reflect.DeepEqual(got, want) {
		t.Fatalf("VideoIDs = %v, want %v", got, want)
	}
	if got := VideoIDs("https://youtu.be/abcdefghij_", 3); !reflect.DeepEqual(got, []string{"abcdefghij_"}) {
		t.Fatalf("id ending in underscore = %v", got)
	}
}
