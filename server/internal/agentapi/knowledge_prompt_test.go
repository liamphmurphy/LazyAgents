package agentapi

import (
	"slices"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agents"
)

// The assistant's Knowledge instructions name the Knowledge tools a chat
// actually has, so personas reach for pages rather than canvases for documents.
func TestAssistantKnowledgeSectionNamesChatKnowledgeTools(t *testing.T) {
	for _, tool := range []string{"capture_note", "daily_note", "create_page", "search_knowledge", "list_pages", "page_outline", "read_page", "patch_page", "set_properties", "suggest_links", "import_vault"} {
		if !strings.Contains(agents.AssistantKnowledgeSection, tool) {
			t.Errorf("Knowledge section doesn't mention %s", tool)
		}
		if !slices.Contains(chatTools, tool) {
			t.Errorf("Knowledge section names %s, which chats don't have", tool)
		}
	}
}
