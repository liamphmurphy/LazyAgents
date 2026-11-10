package playground

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/liamphmurphy/vesper/server/internal/auth"
)

// The gateway serves each running playground on a port of the runner's own,
// on the same host as Vesper. Same host, other port: another origin, so the
// app's scripts can't read Vesper's API (and Vesper refuses their writes,
// httpapi.crossOriginWrite), but the same site, so the app's own cookies work
// inside Vesper's iframe and the browser sends Vesper's session cookie along.
// That cookie is how the gateway knows the person: nothing else gets in, and
// the app never sees it.

// LogLine is one line a playground's container wrote.
type LogLine struct {
	Time   time.Time `json:"t"`
	Source string    `json:"source"` // setup, a process or a service
	Stream string    `json:"stream"` // stdout or stderr
	Text   string    `json:"text"`
}

// Backend is what the gateway needs from whoever runs the containers.
type Backend interface {
	// ReadFile reads a workspace-relative file as the playground sees it.
	ReadFile(ctx context.Context, id uuid.UUID, path string) ([]byte, error)
	// Logs sends the last tail lines of every container, oldest first, then,
	// with follow, new lines until ctx ends.
	Logs(ctx context.Context, id uuid.UUID, tail int, follow bool, emit func(LogLine) error) error
}

// Live is a running playground the gateway serves.
type Live struct {
	ID   uuid.UUID
	Spec Spec
	// Addrs is where each process listens, as the runner reaches it
	// ("lazyagents-play-…-web:3000").
	Addrs map[string]string

	lastUsed atomic.Int64
	proxies  map[string]*httputil.ReverseProxy
}

// Gateway routes a port's requests to the playground served on it.
type Gateway struct {
	// Session reports whether a Vesper session token is valid.
	Session func(ctx context.Context, token string) (bool, error)
	// Origins are Vesper's own (WEB_URL, PUBLIC_URL): the only pages allowed
	// to read a playground across origins, for the API and Logs tabs.
	Origins []string
	Backend Backend
	Log     *slog.Logger

	mu       sync.RWMutex
	live     map[int]*Live
	sessions sync.Map // sha256(token) → time.Time the check expires
}

// sessionCacheTTL bounds how long a signed-out browser keeps reaching a
// playground; it spares the database a lookup for every asset an app loads.
const sessionCacheTTL = 30 * time.Second

// MaxSpecBytes bounds an OpenAPI document the gateway reads.
const MaxSpecBytes = 8 << 20

// Serve starts routing port's requests to p, replacing what it served.
func (g *Gateway) Serve(port int, p *Live) {
	p.proxies = map[string]*httputil.ReverseProxy{}
	for name, addr := range p.Addrs {
		p.proxies[name] = g.proxy(addr)
	}
	p.lastUsed.Store(time.Now().UnixNano())
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.live == nil {
		g.live = map[int]*Live{}
	}
	g.live[port] = p
}

// Stop stops serving port.
func (g *Gateway) Stop(port int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.live, port)
}

// Serving is the playground on port, or nil.
func (g *Gateway) Serving(port int) *Live {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.live[port]
}

// LastUsed is when the person last made a request to the playground.
func (p *Live) LastUsed() time.Time { return time.Unix(0, p.lastUsed.Load()) }

// Handler serves one port.
func (g *Gateway) Handler(port int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := g.Serving(port)
		if p == nil {
			page(w, http.StatusServiceUnavailable, "No playground is running here", "Start it again from its chat in Vesper.")
			return
		}
		vesper := g.vesperOrigin(r)
		if vesper {
			setCORS(w.Header(), r.Header.Get("Origin"))
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", r.Header.Get("Access-Control-Request-Method"))
				if h := r.Header.Get("Access-Control-Request-Headers"); h != "" {
					w.Header().Set("Access-Control-Allow-Headers", h)
				}
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		ok, err := g.signedIn(r)
		if err != nil {
			g.Log.Error("playground session check", "err", err)
			page(w, http.StatusInternalServerError, "Couldn’t check your Vesper session", "Try again in a moment.")
			return
		}
		if !ok {
			page(w, http.StatusUnauthorized, "Open this playground from Vesper", "It’s served only to a browser signed in to Vesper on this host.")
			return
		}
		p.lastUsed.Store(time.Now().UnixNano())
		if under(r.URL.Path, ReservedPrefix) {
			g.own(w, r, p)
			return
		}
		proc, ok := p.Spec.Route(r.URL.Path)
		proxy := p.proxies[proc.Name]
		if !ok || proxy == nil {
			page(w, http.StatusNotFound, "No process serves this path", "")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), routeKey{}, routeInfo{process: proc.Name, port: proc.Port, vesper: vesper}))
		stripSessionCookie(r.Header)
		proxy.ServeHTTP(w, r)
	})
}

func (g *Gateway) vesperOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	return o != "" && slices.Contains(g.Origins, o)
}

func setCORS(h http.Header, origin string) {
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Credentials", "true")
	h.Add("Vary", "Origin")
}

func (g *Gateway) signedIn(r *http.Request) (bool, error) {
	c, err := r.Cookie(auth.SessionCookie)
	if err != nil || c.Value == "" {
		return false, nil
	}
	key := sha256.Sum256([]byte(c.Value))
	if until, ok := g.sessions.Load(key); ok && time.Now().Before(until.(time.Time)) {
		return true, nil
	}
	ok, err := g.Session(r.Context(), c.Value)
	if err != nil || !ok {
		g.sessions.Delete(key)
		return false, err
	}
	g.sessions.Store(key, time.Now().Add(sessionCacheTTL))
	return true, nil
}

// stripSessionCookie removes Vesper's session from what the app is sent.
func stripSessionCookie(h http.Header) {
	var kept []string
	for _, line := range h.Values("Cookie") {
		var parts []string
		for _, part := range strings.Split(line, ";") {
			name, _, _ := strings.Cut(strings.TrimSpace(part), "=")
			if name != auth.SessionCookie && strings.TrimSpace(part) != "" {
				parts = append(parts, strings.TrimSpace(part))
			}
		}
		if len(parts) > 0 {
			kept = append(kept, strings.Join(parts, "; "))
		}
	}
	h.Del("Cookie")
	if len(kept) > 0 {
		h.Set("Cookie", strings.Join(kept, "; "))
	}
}

type routeKey struct{}

type routeInfo struct {
	process string
	port    int
	vesper  bool
}

func (g *Gateway) proxy(addr string) *httputil.ReverseProxy {
	target := &url.URL{Scheme: "http", Host: addr}
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()
			// The app sees the address the browser used, so the absolute
			// URLs and redirects it builds point back through the gateway.
			pr.Out.Host = pr.In.Host
			// A request from the API tab reaches the app as one from its own
			// page would, so the app's Origin and fetch-metadata checks (such
			// as Vesper's own) let it through.
			if info, _ := pr.In.Context().Value(routeKey{}).(routeInfo); info.vesper {
				scheme := "http"
				if pr.In.TLS != nil {
					scheme = "https"
				}
				pr.Out.Header.Set("Origin", scheme+"://"+pr.In.Host)
				pr.Out.Header.Set("Sec-Fetch-Site", "same-origin")
			}
		},
		// Stream server-sent events and dev-server output as they come.
		FlushInterval: -1,
		ModifyResponse: func(res *http.Response) error {
			info, _ := res.Request.Context().Value(routeKey{}).(routeInfo)
			h := res.Header
			// The app can't sign the person out of Vesper or plant a session.
			var cookies []string
			for _, c := range h.Values("Set-Cookie") {
				if name, _, _ := strings.Cut(c, "="); strings.TrimSpace(name) != auth.SessionCookie {
					cookies = append(cookies, c)
				}
			}
			h.Del("Set-Cookie")
			for _, c := range cookies {
				h.Add("Set-Cookie", c)
			}
			// The Preview tab is an iframe in Vesper; an app that forbids
			// framing would show nothing there.
			h.Del("X-Frame-Options")
			if csp := h.Get("Content-Security-Policy"); csp != "" {
				h.Set("Content-Security-Policy", withoutFrameAncestors(csp))
			}
			if info.vesper {
				// The API tab reads responses from Vesper's page: the app's
				// own CORS answer gives way to the gateway's, and every
				// header the app sent is readable there.
				origin := res.Request.Header.Get("Origin")
				for k := range h {
					if strings.HasPrefix(strings.ToLower(k), "access-control-") {
						h.Del(k)
					}
				}
				setCORS(h, origin)
				names := make([]string, 0, len(h))
				for k := range h {
					names = append(names, k)
				}
				slices.Sort(names)
				h.Set("Access-Control-Expose-Headers", strings.Join(names, ", "))
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			info, _ := r.Context().Value(routeKey{}).(routeInfo)
			if errors.Is(err, context.Canceled) {
				return
			}
			page(w, http.StatusBadGateway, fmt.Sprintf("%s isn’t answering on port %d", info.process, info.port),
				"It may still be starting, or it stopped. Its output is in the Logs tab.")
		},
	}
}

func withoutFrameAncestors(csp string) string {
	var kept []string
	for _, d := range strings.Split(csp, ";") {
		if d = strings.TrimSpace(d); d != "" && !strings.HasPrefix(strings.ToLower(d), "frame-ancestors") {
			kept = append(kept, d)
		}
	}
	return strings.Join(kept, "; ")
}

// own serves the gateway's pages under ReservedPrefix.
func (g *Gateway) own(w http.ResponseWriter, r *http.Request, p *Live) {
	w.Header().Set("Cache-Control", "no-store")
	switch strings.TrimPrefix(r.URL.Path, ReservedPrefix) {
	case "/openapi":
		g.openAPI(w, r, p)
	case "/logs":
		g.logs(w, r, p)
	default:
		page(w, http.StatusNotFound, "Not found", "")
	}
}

// openAPI serves the app's OpenAPI document as JSON, read from the workspace
// on each request so the API tab follows the agent's edits.
func (g *Gateway) openAPI(w http.ResponseWriter, r *http.Request, p *Live) {
	if p.Spec.OpenAPI == "" {
		writeJSONError(w, http.StatusNotFound, "This playground has no OpenAPI document.")
		return
	}
	b, err := g.Backend.ReadFile(r.Context(), p.ID, p.Spec.OpenAPI)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, fmt.Sprintf("Couldn’t read %s: %v", p.Spec.OpenAPI, err))
		return
	}
	doc, err := SpecJSON(b)
	if err != nil {
		writeJSONError(w, http.StatusUnprocessableEntity, fmt.Sprintf("%s isn’t valid YAML or JSON: %v", p.Spec.OpenAPI, err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(doc)
}

// SpecJSON turns an OpenAPI document in YAML or JSON into JSON.
func SpecJSON(b []byte) ([]byte, error) {
	if len(b) > MaxSpecBytes {
		return nil, fmt.Errorf("larger than %d MB", MaxSpecBytes>>20)
	}
	var doc any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	doc = jsonable(doc)
	if _, ok := doc.(map[string]any); !ok {
		return nil, errors.New("not an object")
	}
	return json.Marshal(doc)
}

// jsonable gives YAML's non-string keys (an unquoted 200: response) string
// form, which JSON needs.
func jsonable(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, e := range t {
			t[k] = jsonable(e)
		}
		return t
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[fmt.Sprint(k)] = jsonable(e)
		}
		return out
	case []any:
		for i, e := range t {
			t[i] = jsonable(e)
		}
		return t
	}
	return v
}

// maxLogTail bounds the history a Logs request starts with.
const maxLogTail = 2000

// logs streams the playground's container output as server-sent events,
// one LogLine per event.
func (g *Gateway) logs(w http.ResponseWriter, r *http.Request, p *Live) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	tail := 500
	if _, err := fmt.Sscan(r.URL.Query().Get("tail"), &tail); err != nil || tail < 0 || tail > maxLogTail {
		tail = 500
	}
	follow := r.URL.Query().Get("follow") != "0"
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	var buf bytes.Buffer
	err := g.Backend.Logs(r.Context(), p.ID, tail, follow, func(l LogLine) error {
		buf.Reset()
		buf.WriteString("data: ")
		if err := json.NewEncoder(&buf).Encode(l); err != nil { // ends with \n
			return err
		}
		buf.WriteString("\n")
		if _, err := w.Write(buf.Bytes()); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) && r.Context().Err() == nil {
		g.Log.Warn("playground logs", "playground", p.ID, "err", err)
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", strings.ReplaceAll(err.Error(), "\n", " "))
		flusher.Flush()
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
}

// page is the gateway's own answer, styled like nothing in particular: it
// shows inside the Preview tab where the app would be.
func page(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>%[1]s</title>`+
		`<body style="font:14px/1.5 system-ui,sans-serif;margin:0;display:grid;place-items:center;min-height:100vh;background:#0E0D15;color:#EEEDF3">`+
		`<div style="max-width:420px;padding:24px;text-align:center"><h1 style="font-size:16px;margin:0 0 8px">%[1]s</h1><p style="margin:0;color:#A4A1B3">%[2]s</p></div>`,
		html.EscapeString(title), html.EscapeString(detail))
}
