// Package links parses, resolves and indexes [[wikilinks]] (docs/dev/plan.md §12.2).
// It sits below artifacts and tracker, which call Index whenever markdown
// they own is saved, so backlinks stay current wherever a link is written.
package links

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Ref is one [[link]] as written.
type Ref struct {
	Raw     string // the whole [[…]] (with "!" for embeds)
	Target  string // before "#" and "|", trimmed
	Anchor  string // heading text after "#", as written
	Display string // after "|"
	Embed   bool   // ![[…]]
	Snippet string // the line around the link, markup stripped
	Line    int    // 0-based
}

var (
	wikilink = regexp.MustCompile(`(!?)\[\[([^\[\]\n]+?)\]\]`)
	codeSpan = regexp.MustCompile("`+[^`\n]*`+")
)

// fenceMarker is the fence a line opens or closes ("```" or "~~~" after at
// most three spaces), or "" if it isn't a fence line.
func fenceMarker(line string) string {
	for n := 0; n < 3 && strings.HasPrefix(line, " "); n++ {
		line = line[1:]
	}
	if strings.HasPrefix(line, "```") {
		return "```"
	}
	if strings.HasPrefix(line, "~~~") {
		return "~~~"
	}
	return ""
}

// Parse returns the links in a markdown body in document order, skipping
// fenced code blocks and inline code spans.
func Parse(md string) []Ref {
	var out []Ref
	inFence := ""
	for i := 0; md != ""; i++ {
		line, rest, _ := strings.Cut(md, "\n")
		md = rest
		line = strings.TrimRight(line, "\r")
		if f := fenceMarker(line); f != "" {
			switch inFence {
			case "":
				inFence = f
			case f:
				inFence = ""
			}
			continue
		}
		// Most lines hold no link; skip the regexps for them. Blanking code
		// spans only removes brackets, so it can't make a "[[".
		if inFence != "" || !strings.Contains(line, "[[") {
			continue
		}
		// Blank out code spans so positions still line up with the original.
		scan := codeSpan.ReplaceAllStringFunc(line, func(s string) string { return strings.Repeat(" ", len(s)) })
		for _, m := range wikilink.FindAllStringSubmatchIndex(scan, -1) {
			inner := line[m[4]:m[5]]
			r := split(inner)
			if r.Target == "" && r.Anchor == "" {
				continue
			}
			r.Raw = line[m[0]:m[1]]
			r.Embed = m[3] > m[2]
			r.Line = i
			r.Snippet = snippet(line, m[0], m[1])
			out = append(out, r)
		}
	}
	return out
}

// split reads "Target#Anchor|Display". In tables "\|" stands for "|".
func split(inner string) Ref {
	inner = strings.ReplaceAll(inner, `\|`, "|")
	var r Ref
	if t, d, ok := strings.Cut(inner, "|"); ok {
		inner, r.Display = t, strings.TrimSpace(d)
	}
	if t, a, ok := strings.Cut(inner, "#"); ok {
		inner, r.Anchor = t, strings.TrimSpace(a)
	}
	r.Target = strings.TrimSpace(inner)
	return r
}

// ParseRef reads a single reference as a person or agent would type it:
// "Rollback runbook", "[[ADR-012#Decision]]", "PAY-214".
func ParseRef(s string) Ref {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "!")
	if strings.HasPrefix(s, "[[") && strings.HasSuffix(s, "]]") {
		s = s[2 : len(s)-2]
	}
	return split(s)
}

var (
	listMarker = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)]|>|#{1,6})\s+`)
	// Only * and ~~ are stripped: underscores and backticks are usually
	// identifiers (ledger_entries) worth keeping.
	emphasis = regexp.MustCompile(`\*{1,3}|~~`)
)

const snippetMax = 200

// snippet is the source line around a link with list markers and emphasis
// dropped, clipped to about 200 characters centred on the link.
func snippet(line string, start, end int) string {
	if loc := listMarker.FindStringIndex(line); loc != nil && loc[1] <= start {
		line, start, end = line[loc[1]:], start-loc[1], end-loc[1]
	}
	if utf8.RuneCountInString(line) > snippetMax {
		mid := (start + end) / 2
		from := max(0, mid-snippetMax/2)
		to := min(len(line), from+snippetMax)
		from = max(0, to-snippetMax)
		for from > 0 && !utf8.RuneStart(line[from]) {
			from--
		}
		for to < len(line) && !utf8.RuneStart(line[to]) {
			to++
		}
		prefix, suffix := "", ""
		if from > 0 {
			prefix = "…"
		}
		if to < len(line) {
			suffix = "…"
		}
		line = prefix + strings.TrimSpace(line[from:to]) + suffix
	}
	return strings.TrimSpace(emphasis.ReplaceAllString(line, ""))
}

// Slug lowercases, keeps letters and digits, and joins the rest with single
// hyphens: "§4.2 Partition by month" → "4-2-partition-by-month". Shared with
// artifact outlines and web/lib/slug.ts.
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
