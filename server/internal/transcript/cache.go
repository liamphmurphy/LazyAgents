package transcript

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"time"
)

// A person's link starts the fetch as soon as the message is in, and the chat's
// get_video_transcript joins it or reads the result. Only successes are kept.
const (
	cacheTTL     = time.Hour
	cacheEntries = 32
	// fetchTimeout bounds a whole fetch, which outlives whoever asked first.
	fetchTimeout = time.Minute
	// MaxPrefetch is how many videos one message can start fetching.
	MaxPrefetch = 3
)

// Shared is the fetcher the chat tool and message prefetch have in common.
var Shared = NewYouTube()

var linkRe = regexp.MustCompile(`https?://[^\s<>"']+`)

// flight is a fetch underway (until is zero) or a finished one.
type flight struct {
	done  chan struct{}
	t     Transcript
	err   error
	until time.Time
}

func cacheKey(id, lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		lang = "en"
	}
	return id + "/" + lang
}

// start joins the live or cached fetch for a video, or begins one.
func (y *YouTube) start(id, lang string) (f *flight, joined bool) {
	key := cacheKey(id, lang)
	y.mu.Lock()
	defer y.mu.Unlock()
	if have, ok := y.flights[key]; ok && (have.until.IsZero() || time.Now().Before(have.until)) {
		return have, true
	}
	if y.flights == nil {
		y.flights = map[string]*flight{}
	}
	y.evict()
	f = &flight{done: make(chan struct{})}
	y.flights[key] = f
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		t, err := y.fetch(ctx, id, lang)
		y.mu.Lock()
		f.t, f.err = t, err
		if err != nil {
			delete(y.flights, key)
		} else {
			f.until = time.Now().Add(cacheTTL)
		}
		y.mu.Unlock()
		close(f.done)
	}()
	return f, false
}

// evict makes room for one entry, oldest first. The caller holds y.mu.
func (y *YouTube) evict() {
	now := time.Now()
	for key, f := range y.flights {
		if !f.until.IsZero() && now.After(f.until) {
			delete(y.flights, key)
		}
	}
	for len(y.flights) >= cacheEntries {
		oldest := ""
		for key, f := range y.flights {
			if !f.until.IsZero() && (oldest == "" || f.until.Before(y.flights[oldest].until)) {
				oldest = key
			}
		}
		if oldest == "" {
			return // all still running
		}
		delete(y.flights, oldest)
	}
}

// Fetch loads the transcript of a video in the preferred language (a code like
// "en"; empty prefers English), falling back to whatever the video has. It
// reuses a recent or running fetch; if one it joined failed, it tries once
// itself.
func (y *YouTube) Fetch(ctx context.Context, id, lang string) (Transcript, error) {
	t, joined, err := y.wait(ctx, id, lang)
	if err != nil && joined && ctx.Err() == nil {
		t, _, err = y.wait(ctx, id, lang)
	}
	return t, err
}

func (y *YouTube) wait(ctx context.Context, id, lang string) (Transcript, bool, error) {
	f, joined := y.start(id, lang)
	select {
	case <-f.done:
		return f.t, joined, f.err
	case <-ctx.Done():
		return Transcript{VideoID: id}, joined, ctx.Err()
	}
}

// VideoIDs are the distinct YouTube videos linked in text, at most limit.
func VideoIDs(text string, limit int) []string {
	var ids []string
	for _, raw := range linkRe.FindAllString(text, -1) {
		id, err := VideoID(strings.TrimRight(raw, ".,;:!?)]}*`"))
		if err == nil && !slices.Contains(ids, id) {
			if ids = append(ids, id); len(ids) == limit {
				break
			}
		}
	}
	return ids
}

// Prefetch starts fetching the videos linked in text and returns at once.
func (y *YouTube) Prefetch(text string) {
	for _, id := range VideoIDs(text, MaxPrefetch) {
		y.start(id, "")
	}
}
