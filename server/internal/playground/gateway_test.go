package playground

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/auth"
)

const vesperOrigin = "http://localhost:3000"

type fakeBackend struct {
	files map[string]string
	lines []LogLine
}

func (f *fakeBackend) ReadFile(_ context.Context, _ uuid.UUID, path string) ([]byte, error) {
	if b, ok := f.files[path]; ok {
		return []byte(b), nil
	}
	return nil, io.ErrUnexpectedEOF
}

func (f *fakeBackend) Logs(_ context.Context, _ uuid.UUID, tail int, _ bool, emit func(LogLine) error) error {
	for _, l := range f.lines {
		if err := emit(l); err != nil {
			return err
		}
	}
	return nil
}

// seen is what an upstream process was sent.
type seen struct {
	host, cookie, origin, path string
}

func upstream(t *testing.T, name string, got chan<- seen, header http.Header) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- seen{host: r.Host, cookie: r.Header.Get("Cookie"), origin: r.Header.Get("Origin"), path: r.URL.Path}
		for k, vs := range header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		io.WriteString(w, name)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func newGateway(t *testing.T, header http.Header) (http.Handler, *fakeBackend, chan seen) {
	t.Helper()
	got := make(chan seen, 10)
	backend := &fakeBackend{files: map[string]string{}}
	g := &Gateway{
		Session: func(_ context.Context, token string) (bool, error) { return token == "good", nil },
		Origins: []string{vesperOrigin},
		Backend: backend,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	spec := vesperSpec()
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	g.Serve(8091, &Live{ID: uuid.New(), Spec: spec, Addrs: map[string]string{
		"api": upstream(t, "api", got, header),
		"web": upstream(t, "web", got, header),
	}})
	return g.Handler(8091), backend, got
}

func request(h http.Handler, method, path string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://localhost:8091"+path, nil)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func signedIn(r *http.Request) {
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: "good"})
	r.AddCookie(&http.Cookie{Name: "app_session", Value: "abc"})
}

func TestGatewayServesOnlyASignedInVesperBrowser(t *testing.T) {
	h, _, got := newGateway(t, nil)
	for name, mutate := range map[string]func(*http.Request){
		"no cookie": nil,
		"forged":    func(r *http.Request) { r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: "bad"}) },
	} {
		if rec := request(h, http.MethodGet, "/", mutate); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d, want 401", name, rec.Code)
		}
	}
	select {
	case s := <-got:
		t.Fatalf("the app was reached without a session: %+v", s)
	default:
	}
}

func TestGatewayRoutesAndKeepsVesperSessionFromTheApp(t *testing.T) {
	h, _, got := newGateway(t, http.Header{
		"Set-Cookie":              {auth.SessionCookie + "=planted; Path=/", "app_session=new; Path=/"},
		"X-Frame-Options":         {"DENY"},
		"Content-Security-Policy": {"default-src 'self'; frame-ancestors 'none'"},
	})
	rec := request(h, http.MethodGet, "/api/v1/runs", signedIn)
	if rec.Code != http.StatusOK || rec.Body.String() != "api" {
		t.Fatalf("GET /api/v1/runs = %d %q, want api's answer", rec.Code, rec.Body)
	}
	s := <-got
	if strings.Contains(s.cookie, auth.SessionCookie) || !strings.Contains(s.cookie, "app_session=abc") {
		t.Errorf("app got Cookie %q: want its own cookie and not Vesper's session", s.cookie)
	}
	if s.host != "localhost:8091" {
		t.Errorf("app saw Host %q, want the address the browser used", s.host)
	}
	if cookies := rec.Header().Values("Set-Cookie"); len(cookies) != 1 || !strings.HasPrefix(cookies[0], "app_session=") {
		t.Errorf("Set-Cookie = %q: want the app's own and not a Vesper session", cookies)
	}
	if rec.Header().Get("X-Frame-Options") != "" || strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors") {
		t.Errorf("framing still refused: %v", rec.Header())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("a request from the app's own page got CORS headers")
	}

	if rec := request(h, http.MethodGet, "/chat/1", signedIn); rec.Body.String() != "web" {
		t.Errorf("GET /chat/1 = %q, want web's answer", rec.Body)
	}
}

func TestGatewayLetsOnlyVesperReadAcrossOrigins(t *testing.T) {
	h, _, got := newGateway(t, http.Header{"Access-Control-Allow-Origin": {"*"}, "X-Total": {"3"}})
	// The preflight carries no cookie; the gateway answers it for Vesper.
	pre := request(h, http.MethodOptions, "/api/v1/runner/test-environments", func(r *http.Request) {
		r.Header.Set("Origin", vesperOrigin)
		r.Header.Set("Access-Control-Request-Method", "PUT")
		r.Header.Set("Access-Control-Request-Headers", "content-type")
	})
	if pre.Code != http.StatusNoContent || pre.Header().Get("Access-Control-Allow-Origin") != vesperOrigin ||
		pre.Header().Get("Access-Control-Allow-Methods") != "PUT" || pre.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("preflight = %d %v", pre.Code, pre.Header())
	}

	rec := request(h, http.MethodPut, "/api/v1/runner/test-environments", func(r *http.Request) {
		signedIn(r)
		r.Header.Set("Origin", vesperOrigin)
	})
	if rec.Header().Get("Access-Control-Allow-Origin") != vesperOrigin || !strings.Contains(rec.Header().Get("Access-Control-Expose-Headers"), "X-Total") {
		t.Errorf("Vesper's request: CORS %v", rec.Header())
	}
	if s := <-got; s.origin != "http://localhost:8091" {
		t.Errorf("app saw Origin %q, want its own, as from its own page", s.origin)
	}

	other := request(h, http.MethodGet, "/api/v1/runs", func(r *http.Request) {
		signedIn(r)
		r.Header.Set("Origin", "http://localhost:5173")
	})
	<-got
	if other.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("another origin got the gateway's CORS answer, not the app's: %v", other.Header())
	}
}

func TestGatewayServesTheOpenAPIDocumentAsJSON(t *testing.T) {
	h, backend, _ := newGateway(t, nil)
	backend.files["api/openapi.yaml"] = "openapi: 3.0.3\npaths:\n  /runs:\n    get:\n      responses:\n        200: { description: ok }\n"
	rec := request(h, http.MethodGet, "/__vesper/openapi", signedIn)
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]any
		}
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &doc) != nil || doc.Paths["/runs"]["get"].Responses["200"] == nil {
		t.Fatalf("GET /__vesper/openapi = %d %s", rec.Code, rec.Body)
	}
	if rec := request(h, http.MethodGet, "/__vesper/openapi", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("the document without a session = %d, want 401", rec.Code)
	}
}

func TestGatewayStreamsLogsAsServerSentEvents(t *testing.T) {
	h, backend, _ := newGateway(t, nil)
	at := time.Date(2026, 11, 10, 17, 44, 19, 0, time.UTC)
	backend.lines = []LogLine{{Time: at, Source: "api", Stream: "stderr", Text: `null value in column "packages"`}}
	rec := request(h, http.MethodGet, "/__vesper/logs?tail=10&follow=0", signedIn)
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	sc := bufio.NewScanner(rec.Body)
	sc.Scan()
	var l LogLine
	if err := json.Unmarshal([]byte(strings.TrimPrefix(sc.Text(), "data: ")), &l); err != nil || l.Source != "api" || !l.Time.Equal(at) {
		t.Fatalf("first event %q: %v %+v", sc.Text(), err, l)
	}
}

func TestGatewayAnswersAPortWithNothingOnIt(t *testing.T) {
	g := &Gateway{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if rec := request(g.Handler(8092), http.MethodGet, "/", signedIn); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("= %d, want 503", rec.Code)
	}
}
