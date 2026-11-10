package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

// pinnedKinds are the rich blocks a persona revises from turn to turn. The
// newest of each kind is handed to a fresh segment whole: clipping the message
// that holds it would cut a mockup mid-element, and the persona would redraw it
// from a fragment.
var pinnedKinds = map[string]bool{"svg-image": true, "svg-steps": true, "app": true}

const (
	// pinnedBlockChars bounds one pinned block, fences included.
	pinnedBlockChars = 40000
	// handoffCanvasChars bounds the canvas body a fresh segment gets inline.
	handoffCanvasChars = 40000
	// handoffCanvasList bounds how many canvases the handoff names.
	handoffCanvasList = 10
	// handoffImagePrompts bounds how many generated images the handoff describes.
	handoffImagePrompts = 5
	// handoffImagePromptChars clips one image prompt.
	handoffImagePromptChars = 2000
	// handoffFetchChars clips one message body in SQL. It only guards against a
	// pathological row: artifacts are compacted and prose clipped in Go, at
	// handoffRowChars, once the blocks have been found.
	handoffFetchChars = 200000
)

// HandoffArtifact is the newest block of one pinned kind in the history.
type HandoffArtifact struct {
	Seq    int64
	Kind   string
	Source string // the whole fenced block, fences included
}

// HandoffContext is what a fresh segment gets about the chat besides its
// messages: the work its messages only point at.
type HandoffContext struct {
	Canvases []HandoffCanvas // oldest first
	// ImagePrompts are the prompts behind the chat's newest generated images,
	// newest first. The images themselves live outside the messages.
	ImagePrompts []string
}

// HandoffCanvas is a canvas in the chat. Body is set only for the one the
// handoff shows inline.
type HandoffCanvas struct {
	ID      string
	Kind    string
	Title   string
	Version int32
	Body    string
}

// fencedBlock is one fenced code block in a message, as byte offsets covering
// its opening and closing fence lines.
type fencedBlock struct {
	start, end int
	kind       string
}

// fencedBlocks finds the fenced code blocks in a Markdown body, in order. An
// unclosed block runs to the end of the body, as Markdown renders it.
func fencedBlocks(body string) []fencedBlock {
	var out []fencedBlock
	var open *fencedBlock
	var fence string
	for pos := 0; pos < len(body); {
		next := strings.IndexByte(body[pos:], '\n')
		end := len(body)
		if next >= 0 {
			end = pos + next + 1
		}
		line := strings.TrimSpace(body[pos:end])
		if open != nil {
			if strings.HasPrefix(line, fence) && strings.Trim(line, fence[:1]) == "" {
				open.end = end
				out = append(out, *open)
				open = nil
			}
		} else if f := fenceRun(line); f != "" {
			kind, _, _ := strings.Cut(strings.TrimSpace(line[len(f):]), " ")
			open, fence = &fencedBlock{start: pos, kind: strings.ToLower(kind)}, f
		}
		pos = end
	}
	if open != nil {
		open.end = len(body)
		out = append(out, *open)
	}
	return out
}

// fenceRun is the run of three or more backticks or tildes a line opens with,
// or "" when it doesn't open a fence.
func fenceRun(line string) string {
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return ""
	}
	n := len(line) - len(strings.TrimLeft(line, line[:1]))
	if n < 3 {
		return ""
	}
	return line[:n]
}

// compactArtifacts takes history newest first and pins the newest block of
// each pinned kind, replacing every pinned-kind block in the messages with a
// short note. Earlier versions are dropped: the persona revises the latest, and
// each old copy would crowd conversation out of the budget. Message bodies are
// then clipped to handoffRowChars.
func compactArtifacts(history []HandoffMessage) ([]HandoffMessage, []HandoffArtifact) {
	out := make([]HandoffMessage, len(history))
	var pinned []HandoffArtifact
	seen := map[string]bool{}
	for i, m := range history {
		blocks := fencedBlocks(m.Body)
		body := m.Body
		// Newest block first, so the last in a message wins, and replacing from
		// the end keeps earlier offsets valid.
		for j := len(blocks) - 1; j >= 0; j-- {
			b := blocks[j]
			if !pinnedKinds[b.kind] {
				continue
			}
			src := m.Body[b.start:b.end]
			note := fmt.Sprintf("[earlier %s block omitted (%d chars); the latest is under \"Latest artifacts\" below]\n", b.kind, utf8.RuneCountInString(src))
			if !seen[b.kind] {
				seen[b.kind] = true
				pinned = append(pinned, HandoffArtifact{Seq: m.Seq, Kind: b.kind, Source: clipRunes(src, pinnedBlockChars)})
				note = fmt.Sprintf("[%s block: shown in full under \"Latest artifacts\" below]\n", b.kind)
			}
			body = body[:b.start] + note + body[b.end:]
		}
		m.Body = clipRunes(body, handoffRowChars)
		out[i] = m
	}
	return out, pinned
}

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + handoffClip
}

// renderArtifacts is the handoff section that carries the persona's latest
// artifacts and the chat's canvases. It sits outside the history budget, which
// would otherwise drop exactly the large blocks a redraw needs.
func renderArtifacts(pinned []HandoffArtifact, hc HandoffContext) string {
	canvases := hc.Canvases
	if len(pinned) == 0 && len(canvases) == 0 && len(hc.ImagePrompts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("---\nLatest artifacts from earlier in this chat, in full. When asked to change one, start from this version rather than redrawing it from memory.\n\n")
	// Oldest first, matching the history above.
	for i := len(pinned) - 1; i >= 0; i-- {
		p := pinned[i]
		fmt.Fprintf(&b, "From message [%d] (%s):\n%s", p.Seq, p.Kind, p.Source)
		if !strings.HasSuffix(p.Source, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(canvases) > 0 {
		b.WriteString("Canvases in this chat. canvas_read one before changing it, then canvas_patch with the version you read as base_version:\n")
		for _, c := range canvases {
			fmt.Fprintf(&b, "- %s %q (%s, version %d)\n", c.ID, c.Title, c.Kind, c.Version)
		}
		for _, c := range canvases {
			if c.Body == "" {
				continue
			}
			fence := strings.Repeat("`", max(3, longestRun(c.Body, '`')+1))
			fmt.Fprintf(&b, "\nMost recently changed canvas, %q version %d:\n%s%s\n%s\n%s\n", c.Title, c.Version, fence, c.Kind, clipRunes(c.Body, handoffCanvasChars), fence)
		}
		b.WriteString("\n")
	}
	if len(hc.ImagePrompts) > 0 {
		b.WriteString("Images generated in this chat, newest first, by the prompt that made each. To change one, start from its prompt:\n")
		for i, p := range hc.ImagePrompts {
			fmt.Fprintf(&b, "%d. %s\n", i+1, clipRunes(strings.TrimSpace(p), handoffImagePromptChars))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func longestRun(s string, c byte) int {
	best, n := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			n++
			best = max(best, n)
		} else {
			n = 0
		}
	}
	return best
}

// loadHandoffContext gathers what a fresh segment needs beyond the messages.
func loadHandoffContext(ctx context.Context, q *store.Queries, chatID uuid.UUID) (HandoffContext, error) {
	var hc HandoffContext
	var err error
	if hc.Canvases, err = handoffCanvases(ctx, q, chatID); err != nil {
		return hc, err
	}
	images, err := q.RecentChatImagePrompts(ctx, store.RecentChatImagePromptsParams{ChatID: chatID, RowLimit: handoffImagePrompts})
	if err != nil {
		return hc, err
	}
	for _, im := range images {
		hc.ImagePrompts = append(hc.ImagePrompts, im.Prompt)
	}
	return hc, nil
}

// handoffCanvases lists the chat's most recently changed canvases, oldest
// first, with the latest version of the newest one inline. A message only
// carries a canvas's id, so without this a fresh segment knows a canvas exists
// but not what is on it.
func handoffCanvases(ctx context.Context, q *store.Queries, chatID uuid.UUID) ([]HandoffCanvas, error) {
	rows, err := q.ListChatCanvases(ctx, chatID)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	slices.SortStableFunc(rows, func(a, b store.ListChatCanvasesRow) int { return a.UpdatedAt.Compare(b.UpdatedAt) })
	rows = rows[max(0, len(rows)-handoffCanvasList):]
	out := make([]HandoffCanvas, len(rows))
	for i, r := range rows {
		out[i] = HandoffCanvas{ID: r.ID.String(), Kind: r.Kind, Title: r.Title, Version: r.Seq}
	}
	newest := rows[len(rows)-1]
	v, err := q.LatestChatCanvasVersion(ctx, newest.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out[len(out)-1].Version, out[len(out)-1].Body = v.Seq, v.Body
	return out, nil
}
