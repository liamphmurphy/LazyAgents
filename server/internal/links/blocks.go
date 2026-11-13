package links

import (
	"regexp"
	"strings"
)

// Block references (Obsidian's "^id"): a paragraph or list item ending in
// " ^id", or followed by a line holding only "^id", can be linked as
// [[Page#^id]] and embedded as ![[Page#^id]].

var (
	blockIDSuffix = regexp.MustCompile(`(?:^|\s)\^([A-Za-z0-9-]{1,64})\s*$`)
	blockIDOnly   = regexp.MustCompile(`^\s*\^([A-Za-z0-9-]{1,64})\s*$`)
	headingLine   = regexp.MustCompile(`^ {0,3}#{1,6}\s`)
	listItemLine  = regexp.MustCompile(`^(\s*)(?:[-*+]|\d+[.)])\s+`)
)

// BlockID returns the id of a "#^id" anchor.
func BlockID(anchor string) (string, bool) {
	id, ok := strings.CutPrefix(strings.TrimSpace(anchor), "^")
	if !ok || !blockIDOnly.MatchString("^"+id) {
		return "", false
	}
	return id, true
}

// Block is one identified paragraph or list item.
type Block struct {
	ID   string
	Text string // the block's markdown without its "^id" marker
}

// mdLine is a source line outside fenced code.
type mdLine struct {
	text  string
	fence bool // inside or delimiting a fenced code block
}

func splitLines(md string) []mdLine {
	raw := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	out := make([]mdLine, len(raw))
	inFence := ""
	for i, l := range raw {
		out[i].text = l
		if f := fenceMarker(l); f != "" {
			out[i].fence = true
			switch inFence {
			case "":
				inFence = f
			case f:
				inFence = ""
			}
			continue
		}
		out[i].fence = inFence != ""
	}
	return out
}

func indentOf(s string) int { return len(s) - len(strings.TrimLeft(s, " \t")) }

func blank(l mdLine) bool { return l.fence || strings.TrimSpace(l.text) == "" || headingLine.MatchString(l.text) }

// Blocks returns the identified blocks of a markdown body in document order.
// The first block with a given id wins, as in Obsidian.
func Blocks(md string) []Block {
	lines := splitLines(md)
	var out []Block
	seen := map[string]bool{}
	add := func(id string, from, to int, last string) {
		if seen[id] {
			return
		}
		seen[id] = true
		parts := make([]string, 0, to-from+1)
		for i := from; i < to; i++ {
			parts = append(parts, lines[i].text)
		}
		if last != "" {
			parts = append(parts, last)
		}
		out = append(out, Block{ID: id, Text: strings.TrimRight(strings.Join(parts, "\n"), " \t")})
	}
	for i, l := range lines {
		if l.fence || headingLine.MatchString(l.text) {
			continue
		}
		if m := blockIDOnly.FindStringSubmatch(l.text); m != nil {
			// "^id" on its own line names the block just above it.
			end := i
			if end == 0 || blank(lines[end-1]) {
				continue
			}
			start := end - 1
			for start > 0 && !blank(lines[start-1]) {
				start--
			}
			add(m[1], start, end, "")
			continue
		}
		m := blockIDSuffix.FindStringSubmatchIndex(l.text)
		if m == nil {
			continue
		}
		id := l.text[m[2]:m[3]]
		text := strings.TrimRight(l.text[:m[0]], " \t")
		if lm := listItemLine.FindStringSubmatch(l.text); lm != nil {
			// A list item with its nested lines.
			ind := len(lm[1])
			parts := []string{text}
			for j := i + 1; j < len(lines) && !lines[j].fence && strings.TrimSpace(lines[j].text) != "" && indentOf(lines[j].text) > ind; j++ {
				parts = append(parts, lines[j].text)
			}
			if !seen[id] {
				seen[id] = true
				out = append(out, Block{ID: id, Text: strings.Join(parts, "\n")})
			}
			continue
		}
		// A paragraph (or a list item's continuation line): back to the
		// blank line or the start of the list item it belongs to.
		start := i
		for start > 0 && !blank(lines[start-1]) && listItemLine.FindString(lines[start].text) == "" {
			start--
		}
		add(id, start, i, text)
	}
	return out
}

// FindBlock returns the block with the given id.
func FindBlock(md, id string) (Block, bool) {
	for _, b := range Blocks(md) {
		if b.ID == id {
			return b, true
		}
	}
	return Block{}, false
}
