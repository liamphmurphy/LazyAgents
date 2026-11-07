// Package httpapi implements the generated REST interfaces and mounts them
// alongside the other HTTP surfaces (MCP, SSE) on one mux.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/lazyagents/server/internal/agentapi"
	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/auth"
	"github.com/liamphmurphy/lazyagents/server/internal/blob"
	"github.com/liamphmurphy/lazyagents/server/internal/realtime"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/secrets"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
	"github.com/liamphmurphy/lazyagents/server/internal/wiki"
)

// Pinger is anything the health endpoint can check.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Server struct {
	Events  *realtime.Hub
	Version string
	Checks  map[string]Pinger
	Pool    *pgxpool.Pool
	Q       *store.Queries
	Tracker *tracker.Service
	Log     *slog.Logger

	Artifacts *artifacts.Service
	Agents    *agents.Service
	Secrets   *secrets.Service
	Runs      *runs.Service
	Wiki      *wiki.Service
	// PublicURL is how agents reach this server (the /mcp link shown with tokens).
	PublicURL      string
	Blobs          *blob.Store
	ArtifactBucket string
}

var _ api.StrictServerInterface = (*Server)(nil)

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	if s.Log == nil {
		s.Log = slog.Default()
	}
	mux := http.NewServeMux()
	if s.Events != nil {
		mux.Handle("GET /events", s.Events)
	}
	strict := api.NewStrictHandlerWithOptions(s, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, http.StatusBadRequest, api.Invalid, err.Error(), "")
		},
		ResponseErrorHandlerFunc: s.responseError,
	})
	api.HandlerWithOptions(strict, api.StdHTTPServerOptions{
		BaseURL:    "/api/v1",
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, http.StatusBadRequest, api.Invalid, err.Error(), "")
		},
	})
	// Agent API (PLAN.md §5.2): bearer agent tokens, not the local user.
	deps := agentapi.Deps{Q: s.Q, Runs: s.Runs, Tracker: s.Tracker, Wiki: s.Wiki}
	mux.Handle("/mcp", agentapi.MCPHandler(deps, s.Version, s.Log))
	mux.Handle("/api/v1/agent/", agentapi.RESTHandler(deps))
	return requestLog(s.Log, s.localUser(mux))
}

// localUser attaches the acting user to every API request. There is no
// login for now (PLAN.md §5.4): everyone acts as the workspace's first user.
func (s *Server) localUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" || !strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/api/v1/agent/") {
			next.ServeHTTP(w, r)
			return
		}
		u, err := s.Q.GetLocalUser(r.Context())
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusServiceUnavailable, api.NoUser, "no workspace user yet: restart the API to seed one", "")
			return
		}
		if err != nil {
			s.Log.Error("local user lookup", "err", err)
			writeError(w, http.StatusInternalServerError, api.Internal, "internal error", "")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
	})
}

func (s *Server) responseError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *tracker.ValidationError
	var ce *apperr.ConflictError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusUnprocessableEntity, api.Invalid, ve.Error(), ve.Field)
	case errors.As(err, &ce):
		writeError(w, http.StatusConflict, api.Conflict, err.Error(), ce.Field)
	case errors.Is(err, secrets.ErrNoKey):
		writeError(w, http.StatusConflict, api.Conflict, "Set SECRETS_KEY (openssl rand -base64 32) and restart the API to store secrets", "")
	case errors.Is(err, tracker.ErrNotFound):
		writeError(w, http.StatusNotFound, api.NotFound, "not found", "")
	default:
		s.Log.Error("handler", "method", r.Method, "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, api.Internal, "internal error", "")
	}
}

func writeError(w http.ResponseWriter, status int, code api.ErrorCode, msg, field string) {
	e := api.Error{Code: code, Message: msg}
	if field != "" {
		e.Field = &field
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(e)
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

// Unwrap lets http.ResponseController reach the underlying writer
// (streaming MCP responses, deadlines).
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

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
