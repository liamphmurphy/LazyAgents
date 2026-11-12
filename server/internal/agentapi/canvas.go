package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/canvas"
	"github.com/liamphmurphy/vesper/server/internal/deck"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type CanvasCreateIn struct {
	Kind  string `json:"kind" jsonschema:"text for a Markdown document, svg for an SVG mockup"`
	Title string `json:"title" jsonschema:"A short title"`
	Body  string `json:"body" jsonschema:"The Markdown, or one complete <svg> with xmlns and a viewBox, styled with presentation attributes"`
	Note  string `json:"note,omitempty" jsonschema:"What this version is, in a few words"`
}

type CanvasPatchIn struct {
	CanvasID    uuid.UUID   `json:"canvas_id"`
	BaseVersion int32       `json:"base_version" jsonschema:"The version number these edits are based on"`
	Ops         []canvas.Op `json:"ops" jsonschema:"Edits, applied in order. All succeed or none do"`
	Note        string      `json:"note,omitempty" jsonschema:"What changed, in a few words"`
}

type CanvasReadIn struct {
	CanvasID uuid.UUID `json:"canvas_id"`
	Version  int32     `json:"version,omitempty" jsonschema:"An earlier version number; the current one when omitted"`
}

func canvasScope(ctx context.Context, d Deps, c agents.Caller, tool string) (*chatScope, error) {
	s, err := chatScopeFor(ctx, d, c)
	if err == nil && s == nil {
		err = apperr.Conflict("%s only works in a chat with a person", tool)
	}
	return s, err
}

// requireSVG keeps SVG canvases to personas allowed to show SVG images.
func requireSVG(ctx context.Context, d Deps, c agents.Caller, s *chatScope) error {
	chat, err := d.Q.GetChatSession(ctx, store.GetChatSessionParams{ID: s.ChatID, WorkspaceID: c.Agent.WorkspaceID})
	if err != nil {
		return err
	}
	can, err := runs.SpeakerCanCreateSVGs(ctx, d.Q, chat)
	if err == nil && !can {
		err = apperr.Conflict("this persona isn't allowed to create SVG images")
	}
	return err
}

func canvasCreate(ctx context.Context, d Deps, c agents.Caller, in CanvasCreateIn) (any, error) {
	s, err := canvasScope(ctx, d, c, "canvas_create")
	if err != nil {
		return nil, err
	}
	if in.Kind == canvas.KindSVG {
		if err := requireSVG(ctx, d, c, s); err != nil {
			return nil, err
		}
	}
	if in.Kind == canvas.KindDeck {
		return nil, apperr.Invalid("kind", "slide decks are started in the Slides app; ask the person to start one there")
	}
	if in.Kind == canvas.KindDesign {
		return nil, apperr.Invalid("kind", "designs are started in the Designs app; ask the person to start one there")
	}
	svc := canvas.Service{Pool: d.Runs.Pool}
	cv, v, err := svc.Create(ctx, s.ChatID, canvas.New{Kind: in.Kind, Title: in.Title, Body: in.Body, Note: in.Note}, canvas.Actor{Type: "agent", ID: c.Agent.ID})
	if err != nil {
		return nil, err
	}
	return map[string]any{"canvas_id": cv.ID, "version": v.Seq}, nil
}

func canvasPatch(ctx context.Context, d Deps, c agents.Caller, in CanvasPatchIn) (any, error) {
	s, err := canvasScope(ctx, d, c, "canvas_patch")
	if err != nil {
		return nil, err
	}
	cv, err := getCanvas(ctx, d, s, in.CanvasID)
	if err != nil {
		return nil, err
	}
	if cv.Kind == canvas.KindSVG {
		if err := requireSVG(ctx, d, c, s); err != nil {
			return nil, err
		}
	}
	if cv.Kind == canvas.KindDesign {
		// A persona's design edits are held for the person's review
		// (ADR-designs §3.7), which canvas_patch would bypass.
		return nil, apperr.Invalid("canvas_id", "this is a design; persona edits to designs go through design_patch, which isn't available yet")
	}
	if cv.Kind == canvas.KindDeck {
		if err := resolveSavedThemes(ctx, d, s.ChatID, in.Ops); err != nil {
			return nil, err
		}
	}
	svc := canvas.Service{Pool: d.Runs.Pool}
	v, err := svc.Patch(ctx, s.ChatID, in.CanvasID, in.BaseVersion, in.Ops, in.Note, canvas.Actor{Type: "agent", ID: c.Agent.ID})
	if err != nil {
		return nil, err
	}
	return map[string]any{"canvas_id": in.CanvasID, "version": v.Seq}, nil
}

func getCanvas(ctx context.Context, d Deps, s *chatScope, id uuid.UUID) (store.ChatCanvas, error) {
	cv, err := d.Q.GetChatCanvas(ctx, store.GetChatCanvasParams{ID: id, SessionID: s.ChatID})
	if errors.Is(err, pgx.ErrNoRows) {
		return cv, apperr.ErrNotFound
	}
	return cv, err
}

func canvasRead(ctx context.Context, d Deps, c agents.Caller, in CanvasReadIn) (any, error) {
	s, err := canvasScope(ctx, d, c, "canvas_read")
	if err != nil {
		return nil, err
	}
	cv, err := getCanvas(ctx, d, s, in.CanvasID)
	if err != nil {
		return nil, err
	}
	var v store.ChatCanvasVersion
	if in.Version > 0 {
		v, err = d.Q.GetChatCanvasVersion(ctx, store.GetChatCanvasVersionParams{CanvasID: cv.ID, Seq: in.Version})
	} else {
		v, err = d.Q.LatestChatCanvasVersion(ctx, cv.ID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	threads, err := canvasThreads(ctx, d, cv.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"canvas_id": cv.ID, "kind": cv.Kind, "title": cv.Title, "version": v.Seq, "body": v.Body, "comments": threads}, nil
}

func canvasList(ctx context.Context, d Deps, c agents.Caller, _ Empty) (any, error) {
	s, err := canvasScope(ctx, d, c, "canvas_list")
	if err != nil {
		return nil, err
	}
	rows, err := d.Q.ListChatCanvases(ctx, s.ChatID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{"canvas_id": r.ID, "kind": r.Kind, "title": r.Title, "version": r.Seq})
	}
	return map[string]any{"canvases": out}, nil
}

type CanvasCommentIn struct {
	CanvasID   uuid.UUID `json:"canvas_id"`
	Quote      string    `json:"quote,omitempty" jsonschema:"Text canvas: the exact passage; it must occur exactly once"`
	VID        string    `json:"vid,omitempty" jsonschema:"SVG canvas: the data-vid of the element"`
	Body       string    `json:"body" jsonschema:"Your comment"`
	Suggestion *string   `json:"suggestion,omitempty" jsonschema:"Text canvas: replacement text for the passage, which the person can apply"`
}

type CanvasReplyIn struct {
	CanvasID  uuid.UUID `json:"canvas_id"`
	CommentID uuid.UUID `json:"comment_id"`
	Body      string    `json:"body" jsonschema:"Your reply"`
	Addressed bool      `json:"addressed,omitempty" jsonschema:"You have made the change this comment asked for"`
}

func canvasComment(ctx context.Context, d Deps, c agents.Caller, in CanvasCommentIn) (any, error) {
	s, err := canvasScope(ctx, d, c, "canvas_comment")
	if err != nil {
		return nil, err
	}
	svc := canvas.Service{Pool: d.Runs.Pool}
	r, err := svc.AddAgentComment(ctx, s.ChatID, in.CanvasID, canvas.AgentComment{Quote: in.Quote, VID: in.VID, Body: in.Body, Suggestion: in.Suggestion}, canvas.Actor{Type: "agent", ID: c.Agent.ID})
	if err != nil {
		return nil, err
	}
	return map[string]any{"comment_id": r.ID}, nil
}

func canvasReply(ctx context.Context, d Deps, c agents.Caller, in CanvasReplyIn) (any, error) {
	s, err := canvasScope(ctx, d, c, "canvas_reply")
	if err != nil {
		return nil, err
	}
	svc := canvas.Service{Pool: d.Runs.Pool}
	r, err := svc.Reply(ctx, s.ChatID, in.CanvasID, in.CommentID, in.Body, in.Addressed, canvas.Actor{Type: "agent", ID: c.Agent.ID})
	if err != nil {
		return nil, err
	}
	return map[string]any{"reply_id": r.ID, "addressed": in.Addressed}, nil
}

// canvasThreads lists a canvas's open discussion for canvas_read.
func canvasThreads(ctx context.Context, d Deps, id uuid.UUID) ([]map[string]any, error) {
	rows, err := d.Q.ListChatCanvasComments(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if r.Status == "draft" || r.Status == "resolved" {
			continue
		}
		out = append(out, map[string]any{"comment_id": r.ID, "parent_id": r.ParentID, "from": r.AuthorType, "body": r.Body, "status": r.Status, "anchor": json.RawMessage(r.Anchor), "target_lost": r.TargetLost})
	}
	return out, nil
}

// resolveSavedThemes turns set_theme {preset: name} ops naming one of the chat
// owner's saved themes (not a built-in one) into the theme itself, so the deck
// gets its own copy.
func resolveSavedThemes(ctx context.Context, d Deps, chat uuid.UUID, ops []canvas.Op) error {
	var saved []store.DeckTheme
	loaded := false
	for i, op := range ops {
		name, ok := op.Value["preset"].(string)
		if op.Op != "set_theme" || !ok || len(op.Value) != 1 {
			continue
		}
		if _, builtin := deck.Themes[strings.ToLower(strings.TrimSpace(name))]; builtin {
			continue
		}
		if !loaded {
			var err error
			if saved, err = d.Q.ChatOwnerDeckThemes(ctx, chat); err != nil {
				return err
			}
			loaded = true
		}
		names := slices.Clone(deck.ThemeOrder)
		found := false
		for _, t := range saved {
			names = append(names, t.Name)
			if !found && strings.EqualFold(strings.TrimSpace(t.Name), strings.TrimSpace(name)) {
				th, err := deck.SavedTheme(t.Name, t.Fonts, t.Colors)
				if err != nil {
					return err
				}
				ops[i].Value = map[string]any{"name": th.Name, "fonts": th.Fonts, "colors": th.Colors}
				found = true
			}
		}
		if !found {
			return apperr.Invalid("body", "op %d: no theme named %q; use one of: %s", i+1, name, strings.Join(names, ", "))
		}
	}
	return nil
}
