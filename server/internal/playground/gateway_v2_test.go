package playground

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// app is one upstream process: it serves what pages says and records what
// each request carried.
type app struct {
	pages map[string]string // path → body; HTML is served as text/html
	seen  chan http.Header
}

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.seen <- r.Header.Clone()
	body, ok := a.pages[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(body, "<") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	io.WriteString(w, body)
}

func newAppGateway(t *testing.T, pages map[string]string, backend *fakeBackend, ref OpenAPIRef) (http.Handler, *Live, *app) {
	t.Helper()
	a := &app{pages: pages, seen: make(chan http.Header, 50)}
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	if backend == nil {
		backend = &fakeBackend{files: map[string]string{}}
	}
	g := &Gateway{
		Session: func(_ context.Context, token string) (bool, error) { return token == "good", nil },
		Origins: []string{vesperOrigin},
		Backend: backend,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	spec := Spec{Processes: []Process{{Name: "web", Command: "x", Port: 3000}}, OpenAPI: ref.Path, OpenAPIInferred: ref.Inferred, OpenAPICommit: ref.Commit}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	live := &Live{ID: uuid.New(), Spec: spec, Addrs: map[string]string{"web": strings.TrimPrefix(srv.URL, "http://")}}
	g.Serve(8091, live)
	return g.Handler(8091), live, a
}

func TestGatewayStampsEachRequestWithAnID(t *testing.T) {
	h, _, a := newAppGateway(t, map[string]string{"/api/runs": `{}`}, nil, OpenAPIRef{})
	rec := request(h, http.MethodGet, "/api/runs", signedIn)
	rid := rec.Header().Get("X-Request-Id")
	if seen := <-a.seen; rid == "" || seen.Get("X-Request-Id") != rid {
		t.Fatalf("browser got id %q, app saw %q: want one id, the same", rid, seen.Get("X-Request-Id"))
	}
	rec = request(h, http.MethodGet, "/api/runs", func(r *http.Request) { signedIn(r); r.Header.Set("X-Request-Id", "mine") })
	if <-a.seen; rec.Header().Get("X-Request-Id") != "mine" {
		t.Errorf("a caller's id was replaced: %q", rec.Header().Get("X-Request-Id"))
	}
}

func TestGatewayPrefersTheDocumentTheAppServes(t *testing.T) {
	backend := &fakeBackend{files: map[string]string{"api/openapi.yaml": "openapi: 3.0.3\npaths: {/from-file: {}}\n"}}
	h, _, _ := newAppGateway(t, map[string]string{"/v3/api-docs": `{"openapi":"3.0.1","paths":{"/served":{}}}`}, backend, OpenAPIRef{Path: "api/openapi.yaml"})
	rec := request(h, http.MethodGet, "/__vesper/openapi", signedIn)
	if src := rec.Header().Get("X-Vesper-Spec-Source"); src != "served:/v3/api-docs" || !strings.Contains(rec.Body.String(), "/served") {
		t.Fatalf("source %q body %s", src, rec.Body)
	}
}

func TestGatewayFallsBackToTheDeclaredOrInferredDocument(t *testing.T) {
	backend := &fakeBackend{files: map[string]string{".lazyagents/playground/openapi.json": `{"openapi":"3.1.0","paths":{"/inferred":{}}}`}}
	// The app serves JSON on a well-known path that isn't a document.
	h, live, _ := newAppGateway(t, map[string]string{"/openapi.json": `{"status":"ok"}`}, backend, OpenAPIRef{})
	if rec := request(h, http.MethodGet, "/__vesper/openapi", signedIn); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "/v3/api-docs") {
		t.Fatalf("with no document: %d %s", rec.Code, rec.Body)
	}
	// The persona inferred one: the API tab reads it without a restart.
	live.SetOpenAPI(OpenAPIRef{Path: ".lazyagents/playground/openapi.json", Inferred: true, Commit: "9f17116"})
	rec := request(h, http.MethodGet, "/__vesper/openapi", signedIn)
	if src := rec.Header().Get("X-Vesper-Spec-Source"); rec.Code != 200 || src != "inferred:.lazyagents/playground/openapi.json@9f17116" {
		t.Fatalf("inferred: %d %q %s", rec.Code, src, rec.Body)
	}
}

func TestGatewayPutsTheProbeOnlyInPreviewPages(t *testing.T) {
	page := `<!doctype html><html><HEAD lang="en"><title>Acme</title></head><body>hi</body></html>`
	h, _, a := newAppGateway(t, map[string]string{"/": page}, nil, OpenAPIRef{})
	framed := request(h, http.MethodGet, "/", func(r *http.Request) {
		signedIn(r)
		r.Header.Set("Sec-Fetch-Dest", "iframe")
		r.Header.Set("Accept-Encoding", "gzip")
	})
	<-a.seen
	body := framed.Body.String()
	if !strings.Contains(body, `<HEAD lang="en">`+probeTag+`<title>`) || framed.Header().Get("Content-Length") != "" && framed.Header().Get("Content-Length") != itoa(len(body)) {
		t.Fatalf("framed page %q (length %s)", body, framed.Header().Get("Content-Length"))
	}
	if top := request(h, http.MethodGet, "/", signedIn); strings.Contains(top.Body.String(), "probe.js") {
		t.Error("a page opened in its own tab got the probe")
	}
	<-a.seen
	script := request(h, http.MethodGet, "/__vesper/probe.js", signedIn)
	if !strings.Contains(script.Body.String(), `["`+vesperOrigin+`"]`) || !strings.HasPrefix(script.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("probe.js: %s %s", script.Header().Get("Content-Type"), script.Body.String()[:80])
	}
}

func TestGatewayAdmitsTheAgentWithItsToken(t *testing.T) {
	h, live, a := newAppGateway(t, map[string]string{"/": "<p>app</p>"}, nil, OpenAPIRef{})
	if live.Token == "" {
		t.Fatal("Serve gave the playground no token")
	}
	if rec := request(h, http.MethodGet, "/", func(r *http.Request) { r.Header.Set(agentHeader, "wrong") }); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token = %d", rec.Code)
	}
	rec := request(h, http.MethodGet, "/", func(r *http.Request) { r.Header.Set(agentHeader, live.Token) })
	if seen := <-a.seen; rec.Code != 200 || seen.Get(agentHeader) != "" {
		t.Fatalf("with the token: %d; app saw token %q", rec.Code, seen.Get(agentHeader))
	}
	// A browser trades the token for a cookie, then browses like the person.
	enter := request(h, http.MethodGet, "/__vesper/enter?token="+live.Token+"&next=/settings", nil)
	var cookie *http.Cookie
	for _, c := range enter.Result().Cookies() {
		if c.Name == agentCookie {
			cookie = c
		}
	}
	if enter.Code != http.StatusSeeOther || enter.Header().Get("Location") != "/settings" || cookie == nil || !cookie.HttpOnly {
		t.Fatalf("enter: %d %q cookie %+v", enter.Code, enter.Header().Get("Location"), cookie)
	}
	rec = request(h, http.MethodGet, "/", func(r *http.Request) { r.AddCookie(cookie); r.AddCookie(&http.Cookie{Name: "app", Value: "1"}) })
	if seen := <-a.seen; rec.Code != 200 || seen.Get("Cookie") != "app=1" {
		t.Fatalf("with the cookie: %d; app saw Cookie %q", rec.Code, seen.Get("Cookie"))
	}
	for _, next := range []string{"//evil.example", "https://evil.example", `/\evil.example`} {
		if loc := request(h, http.MethodGet, "/__vesper/enter?token="+live.Token+"&next="+next, nil).Header().Get("Location"); loc != "/" {
			t.Errorf("enter sent next=%s to %q", next, loc)
		}
	}
	if rec := request(h, http.MethodGet, "/__vesper/enter?token=nope", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("enter with a bad token = %d", rec.Code)
	}
}

func TestGatewayReportsTheCheckedOutCommit(t *testing.T) {
	backend := &fakeBackend{files: map[string]string{
		".git/HEAD":        "ref: refs/heads/feat/x\n",
		".git/packed-refs": "# pack-refs\n0123456789abcdef0123456789abcdef01234567 refs/heads/feat/x\n",
	}}
	h, _, _ := newAppGateway(t, nil, backend, OpenAPIRef{})
	var meta map[string]string
	rec := request(h, http.MethodGet, "/__vesper/meta", signedIn)
	if json.Unmarshal(rec.Body.Bytes(), &meta) != nil || meta["branch"] != "feat/x" || meta["commit"] != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("meta %s", rec.Body)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

// The API tab reads which document it got from Vesper's page, another
// origin: the header has to be exposed to it, or the browser hides it.
func TestGatewayExposesTheSpecSourceToVesper(t *testing.T) {
	backend := &fakeBackend{files: map[string]string{"api/openapi.yaml": "openapi: 3.0.3\npaths: {}\n"}}
	h, _, _ := newAppGateway(t, nil, backend, OpenAPIRef{Path: "api/openapi.yaml"})
	rec := request(h, http.MethodGet, "/__vesper/openapi", func(r *http.Request) { signedIn(r); r.Header.Set("Origin", vesperOrigin) })
	if rec.Header().Get("Access-Control-Allow-Origin") != vesperOrigin || !strings.Contains(rec.Header().Get("Access-Control-Expose-Headers"), "X-Vesper-Spec-Source") {
		t.Fatalf("CORS headers %v", rec.Header())
	}
}
