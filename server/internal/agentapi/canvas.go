package agentapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/canvas"
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
	return map[string]any{"canvas_id": cv.ID, "kind": cv.Kind, "title": cv.Title, "version": v.Seq, "body": v.Body}, nil
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
