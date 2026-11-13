package wiki

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
)

// Change is a stable, numbered block or property change carried by a version.
// A nil property value means the key is absent (page properties cannot be null).
type Change struct {
	I             int        `json:"i"`
	Kind          string     `json:"kind"`
	Before        any        `json:"before"`
	After         any        `json:"after"`
	ContextBefore string     `json:"context_before"`
	ContextAfter  string     `json:"context_after"`
	Key           string     `json:"key,omitempty"`
	Why           string     `json:"why"`
	State         string     `json:"state"`
	Reason        string     `json:"reason,omitempty"`
	DecidedBy     *uuid.UUID `json:"decided_by,omitempty"`
	DecidedAt     *time.Time `json:"decided_at,omitempty"`
}

// blocks keeps original whitespace, and treats fenced code as a single block.
// Unchanged blocks anchor changes, as in the Docs review's block LCS.
func blocks(md string) []string {
	var out []string
	start, pos := 0, 0
	fence := ""
	blank := false
	for _, line := range strings.SplitAfter(md, "\n") {
		trim := strings.TrimSpace(line)
		if blank && trim != "" && fence == "" {
			out = append(out, md[start:pos])
			start = pos
		}
		if strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~") {
			marker := trim[:3]
			if fence == "" {
				fence = marker
			} else if marker == fence {
				fence = ""
			}
		}
		blank = trim == "" && fence == ""
		pos += len(line)
	}
	if start < len(md) {
		out = append(out, md[start:])
	}
	return out
}

func diffChanges(base, next string, oldProps, newProps artifacts.Properties, why string) []Change {
	out := []Change{}
	a, b := blocks(base), blocks(next)
	// Bound memory and work for large pages. A conservative whole-body change
	// still protects human edits; it simply offers a coarser review.
	if len(a)+len(b) > 5000 || len(a)*len(b) > 1_000_000 {
		a = []string{base}
		b = []string{next}
	}
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		if i < len(a) && j < len(b) && a[i] == b[j] {
			i++
			j++
			continue
		}
		ai, bj := i, j
		for i < len(a) || j < len(b) {
			if i < len(a) && j < len(b) && a[i] == b[j] {
				break
			}
			if j < len(b) && (i == len(a) || dp[i][j+1] > dp[i+1][j]) {
				j++
			} else {
				i++
			}
		}
		c := Change{I: len(out), Kind: "body", Before: strings.Join(a[ai:i], ""), After: strings.Join(b[bj:j], ""), Why: why, State: "pending"}
		if ai > 0 {
			c.ContextBefore = a[ai-1]
		}
		if i < len(a) {
			c.ContextAfter = a[i]
		}
		out = append(out, c)
	}
	keys := map[string]bool{}
	for k := range oldProps {
		keys[k] = true
	}
	for k := range newProps {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		if !equalValue(oldProps[k], newProps[k]) {
			out = append(out, Change{I: len(out), Kind: "property", Key: k, Before: oldProps[k], After: newProps[k], Why: why, State: "pending"})
		}
	}
	return out
}

func equalValue(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// locate requires one unambiguous match including the unchanged neighbor
// blocks. Empty context pins the change to the start/end of the document.
func locate(body string, c Change) (int, int, bool) {
	before, ok := c.Before.(string)
	if !ok {
		return 0, 0, false
	}
	needle := c.ContextBefore + before + c.ContextAfter
	if needle == "" {
		return 0, 0, body == ""
	}
	found := -1
	for offset := 0; offset <= len(body); {
		p := strings.Index(body[offset:], needle)
		if p < 0 {
			break
		}
		p += offset
		end := p + len(needle)
		if (c.ContextBefore != "" || p == 0) && (c.ContextAfter != "" || end == len(body)) {
			if found >= 0 {
				return 0, 0, false
			}
			found = p + len(c.ContextBefore)
		}
		offset = p + 1
	}
	return found, found + len(before), found >= 0
}
