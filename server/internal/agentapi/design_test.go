package agentapi

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// design_render answers over MCP with image content and the size as text.
func TestImageResultIsMCPImageContent(t *testing.T) {
	r := &imageResult{PNG: []byte("png"), Meta: map[string]any{"w": 393, "h": 852}}
	res, err := r.mcpResult()
	if err != nil || len(res.Content) != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	img, ok := res.Content[0].(*mcp.ImageContent)
	if !ok || img.MIMEType != "image/png" || string(img.Data) != "png" {
		t.Fatalf("image content: %#v", res.Content[0])
	}
	var meta map[string]int
	if err := json.Unmarshal([]byte(res.Content[1].(*mcp.TextContent).Text), &meta); err != nil || meta["w"] != 393 || meta["h"] != 852 {
		t.Fatalf("size text: %v %v", meta, err)
	}
}
