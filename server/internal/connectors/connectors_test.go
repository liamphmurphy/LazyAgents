package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A tool whose schema marks parameters with x-mcp-header gets the matching
// Mcp-Param-<name> headers, which servers such as GitHub's reject calls without.
func TestCallSendsParamHeaders(t *testing.T) {
	ctx := context.Background()
	srv := mcp.NewServer(&mcp.Implementation{Name: "up", Version: "1"}, nil)
	schema := json.RawMessage(`{"type":"object","properties":{
		"owner":{"type":"string","x-mcp-header":"owner"},
		"repo":{"type":"string","x-mcp-header":"repo"},
		"state":{"type":"string"}},"required":["owner","repo"]}`)
	var got http.Header
	srv.AddTool(&mcp.Tool{Name: "list_issues", InputSchema: schema}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
	})
	// Stateless, like GitHub's hosted server: only then does the session use
	// protocol 2026-07-28, where the server checks Mcp-Param-* headers.
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true, DisableLocalhostProtection: true})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Mcp-Method") == "tools/call" {
			got = r.Header.Clone()
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(up.Close)

	svc := &Service{}
	sess, _, err := svc.open(ctx, up.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	tools, err := list(ctx, sess)
	sess.Close()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(tools)
	c := store.Connector{Name: "Up", Url: up.URL, AuthKind: AuthNone, Tools: raw}

	res, err := svc.Call(ctx, c, "list_issues", json.RawMessage(`{"owner":"liamphmurphy","repo":"Vesper","state":"open"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("call failed: %+v", res.Content[0])
	}
	if got.Get("Mcp-Param-owner") != "liamphmurphy" || got.Get("Mcp-Param-repo") != "Vesper" {
		t.Fatalf("headers = %v", got)
	}
}
