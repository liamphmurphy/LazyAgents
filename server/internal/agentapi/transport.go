package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/broker"
)

type mcpServer = *mcp.Server

func brokerFor(scope string) (broker.Action, bool) { return broker.For(scope) }

// define binds a typed tool function to both transports.
func define[In any](name, desc string, fn func(context.Context, Deps, agents.Caller, In) (any, error)) Tool {
	return Tool{
		Name:        name,
		Description: desc,
		call: func(ctx context.Context, d Deps, c agents.Caller, raw json.RawMessage) (any, error) {
			var in In
			if len(raw) > 0 && string(raw) != "null" {
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, apperr.Invalid("arguments", "%v", err)
				}
			}
			return fn(ctx, d, c, in)
		},
		addMCP: func(srv mcpServer, d Deps) {
			mcp.AddTool(srv, &mcp.Tool{Name: name, Description: desc},
				func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
					c, ok := callerFrom(req)
					if !ok {
						return nil, nil, errors.New("unauthenticated")
					}
					out, err := fn(ctx, d, c, in)
					if err != nil {
						return nil, nil, toolError(d, name, c, err)
					}
					return nil, out, nil
				})
		},
	}
}

var logger = slog.Default()

func toolError(d Deps, name string, c agents.Caller, err error) error {
	msg, internal := PublicError(err)
	if internal {
		logger.Error("agent tool", "tool", name, "agent", c.Agent.Handle, "err", err)
	}
	return errors.New(msg)
}

const callerKey = "caller"

func callerFrom(req *mcp.CallToolRequest) (agents.Caller, bool) {
	if req == nil || req.Extra == nil || req.Extra.TokenInfo == nil {
		return agents.Caller{}, false
	}
	c, ok := req.Extra.TokenInfo.Extra[callerKey].(agents.Caller)
	return c, ok
}

// verifier resolves an agent token on every request, so a revoked token stops
// working immediately, even mid-session.
func verifier(d Deps) auth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		c, err := agents.ResolveToken(ctx, d.Q, token)
		if err != nil {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{
			UserID:     c.Agent.ID.String(), // binds MCP sessions to one agent
			Expiration: time.Now().Add(time.Hour),
			Extra:      map[string]any{callerKey: c},
		}, nil
	}
}

const instructions = `You are working through LazyAgents, the tracker your team uses to supervise agents.
Start with whoami, then claim_ticket and get_ticket. Read attached artifacts with read_artifact before
planning, then post_plan before editing anything and keep steps current with update_step.
Scopes marked requires_approval need request_approval first; brokered ones (like db:migrate) run via
run_gated after approval. When you need a person, comment and call await_human. Finish with complete_run.`

// MCPHandler serves the Agent API over streamable HTTP at /mcp.
func MCPHandler(d Deps, version string, log *slog.Logger) http.Handler {
	if log != nil {
		logger = log
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "lazyagents", Version: version}, &mcp.ServerOptions{Instructions: instructions})
	for _, t := range Tools {
		t.addMCP(srv, d)
	}
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{
		// Off, by the user's decision (2026-11-02). The SDK rejects non-localhost
		// Host headers on loopback connections (DNS rebinding), but run
		// containers reach an API on the host as host.docker.internal, which
		// Docker Desktop and OrbStack deliver over loopback. Rebinding needs an
		// endpoint that trusts ambient browser credentials; every /mcp request
		// must carry an agent bearer token, checked before this handler runs.
		DisableLocalhostProtection: true,
	})
	return auth.RequireBearerToken(verifier(d), nil)(h)
}

// RESTHandler mirrors the tools at POST /api/v1/agent/tools/{name} (JSON
// arguments in, JSON result out) and lists them at GET /api/v1/agent/tools.
func RESTHandler(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/agent/tools", func(w http.ResponseWriter, r *http.Request) {
		list := make([]map[string]string, len(Tools))
		for i, t := range Tools {
			list[i] = map[string]string{"name": t.Name, "description": t.Description}
		}
		writeJSON(w, http.StatusOK, map[string]any{"tools": list})
	})
	mux.HandleFunc("POST /api/v1/agent/tools/{name}", func(w http.ResponseWriter, r *http.Request) {
		c := r.Context().Value(restCallerKey{}).(agents.Caller)
		t, ok := ByName[r.PathValue("name")]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"code": "not_found", "message": "no such tool"})
			return
		}
		var raw json.RawMessage
		if r.ContentLength != 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&raw); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid", "message": "body must be JSON arguments"})
				return
			}
		}
		out, err := t.call(r.Context(), d, c, raw)
		if err != nil {
			msg, internal := PublicError(err)
			if internal {
				logger.Error("agent tool", "tool", t.Name, "agent", c.Agent.Handle, "err", err)
			}
			status := http.StatusInternalServerError
			switch {
			case errors.As(err, new(*apperr.ValidationError)):
				status = http.StatusUnprocessableEntity
			case errors.As(err, new(*apperr.ConflictError)):
				status = http.StatusConflict
			case msg == "not found":
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]string{"code": "error", "message": msg})
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	return requireAgent(d, mux)
}

type restCallerKey struct{}

func requireAgent(d Deps, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="lazyagents"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"code": "unauthenticated", "message": "send Authorization: Bearer la_…"})
			return
		}
		c, err := agents.ResolveToken(r.Context(), d.Q, strings.TrimSpace(tok))
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"code": "unauthenticated", "message": "invalid or revoked token"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), restCallerKey{}, c)))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
