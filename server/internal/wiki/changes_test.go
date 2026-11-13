package wiki

import (
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/artifacts"
)

func TestChangeTargets(t *testing.T) {
	tests := []struct {
		name, base, next, head, want string
		stale                        bool
	}{
		{"empty insertion", "", "New page", "", "New page", false},
		{"empty insertion after human write", "", "New page", "Human page", "Human page", true},
		{"append", "First.\n\n", "First.\n\nLast.", "First.\n\n", "First.\n\nLast.", false},
		{"prepend", "Last.", "First.\n\nLast.", "Last.", "First.\n\nLast.", false},
		{"delete", "First.\n\nLast.", "Last.", "First.\n\nLast.", "Last.", false},
		{"missing context", "First.\n\nAnchor.\n\nLast.", "Edited.\n\nAnchor.\n\nLast.", "First.\n\nHuman anchor.\n\nLast.", "First.\n\nHuman anchor.\n\nLast.", true},
		{"fenced block", "```go\nline\n\nline\n```\n\nLast.\n", "```go\nchanged\n\nline\n```\n\nLast.\n", "```go\nline\n\nline\n```\n\nLast.\n", "```go\nchanged\n\nline\n```\n\nLast.\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := diffChanges(tt.base, tt.next, artifacts.Properties{}, artifacts.Properties{}, "why")
			if len(cs) != 1 {
				t.Fatalf("changes: %+v", cs)
			}
			start, end, ok := locate(tt.head, cs[0])
			if ok == tt.stale {
				t.Fatalf("found=%v expected stale=%v", ok, tt.stale)
			}
			got := tt.head
			if ok {
				got = got[:start] + cs[0].After.(string) + got[end:]
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
	c := Change{Kind: "body", Before: "Same.\n\n", After: "Edit.\n\n", ContextBefore: "Before.\n\n", ContextAfter: "After.\n\n"}
	if _, _, ok := locate(strings.Repeat("Before.\n\nSame.\n\nAfter.\n\n", 2), c); ok {
		t.Fatal("ambiguous target must be stale")
	}
}

func TestPropertyChangesIncludeRemoval(t *testing.T) {
	cs := diffChanges("body", "body", artifacts.Properties{"keep": true, "remove": "old", "tags": []string{"old"}}, artifacts.Properties{"keep": true, "add": "new", "tags": []string{"new"}}, "Update metadata")
	if len(cs) != 3 || cs[0].Key != "add" || cs[0].Before != nil || cs[1].Key != "remove" || cs[1].After != nil || cs[2].Key != "tags" {
		t.Fatalf("changes: %+v", cs)
	}
	for _, c := range cs {
		if c.Why != "Update metadata" {
			t.Fatal("missing why")
		}
	}
}
