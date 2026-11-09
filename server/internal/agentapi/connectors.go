package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/connectors"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// connectorApprovalWait is how long a tool call waits for the person; the
// harness's own tool timeout is set just above it.
const connectorApprovalWait = 30 * time.Minute

func toolFailure(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

// connectorFor is the connector this run's persona may reach under slug.
// Anything else fails closed.
func connectorFor(ctx context.Context, d Deps, c agents.Caller, slug string) (store.Connector, store.ChatSession, error) {
	var out store.Connector
	var chat store.ChatSession
	if c.RunID == nil {
		return out, chat, apperr.ErrNotFound
	}
	chat, err := d.Q.ChatSessionForRun(ctx, c.RunID)
	if err != nil {
		return out, chat, err
	}
	speaker := runs.ChatSpeakerID(chat)
	if speaker == nil {
		return out, chat, apperr.ErrNotFound
	}
	rows, err := d.Q.ConnectorsForPersona(ctx, store.ConnectorsForPersonaParams{WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy, PersonaID: *speaker, AllowedIds: chat.ConnectorIds})
	if err != nil {
		return out, chat, err
	}
	for _, row := range rows {
		if row.Slug == slug {
			return row, chat, nil
		}
	}
	return out, chat, apperr.ErrNotFound
}

// ConnectorHandler serves each connector to runs at /mcp/connectors/{slug}. It
// is the enforcement point: the run holds only its own token, Off tools don't
// exist here, and Ask tools wait for the person. Every request rebuilds the
// server, so a changed permission applies to the next call.
func ConnectorHandler(d Deps, version string) http.Handler {
	h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		info := auth.TokenInfoFromContext(r.Context())
		if info == nil {
			return nil
		}
		caller, _ := info.Extra[callerKey].(agents.Caller)
		conn, chat, err := connectorFor(r.Context(), d, caller, r.PathValue("slug"))
		if err != nil {
			return nil
		}
		srv := mcp.NewServer(&mcp.Implementation{Name: conn.Slug, Version: version}, nil)
		for _, t := range connectors.Tools(conn) {
			if t.Policy == connectors.PolicyOff {
				continue
			}
			schema := map[string]any{}
			_ = json.Unmarshal(t.InputSchema, &schema)
			if schema["type"] != "object" {
				schema = map[string]any{"type": "object"}
			}
			srv.AddTool(&mcp.Tool{Name: t.Name, Description: t.Description, InputSchema: schema}, connectorTool(d, conn, chat, t))
		}
		return srv
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: true})
	return auth.RequireBearerToken(verifier(d), nil)(h)
}

func connectorTool(d Deps, conn store.Connector, chat store.ChatSession, t connectors.Tool) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		if t.Policy == connectors.PolicyAsk {
			if refusal := awaitConnectorApproval(ctx, d, conn, chat, t.Name, args); refusal != "" {
				return toolFailure(refusal), nil
			}
		}
		res, err := d.Connectors.Call(ctx, conn, t.Name, args)
		if err != nil {
			return toolFailure(err.Error()), nil
		}
		return res, nil
	}
}

// awaitConnectorApproval blocks until the person decides, returning why the
// tool may not run, or "" to go ahead.
func awaitConnectorApproval(ctx context.Context, d Deps, conn store.Connector, chat store.ChatSession, tool string, args json.RawMessage) string {
	store0 := context.WithoutCancel(ctx)
	a, err := d.Q.CreateConnectorApproval(store0, store.CreateConnectorApprovalParams{WorkspaceID: conn.WorkspaceID, UserID: conn.UserID, ConnectorID: conn.ID,
		PersonaID: runs.ChatSpeakerID(chat), ChatID: chat.ID, Tool: tool, Arguments: args})
	if err != nil {
		return "Couldn't ask the person for approval."
	}
	ctx, cancel := context.WithTimeout(ctx, connectorApprovalWait)
	defer cancel()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = d.Q.ExpireConnectorApproval(store0, a.ID)
			return "The person didn't approve this in time. Tell them it's still needed."
		case <-tick.C:
			cur, err := d.Q.GetConnectorApproval(ctx, a.ID)
			if err != nil {
				continue
			}
			switch cur.State {
			case "allowed":
				return ""
			case "denied":
				return "The person denied this. Don't try it again."
			case "expired":
				return "This approval expired."
			}
		}
	}
}

type ProposeConnectorIn struct {
	Name   string `json:"name" jsonschema:"A short display name, 1–60 characters"`
	URL    string `json:"url" jsonschema:"The MCP server's URL. Must be https, or http on localhost"`
	Auth   string `json:"auth,omitempty" jsonschema:"How the person signs in: oauth (default), token or none. Never ask them for a token in chat"`
	Reason string `json:"reason,omitempty" jsonschema:"One sentence for the person on why it helps, at most 500 characters"`
}

var proposeConnector = define("propose_connector", "Ask the person to add an MCP connector. Nothing connects until they review it, sign in and approve; you then get access with write tools on Ask.",
	func(ctx context.Context, d Deps, c agents.Caller, in ProposeConnectorIn) (any, error) {
		s, err := chatScopeFor(ctx, d, c)
		if err != nil {
			return nil, err
		}
		if s == nil || s.PersonaID == nil {
			return nil, apperr.Conflict("propose_connector only works in a chat with a persona")
		}
		p, err := d.Q.GetPersona(ctx, store.GetPersonaParams{ID: *s.PersonaID, WorkspaceID: c.Agent.WorkspaceID, UserID: s.UserID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !p.CanCreateConnectors) {
			return nil, apperr.Conflict("This persona can't propose connectors; tell the person it isn't enabled in Manage personas.")
		}
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(in.Name)
		if name == "" || len([]rune(name)) > 60 {
			return nil, apperr.Invalid("name", "must be 1–60 characters")
		}
		server, err := connectors.ValidateURL(in.URL)
		if err != nil {
			return nil, err
		}
		kind := in.Auth
		if kind == "" {
			kind = connectors.AuthOAuth
		}
		if kind != connectors.AuthOAuth && kind != connectors.AuthToken && kind != connectors.AuthNone {
			return nil, apperr.Invalid("auth", "must be oauth, token or none")
		}
		reason := strings.TrimSpace(in.Reason)
		if len([]rune(reason)) > 500 {
			return nil, apperr.Invalid("reason", "must be at most 500 characters")
		}
		n, err := d.Q.CountPendingConnectorProposals(ctx, s.ChatID)
		if err != nil {
			return nil, err
		}
		if n >= 3 {
			return nil, apperr.Conflict("the person already has connector proposals waiting in this chat")
		}
		out, err := d.Q.CreateConnectorProposal(ctx, store.CreateConnectorProposalParams{ChatID: s.ChatID, RunID: *c.RunID, ProposedBy: s.PersonaID, Name: name, Url: server, AuthKind: kind, Reason: reason})
		if err != nil {
			return nil, err
		}
		return map[string]any{"proposal_id": out.ID, "state": out.State, "tell_the_person": "Proposed. The person sees a card in the chat to review it and sign in. Don't describe it as connected yet."}, nil
	})
