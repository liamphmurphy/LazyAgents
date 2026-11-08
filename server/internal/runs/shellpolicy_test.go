package runs_test

import (
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"slices"
	"testing"
)

func TestScoperShellPermissions(t *testing.T) {
	for _, command := range []string{
		"rg -n 'func.*Active' server; cat README.md",
		"sed -n '1,165p' internal/tracker/service.go; sed -n '280,601p' internal/tracker/service.go",
		"/bin/zsh -lc 'git status --short --branch'",
		"/bin/zsh -lc 'cd server && rg -n \"tests:run\" internal | head -40'",
		"git -C server log -5 --oneline; git diff --exit-code",
		"rg missing . 2>/dev/null || pwd",
	} {
		got := runs.ToolScopes("shell", map[string]any{"command": command})
		if !slices.Equal(got, []string{"repo:read"}) {
			t.Errorf("%s: %v", command, got)
		}
	}
	for _, command := range []string{
		"cat README.md; go test ./...", "rg --pre=evil x .", "rg --hostname-bin=evil x .",
		"cat $(touch owned)", "cat <(touch owned)", "cat README.md > owned", "cat README.md &",
		"git diff --output=owned", "git show --textconv HEAD", "git -c core.pager=evil log",
		"git grep --open-files-in-pager=evil x", "git grep -Oevil x", "git diff -Oowned",
		"sed -n '1,3p' -e 'e touch owned' README.md", "sed -n '1w owned' README.md",
		"eval 'cat README.md'", "PATH=evil cat README.md", "./cat README.md",
		"/bin/zsh -lc 'cat README.md; touch owned'",
	} {
		got := runs.ToolScopes("shell", map[string]any{"command": command})
		if !slices.Contains(got, "tests:run") {
			t.Errorf("execution allowed as read: %s: %v", command, got)
		}
	}
}

func TestShippingDoesNotRequireTests(t *testing.T) {
	for _, command := range []string{"git push origin HEAD", "/bin/zsh -lc 'git push -u origin HEAD'"} {
		got := runs.ToolScopes("shell", map[string]any{"command": command})
		if !slices.Equal(got, []string{"repo:write"}) {
			t.Errorf("%s: %v", command, got)
		}
	}
	got := runs.ToolScopes("shell", map[string]any{"command": "git push origin $(printf HEAD) && go test ./..."})
	if !slices.Contains(got, "tests:run") || !slices.Contains(got, "repo:write") {
		t.Fatal(got)
	}
}

func TestWebToolsNeedWebRead(t *testing.T) {
	for _, tool := range []string{"WebFetch", "WebSearch", "webfetch", "websearch", "web_search"} {
		if got := runs.ToolScopes(tool, map[string]any{"url": "https://example.com"}); !slices.Equal(got, []string{"web:read"}) {
			t.Errorf("%s: %v", tool, got)
		}
	}
}

// ToolSearch only loads deferred tool definitions (Claude Code defers
// WebFetch); it needs no scope, and loaded tools are checked when called.
func TestToolSearchNeedsNoScope(t *testing.T) {
	if got := runs.ToolScopes("ToolSearch", map[string]any{"query": "select:WebFetch"}); got != nil {
		t.Fatalf("ToolSearch scopes = %v", got)
	}
}
