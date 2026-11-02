package artifacts

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
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
	var out []Section
	seen := map[string]int{}
	inFence := ""
	for _, line := range strings.Split(md, "\n") {
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
		out = append(out, Section{Level: len(m[1]), Heading: text, Anchor: anchor})
	}
	return out
}

func stripInline(s string) string {
	return inlineMD.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, "[") {
			return inlineMD.FindStringSubmatch(m)[1]
		}
		return ""
	})
}

// Slug lowercases, keeps letters and digits, and joins the rest with single
// hyphens: "§4.2 Partition by month" → "4-2-partition-by-month".
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		} else {
			dash = true
		}
	}
	if b.Len() == 0 {
		return "section"
	}
	return b.String()
}

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
