// Package httpapi implements the generated REST interfaces and mounts them
// alongside the other HTTP surfaces (MCP, SSE) on one mux.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
)

// Pinger is anything the health endpoint can check.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Server struct {
	Version string
	Checks  map[string]Pinger
}

var _ api.StrictServerInterface = (*Server)(nil)

// Handler returns the root HTTP handler.
func (s *Server) Handler(log *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	api.HandlerWithOptions(api.NewStrictHandler(s, nil), api.StdHTTPServerOptions{
		BaseURL:    "/api/v1",
		BaseRouter: mux,
	})
	return requestLog(log, mux)
}

func (s *Server) GetHealth(ctx context.Context, _ api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		checks = make(map[string]api.HealthCheck, len(s.Checks))
	)
	for name, p := range s.Checks {
		wg.Go(func() {
			start := time.Now()
			err := p.Ping(ctx)
			ms := int(time.Since(start).Milliseconds())
			c := api.HealthCheck{Ok: err == nil, LatencyMs: &ms}
			if err != nil {
				msg := err.Error()
				c.Error = &msg
			}
			mu.Lock()
			checks[name] = c
			mu.Unlock()
		})
	}
	wg.Wait()

	h := api.Health{Status: api.Ok, Version: s.Version, Checks: checks}
	for _, c := range checks {
		if !c.Ok {
			h.Status = api.Degraded
			return api.GetHealth503JSONResponse(h), nil
		}
	}
	return api.GetHealth200JSONResponse(h), nil
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush keeps SSE streaming working through the logging wrapper.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func requestLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds())
	})
}
