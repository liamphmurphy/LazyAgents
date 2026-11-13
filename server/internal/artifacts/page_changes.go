package artifacts

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// PageChange keeps the exact text and adjacent context needed to rebase a
// review onto the current head without overwriting intervening human edits.
type PageChange struct {
	Index  int    `json:"index"`
	Kind   string `json:"kind"`
	Key    string `json:"key,omitempty"`
	Before string `json:"before"`
	After  string `json:"after"`
	Prefix string `json:"prefix,omitempty"`
	Suffix string `json:"suffix,omitempty"`
	Why    string `json:"why,omitempty"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

func PageChanges(before, after string, old, next Properties, why string) []PageChange {
	out := []PageChange{}
	a, b := strings.SplitAfter(before, "\n"), strings.SplitAfter(after, "\n")
	// Bound memory on large pages. A single conservative hunk is preferable to
	// an unbounded quadratic diff; if its context changes it becomes stale.
	if len(a)*len(b) > 1000000 {
		if before != after {
			out = append(out, PageChange{Kind: "body", Before: before, After: after})
		}
	} else {
		dp := make([][]int, len(a)+1)
		for i := range dp {
			dp[i] = make([]int, len(b)+1)
		}
		for i := len(a) - 1; i >= 0; i-- {
			for j := len(b) - 1; j >= 0; j-- {
				if a[i] == b[j] {
					dp[i][j] = 1 + dp[i+1][j+1]
				} else {
					dp[i][j] = max(dp[i+1][j], dp[i][j+1])
				}
			}
		}
		i, j, pos := 0, 0, 0
		for i < len(a) || j < len(b) {
			if i < len(a) && j < len(b) && a[i] == b[j] {
				pos += len(a[i])
				i++
				j++
				continue
			}
			ai := i
			var removed, added strings.Builder
			for i < len(a) || j < len(b) {
				if i < len(a) && j < len(b) && a[i] == b[j] {
					break
				}
				if j < len(b) && (i == len(a) || dp[i][j+1] >= dp[i+1][j]) {
					added.WriteString(b[j])
					j++
				} else {
					removed.WriteString(a[i])
					pos += len(a[i])
					i++
				}
			}
			out = append(out, PageChange{Kind: "body", Before: removed.String(), After: added.String(), Prefix: strings.Join(a[max(0, ai-1):ai], ""), Suffix: strings.Join(a[i:min(len(a), i+1)], "")})
		}
	}
	keys := map[string]bool{}
	for k := range old {
		keys[k] = true
	}
	for k := range next {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		x, xok := old[k]
		y, yok := next[k]
		xb, _ := json.Marshal(x)
		yb, _ := json.Marshal(y)
		if xok == yok && bytes.Equal(xb, yb) {
			continue
		}
		if !xok {
			xb = nil
		}
		if !yok {
			yb = nil
		}
		out = append(out, PageChange{Kind: "property", Key: k, Before: string(xb), After: string(yb)})
	}
	for i := range out {
		out[i].Index = i
		out[i].State = "pending"
		out[i].Why = why
	}
	return out
}

// ApplyPageChange fails closed on ambiguous or missing context. For a direct
// edit rejection reverse=true applies the inverse to the current head.
func ApplyPageChange(body string, props Properties, c PageChange, reverse bool) (string, bool) {
	before, after := c.Before, c.After
	if reverse {
		before, after = after, before
	}
	if c.Kind == "property" {
		val, exists := props[c.Key]
		raw, _ := json.Marshal(val)
		if !exists {
			raw = nil
		}
		if string(raw) != before {
			return body, false
		}
		if after == "" {
			delete(props, c.Key)
		} else {
			var v any
			if json.Unmarshal([]byte(after), &v) != nil {
				return body, false
			}
			props[c.Key] = v
		}
		return body, true
	}
	needle := c.Prefix + before + c.Suffix
	if needle == "" {
		if body != "" {
			return body, false
		}
		return after, true
	}
	if strings.Count(body, needle) != 1 {
		return body, false
	}
	return strings.Replace(body, needle, c.Prefix+after+c.Suffix, 1), true
}
