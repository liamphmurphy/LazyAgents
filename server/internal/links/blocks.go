package links

import (
	"regexp"
	"strings"
)

var blockID = regexp.MustCompile(`(?:^|\s)\^([A-Za-z0-9-]+)\s*$`)

// Blocks returns Obsidian paragraph and list-item block identifiers outside
// fenced code. A standalone ^id belongs to the preceding paragraph.
func Blocks(md string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(md, "\n")
	fenced := ""
	for i, line := range lines {
		if marker := fenceMarker(line); marker != "" {
			if fenced == "" {
				fenced = marker
			} else if fenced == marker {
				fenced = ""
			}
			continue
		}
		if fenced != "" {
			continue
		}
		m := blockID.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		id := line[m[2]:m[3]]
		start := i
		end := strings.TrimSpace(line[:m[0]])
		if end == "" {
			start = i - 1
			for start >= 0 && strings.TrimSpace(lines[start]) == "" {
				start--
			}
			if start < 0 {
				continue
			}
			end = lines[start]
		}
		if !listMarker.MatchString(strings.TrimLeft(end, " \t")) {
			for start > 0 && strings.TrimSpace(lines[start-1]) != "" && !strings.HasPrefix(strings.TrimSpace(lines[start-1]), "#") {
				start--
			}
		}
		preceding := ""
		if start < i {
			preceding = strings.Join(lines[start:i], "\n")
			if strings.TrimSpace(line[:m[0]]) == "" {
				preceding = strings.Join(lines[start:i], "\n")
				end = ""
			}
		}
		text := strings.TrimSpace(preceding + "\n" + end)
		if _, exists := out[id]; !exists {
			out[id] = text + "\n"
		}
	}
	return out
}
