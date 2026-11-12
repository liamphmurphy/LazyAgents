package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/design"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

const (
	contextLen = 32
	maxQuote   = 4000
	maxLabel   = 60
	maxExcerpt = 400
	// minContext is how much surrounding text must match to pick between
	// repeats of a quote; a character or two matches by chance.
	minContext = 4
)

// Point is where on an SVG a comment was placed, as fractions of its size.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Anchor is what a comment points at. Text offsets are UTF-16 code units, as a
// browser counts them, into the version named by the comment's seq.
type Anchor struct {
	Start  *int   `json:"start,omitempty"`
	End    *int   `json:"end,omitempty"`
	Quote  string `json:"quote,omitempty"`
	Prefix string `json:"prefix,omitempty"`
	Suffix string `json:"suffix,omitempty"`
	VID    string `json:"vid,omitempty"`
	// Node is a design node id; Point is where in its box (fractions 0–1).
	Node  string `json:"node,omitempty"`
	Tag   string `json:"tag,omitempty"`
	Label string `json:"label,omitempty"`
	Point *Point `json:"point,omitempty"`
}

// byteOffset converts a UTF-16 offset, or reports that it splits a character
// or runs past the end.
func byteOffset(s string, u16 int) (int, bool) {
	if u16 < 0 {
		return 0, false
	}
	n := 0
	for i, r := range s {
		if n == u16 {
			return i, true
		}
		if n > u16 {
			return 0, false
		}
		n += utf16.RuneLen(r)
	}
	return len(s), n == u16
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	i := len(s) - n
	for i < len(s) && !utf8.RuneStart(s[i]) {
		i++
	}
	return s[i:]
}

func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func textAnchorAt(body string, start, end int) Anchor {
	s, e := utf16Len(body[:start]), utf16Len(body[:end])
	return Anchor{Start: &s, End: &e, Quote: body[start:end], Prefix: tail(body[:start], contextLen), Suffix: head(body[end:], contextLen)}
}

func newTextAnchor(body string, start, end int) (Anchor, error) {
	s, ok1 := byteOffset(body, start)
	e, ok2 := byteOffset(body, end)
	if !ok1 || !ok2 || s >= e {
		return Anchor{}, apperr.Invalid("anchor", "that selection isn't in this version")
	}
	if e-s > maxQuote {
		return Anchor{}, apperr.Invalid("anchor", "select a shorter passage")
	}
	return textAnchorAt(body, s, e), nil
}

func commonSuffix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[len(a)-1-n] == b[len(b)-1-n] {
		n++
	}
	return n
}

func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// moveTextAnchor finds a passage in a new body. The occurrence with the most
// matching context wins, then the nearest; a quote that recurs with none of
// its context is treated as gone rather than pinned to a guess.
func moveTextAnchor(body string, a Anchor) (Anchor, bool) {
	if a.Quote == "" {
		return a, false
	}
	old := 0
	if a.Start != nil {
		old, _ = byteOffset(body, *a.Start)
	}
	best, bestScore, bestDist, found := -1, -1, 0, 0
	for from := 0; ; {
		i := strings.Index(body[from:], a.Quote)
		if i < 0 {
			break
		}
		at := from + i
		from = at + 1
		found++
		score := commonSuffix(body[:at], a.Prefix) + commonPrefix(body[at+len(a.Quote):], a.Suffix)
		dist := at - old
		if dist < 0 {
			dist = -dist
		}
		if score > bestScore || (score == bestScore && dist < bestDist) {
			best, bestScore, bestDist = at, score, dist
		}
	}
	if best < 0 || (found > 1 && bestScore < minContext) {
		return a, false
	}
	return textAnchorAt(body, best, best+len(a.Quote)), true
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func describe(body string, e *element) (tag, label string) {
	open := body[e.start+1 : e.tagEnd]
	tag = open[:nameEnd(open)]
	if !e.selfClosed() {
		label = strings.Join(strings.Fields(html.UnescapeString(tagRe.ReplaceAllString(body[e.tagEnd:e.closeAt], " "))), " ")
	}
	if r := []rune(label); len(r) > maxLabel {
		label = string(r[:maxLabel]) + "…"
	}
	return tag, label
}

func svgAnchor(body, vid string, p *Point) (Anchor, error) {
	all, err := scan(body)
	if err != nil {
		return Anchor{}, err
	}
	e := find(all, vid)
	if e == nil {
		return Anchor{}, apperr.Invalid("anchor", "no element %q in this version", vid)
	}
	if p != nil && (p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1) {
		return Anchor{}, apperr.Invalid("anchor", "point must be within the drawing")
	}
	tag, label := describe(body, e)
	return Anchor{VID: vid, Tag: tag, Label: label, Point: p}, nil
}

// designAnchor pins a comment on a design node, at a point in its box.
func designAnchor(body, node string, p *Point) (Anchor, error) {
	if !design.HasNode(body, node) {
		return Anchor{}, apperr.Invalid("anchor", "no node %q in this version", node)
	}
	if p != nil && (p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1) {
		return Anchor{}, apperr.Invalid("anchor", "point must be within the node's box (fractions 0 to 1)")
	}
	d, err := design.Describe(body, node)
	if err != nil {
		return Anchor{}, err
	}
	return Anchor{Node: node, Tag: d.Type, Label: d.Name, Point: p}, nil
}

// reanchor carries every comment thread on to a newly written version.
func reanchor(ctx context.Context, q *store.Queries, c store.ChatCanvas, v store.ChatCanvasVersion) error {
	rows, err := q.ListChatCanvasComments(ctx, c.ID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ParentID != nil {
			continue
		}
		var a Anchor
		if err := json.Unmarshal(r.Anchor, &a); err != nil {
			return err
		}
		ok := false
		if c.Kind == KindDesign {
			if next, err := designAnchor(v.Body, a.Node, a.Point); err == nil {
				a, ok = next, true
			}
		} else if c.Kind == KindSVG {
			if next, err := svgAnchor(v.Body, a.VID, a.Point); err == nil {
				a, ok = next, true
			}
		} else {
			a, ok = moveTextAnchor(v.Body, a)
		}
		raw, err := json.Marshal(a)
		if err != nil {
			return err
		}
		if err := q.ReanchorChatCanvasComment(ctx, store.ReanchorChatCanvasCommentParams{ID: r.ID, Anchor: raw, Seq: v.Seq, TargetLost: !ok}); err != nil {
			return err
		}
	}
	return nil
}

func writeVersion(ctx context.Context, q *store.Queries, c store.ChatCanvas, cur store.ChatCanvasVersion, body, note string, by Actor) (store.ChatCanvasVersion, error) {
	if body == cur.Body {
		return cur, apperr.Invalid("body", "this changes nothing")
	}
	v, err := q.CreateChatCanvasVersion(ctx, store.CreateChatCanvasVersionParams{CanvasID: c.ID, Seq: cur.Seq + 1, Body: body, Note: note, AuthorType: by.Type, AuthorID: by.ID})
	if err != nil {
		return v, err
	}
	if err := q.TouchChatCanvas(ctx, c.ID); err != nil {
		return v, err
	}
	if err := syncFrameLinks(ctx, q, c, body); err != nil {
		return v, err
	}
	return v, reanchor(ctx, q, c, v)
}

// Locked runs fn in a transaction holding the canvas row, with its current
// version, for callers that write beside a version (designs' proposals).
func (s *Service) Locked(ctx context.Context, chat, id uuid.UUID, fn func(*store.Queries, store.ChatCanvas, store.ChatCanvasVersion) error) error {
	return s.locked(ctx, chat, id, fn)
}

// WriteVersion writes body as the version after cur, inside Locked.
func WriteVersion(ctx context.Context, q *store.Queries, c store.ChatCanvas, cur store.ChatCanvasVersion, body, note string, by Actor) (store.ChatCanvasVersion, error) {
	return writeVersion(ctx, q, c, cur, body, note, by)
}

// locked runs fn holding the canvas row, so versions and anchors change together.
func (s *Service) locked(ctx context.Context, chat, id uuid.UUID, fn func(*store.Queries, store.ChatCanvas, store.ChatCanvasVersion) error) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		c, err := q.LockChatCanvas(ctx, store.LockChatCanvasParams{ID: id, SessionID: chat})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		cur, err := q.LatestChatCanvasVersion(ctx, c.ID)
		if err != nil {
			return err
		}
		return fn(q, c, cur)
	})
}

func stale(cur, base int32) error {
	return apperr.Conflict("this canvas is at version %d, not %d; reload it and try again", cur, base)
}

// Save writes a person's own edit as a new version.
func (s *Service) Save(ctx context.Context, chat, id uuid.UUID, base int32, body, note string, by Actor) (store.ChatCanvasVersion, error) {
	var v store.ChatCanvasVersion
	err := s.locked(ctx, chat, id, func(q *store.Queries, c store.ChatCanvas, cur store.ChatCanvasVersion) error {
		if base != cur.Seq {
			return stale(cur.Seq, base)
		}
		next, err := normalize(c.Kind, body)
		if err != nil {
			return err
		}
		v, err = writeVersion(ctx, q, c, cur, next, note, by)
		return err
	})
	return v, err
}

// Restore makes an earlier version current by writing it again.
func (s *Service) Restore(ctx context.Context, chat, id uuid.UUID, seq int32, by Actor) (store.ChatCanvasVersion, error) {
	var v store.ChatCanvasVersion
	err := s.locked(ctx, chat, id, func(q *store.Queries, c store.ChatCanvas, cur store.ChatCanvasVersion) error {
		old, err := q.GetChatCanvasVersion(ctx, store.GetChatCanvasVersionParams{CanvasID: c.ID, Seq: seq})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		v, err = writeVersion(ctx, q, c, cur, old.Body, fmt.Sprintf("Restored v%d", seq), by)
		return err
	})
	return v, err
}

// NewComment is a person's comment. A reply sets ParentID; anything else sets
// either Start and End or VID.
type NewComment struct {
	Seq        int32
	ParentID   *uuid.UUID
	Start, End *int
	VID        string
	Node       string // a design node id
	Point      *Point
	Body       string
}

func cleanBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if n := utf8.RuneCountInString(body); n < 1 || n > 4000 {
		return "", apperr.Invalid("body", "provide 1–4000 characters")
	}
	return body, nil
}

func addReply(ctx context.Context, q *store.Queries, c store.ChatCanvas, cur store.ChatCanvasVersion, parent uuid.UUID, body string, by Actor, status string) (store.ChatCanvasComment, error) {
	var out store.ChatCanvasComment
	p, err := q.GetChatCanvasComment(ctx, store.GetChatCanvasCommentParams{ID: parent, CanvasID: c.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperr.Invalid("parent_id", "no such comment on this canvas")
	} else if err != nil {
		return out, err
	}
	if p.ParentID != nil {
		return out, apperr.Invalid("parent_id", "reply to the first comment of a thread")
	}
	if p.Status == "draft" {
		return out, apperr.Invalid("parent_id", "that comment hasn't been sent yet")
	}
	if status != "" && status != p.Status {
		if _, err := q.SetChatCanvasCommentStatus(ctx, store.SetChatCanvasCommentStatusParams{ID: p.ID, Status: status}); err != nil {
			return out, err
		}
	}
	return q.CreateChatCanvasComment(ctx, store.CreateChatCanvasCommentParams{CanvasID: c.ID, ParentID: &p.ID, Seq: cur.Seq, AuthorType: by.Type, AuthorID: by.ID, Body: body, Anchor: []byte("{}"), Status: "open"})
}

// Decks have no comments yet (ADR: deferred); the persona's suggestions are
// reviewed as changes instead. Designs take node-anchored comments.
var errNoDeckComments = apperr.Invalid("canvas_id", "a slide deck can't take comments yet")

func noComments(kind string) error {
	if kind == KindDeck {
		return errNoDeckComments
	}
	return nil
}

// AddComment adds a person's draft comment, or a reply to an existing thread,
// which goes back to open if it had been addressed.
func (s *Service) AddComment(ctx context.Context, chat, id uuid.UUID, in NewComment, by Actor) (store.ChatCanvasComment, error) {
	var out store.ChatCanvasComment
	body, err := cleanBody(in.Body)
	if err != nil {
		return out, err
	}
	err = s.locked(ctx, chat, id, func(q *store.Queries, c store.ChatCanvas, cur store.ChatCanvasVersion) error {
		var err error
		if err := noComments(c.Kind); err != nil {
			return err
		}
		if in.ParentID != nil {
			out, err = addReply(ctx, q, c, cur, *in.ParentID, body, by, "open")
			return err
		}
		if in.Seq != cur.Seq {
			return stale(cur.Seq, in.Seq)
		}
		var a Anchor
		switch {
		case c.Kind == KindDesign:
			a, err = designAnchor(cur.Body, in.Node, in.Point)
		case c.Kind == KindSVG:
			a, err = svgAnchor(cur.Body, in.VID, in.Point)
		case in.Start == nil || in.End == nil:
			err = apperr.Invalid("anchor", "start and end are required on a text canvas")
		default:
			a, err = newTextAnchor(cur.Body, *in.Start, *in.End)
		}
		if err != nil {
			return err
		}
		raw, err := json.Marshal(a)
		if err != nil {
			return err
		}
		out, err = q.CreateChatCanvasComment(ctx, store.CreateChatCanvasCommentParams{CanvasID: c.ID, Seq: cur.Seq, AuthorType: by.Type, AuthorID: by.ID, Body: body, Anchor: raw, Status: "draft"})
		return err
	})
	return out, err
}

// AgentComment is a comment an agent starts, on one passage or element.
type AgentComment struct {
	Quote      string
	VID        string
	Node       string // a design node id
	Point      *Point
	Body       string
	Suggestion *string
}

// AddAgentComment opens a thread from an agent. Text is found by quote, which
// must occur exactly once. A suggestion is only offered on a text canvas.
func (s *Service) AddAgentComment(ctx context.Context, chat, id uuid.UUID, in AgentComment, by Actor) (store.ChatCanvasComment, error) {
	var out store.ChatCanvasComment
	body, err := cleanBody(in.Body)
	if err != nil {
		return out, err
	}
	err = s.locked(ctx, chat, id, func(q *store.Queries, c store.ChatCanvas, cur store.ChatCanvasVersion) error {
		var a Anchor
		var err error
		if err := noComments(c.Kind); err != nil {
			return err
		}
		if c.Kind == KindDesign {
			if in.Suggestion != nil {
				return apperr.Invalid("suggestion", "only a text canvas takes suggestions")
			}
			if a, err = designAnchor(cur.Body, in.Node, in.Point); err != nil {
				return err
			}
		} else if c.Kind == KindSVG {
			if in.Suggestion != nil {
				return apperr.Invalid("suggestion", "only a text canvas takes suggestions")
			}
			if a, err = svgAnchor(cur.Body, in.VID, nil); err != nil {
				return err
			}
		} else {
			if in.Quote == "" || len(in.Quote) > maxQuote {
				return apperr.Invalid("quote", "quote the passage, at most %d bytes", maxQuote)
			}
			if n := strings.Count(cur.Body, in.Quote); n != 1 {
				return apperr.Invalid("quote", "matched %d times; it must match exactly once, so include more surrounding text", n)
			}
			at := strings.Index(cur.Body, in.Quote)
			a = textAnchorAt(cur.Body, at, at+len(in.Quote))
		}
		raw, err := json.Marshal(a)
		if err != nil {
			return err
		}
		out, err = q.CreateChatCanvasComment(ctx, store.CreateChatCanvasCommentParams{CanvasID: c.ID, Seq: cur.Seq, AuthorType: by.Type, AuthorID: by.ID, Body: body, Anchor: raw, Status: "open", Suggestion: in.Suggestion})
		return err
	})
	return out, err
}

// Reply adds an agent's reply to a thread, marking it addressed if asked.
func (s *Service) Reply(ctx context.Context, chat, id, comment uuid.UUID, body string, addressed bool, by Actor) (store.ChatCanvasComment, error) {
	var out store.ChatCanvasComment
	text, err := cleanBody(body)
	if err != nil {
		return out, err
	}
	status := ""
	if addressed {
		status = "addressed"
	}
	err = s.locked(ctx, chat, id, func(q *store.Queries, c store.ChatCanvas, cur store.ChatCanvasVersion) error {
		var err error
		out, err = addReply(ctx, q, c, cur, comment, text, by, status)
		return err
	})
	return out, err
}

func topLevel(ctx context.Context, q *store.Queries, canvas, id uuid.UUID) (store.ChatCanvasComment, error) {
	r, err := q.GetChatCanvasComment(ctx, store.GetChatCanvasCommentParams{ID: id, CanvasID: canvas})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && r.ParentID != nil) {
		return r, apperr.ErrNotFound
	}
	return r, err
}

// SetStatus moves a sent thread between open, addressed and resolved.
func (s *Service) SetStatus(ctx context.Context, chat, id, comment uuid.UUID, status string) (store.ChatCanvasComment, error) {
	var out store.ChatCanvasComment
	if status != "open" && status != "addressed" && status != "resolved" {
		return out, apperr.Invalid("status", "use open, addressed or resolved")
	}
	err := s.locked(ctx, chat, id, func(q *store.Queries, c store.ChatCanvas, _ store.ChatCanvasVersion) error {
		r, err := topLevel(ctx, q, c.ID, comment)
		if err != nil {
			return err
		}
		if r.Status == "draft" {
			return apperr.Conflict("send this comment first, or delete it")
		}
		out, err = q.SetChatCanvasCommentStatus(ctx, store.SetChatCanvasCommentStatusParams{ID: r.ID, Status: status})
		return err
	})
	return out, err
}

// DeleteDraft removes a comment that was never sent.
func (s *Service) DeleteDraft(ctx context.Context, chat, id, comment uuid.UUID) error {
	return s.locked(ctx, chat, id, func(q *store.Queries, c store.ChatCanvas, _ store.ChatCanvasVersion) error {
		n, err := q.DeleteChatCanvasDraftComment(ctx, store.DeleteChatCanvasDraftCommentParams{ID: comment, CanvasID: c.ID})
		if err == nil && n == 0 {
			return apperr.ErrNotFound
		}
		return err
	})
}

// ApplySuggestion replaces a comment's passage with the agent's suggested text.
func (s *Service) ApplySuggestion(ctx context.Context, chat, id, comment uuid.UUID, by Actor) (store.ChatCanvasVersion, error) {
	var v store.ChatCanvasVersion
	err := s.locked(ctx, chat, id, func(q *store.Queries, c store.ChatCanvas, cur store.ChatCanvasVersion) error {
		r, err := topLevel(ctx, q, c.ID, comment)
		if err != nil {
			return err
		}
		if r.Suggestion == nil {
			return apperr.Conflict("this comment has no suggestion to apply")
		}
		if r.TargetLost {
			return apperr.Conflict("the passage this comment was on has changed")
		}
		var a Anchor
		if err := json.Unmarshal(r.Anchor, &a); err != nil {
			return err
		}
		start, ok1 := byteOffset(cur.Body, *a.Start)
		end, ok2 := byteOffset(cur.Body, *a.End)
		if !ok1 || !ok2 || cur.Body[start:end] != a.Quote {
			return apperr.Conflict("the passage this comment was on has changed")
		}
		next, err := normalize(c.Kind, cur.Body[:start]+*r.Suggestion+cur.Body[end:])
		if err != nil {
			return err
		}
		if v, err = writeVersion(ctx, q, c, cur, next, "Applied a suggestion", by); err != nil {
			return err
		}
		if err := q.ClearChatCanvasCommentSuggestion(ctx, r.ID); err != nil {
			return err
		}
		_, err = q.SetChatCanvasCommentStatus(ctx, store.SetChatCanvasCommentStatusParams{ID: r.ID, Status: "addressed"})
		return err
	})
	return v, err
}

// Open moves drafts to open and returns which ones did. At least one of ids
// must be on this canvas and still unresolved.
func (s *Service) Open(ctx context.Context, chat, id uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	var opened []uuid.UUID
	err := s.locked(ctx, chat, id, func(q *store.Queries, c store.ChatCanvas, _ store.ChatCanvasVersion) error {
		live := false
		for _, cid := range ids {
			r, err := topLevel(ctx, q, c.ID, cid)
			if errors.Is(err, apperr.ErrNotFound) {
				return apperr.Invalid("comment_ids", "%s isn't a comment on this canvas", cid)
			} else if err != nil {
				return err
			}
			live = live || r.Status != "resolved"
		}
		if !live {
			return apperr.Invalid("comment_ids", "name at least one unresolved comment")
		}
		var err error
		opened, err = q.OpenChatCanvasDraftComments(ctx, store.OpenChatCanvasDraftCommentsParams{CanvasID: c.ID, Ids: ids})
		return err
	})
	return opened, err
}

func excerpt(body string, a Anchor) string {
	all, err := scan(body)
	if err != nil {
		return ""
	}
	e := find(all, a.VID)
	if e == nil {
		return ""
	}
	out := body[e.start:e.end]
	if len(out) > maxExcerpt {
		out = head(out, maxExcerpt) + "…"
	}
	return out
}

// ContextDoc is what a turn is given when a person sends comments: the canvas
// as it stands and every thread still waiting on a change.
func ContextDoc(ctx context.Context, q *store.Queries, chat, id uuid.UUID) (string, string, error) {
	c, err := q.GetChatCanvas(ctx, store.GetChatCanvasParams{ID: id, SessionID: chat})
	if err != nil {
		return "", "", err
	}
	v, err := q.LatestChatCanvasVersion(ctx, c.ID)
	if err != nil {
		return "", "", err
	}
	rows, err := q.ListChatCanvasComments(ctx, c.ID)
	if err != nil {
		return "", "", err
	}
	who := func(r store.ChatCanvasComment) string {
		if r.AuthorType == "user" {
			return "Person"
		}
		return "You"
	}
	var b strings.Builder
	reply := "canvas_reply"
	if c.Kind == KindDesign {
		reply = "design_reply"
	}
	fmt.Fprintf(&b, "# Canvas %q\n\ncanvas_id: %s\nkind: %s\nversion: %d\n\n## Open comments\n\nAct on these, then answer each with %s (addressed: true once it's done).\n", c.Title, c.ID, c.Kind, v.Seq, reply)
	for _, r := range rows {
		if r.ParentID != nil || r.Status != "open" {
			continue
		}
		var a Anchor
		if err := json.Unmarshal(r.Anchor, &a); err != nil {
			return "", "", err
		}
		fmt.Fprintf(&b, "\n### Comment %s\n", r.ID)
		switch {
		case r.TargetLost:
			b.WriteString("Its original target is no longer in this version.\n")
		case c.Kind == KindDesign:
			fmt.Fprintf(&b, "On node %s (%s %q)", a.Node, a.Tag, a.Label)
			if a.Point != nil {
				fmt.Fprintf(&b, " at %.0f%% across, %.0f%% down", a.Point.X*100, a.Point.Y*100)
			}
			b.WriteString(":\n")
		case c.Kind == KindSVG:
			fmt.Fprintf(&b, "On element data-vid=%q (<%s>):\n```xml\n%s\n```\n", a.VID, a.Tag, excerpt(v.Body, a))
		default:
			fmt.Fprintf(&b, "On this passage:\n> %s\n", strings.ReplaceAll(a.Quote, "\n", "\n> "))
		}
		fmt.Fprintf(&b, "\n%s: %s\n", who(r), r.Body)
		for _, x := range rows {
			if x.ParentID != nil && *x.ParentID == r.ID {
				fmt.Fprintf(&b, "%s: %s\n", who(x), x.Body)
			}
		}
	}
	if c.Kind == KindDesign {
		b.WriteString("\n## Current content\n\nA design is big: look at it with design_outline, design_read and design_render, and change it with design_patch.\n")
		return c.Title, b.String(), nil
	}
	fmt.Fprintf(&b, "\n## Current content\n\n%s\n", v.Body)
	return c.Title, b.String(), nil
}

// Viewport is the part of a design page the person was looking at.
type Viewport struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
	Zoom float64 `json:"zoom"`
}

// Selection is what a person had selected in a design when they sent a
// message (ADR-designs §3.7): the page, the node ids and the viewport.
type Selection struct {
	Page     string   `json:"page"`
	Nodes    []string `json:"nodes"`
	Viewport Viewport `json:"viewport"`
}

// MaxSelectedNodes bounds a selection.
const MaxSelectedNodes = 100

var reSelectedNode = regexp.MustCompile(`^n_[a-z0-9_]{1,40}$`)

// Check validates a selection's shape (not that it exists).
func (s Selection) Check() error {
	bad := func(format string, args ...any) error {
		return apperr.Invalid("context_refs", "selection: "+format, args...)
	}
	if !regexp.MustCompile(`^p_[a-z0-9_]{1,40}$`).MatchString(s.Page) {
		return bad("page must be a page id")
	}
	if len(s.Nodes) > MaxSelectedNodes {
		return bad("select at most %d nodes", MaxSelectedNodes)
	}
	for _, n := range s.Nodes {
		if !reSelectedNode.MatchString(n) {
			return bad("%q isn't a node id", n)
		}
	}
	v := s.Viewport
	for _, f := range []float64{v.X, v.Y, v.W, v.H, v.Zoom} {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return bad("viewport numbers must be finite")
		}
	}
	if v.W < 0 || v.H < 0 || v.Zoom <= 0 {
		return bad("viewport needs a size and a positive zoom")
	}
	return nil
}

// ErrNotDesign says a selection was attached to something that isn't a design.
var ErrNotDesign = apperr.Invalid("context_refs", "a selection can only be attached to a design canvas")

// SelectionDoc is the context a turn gets for a design with a selection: short,
// naming the design, page, viewport and each selected node, not the body.
func SelectionDoc(ctx context.Context, q *store.Queries, chat, id uuid.UUID, sel Selection) (string, string, error) {
	c, err := q.GetChatCanvas(ctx, store.GetChatCanvasParams{ID: id, SessionID: chat})
	if err != nil {
		return "", "", err
	}
	if c.Kind != KindDesign {
		return "", "", ErrNotDesign
	}
	v, err := q.LatestChatCanvasVersion(ctx, c.ID)
	if err != nil {
		return "", "", err
	}
	if !design.HasPage(v.Body, sel.Page) {
		return "", "", apperr.Invalid("context_refs", "selection: page %s isn't in this design", sel.Page)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Design %q\n\ncanvas_id: %s\nversion: %d\npage: %s\nviewport: x=%g y=%g w=%g h=%g zoom=%g\n\n## Selected nodes\n\n", c.Title, c.ID, v.Seq, sel.Page, sel.Viewport.X, sel.Viewport.Y, sel.Viewport.W, sel.Viewport.H, sel.Viewport.Zoom)
	if len(sel.Nodes) == 0 {
		b.WriteString("Nothing is selected.\n")
	}
	for _, id := range sel.Nodes {
		n, err := design.Describe(v.Body, id)
		if err != nil {
			return "", "", err
		}
		if !n.Found {
			fmt.Fprintf(&b, "- %s: no longer in this design\n", id)
			continue
		}
		fmt.Fprintf(&b, "- %s: %s %q", n.ID, n.Type, n.Name)
		if n.Box != nil {
			fmt.Fprintf(&b, " box x=%g y=%g w=%g h=%g", n.Box.X, n.Box.Y, n.Box.W, n.Box.H)
		}
		b.WriteString("\n")
	}
	b.WriteString("\nWhen the person says this/these, they mean the selected nodes. Look at them with design_read (node_ids) or design_render; this note doesn't include the design.\n")
	return c.Title, b.String(), nil
}
