package artifacts

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/liamphmurphy/vesper/server/internal/links"
)

// Section is one markdown heading, in document order.
type Section struct {
	Level   int
	Heading string
	Anchor  string
}

var (
	atxHeading = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
	fence      = regexp.MustCompile("^ {0,3}(```|~~~)")
	// Underscores stay: identifiers like posted_at matter more than _emphasis_.
	inlineMD = regexp.MustCompile("[*`~]|\\[([^\\]]*)\\]\\([^)]*\\)")
)

// Outline extracts ATX headings (# …), skipping fenced code blocks. Anchors
// match web/lib/slug.ts so the UI can link to sections it renders itself.
func Outline(md string) []Section {
	lines := outlineLines(md)
	out := make([]Section, len(lines))
	for i, l := range lines {
		out[i] = l.Section
	}
	return out
}

type lineSection struct {
	Section
	line int // 0-based line index of the heading
}

func outlineLines(md string) []lineSection {
	var out []lineSection
	seen := map[string]int{}
	inFence := ""
	for idx, line := range strings.Split(md, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := fence.FindStringSubmatch(line); m != nil {
			switch inFence {
			case "":
				inFence = m[1]
			case m[1]:
				inFence = ""
			}
			continue
		}
		if inFence != "" {
			continue
		}
		m := atxHeading.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text := strings.TrimSpace(stripInline(m[2]))
		if text == "" {
			continue
		}
		anchor := Slug(text)
		if n := seen[anchor]; n > 0 {
			seen[anchor] = n + 1
			anchor = anchor + "-" + strconv.Itoa(n+1)
		} else {
			seen[anchor] = 1
		}
		out = append(out, lineSection{Section{Level: len(m[1]), Heading: text, Anchor: anchor}, idx})
	}
	return out
}

// SectionText returns one section: its heading line through to the next
// heading of the same or a higher level. ok is false for an unknown anchor.
func SectionText(md, anchor string) (string, bool) {
	secs := outlineLines(md)
	lines := strings.Split(md, "\n")
	for i, s := range secs {
		if s.Anchor != anchor {
			continue
		}
		end := len(lines)
		for _, n := range secs[i+1:] {
			if n.Level <= s.Level {
				end = n.line
				break
			}
		}
		return strings.TrimRight(strings.Join(lines[s.line:end], "\n"), "\n") + "\n", true
	}
	return "", false
}

func stripInline(s string) string {
	return inlineMD.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, "[") {
			return inlineMD.FindStringSubmatch(m)[1]
		}
		return ""
	})
}

// Slug is links.Slug: "§4.2 Partition by month" → "4-2-partition-by-month".
func Slug(s string) string { return links.Slug(s) }

// PlainText is the searchable text of a markdown body: markup characters
// removed, link text kept. Good enough for tsvector input.
func PlainText(md string) string {
	var b strings.Builder
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimLeft(line, " \t>#-*+")
		b.WriteString(stripInline(line))
		b.WriteByte('\n')
	}
	return b.String()
}

// Summary returns the first prose paragraph (no heading, list or code),
// trimmed to about 240 characters, for artifacts saved without one.
func Summary(md string) string {
	var para []string
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if fence.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if t == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(t, "#") || strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "|") || strings.HasPrefix(t, ">") {
			if len(para) > 0 {
				break
			}
			continue
		}
		para = append(para, t)
	}
	s := strings.TrimSpace(stripInline(strings.Join(para, " ")))
	if r := []rune(s); len(r) > 240 {
		cut := string(r[:240])
		if i := strings.LastIndex(cut, " "); i > 160 {
			cut = cut[:i]
		}
		s = cut + "…"
	}
	return s
}
